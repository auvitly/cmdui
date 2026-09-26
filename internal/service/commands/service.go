package commands

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/auvitly/cmdui.git/internal/config"
	"github.com/auvitly/cmdui.git/internal/domain"
	commandrepo "github.com/auvitly/cmdui.git/internal/repository/commands"
	runrepo "github.com/auvitly/cmdui.git/internal/repository/runs"
	userrepo "github.com/auvitly/cmdui.git/internal/repository/users"
)

var (
	ErrForbidden            = errors.New("command access denied")
	ErrExecutableNotAllowed = errors.New("command executable is not allowed")
	ErrTooManyConcurrent    = errors.New("command concurrency limit reached")
	ErrInvalidCommand       = errors.New("invalid command definition")
	ErrRunNotActive         = errors.New("command run is not active")
)

var labelColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)

var defaultLabelColors = []string{"#DCEBFA", "#DDF2E1", "#FCE8D5", "#F7DDE3", "#E9DFF5", "#F5F0CF", "#D8EFEE", "#E8E4DC"}

var allowedIcons = map[string]struct{}{
	"terminal": {}, "database": {}, "settings": {}, "search": {}, "check": {}, "play": {},
	"refresh": {}, "star": {}, "home": {}, "cloud": {}, "chart": {}, "user": {},
	"go-color": {}, "go-mono": {}, "grpcui-color": {}, "grpcui-mono": {}, "podman-color": {}, "podman-mono": {},
}

type Service struct {
	config          config.Config
	commands        commandrepo.Repository
	runs            runrepo.Repository
	users           userrepo.Repository
	activeMu        sync.Mutex
	active          map[int64]*activeRun
	runningCommands map[int64]struct{}
}

type activeRun struct {
	mu                 sync.Mutex
	interruptOnce      sync.Once
	process            *exec.Cmd
	processDone        chan struct{}
	interruptRequested bool
	interruptError     error
	ctx                context.Context
	cancel             context.CancelFunc
	command            domain.Command
	username           string
	role               string
	startedAt          time.Time
}

type customIconRepository interface {
	SaveCustomIcon(domain.CustomIcon) (int64, error)
	ListCustomIcons() ([]domain.CustomIcon, error)
	GetCustomIcon(int64) (domain.CustomIcon, error)
}

func (run *activeRun) setProcess(process *exec.Cmd) {
	run.mu.Lock()
	run.process = process
	requested := run.interruptRequested
	run.mu.Unlock()
	if requested {
		run.sendInterrupt(process)
	}
}

func (run *activeRun) requestInterrupt() error {
	run.mu.Lock()
	if run.interruptRequested {
		run.mu.Unlock()
		return nil
	}
	run.interruptRequested = true
	process := run.process
	run.mu.Unlock()
	if process == nil {
		return nil
	}
	return run.sendInterrupt(process)
}

func (run *activeRun) sendInterrupt(process *exec.Cmd) error {
	if process.Process == nil {
		return nil
	}
	var interruptErr error
	run.interruptOnce.Do(func() {
		if err := interruptProcess(process); err != nil {
			interruptErr = err
			if killErr := forceTerminateProcess(process); killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
				interruptErr = errors.Join(err, killErr)
			}
			return
		}
		go func() {
			timer := time.NewTimer(2 * time.Second)
			defer timer.Stop()
			select {
			case <-run.processDone:
				return
			case <-timer.C:
				if err := forceTerminateProcess(process); err != nil && !errors.Is(err, os.ErrProcessDone) {
					run.mu.Lock()
					run.interruptError = err
					run.mu.Unlock()
				}
			}
		}()
	})
	return interruptErr
}

func (run *activeRun) wasInterrupted() bool {
	run.mu.Lock()
	defer run.mu.Unlock()
	return run.interruptRequested
}

func New(cfg config.Config, commandRepository commandrepo.Repository, runRepository runrepo.Repository, userRepository userrepo.Repository) *Service {
	return &Service{
		config: cfg, commands: commandRepository, runs: runRepository, users: userRepository,
		active: make(map[int64]*activeRun), runningCommands: make(map[int64]struct{}),
	}
}

func (s *Service) ListVisible(user domain.User) ([]domain.Command, error) {
	all, err := s.commands.ListCommands(false)
	if err != nil {
		return nil, err
	}
	visible := make([]domain.Command, 0, len(all))
	for _, command := range all {
		if CanView(user, command) {
			visible = append(visible, command)
		}
	}
	return visible, nil
}

