package api

import (
	"crypto/rand"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/auvitly/cmdui.git/internal/config"
	"github.com/auvitly/cmdui.git/internal/domain"
	"github.com/auvitly/cmdui.git/internal/platform"
	authservice "github.com/auvitly/cmdui.git/internal/service/auth"
	commandservice "github.com/auvitly/cmdui.git/internal/service/commands"
	"github.com/auvitly/cmdui.git/ui"
)

type User = domain.User
type Command = domain.Command
type Run = domain.Run
type RunFilter = domain.RunFilter
type Config = config.Config

var (
	ErrLastAdmin         = domain.ErrLastAdmin
	ErrCannotDeleteAdmin = domain.ErrCannotDeleteAdmin
	ErrCannotDeleteSelf  = domain.ErrCannotDeleteSelf
)

const sessionCookieName = "cmdui_session"

type webSession struct {
	Username string
	Role     string
	CSRF     string
	Expires  time.Time
}

type App struct {
	config   config.Config
	auth     *authservice.Service
	commands *commandservice.Service
	logger   *slog.Logger
	sessions map[string]webSession
	platform string
	mu       sync.Mutex
}

type pageData struct {
	User                  User
	CSRF                  string
	Commands              []Command
	Runs                  []Run
	Editing               *Command
	Executables           []string
	Filter                RunFilter
	Notice                string
	RunPanels             []Run
	RunningCommands       map[int64]bool
	MaxTimeoutSeconds     int
	DefaultTimeoutSeconds int
	Users                 []User
	Operators             []User
	Platform              string
}

type authPageData struct {
	Error      string
	Registered bool
}

type usersPageData struct {
	User     User
	CSRF     string
	Users    []User
	Notice   string
	Platform string
}

type runsPageData struct {
	User        User
	CSRF        string
	Runs        []Run
	Usernames   []string
	Filter      RunFilter
	Platform    string
	PageSize    int
	PageSizes   []int
	Page        int
	PageCount   int
	TotalRuns   int
	StartRun    int
	EndRun      int
	PreviousURL string
	NextURL     string
}

func NewApp(config config.Config, auth *authservice.Service, commands *commandservice.Service, logger *slog.Logger) *App {
	if logger == nil {
		logger = slog.Default()
	}
	return &App{config: config, auth: auth, commands: commands, logger: logger, sessions: make(map[string]webSession), platform: platform.Current()}
}

func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /login", a.loginPage)
	mux.HandleFunc("POST /login", a.login)
	mux.HandleFunc("GET /register", a.registerPage)
	mux.HandleFunc("POST /register", a.register)
	mux.HandleFunc("GET /{$}", a.protect(false, a.home))
	mux.HandleFunc("GET /users", a.protect(true, a.usersPage))
	mux.HandleFunc("GET /runs", a.protectAdminOrOperator(a.runsPage))
	mux.HandleFunc("GET /runs/availability", a.protect(false, a.runAvailability))
	mux.HandleFunc("GET /runs/{id}/status", a.protectAdminOrOperator(a.runStatus))
	mux.HandleFunc("GET /admin/icons", a.protect(true, a.listCustomIcons))
	mux.HandleFunc("POST /admin/icons", a.protect(true, a.uploadCustomIcon))
	mux.HandleFunc("GET /icons/custom/{id}", a.protect(false, a.customIconAsset))
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(ui.StaticFiles()))))
	mux.HandleFunc("POST /logout", a.protect(false, a.logout))
	mux.HandleFunc("POST /admin/commands", a.protect(true, a.saveCommand))
	mux.HandleFunc("POST /admin/commands/{id}/delete", a.protect(true, a.deleteCommand))
	mux.HandleFunc("POST /admin/users/{username}/role", a.protect(true, a.updateUserRole))
	mux.HandleFunc("POST /admin/users/{username}/delete", a.protect(true, a.deleteUser))
	mux.HandleFunc("POST /commands/{id}/run", a.protect(false, a.runCommand))
	mux.HandleFunc("POST /runs/{id}/interrupt", a.protect(true, a.interruptRun))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'")
		mux.ServeHTTP(w, r)
	})
}

