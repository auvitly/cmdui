package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/auvitly/cmdui.git/internal/repository/sqlite"
	authservice "github.com/auvitly/cmdui.git/internal/service/auth"
	commandservice "github.com/auvitly/cmdui.git/internal/service/commands"

	"golang.org/x/crypto/bcrypt"
)

var csrfPattern = regexp.MustCompile(`name="csrf" value="([a-f0-9]+)"`)

func TestOperatorCannotSeeOrRunUnpublishedCommand(t *testing.T) {
	store, app, program := newTestApp(t)
	defer store.Close()
	commandID, err := store.SaveCommand(Command{Name: "private diagnostic", Program: program, Args: []string{"--debug"}, TimeoutSeconds: 3})
	if err != nil {
		t.Fatal(err)
	}

	cookie := loginForTest(t, app.Handler(), "operator", "operator-password")
	page := requestWithCookie(app.Handler(), http.MethodGet, "/", nil, cookie)
	if page.Code != http.StatusOK {
		t.Fatalf("GET / status = %d", page.Code)
	}
	if strings.Contains(page.Body.String(), "private diagnostic") {
		t.Fatal("operator can see an unpublished command")
	}
	csrf := csrfPattern.FindStringSubmatch(page.Body.String())
	if len(csrf) != 2 {
		t.Fatal("dashboard did not include a CSRF token")
	}

	adminPost := requestWithCookie(app.Handler(), http.MethodPost, "/admin/commands", strings.NewReader(""), cookie)
	if adminPost.Code != http.StatusForbidden {
		t.Fatalf("operator admin-route status = %d, want 403", adminPost.Code)
	}

	form := url.Values{"csrf": {csrf[1]}}
	runResponse := requestWithCookie(app.Handler(), http.MethodPost, "/commands/"+strconv.FormatInt(commandID, 10)+"/run", strings.NewReader(form.Encode()), cookie)
	if runResponse.Code != http.StatusForbidden {
		t.Fatalf("operator unpublished-command status = %d, want 403", runResponse.Code)
	}
}

func TestRegistrationCreatesReadOnlyUser(t *testing.T) {
	store, app, program := newTestApp(t)
	defer store.Close()
	form := url.Values{
		"username": {"reader.one"}, "password": {"reader-password-123"},
		"password_confirm": {"reader-password-123"}, "role": {"admin"},
	}
	request := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/login?registered=1" {
		t.Fatalf("registration status/location = %d/%q", response.Code, response.Header().Get("Location"))
	}
	user, err := store.GetUser("reader.one")
	if err != nil {
		t.Fatal(err)
	}
	if user.Role != "user" {
		t.Fatalf("registered role = %q, want user", user.Role)
	}
	if err := bcrypt.CompareHashAndPassword(user.PasswordHash, []byte("reader-password-123")); err != nil {
		t.Fatalf("registered password is not stored as a bcrypt hash: %v", err)
	}
	runID, err := store.SaveRun(Run{
		Username: "reader.one", CommandName: "old command", Status: "success", ExitCode: 0,
		StartedAt: time.Now().UTC(), Stdout: "private prior output",
	})
	if err != nil {
		t.Fatal(err)
	}

	commandID, err := store.SaveCommand(Command{Name: "read-only visible", Program: program, TimeoutSeconds: 3, AccessAllUsers: true, OperatorsCanRun: true})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginForTest(t, app.Handler(), "reader.one", "reader-password-123")
	page := requestWithCookie(app.Handler(), http.MethodGet, "/?run="+strconv.FormatInt(runID, 10), nil, cookie)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "read-only visible") {
		t.Fatalf("registered user cannot view shared command, status = %d", page.Code)
	}
	if strings.Contains(page.Body.String(), "Запустить") || strings.Contains(page.Body.String(), "История запусков") || strings.Contains(page.Body.String(), "private prior output") {
		t.Fatal("read-only user sees command execution or run-history controls")
	}
	csrf := csrfPattern.FindStringSubmatch(page.Body.String())
	if len(csrf) != 2 {
		t.Fatal("dashboard did not include a CSRF token")
	}
	runForm := url.Values{"csrf": {csrf[1]}}
	runResponse := requestWithCookie(app.Handler(), http.MethodPost, "/commands/"+strconv.FormatInt(commandID, 10)+"/run", strings.NewReader(runForm.Encode()), cookie)
	if runResponse.Code != http.StatusForbidden {
		t.Fatalf("read-only user command-run status = %d, want 403", runResponse.Code)
	}
}

func TestRegistrationAcceptsFourCharacterPasswordAndRejectsThree(t *testing.T) {
	store, app, _ := newTestApp(t)
	defer store.Close()
	register := func(username, password string) int {
		form := url.Values{"username": {username}, "password": {password}, "password_confirm": {password}}
		request := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response := httptest.NewRecorder()
		app.Handler().ServeHTTP(response, request)
		return response.Code
	}
	if status := register("readerfour", "abcd"); status != http.StatusSeeOther {
		t.Fatalf("four-character password registration status = %d, want 303", status)
	}
	if status := register("readerthree", "abc"); status != http.StatusBadRequest {
		t.Fatalf("three-character password registration status = %d, want 400", status)
	}
}