func CanView(user domain.User, command domain.Command) bool {
	if user.Role == "admin" || command.AccessAllUsers {
		return true
	}
	if user.Role == "operator" && command.AccessOperators {
		return true
	}
	if contains(command.SpecificUsers, user.Username) {
		return true
	}
	return user.Role == "operator" && contains(command.SpecificOperators, user.Username)
}

func (s *Service) IsExecutableAllowed(program string) bool {
	cleaned := filepath.Clean(program)
	for allowed := range s.config.AllowedExecutables {
		if strings.EqualFold(cleaned, filepath.Clean(allowed)) {
			return true
		}
	}
	return false
}

func (s *Service) AllowedExecutables() []string {
	list := make([]string, 0, len(s.config.AllowedExecutables))
	for executable := range s.config.AllowedExecutables {
		list = append(list, executable)
	}
	return list
}

func (s *Service) SaveCustomIcon(icon domain.CustomIcon) (int64, error) {
	repository, ok := s.commands.(customIconRepository)
	if !ok {
		return 0, errors.New("custom icons are not supported")
	}
	return repository.SaveCustomIcon(icon)
}

func (s *Service) ListCustomIcons() ([]domain.CustomIcon, error) {
	repository, ok := s.commands.(customIconRepository)
	if !ok {
		return nil, errors.New("custom icons are not supported")
	}
	return repository.ListCustomIcons()
}

func (s *Service) GetCustomIcon(id int64) (domain.CustomIcon, error) {
	repository, ok := s.commands.(customIconRepository)
	if !ok {
		return domain.CustomIcon{}, errors.New("custom icons are not supported")
	}
	return repository.GetCustomIcon(id)
}

func (s *Service) Save(command domain.Command) (int64, error) {
	command.Script = strings.TrimSpace(command.Script)
	if command.Script != "" {
		command.Program = ""
		command.Args = nil
	} else if !s.IsExecutableAllowed(command.Program) {
		return 0, ErrExecutableNotAllowed
	}
	if strings.TrimSpace(command.Name) == "" || (command.Script == "" && (command.Program == "" || command.Program == ".")) {
		return 0, ErrInvalidCommand
	}
	if command.Icon != "" {
		if customID, custom := customIconID(command.Icon); custom {
			repository, ok := s.commands.(customIconRepository)
			if !ok {
				return 0, fmt.Errorf("%w: custom icons are not supported", ErrInvalidCommand)
			}
			if _, err := repository.GetCustomIcon(customID); err != nil {
				return 0, fmt.Errorf("%w: unknown custom icon %q", ErrInvalidCommand, command.Icon)
			}
		} else if _, allowed := allowedIcons[command.Icon]; !allowed {
			return 0, fmt.Errorf("%w: unsupported icon %q", ErrInvalidCommand, command.Icon)
		}
	}
	if command.TimeoutSeconds < 1 || time.Duration(command.TimeoutSeconds)*time.Second > s.config.MaxCommandTimeout {
		return 0, fmt.Errorf("%w: timeout must be within configured limits", ErrInvalidCommand)
	}
	seenKeys := make(map[string]struct{}, len(command.Labels))
	for index := range command.Labels {
		label := &command.Labels[index]
		label.Key = strings.TrimSpace(label.Key)
		label.Value = strings.TrimSpace(label.Value)
		if label.Key == "" || label.Value == "" {
			return 0, fmt.Errorf("%w: label key and value are required", ErrInvalidCommand)
		}
		if _, exists := seenKeys[label.Key]; exists {
			return 0, fmt.Errorf("%w: duplicate label key %q", ErrInvalidCommand, label.Key)
		}
		seenKeys[label.Key] = struct{}{}
		if label.Color == "" {
			label.Color = defaultLabelColors[index%len(defaultLabelColors)]
		}
		if !labelColorPattern.MatchString(label.Color) {
			return 0, fmt.Errorf("%w: invalid label color %q", ErrInvalidCommand, label.Color)
		}
		label.Color = strings.ToUpper(label.Color)
	}
	for _, username := range command.SpecificUsers {
		if _, err := s.users.GetUser(username); err != nil {
			return 0, fmt.Errorf("unknown command-view user %q: %w", username, err)
		}
	}
	for _, username := range command.SpecificOperators {
		user, err := s.users.GetUser(username)
		if err != nil || user.Role != "operator" {
			return 0, fmt.Errorf("specific command-view operator %q is not an operator", username)
		}
	}
	return s.commands.SaveCommand(command)
}