func (a *App) listCustomIcons(w http.ResponseWriter, r *http.Request, user User, csrf string) {
	icons, err := a.commands.ListCustomIcons()
	if err != nil {
		a.serverError(w, "list custom icons", err)
		return
	}
	type iconResponse struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		Key  string `json:"key"`
	}
	response := make([]iconResponse, 0, len(icons))
	for _, icon := range icons {
		response = append(response, iconResponse{ID: icon.ID, Name: icon.Name, Key: fmt.Sprintf("custom-%d", icon.ID)})
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(response)
}

func (a *App) uploadCustomIcon(w http.ResponseWriter, r *http.Request, user User, csrf string) {
	r.Body = http.MaxBytesReader(w, r.Body, 300*1024)
	if err := r.ParseMultipartForm(300 * 1024); err != nil {
		http.Error(w, "SVG-файл слишком большой или поврежден", http.StatusBadRequest)
		return
	}
	file, header, err := r.FormFile("icon")
	if err != nil {
		http.Error(w, "Выберите SVG-файл", http.StatusBadRequest)
		return
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 256*1024+1))
	if err != nil {
		http.Error(w, "Не удалось прочитать SVG-файл", http.StatusBadRequest)
		return
	}
	if err := validateCustomSVG(data); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	if name == "" {
		name = strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
	}
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 80 {
		http.Error(w, "Название иконки должно содержать от 1 до 80 символов", http.StatusBadRequest)
		return
	}
	iconID, err := a.commands.SaveCustomIcon(domain.CustomIcon{Name: name, SVG: string(data)})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			http.Error(w, "Иконка с таким названием уже существует", http.StatusConflict)
			return
		}
		a.serverError(w, "save custom icon", err)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(struct {
		ID   int64  `json:"id"`
		Name string `json:"name"`
		Key  string `json:"key"`
	}{ID: iconID, Name: name, Key: fmt.Sprintf("custom-%d", iconID)})
}

func (a *App) customIconAsset(w http.ResponseWriter, r *http.Request, user User, csrf string) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		http.NotFound(w, r)
		return
	}
	icon, err := a.commands.GetCustomIcon(id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write([]byte(icon.SVG))
}

func validateCustomSVG(data []byte) error {
	if len(data) == 0 || len(data) > 256*1024 {
		return errors.New("SVG-файл должен быть размером от 1 до 256 КБ")
	}
	var root struct {
		XMLName xml.Name
	}
	if err := xml.Unmarshal(data, &root); err != nil || root.XMLName.Local != "svg" {
		return errors.New("файл должен содержать корректный SVG")
	}
	lower := strings.ToLower(string(data))
	for _, marker := range []string{"<script", "javascript:", "onload=", "onclick=", "onerror=", "href=\"http", "xlink:href=\"http"} {
		if strings.Contains(lower, marker) {
			return errors.New("SVG содержит запрещенное содержимое")
		}
	}
	return nil
}

func (a *App) loginPage(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.sessionFromRequest(r); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if err := ui.LoginTemplate.Execute(w, authPageData{Registered: r.URL.Query().Get("registered") == "1"}); err != nil {
		a.logger.Error("render login page", "error", err)
	}
}

