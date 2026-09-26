package sqlite

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/auvitly/cmdui.git/internal/domain"
)

func TestSQLitePersistsCommandsAndRuns(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "state", "cmdui.sqlite")
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	commandID, err := store.SaveCommand(Command{
		Name: "health check", Icon: "database", Program: `C:\tools\check.exe`, Args: []string{"--fast", "value with spaces"}, TimeoutSeconds: 10,
		Script: "echo script", Labels: []domain.Label{{Key: "env", Value: "test", Color: "#DCEBFA"}},
		AccessOperators: true, AccessAllUsers: true, SpecificUsers: []string{"reader-a"},
		SpecificOperators: []string{"ops-a"}, OperatorsCanRun: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetOperatorAllowed(commandID, true); err != nil {
		t.Fatal(err)
	}
	startedAt := time.Now().UTC().Truncate(time.Millisecond)
	_, err = store.SaveRun(Run{Username: "operator", CommandName: "health check", Status: "success", ExitCode: 0, StartedAt: startedAt, Duration: 125 * time.Millisecond, Stdout: "ok"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = OpenStore(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	commands, err := store.ListCommands(true)
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 1 || commands[0].Name != "health check" || commands[0].Icon != "database" || !commands[0].OperatorsCanRun || len(commands[0].Args) != 2 ||
		!commands[0].AccessOperators || !commands[0].AccessAllUsers || len(commands[0].SpecificUsers) != 1 || len(commands[0].SpecificOperators) != 1 ||
		commands[0].Script != "echo script" || len(commands[0].Labels) != 1 || commands[0].Labels[0].Color != "#DCEBFA" {
		t.Fatalf("persisted commands = %#v", commands)
	}
	runs, err := store.ListRuns(RunFilter{Username: "operator", UsernameExact: true}, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1 || runs[0].Status != "success" || runs[0].Stdout != "ok" {
		t.Fatalf("persisted runs = %#v", runs)
	}
	if err := store.SetLastApplied(commandID, "operator"); err != nil {
		t.Fatal(err)
	}
	commandAfterRun, err := store.GetCommand(commandID)
	if err != nil {
		t.Fatal(err)
	}
	if commandAfterRun.LastAppliedBy != "operator" || commandAfterRun.LastAppliedAt.IsZero() {
		t.Fatalf("last application metadata = (%q, %v)", commandAfterRun.LastAppliedBy, commandAfterRun.LastAppliedAt)
	}

	commands[0].Name = "updated check"
	if _, err := store.SaveCommand(commands[0]); err != nil {
		t.Fatal(err)
	}
	updated, err := store.GetCommand(commandID)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "updated check" || updated.Icon != "database" || !updated.OperatorsCanRun || !updated.AccessAllUsers || len(updated.SpecificUsers) != 1 || updated.LastAppliedBy != "operator" || updated.LastAppliedAt.IsZero() ||
		updated.Script != "echo script" || len(updated.Labels) != 1 || updated.Labels[0].Color != "#DCEBFA" {
		t.Fatalf("updated command lost data: %#v", updated)
	}
}

func TestSQLiteNormalizesInterruptedExitCodes(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "interrupted.sqlite")
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	runID, err := store.SaveRun(Run{
		Username: "operator", CommandName: "interrupted command", Status: "interrupted",
		ExitCode: 4294967295, StartedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = OpenStore(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	run, err := store.GetRun(runID)
	if err != nil {
		t.Fatal(err)
	}
	if run.ExitCode != -1 {
		t.Fatalf("interrupted exit code = %d, want -1", run.ExitCode)
	}
	var storedExitCode int
	if err := store.db.QueryRow(`SELECT exit_code FROM command_runs WHERE id = ?`, runID).Scan(&storedExitCode); err != nil {
		t.Fatal(err)
	}
	if storedExitCode != -1 {
		t.Fatalf("persisted interrupted exit code = %d, want -1", storedExitCode)
	}
}

func TestSQLitePersistsCustomIcons(t *testing.T) {
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	iconID, err := store.SaveCustomIcon(CustomIcon{Name: "Company mark", SVG: `<svg viewBox="0 0 10 10"><path d="M0 0h10v10H0z"/></svg>`})
	if err != nil {
		t.Fatal(err)
	}
	icon, err := store.GetCustomIcon(iconID)
	if err != nil {
		t.Fatal(err)
	}
	if icon.ID != iconID || icon.Name != "Company mark" || !strings.Contains(icon.SVG, "viewBox") {
		t.Fatalf("stored custom icon = %#v", icon)
	}
	icons, err := store.ListCustomIcons()
	if err != nil {
		t.Fatal(err)
	}
	if len(icons) != 1 || icons[0].ID != iconID {
		t.Fatalf("listed custom icons = %#v", icons)
	}
	if err := store.DeleteCustomIcon(iconID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetCustomIcon(iconID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deleted custom icon lookup error = %v, want sql.ErrNoRows", err)
	}
}

func TestSQLiteCountsAndPaginatesRunsWithFilters(t *testing.T) {
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	for _, run := range []Run{
		{Username: "operator", CommandName: "deploy api", Status: "running", StartedAt: time.Now().UTC()},
		{Username: "operator", CommandName: "deploy api", Status: "running", StartedAt: time.Now().UTC()},
		{Username: "operator", CommandName: "deploy api", Status: "running", StartedAt: time.Now().UTC()},
		{Username: "operator", CommandName: "deploy worker", Status: "running", StartedAt: time.Now().UTC()},
		{Username: "admin", CommandName: "deploy api", Status: "running", StartedAt: time.Now().UTC()},
	} {
		if _, err := store.SaveRun(run); err != nil {
			t.Fatal(err)
		}
	}

	filter := RunFilter{Username: "operator", UsernameExact: true, Command: "api", Status: "running"}
	count, err := store.CountRuns(filter)
	if err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("filtered run count = %d, want 3", count)
	}
	runs, err := store.ListRuns(filter, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 2 || runs[0].CommandName != "deploy api" || runs[1].CommandName != "deploy api" {
		t.Fatalf("filtered page = %#v, want 2 matching runs", runs)
	}
}

func TestSQLitePersistsRunInvocationSnapshot(t *testing.T) {
	store, err := OpenStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	scriptID, err := store.SaveRun(Run{
		Username: "admin", CommandName: "script task", CommandScript: "echo first\necho second",
		Status: "success", StartedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	programID, err := store.SaveRun(Run{
		Username: "admin", CommandName: "program task", Program: `C:\tools\worker.exe`,
		Args: []string{"--mode", "dry run"}, Status: "success", StartedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}

	scriptRun, err := store.GetRun(scriptID)
	if err != nil {
		t.Fatal(err)
	}
	if scriptRun.CommandScript != "echo first\necho second" {
		t.Fatalf("persisted run script = %q", scriptRun.CommandScript)
	}
	programRun, err := store.GetRun(programID)
	if err != nil {
		t.Fatal(err)
	}
	if programRun.Program != `C:\tools\worker.exe` || len(programRun.Args) != 2 || programRun.Args[0] != "--mode" || programRun.Args[1] != "dry run" {
		t.Fatalf("persisted invocation = %#v", programRun)
	}
}

func TestSQLiteMigratesRunInvocationSnapshot(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "legacy-runs.sqlite")
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE command_runs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		username TEXT NOT NULL,
		command_name TEXT NOT NULL,
		status TEXT NOT NULL,
		exit_code INTEGER NOT NULL,
		started_at TEXT NOT NULL,
		duration_ms INTEGER NOT NULL,
		stdout TEXT NOT NULL,
		stderr TEXT NOT NULL
	);
	INSERT INTO command_runs(username, command_name, status, exit_code, started_at, duration_ms, stdout, stderr)
	VALUES('operator', 'legacy task', 'success', 0, '2026-09-26T00:00:00Z', 12, 'ok', '');`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	run, err := store.GetRun(1)
	if err != nil {
		t.Fatal(err)
	}
	if run.CommandName != "legacy task" || run.CommandScript != "" || run.Program != "" || len(run.Args) != 0 {
		t.Fatalf("migrated legacy run = %#v", run)
	}
}

func TestSQLiteBackfillsLastAppliedTimestampFromRunHistory(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "last-applied-time.sqlite")
	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	commandID, err := store.SaveCommand(Command{Name: "legacy task", Program: `C:\tools\task.exe`, TimeoutSeconds: 10})
	if err != nil {
		t.Fatal(err)
	}
	startedAt := time.Date(2026, time.September, 25, 20, 30, 0, 0, time.UTC)
	if _, err := store.SaveRun(Run{Username: "operator", CommandName: "legacy task", Status: "success", StartedAt: startedAt}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE commands SET last_applied_by = ?, last_applied_at = '' WHERE id = ?`, "operator", commandID); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = OpenStore(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	command, err := store.GetCommand(commandID)
	if err != nil {
		t.Fatal(err)
	}
	if command.LastAppliedBy != "operator" || !command.LastAppliedAt.Equal(startedAt) {
		t.Fatalf("backfilled last run metadata = (%q, %s), want (operator, %s)", command.LastAppliedBy, command.LastAppliedAt, startedAt)
	}
}

func TestSQLiteMigrationPreservesLegacyOperatorPublication(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "legacy.sqlite")
	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE commands (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		description TEXT NOT NULL DEFAULT '',
		program TEXT NOT NULL,
		args_json TEXT NOT NULL,
		timeout_seconds INTEGER NOT NULL,
		operator_allowed INTEGER NOT NULL DEFAULT 0,
		updated_at TEXT NOT NULL
	);
	INSERT INTO commands(name, program, args_json, timeout_seconds, operator_allowed, updated_at)
	VALUES('legacy published', 'C:\\tools\\check.exe', '[]', 10, 1, '2026-09-25T00:00:00Z');`)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := OpenStore(databasePath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	commands, err := store.ListCommands(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(commands) != 1 || !commands[0].AccessOperators || !commands[0].OperatorsCanRun {
		t.Fatalf("legacy command permissions not preserved: %#v", commands)
	}
}

func TestEnsureUserDoesNotResetManagedRole(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "users.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.EnsureUser(User{Username: "admin1", PasswordHash: []byte("hash1"), Role: "admin"}); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureUser(User{Username: "admin2", PasswordHash: []byte("hash2"), Role: "admin"}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateUserRole("admin1", "operator"); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureUser(User{Username: "admin1", PasswordHash: []byte("seed-hash"), Role: "admin"}); err != nil {
		t.Fatal(err)
	}
	user, err := store.GetUser("admin1")
	if err != nil {
		t.Fatal(err)
	}
	if user.Role != "operator" || string(user.PasswordHash) != "hash1" {
		t.Fatalf("managed user was reset by bootstrap: %#v", user)
	}
}