func customIconID(key string) (int64, bool) {
	if !strings.HasPrefix(key, "custom-") {
		return 0, false
	}
	id, err := strconv.ParseInt(strings.TrimPrefix(key, "custom-"), 10, 64)
	return id, err == nil && id > 0
}

func (s *Service) Get(id int64) (domain.Command, error) {
	return s.commands.GetCommand(id)
}

func (s *Service) Delete(id int64) error {
	return s.commands.DeleteCommand(id)
}

func (s *Service) ListRuns(filter domain.RunFilter, limit, offset int) ([]domain.Run, error) {
	return s.runs.ListRuns(filter, limit, offset)
}

func (s *Service) CountRuns(filter domain.RunFilter) (int, error) {
	return s.runs.CountRuns(filter)
}

func (s *Service) GetRun(id int64) (domain.Run, error) {
	return s.runs.GetRun(id)
}

func (s *Service) Run(username, role string, commandID int64) (domain.Run, error) {
	run, active, err := s.beginRun(username, role, commandID)
	if err != nil {
		return domain.Run{}, err
	}
	s.finishRun(run.ID, active)
	return s.runs.GetRun(run.ID)
}

func (s *Service) Start(username, role string, commandID int64) (domain.Run, error) {
	run, active, err := s.beginRun(username, role, commandID)
	if err != nil {
		return domain.Run{}, err
	}
	go s.finishRun(run.ID, active)
	return run, nil
}

func (s *Service) beginRun(username, role string, commandID int64) (domain.Run, *activeRun, error) {
	command, err := s.commands.GetCommand(commandID)
	if err != nil {
		return domain.Run{}, nil, err
	}
	if role != "admin" && (role != "operator" || !command.OperatorsCanRun || !CanView(domain.User{Username: username, Role: role}, command)) {
		return domain.Run{}, nil, ErrForbidden
	}
	if command.Script == "" && !s.IsExecutableAllowed(command.Program) {
		return domain.Run{}, nil, ErrExecutableNotAllowed
	}
	s.activeMu.Lock()
	if _, running := s.runningCommands[commandID]; running {
		s.activeMu.Unlock()
		return domain.Run{}, nil, ErrTooManyConcurrent
	}
	s.runningCommands[commandID] = struct{}{}
	s.activeMu.Unlock()
	run, active, err := s.prepareReservedRun(username, role, command)
	if err != nil {
		s.releaseCommand(commandID)
		return domain.Run{}, nil, err
	}
	return run, active, nil
}