func (a *App) registerPage(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := a.sessionFromRequest(r); ok {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	_ = ui.RegisterTemplate.Execute(w, nil)
}

var usernamePattern = regexp.MustCompile(`^[A-Za-zА-Яа-яЁё0-9._-]{4,32}$`)

func (a *App) register(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Некорректная форма", http.StatusBadRequest)
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	if !usernamePattern.MatchString(username) || len(password) < 4 || len(password) > 72 || password != r.FormValue("password_confirm") {
		w.WriteHeader(http.StatusBadRequest)
		_ = ui.RegisterTemplate.Execute(w, struct{ Error string }{Error: "Логин: 4–32 символа (латиница, кириллица, цифры, . _ -). Пароль: 4–72 байта, поля должны совпадать."})
		return
	}
	if err := a.auth.Register(username, password); err != nil {
		w.WriteHeader(http.StatusConflict)
		_ = ui.RegisterTemplate.Execute(w, struct{ Error string }{Error: "Не удалось создать учетную запись. Возможно, такой логин уже занят."})
		return
	}
	a.logger.Info("user registered", "username", username)
	http.Redirect(w, r, "/login?registered=1", http.StatusSeeOther)
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Invalid form submission", http.StatusBadRequest)
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	user, err := a.auth.Authenticate(username, r.FormValue("password"))
	if err != nil {
		w.WriteHeader(http.StatusUnauthorized)
		_ = ui.LoginTemplate.Execute(w, authPageData{Error: "Логин или пароль не подошел"})
		return
	}
	sessionID, err := randomToken(32)
	if err != nil {
		http.Error(w, "Could not create session", http.StatusInternalServerError)
		return
	}
	csrf, err := randomToken(32)
	if err != nil {
		http.Error(w, "Could not create session", http.StatusInternalServerError)
		return
	}
	expires := time.Now().Add(a.config.SessionTTL)
	a.mu.Lock()
	a.sessions[sessionID] = webSession{Username: user.Username, Role: user.Role, CSRF: csrf, Expires: expires}
	a.mu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: sessionID, Path: "/", Expires: expires, MaxAge: int(a.config.SessionTTL.Seconds()), HttpOnly: true, Secure: a.config.CookieSecure, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) logout(w http.ResponseWriter, r *http.Request, user User, csrf string) {
	if cookie, err := r.Cookie(sessionCookieName); err == nil {
		a.mu.Lock()
		delete(a.sessions, cookie.Value)
		a.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookieName, Value: "", Path: "/", MaxAge: -1, Expires: time.Unix(0, 0), HttpOnly: true, Secure: a.config.CookieSecure, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

type protectedHandler func(http.ResponseWriter, *http.Request, User, string)

func (a *App) protect(adminOnly bool, next protectedHandler) http.HandlerFunc {
	return a.protectRoles(adminOnly, false, next)
}

func (a *App) protectAdminOrOperator(next protectedHandler) http.HandlerFunc {
	return a.protectRoles(false, true, next)
}

func (a *App) protectRoles(adminOnly, adminOrOperatorOnly bool, next protectedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, csrf, ok := a.sessionFromRequest(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if adminOnly && user.Role != "admin" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if adminOrOperatorOnly && user.Role != "admin" && user.Role != "operator" {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		if r.Method == http.MethodPost {
			if err := r.ParseForm(); err != nil {
				http.Error(w, "Invalid form submission", http.StatusBadRequest)
				return
			}
			supplied := r.FormValue("csrf")
			if len(supplied) != len(csrf) || subtle.ConstantTimeCompare([]byte(supplied), []byte(csrf)) != 1 {
				http.Error(w, "Invalid request token", http.StatusForbidden)
				return
			}
		}
		next(w, r, user, csrf)
	}
}

func (a *App) sessionFromRequest(r *http.Request) (User, string, bool) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return User{}, "", false
	}
	a.mu.Lock()
	session, ok := a.sessions[cookie.Value]
	if ok && time.Now().After(session.Expires) {
		delete(a.sessions, cookie.Value)
		ok = false
	}
	a.mu.Unlock()
	if !ok {
		return User{}, "", false
	}
	user, err := a.auth.GetUser(session.Username)
	if err != nil {
		return User{}, "", false
	}
	return user, session.CSRF, true
}

func (a *App) usersPage(w http.ResponseWriter, r *http.Request, user User, csrf string) {
	users, err := a.auth.ListUsers("")
	if err != nil {
		a.serverError(w, "list users", err)
		return
	}
	if err := ui.UsersTemplate.Execute(w, usersPageData{User: user, CSRF: csrf, Users: users, Notice: r.URL.Query().Get("notice"), Platform: a.platform}); err != nil {
		a.logger.Error("render users page", "error", err)
	}
}

func (a *App) runsPage(w http.ResponseWriter, r *http.Request, user User, csrf string) {
	pageSizes := []int{10, 20, 50, 100}
	pageSize := 10
	if rawPageSize := r.URL.Query().Get("page_size"); rawPageSize != "" {
		parsedPageSize, err := strconv.Atoi(rawPageSize)
		validPageSize := false
		for _, supportedSize := range pageSizes {
			if parsedPageSize == supportedSize {
				validPageSize = true
				break
			}
		}
		if err != nil || !validPageSize {
			http.Error(w, "Invalid page size", http.StatusBadRequest)
			return
		}
		pageSize = parsedPageSize
	}
	filter := RunFilter{}
	usernames := make([]string, 0)
	if user.Role == "admin" {
		filter = RunFilter{
			Username: r.URL.Query().Get("user"), Command: r.URL.Query().Get("command"),
			Status: r.URL.Query().Get("status"), From: r.URL.Query().Get("from"), To: r.URL.Query().Get("to"),
		}
		if !validDate(filter.From) || !validDate(filter.To) {
			http.Error(w, "Invalid date filter", http.StatusBadRequest)
			return
		}
		if filter.Status != "" && !validRunStatus(filter.Status) {
			http.Error(w, "Invalid status filter", http.StatusBadRequest)
			return
		}
		users, err := a.auth.ListUsers("")
		if err != nil {
			a.serverError(w, "list users for run filter", err)
			return
		}
		for _, account := range users {
			usernames = append(usernames, account.Username)
		}
	} else {
		filter.Username = user.Username
		filter.UsernameExact = true
	}
	page := 1
	if rawPage := r.URL.Query().Get("page"); rawPage != "" {
		parsedPage, err := strconv.Atoi(rawPage)
		if err != nil || parsedPage < 1 {
			http.Error(w, "Invalid page", http.StatusBadRequest)
			return
		}
		page = parsedPage
	}
	totalRuns, err := a.commands.CountRuns(filter)
	if err != nil {
		a.serverError(w, "count command runs", err)
		return
	}
	pageCount := (totalRuns + pageSize - 1) / pageSize
	if pageCount == 0 {
		pageCount = 1
	}
	if page > pageCount {
		page = pageCount
	}
	offset := (page - 1) * pageSize
	runs, err := a.commands.ListRuns(filter, pageSize, offset)
	if err != nil {
		a.serverError(w, "list command runs", err)
		return
	}
	pageURL := func(targetPage int) string {
		query := url.Values{
			"page":      {strconv.Itoa(targetPage)},
			"page_size": {strconv.Itoa(pageSize)},
		}
		if user.Role == "admin" {
			query.Set("user", filter.Username)
			query.Set("command", filter.Command)
			query.Set("status", filter.Status)
			query.Set("from", filter.From)
			query.Set("to", filter.To)
		}
		return "/runs?" + query.Encode()
	}
	startRun, endRun := 0, 0
	if totalRuns > 0 {
		startRun = offset + 1
		endRun = offset + len(runs)
	}
	data := runsPageData{User: user, CSRF: csrf, Runs: runs, Usernames: usernames, Filter: filter, Platform: a.platform, PageSize: pageSize, PageSizes: pageSizes, Page: page, PageCount: pageCount, TotalRuns: totalRuns, StartRun: startRun, EndRun: endRun}
	if page > 1 {
		data.PreviousURL = pageURL(page - 1)
	}
	if page < pageCount {
		data.NextURL = pageURL(page + 1)
	}
	if err := ui.RunsTemplate.Execute(w, data); err != nil {
		a.logger.Error("render run history", "error", err)
	}
}

func validRunStatus(status string) bool {
	switch status {
	case "running", "success", "failed", "timeout", "interrupted":
		return true
	default:
		return false
	}
}

func (a *App) runStatus(w http.ResponseWriter, r *http.Request, user User, csrf string) {
	runID, ok := pathID(w, r)
	if !ok {
		return
	}
	run, err := a.commands.GetRun(runID)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if user.Role != "admin" && run.Username != user.Username {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	duration := run.Duration
	if run.Status == "running" {
		duration = time.Since(run.StartedAt)
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(struct {
		Status             string `json:"status"`
		ExitCode           int    `json:"exit_code"`
		Duration           string `json:"duration"`
		Stdout             string `json:"stdout"`
		Stderr             string `json:"stderr"`
		InterruptRequested bool   `json:"interrupt_requested"`
	}{Status: run.Status, ExitCode: run.ExitCode, Duration: duration.String(), Stdout: run.Stdout, Stderr: run.Stderr, InterruptRequested: a.commands.InterruptRequested(runID)})
}

func (a *App) interruptRun(w http.ResponseWriter, r *http.Request, user User, csrf string) {
	runID, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := a.commands.Interrupt(runID, user.Username, user.Role); err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			http.NotFound(w, r)
		case errors.Is(err, commandservice.ErrForbidden):
			http.Error(w, "Forbidden", http.StatusForbidden)
		case errors.Is(err, commandservice.ErrRunNotActive):
			http.Error(w, "Запуск уже завершен", http.StatusConflict)
		default:
			a.serverError(w, "interrupt command run", err)
		}
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte(`{"interrupt_requested":true}`))
}

func (a *App) runAvailability(w http.ResponseWriter, r *http.Request, user User, csrf string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	visibleCommands, err := a.commands.ListVisible(user)
	if err != nil {
		a.serverError(w, "list visible commands for run availability", err)
		return
	}
	visibleCommandIDs := make(map[int64]struct{}, len(visibleCommands))
	for _, command := range visibleCommands {
		visibleCommandIDs[command.ID] = struct{}{}
	}
	commandIDs := make([]int64, 0)
	activeCommandRuns := a.commands.ActiveCommandRuns()
	commandRuns := make(map[int64]int64)
	for _, commandID := range a.commands.ActiveCommandIDs() {
		if _, visible := visibleCommandIDs[commandID]; visible {
			commandIDs = append(commandIDs, commandID)
			if runID, active := activeCommandRuns[commandID]; active {
				commandRuns[commandID] = runID
			}
		}
	}
	_ = json.NewEncoder(w).Encode(struct {
		Busy         bool            `json:"busy"`
		CommandIDs   []int64         `json:"command_ids"`
		CommandRuns  map[int64]int64 `json:"command_runs"`
		CanInterrupt bool            `json:"can_interrupt"`
	}{Busy: a.commands.HasActiveRun(), CommandIDs: commandIDs, CommandRuns: commandRuns, CanInterrupt: user.Role == "admin"})
}

func (a *App) updateUserRole(w http.ResponseWriter, r *http.Request, actor User, csrf string) {
	username := r.PathValue("username")
	if !usernamePattern.MatchString(username) {
		http.Error(w, "Invalid username", http.StatusBadRequest)
		return
	}
	role := r.FormValue("role")
	if err := a.auth.UpdateRole(username, role); err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			http.NotFound(w, r)
		case errors.Is(err, ErrLastAdmin):
			http.Error(w, "Нельзя снять роль с последнего администратора", http.StatusConflict)
		default:
			http.Error(w, "Недопустимая роль", http.StatusBadRequest)
		}
		return
	}
	a.logger.Info("user role changed", "actor", actor.Username, "username", username, "role", role)
	http.Redirect(w, r, "/users?notice=Роль+пользователя+сохранена", http.StatusSeeOther)
}