func TestUsernameValidationSupportsCyrillicAndRequiresFourCharacters(t *testing.T) {
	for _, username := range []string{"user1", "Иван42", "Ёжик_7", "тест-user"} {
		if !usernamePattern.MatchString(username) {
			t.Errorf("usernamePattern rejected valid username %q", username)
		}
	}
	for _, username := range []string{"abc", "ab!cd", "user name", "имя🙂"} {
		if usernamePattern.MatchString(username) {
			t.Errorf("usernamePattern accepted invalid username %q", username)
		}
	}
}

func TestAdminUsersPageChangesRolesAndProtectsLastAdmin(t *testing.T) {
	store, app, _ := newTestApp(t)
	defer store.Close()
	adminCookie := loginForTest(t, app.Handler(), "admin", "admin-password")
	page := requestWithCookie(app.Handler(), http.MethodGet, "/users", nil, adminCookie)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "Пользователи") {
		t.Fatalf("admin users page status/body mismatch: %d", page.Code)
	}
	if !strings.Contains(page.Body.String(), "operator") {
		t.Fatal("admin users page does not list configured users")
	}
	for _, expected := range []string{`data-confirm-action`, `data-confirm-title="Удалить пользователя?"`, `data-confirm-dialog`, `/static/confirm-actions.js`} {
		if !strings.Contains(page.Body.String(), expected) {
			t.Errorf("admin users page does not include confirmation UI %q", expected)
		}
	}
	if strings.Contains(page.Body.String(), "onsubmit=") || strings.Contains(page.Body.String(), "window.confirm") {
		t.Fatal("user deletion still uses a browser-native confirmation")
	}
	csrf := csrfPattern.FindStringSubmatch(page.Body.String())
	if len(csrf) != 2 {
		t.Fatal("users page did not include a CSRF token")
	}

	roleForm := url.Values{"csrf": {csrf[1]}, "role": {"user"}}
	lastAdminResponse := requestWithCookie(app.Handler(), http.MethodPost, "/admin/users/admin/role", strings.NewReader(roleForm.Encode()), adminCookie)
	if lastAdminResponse.Code != http.StatusConflict {
		t.Fatalf("demoting the last admin status = %d, want 409", lastAdminResponse.Code)
	}

	operatorCookie := loginForTest(t, app.Handler(), "operator", "operator-password")
	roleForm.Set("role", "user")
	changeResponse := requestWithCookie(app.Handler(), http.MethodPost, "/admin/users/operator/role", strings.NewReader(roleForm.Encode()), adminCookie)
	if changeResponse.Code != http.StatusSeeOther {
		t.Fatalf("operator role update status = %d", changeResponse.Code)
	}
	updated, err := store.GetUser("operator")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Role != "user" {
		t.Fatalf("updated role = %q, want user", updated.Role)
	}
	updatedSessionPage := requestWithCookie(app.Handler(), http.MethodGet, "/", nil, operatorCookie)
	if !strings.Contains(updatedSessionPage.Body.String(), "Пользователь") || strings.Contains(updatedSessionPage.Body.String(), "Оператор") {
		t.Fatal("existing session did not pick up the updated role")
	}
	forbidden := requestWithCookie(app.Handler(), http.MethodGet, "/users", nil, operatorCookie)
	if forbidden.Code != http.StatusForbidden {
		t.Fatalf("non-admin users page status = %d, want 403", forbidden.Code)
	}
}