func (s *Service) prepareReservedRun(username, role string, command domain.Command) (domain.Run, *activeRun, error) {
	startedAt := time.Now().UTC()
	run := domain.Run{
		Username: username, CommandName: command.Name, CommandScript: command.Script,
		Program: command.Program, Args: append([]string(nil), command.Args...),
		Status: "running", ExitCode: -1, StartedAt: startedAt,
	}
	runID, err := s.runs.SaveRun(run)
	if err != nil {
		return domain.Run{}, nil, err
	}
	run.ID = runID
	if err := s.commands.SetLastApplied(command.ID, username); err != nil {
		run.Status = "failed"
		run.Stderr = fmt.Sprintf("save last command user: %v", err)
		_ = s.runs.UpdateRun(run)
		return domain.Run{}, nil, err
	}
	timeout := time.Duration(command.TimeoutSeconds) * time.Second
	if timeout > s.config.MaxCommandTimeout {
		timeout = s.config.MaxCommandTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	active := &activeRun{cancel: cancel, command: command, username: username, role: role, startedAt: startedAt, processDone: make(chan struct{})}
	s.activeMu.Lock()
	s.active[runID] = active
	s.activeMu.Unlock()
	active.ctx = ctx
	return run, active, nil
}

func (s *Service) finishRun(runID int64, active *activeRun) {
	defer func() {
		active.cancel()
		s.activeMu.Lock()
		delete(s.active, runID)
		delete(s.runningCommands, active.command.ID)
		s.activeMu.Unlock()
	}()
	run := execute(active.ctx, active.username, active.command, s.config, s.config.MaxOutputBytes, active)
	run.ID = runID
	if err := s.runs.UpdateRun(run); err != nil {
		active.mu.Lock()
		active.interruptError = fmt.Errorf("persist completed run: %w", err)
		active.mu.Unlock()
	}
}

func (s *Service) Interrupt(runID int64, _ string, role string) error {
	if role != "admin" {
		return ErrForbidden
	}
	_, err := s.runs.GetRun(runID)
	if err != nil {
		return err
	}
	s.activeMu.Lock()
	active := s.active[runID]
	s.activeMu.Unlock()
	if active == nil {
		return ErrRunNotActive
	}
	return active.requestInterrupt()
}

func (s *Service) InterruptRequested(runID int64) bool {
	s.activeMu.Lock()
	active := s.active[runID]
	s.activeMu.Unlock()
	return active != nil && active.wasInterrupted()
}

func (s *Service) ActiveRun(username, role string) (domain.Run, bool, error) {
	s.activeMu.Lock()
	var runID int64
	var active *activeRun
	for id, candidate := range s.active {
		if role == "admin" || (role == "operator" && candidate.username == username) {
			runID = id
			active = candidate
			break
		}
	}
	s.activeMu.Unlock()
	if active == nil {
		return domain.Run{}, false, nil
	}
	run, err := s.runs.GetRun(runID)
	if err != nil {
		return domain.Run{}, false, err
	}
	return run, true, nil
}

func (s *Service) ActiveRuns(username, role string) ([]domain.Run, error) {
	s.activeMu.Lock()
	runIDs := make([]int64, 0, len(s.active))
	for runID, active := range s.active {
		if role == "admin" || (role == "operator" && active.username == username) {
			runIDs = append(runIDs, runID)
		}
	}
	s.activeMu.Unlock()
	sort.Slice(runIDs, func(i, j int) bool { return runIDs[i] < runIDs[j] })
	runs := make([]domain.Run, 0, len(runIDs))
	for _, runID := range runIDs {
		run, err := s.runs.GetRun(runID)
		if err != nil {
			return nil, err
		}
		if run.Status != "running" {
			continue
		}
		run.Duration = time.Since(run.StartedAt)
		runs = append(runs, run)
	}
	return runs, nil
}

func (s *Service) HasActiveRun() bool {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	return len(s.active) > 0
}

func (s *Service) IsCommandRunning(commandID int64) bool {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	_, running := s.runningCommands[commandID]
	return running
}

func (s *Service) ActiveCommandIDs() []int64 {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()
	commandIDs := make([]int64, 0, len(s.runningCommands))
	for commandID := range s.runningCommands {
		commandIDs = append(commandIDs, commandID)
	}
	return commandIDs
}

func (s *Service) ActiveCommandRuns() map[int64]int64 {
	s.activeMu.Lock()
	defer s.activeMu.Unlock()

	commandRuns := make(map[int64]int64, len(s.active))
	for runID, active := range s.active {
		commandRuns[active.command.ID] = runID
	}
	return commandRuns
}

func (s *Service) releaseCommand(commandID int64) {
	s.activeMu.Lock()
	delete(s.runningCommands, commandID)
	s.activeMu.Unlock()
}

func (s *Service) Wait(ctx context.Context, runID int64) (domain.Run, error) {
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		run, err := s.runs.GetRun(runID)
		if err != nil {
			return domain.Run{}, err
		}
		if run.Status != "running" {
			return run, nil
		}
		select {
		case <-ctx.Done():
			return run, ctx.Err()
		case <-ticker.C:
		}
	}
}

func execute(ctx context.Context, username string, command domain.Command, cfg config.Config, maxOutputBytes int, active *activeRun) domain.Run {
	startedAt := time.Now().UTC()
	defer close(active.processDone)
	process, cleanup, prepareErr := prepareProcess(ctx, command, cfg.ScriptInterpreter)
	if cleanup != nil {
		defer cleanup()
	}
	stdout := &limitedBuffer{limit: maxOutputBytes / 2}
	stderr := &limitedBuffer{limit: maxOutputBytes / 2}
	var err error
	if prepareErr != nil {
		err = prepareErr
		_, _ = stderr.Write([]byte(prepareErr.Error()))
	} else {
		configureProcess(process)
		process.Stdout = stdout
		process.Stderr = stderr
		err = process.Start()
		if err == nil {
			active.setProcess(process)
			err = process.Wait()
		}
	}
	status := "success"
	exitCode := 0
	if err != nil {
		status = "failed"
		exitCode = -1
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode = normalizeExitCode(exitError.ExitCode())
		}
		if active.wasInterrupted() {
			status = "interrupted"
			exitCode = -1
		} else if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = "timeout"
		}
	}
	return domain.Run{Username: username, CommandName: command.Name, Status: status, ExitCode: exitCode, StartedAt: startedAt, Duration: time.Since(startedAt), Stdout: stdout.String(), Stderr: stderr.String()}
}