func (a *App) deleteUser(w http.ResponseWriter, r *http.Request, actor User, csrf string) {
	username := r.PathValue("username")
	if !usernamePattern.MatchString(username) {
		http.Error(w, "Invalid username", http.StatusBadRequest)
		return
	}
	if err := a.auth.DeleteUser(username, actor.Username); err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			http.NotFound(w, r)
		case errors.Is(err, ErrCannotDeleteAdmin):
			http.Error(w, "Удалять учетные записи администраторов может только суперадминистратор admin", http.StatusForbidden)
		case errors.Is(err, ErrCannotDeleteSelf):
			http.Error(w, "Нельзя удалить активную учетную запись", http.StatusConflict)
		case errors.Is(err, ErrLastAdmin):
			http.Error(w, "Нельзя удалить последнего администратора", http.StatusConflict)
		default:
			a.serverError(w, "delete user", err)
		}
		return
	}
	a.logger.Info("user deleted", "actor", actor.Username, "username", username)
	http.Redirect(w, r, "/users?notice=Учетная+запись+удалена", http.StatusSeeOther)
}

func (a *App) home(w http.ResponseWriter, r *http.Request, user User, csrf string) {
	admin := user.Role == "admin"
	commands, err := a.commands.ListVisible(user)
	if err != nil {
		a.serverError(w, "list commands", err)
		return
	}
	maxTimeout := int(a.config.MaxCommandTimeout / time.Second)
	defaultTimeout := min(30, maxTimeout)
	data := pageData{User: user, CSRF: csrf, Commands: commands, Executables: a.commands.AllowedExecutables(), Notice: r.URL.Query().Get("notice"), MaxTimeoutSeconds: maxTimeout, DefaultTimeoutSeconds: defaultTimeout, Platform: a.platform}
	data.RunPanels, err = a.commands.ActiveRuns(user.Username, user.Role)
	if err != nil {
		a.serverError(w, "load active command run", err)
		return
	}
	data.RunningCommands = make(map[int64]bool)
	for _, commandID := range a.commands.ActiveCommandIDs() {
		data.RunningCommands[commandID] = true
	}
	if admin {
		data.Users, err = a.auth.ListUsers("")
		if err != nil {
			a.serverError(w, "list users for command permissions", err)
			return
		}
		data.Operators, err = a.auth.ListUsers("operator")
		if err != nil {
			a.serverError(w, "list operators for command permissions", err)
			return
		}
	}
	if admin && r.URL.Query().Get("edit") != "" {
		id, err := strconv.ParseInt(r.URL.Query().Get("edit"), 10, 64)
		if err != nil {
			http.Error(w, "Invalid command ID", http.StatusBadRequest)
			return
		}
		command, err := a.commands.Get(id)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		command.Description = base64.StdEncoding.EncodeToString([]byte(command.Description))
		data.Editing = &command
	}
	if len(data.RunPanels) == 0 {
		if runID, err := strconv.ParseInt(r.URL.Query().Get("run"), 10, 64); err == nil && runID > 0 {
			lastRun, err := a.commands.GetRun(runID)
			if err == nil && lastRun.Status != "interrupted" && (admin || (user.Role == "operator" && lastRun.Username == user.Username)) {
				data.RunPanels = append(data.RunPanels, lastRun)
			}
		}
	}
	if err := ui.CommandsTemplate.Execute(w, data); err != nil {
		a.logger.Error("render dashboard", "error", err)
	}
}