func TestUserDeletionPermissionsForAdminAndSuperadmin(t *testing.T) {
	store, app, _ := newTestApp(t)
	defer store.Close()
	managerHash, err := bcrypt.GenerateFromPassword([]byte("manager-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	admin2Hash, err := bcrypt.GenerateFromPassword([]byte("admin2-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range []User{
		{Username: "manager", Role: "admin", PasswordHash: managerHash},
		{Username: "admin2", Role: "admin", PasswordHash: admin2Hash},
		{Username: "reader", Role: "user", PasswordHash: []byte("reader-hash")},
	} {
		if err := store.SaveUser(user); err != nil {
			t.Fatal(err)
		}
	}
	managerCookie := loginForTest(t, app.Handler(), "admin", "admin-password")
	if err := store.UpdateUserRole("admin", "operator"); err != nil {
		t.Fatal(err)
	}
	managerSession := loginForTest(t, app.Handler(), "manager", "manager-password")

	deleteAs := func(cookie *http.Cookie, csrf, username string) *httptest.ResponseRecorder {
		form := url.Values{"csrf": {csrf}}
		return requestWithCookie(app.Handler(), http.MethodPost, "/admin/users/"+username+"/delete", strings.NewReader(form.Encode()), cookie)
	}

	managerPage := requestWithCookie(app.Handler(), http.MethodGet, "/users", nil, managerSession)
	managerCSRF := csrfPattern.FindStringSubmatch(managerPage.Body.String())
	if len(managerCSRF) != 2 {
		t.Fatal("manager page missing CSRF token")
	}
	if response := deleteAs(managerSession, managerCSRF[1], "reader"); response.Code != http.StatusSeeOther {
		t.Fatalf("admin deleting regular user status = %d, want 303", response.Code)
	}
	if response := deleteAs(managerSession, managerCSRF[1], "admin2"); response.Code != http.StatusForbidden {
		t.Fatalf("admin deleting another admin status = %d, want 403", response.Code)
	}

	if err := store.UpdateUserRole("admin", "admin"); err != nil {
		t.Fatal(err)
	}
	superadminPage := requestWithCookie(app.Handler(), http.MethodGet, "/users", nil, managerCookie)
	superadminCSRF := csrfPattern.FindStringSubmatch(superadminPage.Body.String())
	if len(superadminCSRF) != 2 {
		t.Fatal("superadmin page missing CSRF token")
	}
	if response := deleteAs(managerCookie, superadminCSRF[1], "admin2"); response.Code != http.StatusSeeOther {
		t.Fatalf("superadmin deleting another admin status = %d, want 303", response.Code)
	}
	if _, err := store.GetUser("admin2"); err == nil {
		t.Fatal("superadmin deletion did not remove the admin account")
	}
	if response := deleteAs(managerCookie, superadminCSRF[1], "admin"); response.Code != http.StatusConflict {
		t.Fatalf("superadmin deleting own account status = %d, want 409", response.Code)
	}
}

func TestSidebarOrderIsConsistentAcrossAdminPages(t *testing.T) {
	store, app, _ := newTestApp(t)
	defer store.Close()
	cookie := loginForTest(t, app.Handler(), "admin", "admin-password")
	for _, path := range []string{"/", "/users"} {
		response := requestWithCookie(app.Handler(), http.MethodGet, path, nil, cookie)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d", path, response.Code)
		}
		body := response.Body.String()
		commandIndex := strings.Index(body, ">Команды</a>")
		usersIndex := strings.Index(body, ">Пользователи</a>")
		historyIndex := strings.Index(body, ">История запусков</a>")
		if commandIndex < 0 || usersIndex < 0 || historyIndex < 0 || !(commandIndex < usersIndex && usersIndex < historyIndex) {
			t.Fatalf("sidebar order is inconsistent at %s", path)
		}
	}
}

func TestCommandsPageRendersModalEditorAndSeparateHistoryLink(t *testing.T) {
	store, app, _ := newTestApp(t)
	defer store.Close()
	commandID, err := store.SaveCommand(Command{Name: "confirm test", Script: "echo before", TimeoutSeconds: 5})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginForTest(t, app.Handler(), "admin", "admin-password")
	page := requestWithCookie(app.Handler(), http.MethodGet, "/?edit="+strconv.FormatInt(commandID, 10), nil, cookie)
	if page.Code != http.StatusOK {
		t.Fatalf("commands page status = %d", page.Code)
	}
	body := page.Body.String()
	for _, required := range []string{
		`<dialog`, `id="command-dialog"`, `name="script"`, `id="label-row-template"`,
		`type="color"`, `data-rgb="R"`, `data-rgb="G"`, `data-rgb="B"`, `href="/runs"`,
		`id="add-label"`, `data-icon-key="terminal"`, `data-icon-key="database"`, `Основное`, `Скрипт`, `Доступ`,
		`href="/static/app.css"`, `data-icon-key="go-color"`, `data-icon-key="go-mono"`, `data-icon-key="grpcui-color"`,
		`data-icon-key="grpcui-mono"`, `data-icon-key="podman-color"`, `data-icon-key="podman-mono"`,
		`class="icon-picker-options"`, `aria-haspopup="listbox"`, `src="/static/icons/go.svg"`, `src="/static/icons/grpc.svg"`, `src="/static/icons/podman.svg"`,
		`data-confirm-action`, `data-confirm-title="Удалить команду?"`, `data-confirm-dialog`,
		`data-confirm-script`, `src="/static/confirm-actions.js"`,
		`class="color-popover" hidden`, `class="form-section"`, `id="command-icon"`,
	} {
		if !strings.Contains(body, required) {
			t.Errorf("commands page does not contain %q", required)
		}
	}
	if strings.Contains(body, `id="runs"`) {
		t.Fatal("run history is still embedded on the commands page")
	}
	if !strings.Contains(body, `<div id="labels-editor"></div><template id="label-row-template">`) {
		t.Fatal("empty command editor should have no label rows before the plus button is clicked")
	}
}

func TestRegistrationCannotOverwriteExistingAccount(t *testing.T) {
	store, app, _ := newTestApp(t)
	defer store.Close()
	form := url.Values{
		"username": {"operator"}, "password": {"attacker-password-123"},
		"password_confirm": {"attacker-password-123"},
	}
	request := httptest.NewRequest(http.MethodPost, "/register", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("duplicate registration status = %d, want 409", response.Code)
	}
	user, err := store.GetUser("operator")
	if err != nil {
		t.Fatal(err)
	}
	if user.Role != "operator" || bcrypt.CompareHashAndPassword(user.PasswordHash, []byte("operator-password")) != nil {
		t.Fatal("duplicate registration changed the existing account")
	}
}

func TestCommandVisibilityScopes(t *testing.T) {
	operator := User{Username: "ops-a", Role: "operator"}
	reader := User{Username: "reader-a", Role: "user"}
	tests := []struct {
		name    string
		command Command
		user    User
		want    bool
	}{
		{name: "operators scope", command: Command{AccessOperators: true}, user: operator, want: true},
		{name: "operators scope excludes readers", command: Command{AccessOperators: true}, user: reader},
		{name: "all users scope", command: Command{AccessAllUsers: true}, user: reader, want: true},
		{name: "specific user scope", command: Command{SpecificUsers: []string{"reader-a"}}, user: reader, want: true},
		{name: "specific operator scope", command: Command{SpecificOperators: []string{"ops-a"}}, user: operator, want: true},
		{name: "specific operator scope excludes reader", command: Command{SpecificOperators: []string{"ops-a"}}, user: reader},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := canViewCommand(test.user, test.command); got != test.want {
				t.Fatalf("canViewCommand() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestOperatorViewPermissionDoesNotGrantExecution(t *testing.T) {
	store, app, program := newTestApp(t)
	defer store.Close()
	commandID, err := store.SaveCommand(Command{Name: "view only for operators", Program: program, TimeoutSeconds: 3, AccessOperators: true})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginForTest(t, app.Handler(), "operator", "operator-password")
	page := requestWithCookie(app.Handler(), http.MethodGet, "/", nil, cookie)
	if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), "view only for operators") {
		t.Fatalf("operator cannot view permitted command, status = %d", page.Code)
	}
	if strings.Contains(page.Body.String(), "Запустить") {
		t.Fatal("view permission unexpectedly showed a run action")
	}
	csrf := csrfPattern.FindStringSubmatch(page.Body.String())
	if len(csrf) != 2 {
		t.Fatal("dashboard did not include a CSRF token")
	}
	form := url.Values{"csrf": {csrf[1]}}
	response := requestWithCookie(app.Handler(), http.MethodPost, "/commands/"+strconv.FormatInt(commandID, 10)+"/run", strings.NewReader(form.Encode()), cookie)
	if response.Code != http.StatusForbidden {
		t.Fatalf("operator run status = %d, want 403", response.Code)
	}
}

func TestRunsPageIsAvailableOnlyToOperatorsAndAdmins(t *testing.T) {
	store, app, _ := newTestApp(t)
	defer store.Close()
	userHash, err := bcrypt.GenerateFromPassword([]byte("reader-password-123"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveUser(User{Username: "reader", Role: "user", PasswordHash: userHash}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveRun(Run{
		Username: "operator", CommandName: "visible run", Program: "/usr/bin/tool", Args: []string{"--fast", "value with spaces"},
		Status: "success", StartedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.SaveRun(Run{Username: "admin", CommandName: "saved script", CommandScript: "echo saved-script", Status: "success", StartedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	operatorCookie := loginForTest(t, app.Handler(), "operator", "operator-password")
	operatorPage := requestWithCookie(app.Handler(), http.MethodGet, "/runs", nil, operatorCookie)
	if operatorPage.Code != http.StatusOK || !strings.Contains(operatorPage.Body.String(), "visible run") {
		t.Fatalf("operator history page status/body mismatch: %d", operatorPage.Code)
	}
	if strings.Contains(operatorPage.Body.String(), "Показать запуск") {
		t.Fatal("operator history should not expose command invocation snapshots")
	}
	adminCookie := loginForTest(t, app.Handler(), "admin", "admin-password")
	adminPage := requestWithCookie(app.Handler(), http.MethodGet, "/runs", nil, adminCookie)
	for _, expected := range []string{`list="run-user-suggestions"`, `<datalist id="run-user-suggestions">`, `<option value="admin">`, `<option value="operator">`, `<option value="reader">`} {
		if adminPage.Code != http.StatusOK || !strings.Contains(adminPage.Body.String(), expected) {
			t.Fatalf("admin history page missing database-backed suggestion %q: %d", expected, adminPage.Code)
		}
	}
	for _, expected := range []string{`<th>Запуск</th>`, "Показать запуск", "/usr/bin/tool", "--fast", "value with spaces", "echo saved-script"} {
		if !strings.Contains(adminPage.Body.String(), expected) {
			t.Errorf("admin history page does not contain invocation snapshot %q", expected)
		}
	}
	userCookie := loginForTest(t, app.Handler(), "reader", "reader-password-123")
	userPage := requestWithCookie(app.Handler(), http.MethodGet, "/runs", nil, userCookie)
	if userPage.Code != http.StatusForbidden {
		t.Fatalf("user history page status = %d, want 403", userPage.Code)
	}
}

func TestRunsPaginationPreservesFiltersAndIncludesCurrentStatuses(t *testing.T) {
	store, app, _ := newTestApp(t)
	defer store.Close()
	startedAt := time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)
	for index := 0; index < 52; index++ {
		_, err := store.SaveRun(Run{
			Username: "operator", CommandName: "deploy-api", Status: "running", ExitCode: -1,
			StartedAt: startedAt.Add(time.Duration(index) * time.Minute),
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	cookie := loginForTest(t, app.Handler(), "admin", "admin-password")
	page := requestWithCookie(app.Handler(), http.MethodGet,
		"/runs?user=operator&command=deploy-api&status=running&from=2026-09-20&to=2026-09-21&page_size=20&page=2", nil, cookie)
	if page.Code != http.StatusOK {
		t.Fatalf("page 2 status = %d, body=%s", page.Code, page.Body.String())
	}
	body := page.Body.String()
	for _, expected := range []string{
		`Страница <strong>2</strong> из 3`, `<strong>21–40</strong> из <strong>52</strong>`,
		`<option value="20" selected>20</option>`,
		`<button class="button" type="submit">Применить</button>`,
		`<option value="running" selected>Выполняется</option>`,
		`value="interrupted"`, `Прервано</option>`,
		`href="/runs?command=deploy-api&amp;from=2026-09-20&amp;page=1&amp;page_size=20&amp;status=running&amp;to=2026-09-21&amp;user=operator"`,
	} {
		if !strings.Contains(body, expected) {
			t.Errorf("page 2 does not contain %q", expected)
		}
	}
	if got := strings.Count(body, `<span class="status running">Выполняется</span>`); got != 20 {
		t.Fatalf("page 2 rendered %d running entries, want 20", got)
	}
	invalidPageSize := requestWithCookie(app.Handler(), http.MethodGet, "/runs?page_size=15", nil, cookie)
	if invalidPageSize.Code != http.StatusBadRequest {
		t.Fatalf("unsupported page size status = %d, want 400", invalidPageSize.Code)
	}
}

func TestRunsPaginationDefaultsToTenDatabaseRows(t *testing.T) {
	store, app, _ := newTestApp(t)
	defer store.Close()
	startedAt := time.Date(2026, time.September, 20, 12, 0, 0, 0, time.UTC)
	for index := 1; index <= 26; index++ {
		if _, err := store.SaveRun(Run{
			Username: "admin", CommandName: "database row " + strconv.Itoa(index), Status: "success",
			StartedAt: startedAt.Add(time.Duration(index) * time.Minute),
		}); err != nil {
			t.Fatal(err)
		}
	}
	cookie := loginForTest(t, app.Handler(), "admin", "admin-password")
	firstPage := requestWithCookie(app.Handler(), http.MethodGet, "/runs", nil, cookie)
	if firstPage.Code != http.StatusOK {
		t.Fatalf("default history page status = %d, body=%s", firstPage.Code, firstPage.Body.String())
	}
	firstBody := firstPage.Body.String()
	if !strings.Contains(firstBody, `Страница <strong>1</strong> из 3`) || !strings.Contains(firstBody, `<strong>1–10</strong> из <strong>26</strong>`) {
		t.Fatalf("default page summary does not reflect ten-row database paging: %s", firstBody)
	}
	if got := strings.Count(firstBody, `<span class="status success">Успешно</span>`); got != 10 {
		t.Fatalf("default history page returned %d rows, want 10", got)
	}
	if !strings.Contains(firstBody, "database row 26") || strings.Contains(firstBody, "database row 16") {
		t.Fatal("first page does not contain the newest ten database rows")
	}

	secondPage := requestWithCookie(app.Handler(), http.MethodGet, "/runs?page=2&page_size=10", nil, cookie)
	if secondPage.Code != http.StatusOK {
		t.Fatalf("second history page status = %d, body=%s", secondPage.Code, secondPage.Body.String())
	}
	secondBody := secondPage.Body.String()
	if !strings.Contains(secondBody, `Страница <strong>2</strong> из 3`) || !strings.Contains(secondBody, `<strong>11–20</strong> из <strong>26</strong>`) {
		t.Fatalf("second page summary is incorrect: %s", secondBody)
	}
	if got := strings.Count(secondBody, `<span class="status success">Успешно</span>`); got != 10 {
		t.Fatalf("second history page returned %d rows, want 10", got)
	}
	if !strings.Contains(secondBody, "database row 16") || strings.Contains(secondBody, "database row 26") {
		t.Fatal("second page did not fetch the next ten database rows")
	}
}

func TestRunsPageTranslatesStatusesToRussian(t *testing.T) {
	store, app, _ := newTestApp(t)
	defer store.Close()
	statuses := []string{"running", "success", "failed", "timeout", "interrupted"}
	for _, status := range statuses {
		if _, err := store.SaveRun(Run{Username: "admin", CommandName: "status test", Status: status, StartedAt: time.Now().UTC()}); err != nil {
			t.Fatal(err)
		}
	}
	cookie := loginForTest(t, app.Handler(), "admin", "admin-password")
	page := requestWithCookie(app.Handler(), http.MethodGet, "/runs", nil, cookie)
	for _, expected := range []string{
		`class="status running">Выполняется</span>`,
		`class="status success">Успешно</span>`,
		`class="status failed">Ошибка</span>`,
		`class="status timeout">Тайм-аут</span>`,
		`class="status interrupted">Прервано</span>`,
	} {
		if page.Code != http.StatusOK || !strings.Contains(page.Body.String(), expected) {
			t.Errorf("history page missing localized status %q: %d", expected, page.Code)
		}
	}
}

func TestRunningCommandBlocksOnlyItselfAndOtherRunsCanBeInterrupted(t *testing.T) {
	store, app, _ := newTestApp(t)
	defer store.Close()
	script := "Start-Sleep -Seconds 20\nWrite-Output done"
	if runtime.GOOS != "windows" {
		script = "sleep 20\necho done"
	}
	firstID, err := app.commands.Save(Command{Name: "long run", Script: script, TimeoutSeconds: 10})
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := app.commands.Save(Command{Name: "queued run", Script: script, TimeoutSeconds: 10})
	if err != nil {
		t.Fatal(err)
	}
	cookie := loginForTest(t, app.Handler(), "admin", "admin-password")
	page := requestWithCookie(app.Handler(), http.MethodGet, "/", nil, cookie)
	csrf := csrfPattern.FindStringSubmatch(page.Body.String())
	if len(csrf) != 2 {
		t.Fatal("dashboard did not include CSRF token")
	}
	form := url.Values{"csrf": {csrf[1]}}
	firstResponse := requestWithCookie(app.Handler(), http.MethodPost, "/commands/"+strconv.FormatInt(firstID, 10)+"/run", strings.NewReader(form.Encode()), cookie)
	if firstResponse.Code != http.StatusOK || !strings.Contains(firstResponse.Body.String(), "Прервать") || !strings.Contains(firstResponse.Body.String(), "data-run-duration") || !strings.Contains(firstResponse.Body.String(), "run-spinner") || !strings.Contains(firstResponse.Body.String(), "Последний запуск: admin · ") || !strings.Contains(firstResponse.Body.String(), " UTC") || !strings.Contains(firstResponse.Body.String(), `aria-label="Команда уже выполняется"`) || !strings.Contains(firstResponse.Body.String(), `aria-label="Запустить"`) || !strings.Contains(firstResponse.Body.String(), `/static/icons/play.svg`) {
		t.Fatalf("first run response status/body mismatch: %d", firstResponse.Code)
	}
	reloadedPage := requestWithCookie(app.Handler(), http.MethodGet, "/", nil, cookie)
	if reloadedPage.Code != http.StatusOK || !strings.Contains(reloadedPage.Body.String(), "Прервать") {
		t.Fatalf("active run did not survive dashboard reload: %d", reloadedPage.Code)
	}
	runningStatus := requestWithCookie(app.Handler(), http.MethodGet, "/runs/1/status", nil, cookie)
	if runningStatus.Code != http.StatusOK || !strings.Contains(runningStatus.Body.String(), `"duration":"`) {
		t.Fatalf("running status did not include elapsed duration: %d %s", runningStatus.Code, runningStatus.Body.String())
	}
	availabilityResponse := requestWithCookie(app.Handler(), http.MethodGet, "/runs/availability", nil, cookie)
	if availabilityResponse.Code != http.StatusOK || !strings.Contains(availabilityResponse.Body.String(), `"busy":true`) {
		t.Fatalf("active run availability = %d %s, want busy=true", availabilityResponse.Code, availabilityResponse.Body.String())
	}
	operatorCookie := loginForTest(t, app.Handler(), "operator", "operator-password")
	operatorPage := requestWithCookie(app.Handler(), http.MethodGet, "/", nil, operatorCookie)
	if operatorPage.Code != http.StatusOK || strings.Contains(operatorPage.Body.String(), "Прервать") {
		t.Fatalf("operator dashboard should not offer interruption: %d", operatorPage.Code)
	}
	if err := app.commands.Interrupt(1, "operator", "operator"); !errors.Is(err, commandservice.ErrForbidden) {
		t.Fatalf("operator service interrupt error = %v, want forbidden", err)
	}
	operatorInterruptResponse := requestWithCookie(app.Handler(), http.MethodPost, "/runs/1/interrupt", strings.NewReader(form.Encode()), operatorCookie)
	if operatorInterruptResponse.Code != http.StatusForbidden {
		t.Fatalf("operator interrupt status = %d, want 403", operatorInterruptResponse.Code)
	}
	secondResponse := requestWithCookie(app.Handler(), http.MethodPost, "/commands/"+strconv.FormatInt(secondID, 10)+"/run", strings.NewReader(form.Encode()), cookie)
	if secondResponse.Code != http.StatusOK || !strings.Contains(secondResponse.Body.String(), "Последний запуск: queued run") {
		t.Fatalf("different command did not start concurrently: %d %s", secondResponse.Code, secondResponse.Body.String())
	}
	repeatedResponse := requestWithCookie(app.Handler(), http.MethodPost, "/commands/"+strconv.FormatInt(firstID, 10)+"/run", strings.NewReader(form.Encode()), cookie)
	if repeatedResponse.Code != http.StatusSeeOther || repeatedResponse.Header().Get("Location") != "/" {
		t.Fatalf("duplicate command run response = %d, location=%q, want redirect to dashboard", repeatedResponse.Code, repeatedResponse.Header().Get("Location"))
	}
	busyPage := requestWithCookie(app.Handler(), http.MethodGet, "/", nil, cookie)
	if busyPage.Code != http.StatusOK || strings.Count(busyPage.Body.String(), `aria-label="Команда уже выполняется"`) != 2 {
		t.Fatalf("dashboard did not disable exactly the two active commands: %d", busyPage.Code)
	}
	for _, runID := range []int{1, 2} {
		interruptResponse := requestWithCookie(app.Handler(), http.MethodPost, "/runs/"+strconv.Itoa(runID)+"/interrupt", strings.NewReader(form.Encode()), cookie)
		if interruptResponse.Code != http.StatusAccepted {
			t.Fatalf("interrupt response for run %d = %d", runID, interruptResponse.Code)
		}
	}
	waitContext, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	completed, err := app.commands.Wait(waitContext, 1)
	if err != nil {
		t.Fatalf("wait for interrupted run: %v", err)
	}
	if completed.Status != "interrupted" {
		t.Fatalf("run status = %q, want interrupted", completed.Status)
	}
	completedSecond, err := app.commands.Wait(waitContext, 2)
	if err != nil {
		t.Fatalf("wait for second interrupted run: %v", err)
	}
	if completedSecond.Status != "interrupted" {
		t.Fatalf("second run status = %q, want interrupted", completedSecond.Status)
	}
	if completed.ExitCode != -1 {
		t.Fatalf("interrupted run exit code = %d, want -1", completed.ExitCode)
	}
	if completed.CommandScript != script {
		t.Fatalf("interrupted run script snapshot = %q, want %q", completed.CommandScript, script)
	}
	completedPage := requestWithCookie(app.Handler(), http.MethodGet, "/", nil, cookie)
	if completedPage.Code != http.StatusOK || strings.Contains(completedPage.Body.String(), "data-run-status-url") {
		t.Fatalf("interrupted run remained on dashboard: %d", completedPage.Code)
	}
	availabilityResponse = requestWithCookie(app.Handler(), http.MethodGet, "/runs/availability", nil, cookie)
	if availabilityResponse.Code != http.StatusOK || !strings.Contains(availabilityResponse.Body.String(), `"busy":false`) {
		t.Fatalf("completed run availability = %d %s, want busy=false", availabilityResponse.Code, availabilityResponse.Body.String())
	}
	for _, runID := range []int{1, 2} {
		statusResponse := requestWithCookie(app.Handler(), http.MethodGet, "/runs/"+strconv.Itoa(runID)+"/status", nil, cookie)
		if statusResponse.Code != http.StatusOK || !strings.Contains(statusResponse.Body.String(), `"status":"interrupted"`) {
			t.Fatalf("final status response for run %d = %d %s", runID, statusResponse.Code, statusResponse.Body.String())
		}
	}
}

func TestAdminCanCreateCommandButItStartsUnpublished(t *testing.T) {
	store, app, program := newTestApp(t)
	defer store.Close()
	adminCookie := loginForTest(t, app.Handler(), "admin", "admin-password")
	page := requestWithCookie(app.Handler(), http.MethodGet, "/", nil, adminCookie)
	csrf := csrfPattern.FindStringSubmatch(page.Body.String())
	if len(csrf) != 2 {
		t.Fatal("dashboard did not include a CSRF token")
	}
	form := url.Values{
		"csrf": {csrf[1]}, "name": {"safe status"}, "description": {"Shows service status"},
		"program": {program}, "args": {"--short\nvalue with spaces"}, "timeout_seconds": {"5"},
	}
	response := requestWithCookie(app.Handler(), http.MethodPost, "/admin/commands", strings.NewReader(form.Encode()), adminCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("create command status = %d, body=%s", response.Code, response.Body.String())
	}
	commands, err := store.ListCommands(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 1 || commands[0].OperatorAllowed || len(commands[0].Args) != 2 {
		t.Fatalf("new command is not safely initialized: %#v", commands)
	}
}

func TestAdminSavesCommandVisibilityScopes(t *testing.T) {
	store, app, program := newTestApp(t)
	defer store.Close()
	if err := store.SaveUser(User{Username: "reader", Role: "user", PasswordHash: []byte("hash")}); err != nil {
		t.Fatal(err)
	}
	adminCookie := loginForTest(t, app.Handler(), "admin", "admin-password")
	page := requestWithCookie(app.Handler(), http.MethodGet, "/", nil, adminCookie)
	csrf := csrfPattern.FindStringSubmatch(page.Body.String())
	if len(csrf) != 2 {
		t.Fatal("admin dashboard did not include a CSRF token")
	}
	form := url.Values{
		"csrf": {csrf[1]}, "name": {"scoped command"}, "program": {program}, "timeout_seconds": {"5"},
		"access_operators": {"on"}, "access_all_users": {"on"}, "specific_users": {"reader"},
		"specific_operators": {"operator"}, "operators_can_run": {"on"},
	}
	response := requestWithCookie(app.Handler(), http.MethodPost, "/admin/commands", strings.NewReader(form.Encode()), adminCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("save command status = %d, body=%s", response.Code, response.Body.String())
	}
	commands, err := store.ListCommands(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 1 || !commands[0].AccessOperators || !commands[0].AccessAllUsers ||
		!commands[0].OperatorsCanRun || len(commands[0].SpecificUsers) != 1 || commands[0].SpecificUsers[0] != "reader" ||
		len(commands[0].SpecificOperators) != 1 || commands[0].SpecificOperators[0] != "operator" {
		t.Fatalf("saved command ACL = %#v", commands)
	}
}

func TestCommandScriptAndLabelsAreSavedWithColors(t *testing.T) {
	store, app, _ := newTestApp(t)
	defer store.Close()
	adminCookie := loginForTest(t, app.Handler(), "admin", "admin-password")
	page := requestWithCookie(app.Handler(), http.MethodGet, "/", nil, adminCookie)
	csrf := csrfPattern.FindStringSubmatch(page.Body.String())
	if len(csrf) != 2 {
		t.Fatal("admin commands page missing CSRF token")
	}
	form := url.Values{
		"csrf": {csrf[1]}, "name": {"colored script"}, "script": {"Write-Output first\nWrite-Output second"},
		"timeout_seconds": {"5"}, "icon": {"terminal"}, "label_key": {"team", "stage"}, "label_value": {"core", "test"},
		"label_color": {"#dcebfA", "#12ABEF"},
	}
	response := requestWithCookie(app.Handler(), http.MethodPost, "/admin/commands", strings.NewReader(form.Encode()), adminCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("save script command status = %d, body=%s", response.Code, response.Body.String())
	}
	commands, err := store.ListCommands(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 1 || commands[0].Script != "Write-Output first\nWrite-Output second" || len(commands[0].Labels) != 2 || commands[0].Icon != "terminal" {
		t.Fatalf("saved script/labels = %#v", commands)
	}
	if commands[0].Labels[0].Color != "#DCEBFA" || commands[0].Labels[1].Color != "#12ABEF" {
		t.Fatalf("saved label colors = %#v", commands[0].Labels)
	}
}

func TestLoginSetsProtectedSessionCookie(t *testing.T) {
	store, app, _ := newTestApp(t)
	defer store.Close()
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(url.Values{"username": {"admin"}, "password": {"admin-password"}}.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	app.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("login status = %d", response.Code)
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != sessionCookieName || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatalf("session cookie does not have required attributes: %#v", cookies)
	}
}

func TestExecuteCommandCapturesOutput(t *testing.T) {
	store, app, _ := newTestApp(t)
	defer store.Close()
	program, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	app.config.AllowedExecutables = map[string]struct{}{program: {}}
	app.commands = commandservice.New(app.config, store, store, store)
	commandID, err := store.SaveCommand(Command{
		Name:           "test helper",
		Program:        program,
		Args:           []string{"-test.run=^TestCommandHelperProcess$"},
		TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := app.commands.Run("admin", "admin", commandID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "success" || !strings.Contains(run.Stdout, "command-helper-output") {
		t.Fatalf("execution result = %#v", run)
	}
	stored, err := store.GetCommand(commandID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.LastAppliedBy != "admin" {
		t.Fatalf("last applied user = %q, want admin", stored.LastAppliedBy)
	}
}

func TestAdminCanRunMultilineScript(t *testing.T) {
	store, app, _ := newTestApp(t)
	defer store.Close()
	script := "Write-Output 'script-output'\nWrite-Output 'second-line'"
	commandID, err := app.commands.Save(Command{Name: "multi-line script", Script: script, TimeoutSeconds: 5})
	if err != nil {
		t.Fatal(err)
	}
	run, err := app.commands.Run("admin", "admin", commandID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "success" || !strings.Contains(run.Stdout, "script-output") || !strings.Contains(run.Stdout, "second-line") {
		t.Fatalf("script run result = %#v", run)
	}
}

func TestCommandTimeoutIsReported(t *testing.T) {
	store, app, _ := newTestApp(t)
	defer store.Close()
	script := "Start-Sleep -Seconds 5"
	if runtime.GOOS != "windows" {
		script = "sleep 5"
	}
	commandID, err := app.commands.Save(Command{Name: "timed out script", Script: script, TimeoutSeconds: 1})
	if err != nil {
		t.Fatal(err)
	}
	run, err := app.commands.Run("admin", "admin", commandID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "timeout" {
		t.Fatalf("timed out command status = %q, want timeout (run=%#v)", run.Status, run)
	}
}

func TestPowerShellCommandErrorsProduceFailedRun(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("PowerShell error propagation is Windows-specific")
	}
	store, app, _ := newTestApp(t)
	defer store.Close()
	commandID, err := app.commands.Save(Command{
		Name:           "PowerShell command error",
		Script:         "Write-Output 'before-error: проверка'\nTest-CmdUiDefinitelyMissing",
		TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	run, err := app.commands.Run("admin", "admin", commandID)
	if err != nil {
		t.Fatal(err)
	}
	if run.Status != "failed" || run.ExitCode == 0 || !strings.Contains(run.Stdout, "before-error: проверка") || !strings.Contains(run.Stderr, "CommandNotFoundException") {
		t.Fatalf("PowerShell command error result = %#v", run)
	}
}

func TestCommandHelperProcess(t *testing.T) {
	_, _ = io.WriteString(os.Stdout, "command-helper-output\n")
}

func newTestApp(t *testing.T) (*sqlite.Store, *App, string) {
	t.Helper()
	store, err := sqlite.OpenStore(filepath.Join(t.TempDir(), "test.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	adminHash, err := bcrypt.GenerateFromPassword([]byte("admin-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	operatorHash, err := bcrypt.GenerateFromPassword([]byte("operator-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	for _, user := range []User{
		{Username: "admin", Role: "admin", PasswordHash: adminHash},
		{Username: "operator", Role: "operator", PasswordHash: operatorHash},
	} {
		if err := store.SaveUser(user); err != nil {
			t.Fatal(err)
		}
	}
	program := filepath.Join(t.TempDir(), "approved-tool.exe")
	config := Config{
		SessionTTL:         time.Hour,
		MaxCommandTimeout:  10 * time.Second,
		MaxOutputBytes:     4096,
		AllowedExecutables: map[string]struct{}{program: {}},
		Users: map[string]User{
			"admin":    {Username: "admin", Role: "admin", PasswordHash: adminHash},
			"operator": {Username: "operator", Role: "operator", PasswordHash: operatorHash},
		},
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	auth := authservice.New(store)
	commands := commandservice.New(config, store, store, store)
	return store, NewApp(config, auth, commands, logger), program
}

func loginForTest(t *testing.T, handler http.Handler, username, password string) *http.Cookie {
	t.Helper()
	form := url.Values{"username": {username}, "password": {password}}
	request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("login %q status = %d", username, response.Code)
	}
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == sessionCookieName {
			return cookie
		}
	}
	t.Fatal("login response did not set a session cookie")
	return nil
}

func requestWithCookie(handler http.Handler, method, path string, body io.Reader, cookie *http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, body)
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	request.AddCookie(cookie)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