func normalizeExitCode(exitCode int) int {
	unsignedExitCode := uint64(exitCode)
	if unsignedExitCode > uint64(^uint32(0)>>1) && unsignedExitCode <= uint64(^uint32(0)) {
		return int(int32(uint32(exitCode)))
	}
	return exitCode
}

func prepareProcess(ctx context.Context, command domain.Command, configuredInterpreter string) (*exec.Cmd, func(), error) {
	if command.Script == "" {
		return exec.CommandContext(ctx, command.Program, command.Args...), nil, nil
	}
	interpreter := configuredInterpreter
	if interpreter == "" {
		if filepath.Separator == '\\' {
			interpreter = "powershell.exe"
		} else {
			interpreter = "/bin/sh"
		}
	}
	resolvedInterpreter, err := exec.LookPath(interpreter)
	if err != nil {
		return nil, nil, fmt.Errorf("resolve script interpreter: %w", err)
	}
	extension := ".sh"
	args := make([]string, 0, 6)
	if filepath.Separator == '\\' {
		extension = ".ps1"
		args = append(args, "-NoLogo", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File")
	}
	scriptFile, err := os.CreateTemp("", "cmdui-script-*"+extension)
	if err != nil {
		return nil, nil, fmt.Errorf("create temporary script: %w", err)
	}
	scriptPaths := []string{scriptFile.Name()}
	cleanup := func() {
		for _, path := range scriptPaths {
			_ = os.Remove(path)
		}
	}
	if filepath.Separator == '\\' {
		if _, err := scriptFile.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
			_ = scriptFile.Close()
			cleanup()
			return nil, nil, fmt.Errorf("write temporary script encoding marker: %w", err)
		}
	}
	if _, err := scriptFile.WriteString(command.Script); err != nil {
		_ = scriptFile.Close()
		cleanup()
		return nil, nil, fmt.Errorf("write temporary script: %w", err)
	}
	if err := scriptFile.Close(); err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("close temporary script: %w", err)
	}
	scriptPath := scriptFile.Name()
	if filepath.Separator == '\\' {
		runnerFile, err := os.CreateTemp("", "cmdui-runner-*.ps1")
		if err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("create PowerShell runner: %w", err)
		}
		scriptPaths = append(scriptPaths, runnerFile.Name())
		runner := "[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)\r\n$ErrorActionPreference = 'Stop'\r\ntry {\r\n    & '" + strings.ReplaceAll(scriptPath, "'", "''") + "'\r\n    if ($null -ne $LASTEXITCODE -and $LASTEXITCODE -ne 0) { exit $LASTEXITCODE }\r\n} catch {\r\n    [Console]::Error.WriteLine($_.Exception.GetType().FullName)\r\n    [Console]::Error.WriteLine($_.Exception.Message)\r\n    exit 1\r\n}\r\n"
		if _, err := runnerFile.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
			_ = runnerFile.Close()
			cleanup()
			return nil, nil, fmt.Errorf("write PowerShell runner encoding marker: %w", err)
		}
		if _, err := runnerFile.WriteString(runner); err != nil {
			_ = runnerFile.Close()
			cleanup()
			return nil, nil, fmt.Errorf("write PowerShell runner: %w", err)
		}
		if err := runnerFile.Close(); err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("close PowerShell runner: %w", err)
		}
		scriptPath = runnerFile.Name()
	}
	args = append(args, scriptPath)
	return exec.CommandContext(ctx, resolvedInterpreter, args...), cleanup, nil
}

type limitedBuffer struct {
	buffer    bytes.Buffer
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	originalLength := len(p)
	remaining := b.limit - b.buffer.Len()
	if remaining > 0 {
		if remaining > len(p) {
			remaining = len(p)
		}
		_, _ = b.buffer.Write(p[:remaining])
	}
	if remaining < originalLength {
		b.truncated = true
	}
	return originalLength, nil
}

func (b *limitedBuffer) String() string {
	value := b.buffer.String()
	if b.truncated {
		value += "\n[вывод обрезан]"
	}
	return value
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