func (a *App) saveCommand(w http.ResponseWriter, r *http.Request, user User, csrf string) {
	id := int64(0)
	if rawID := r.FormValue("id"); rawID != "" {
		parsedID, err := strconv.ParseInt(rawID, 10, 64)
		if err != nil || parsedID < 1 {
			http.Error(w, "Invalid command ID", http.StatusBadRequest)
			return
		}
		id = parsedID
	}
	name := strings.TrimSpace(r.FormValue("name"))
	script := strings.TrimSpace(r.FormValue("script"))
	program := strings.TrimSpace(r.FormValue("program"))
	description := strings.TrimSpace(r.FormValue("description"))
	timeoutSeconds, err := strconv.Atoi(r.FormValue("timeout_seconds"))
	if name == "" || err != nil || timeoutSeconds < 1 || time.Duration(timeoutSeconds)*time.Second > a.config.MaxCommandTimeout {
		http.Error(w, "Проверьте название и тайм-аут команды", http.StatusBadRequest)
		return
	}
	args := make([]string, 0)
	if script == "" {
		program = filepath.Clean(program)
		if program == "." || !a.commands.IsExecutableAllowed(program) {
			http.Error(w, "Выберите разрешенный исполняемый файл или задайте скрипт", http.StatusBadRequest)
			return
		}
		for _, line := range strings.Split(strings.ReplaceAll(r.FormValue("args"), "\r", ""), "\n") {
			if line != "" {
				args = append(args, line)
			}
		}
	}
	labelKeys := r.Form["label_key"]
	labelValues := r.Form["label_value"]
	labelColors := r.Form["label_color"]
	labels := make([]domain.Label, 0, len(labelKeys))
	for index, key := range labelKeys {
		value := ""
		color := ""
		if index < len(labelValues) {
			value = labelValues[index]
		}
		if index < len(labelColors) {
			color = labelColors[index]
		}
		if strings.TrimSpace(key) == "" && strings.TrimSpace(value) == "" {
			continue
		}
		labels = append(labels, domain.Label{Key: key, Value: value, Color: color})
	}
	specificUsers := uniqueStrings(r.Form["specific_users"])
	specificOperators := uniqueStrings(r.Form["specific_operators"])
	for _, username := range specificUsers {
		if _, err := a.auth.GetUser(username); err != nil {
			http.Error(w, "Выбран неизвестный пользователь", http.StatusBadRequest)
			return
		}
	}
	for _, username := range specificOperators {
		selected, err := a.auth.GetUser(username)
		if err != nil || selected.Role != "operator" {
			http.Error(w, "В списке конкретных операторов может быть только пользователь с ролью operator", http.StatusBadRequest)
			return
		}
	}
	command := Command{
		ID: id, Name: name, Description: description, Icon: r.FormValue("icon"), Program: program, Args: args, Script: script, Labels: labels,
		TimeoutSeconds: timeoutSeconds, AccessOperators: r.FormValue("access_operators") == "on",
		AccessAllUsers: r.FormValue("access_all_users") == "on", SpecificUsers: specificUsers,
		SpecificOperators: specificOperators, OperatorsCanRun: r.FormValue("operators_can_run") == "on",
	}
	if id > 0 {
		if _, err := a.commands.Get(id); err != nil {
			http.NotFound(w, r)
			return
		}
	}
	if _, err := a.commands.Save(command); err != nil {
		if errors.Is(err, commandservice.ErrInvalidCommand) || errors.Is(err, commandservice.ErrExecutableNotAllowed) {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		a.serverError(w, "save command", err)
		return
	}
	a.logger.Info("command saved", "user", user.Username, "command", name)
	http.Redirect(w, r, "/?notice=Команда+сохранена", http.StatusSeeOther)
}

func (a *App) deleteCommand(w http.ResponseWriter, r *http.Request, user User, csrf string) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := a.commands.Delete(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		a.serverError(w, "delete command", err)
		return
	}
	a.logger.Info("command deleted", "user", user.Username, "command_id", id)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *App) runCommand(w http.ResponseWriter, r *http.Request, user User, csrf string) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	run, err := a.commands.Start(user.Username, user.Role, id)
	if err != nil {
		switch {
		case errors.Is(err, sql.ErrNoRows):
			http.NotFound(w, r)
		case errors.Is(err, commandservice.ErrForbidden), errors.Is(err, commandservice.ErrExecutableNotAllowed):
			http.Error(w, "Forbidden", http.StatusForbidden)
		case errors.Is(err, commandservice.ErrTooManyConcurrent):
			http.Redirect(w, r, "/", http.StatusSeeOther)
		default:
			a.serverError(w, "run command", err)
		}
		return
	}
	a.logger.Info("command executed", "user", user.Username, "command_id", id, "status", run.Status, "duration_ms", run.Duration.Milliseconds())
	a.homeWithRun(w, r, user, csrf, run)
}

func (a *App) homeWithRun(w http.ResponseWriter, r *http.Request, user User, csrf string, run Run) {
	if run.Status == "running" {
		run.Duration = time.Since(run.StartedAt)
	}
	admin := user.Role == "admin"
	commands, err := a.commands.ListVisible(user)
	if err != nil {
		a.serverError(w, "list commands", err)
		return
	}
	maxTimeout := int(a.config.MaxCommandTimeout / time.Second)
	runningCommands := make(map[int64]bool)
	for _, commandID := range a.commands.ActiveCommandIDs() {
		runningCommands[commandID] = true
	}
	runPanels, err := a.commands.ActiveRuns(user.Username, user.Role)
	if err != nil {
		a.serverError(w, "load active command runs", err)
		return
	}
	foundRun := false
	for _, activeRun := range runPanels {
		if activeRun.ID == run.ID {
			foundRun = true
			break
		}
	}
	if !foundRun && run.Status != "interrupted" {
		runPanels = append(runPanels, run)
	}
	data := pageData{User: user, CSRF: csrf, Commands: commands, Executables: a.commands.AllowedExecutables(), RunPanels: runPanels, RunningCommands: runningCommands, MaxTimeoutSeconds: maxTimeout, DefaultTimeoutSeconds: min(30, maxTimeout), Platform: a.platform}
	if admin {
		data.Users, err = a.auth.ListUsers("")
		if err != nil {
			a.serverError(w, "list users for command permissions", err)
			return
		}
		data.Operators, err = a.auth.ListUsers("operator")
		if err != nil {
			a.serverError(w, "list operators for command permissions", err)
			return
		}
	}
	if err := ui.CommandsTemplate.Execute(w, data); err != nil {
		a.logger.Error("render dashboard", "error", err)
	}
}

func (a *App) serverError(w http.ResponseWriter, operation string, err error) {
	a.logger.Error(operation, "error", err)
	http.Error(w, "Внутренняя ошибка приложения", http.StatusInternalServerError)
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}

func canViewCommand(user User, command Command) bool {
	return commandservice.CanView(user, command)
}

func validDate(value string) bool {
	if value == "" {
		return true
	}
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return 0, false
	}
	return id, true
}

func randomToken(byteCount int) (string, error) {
	value := make([]byte, byteCount)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}
