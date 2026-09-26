package sqlite

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/auvitly/cmdui.git/internal/domain"
	"github.com/auvitly/cmdui.git/internal/repository/commands"
	"github.com/auvitly/cmdui.git/internal/repository/runs"
	"github.com/auvitly/cmdui.git/internal/repository/users"

	_ "modernc.org/sqlite"
)

type Command = domain.Command
type Run = domain.Run
type RunFilter = domain.RunFilter
type User = domain.User
type CustomIcon = domain.CustomIcon

type Store struct {
	db *sql.DB
}

var (
	ErrLastAdmin         = domain.ErrLastAdmin
	ErrCannotDeleteAdmin = domain.ErrCannotDeleteAdmin
	ErrCannotDeleteSelf  = domain.ErrCannotDeleteSelf
)

var _ users.Repository = (*Store)(nil)
var _ commands.Repository = (*Store)(nil)
var _ runs.Repository = (*Store)(nil)

func OpenStore(path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open SQLite database: %w", err)
	}
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to SQLite database: %w", err)
	}
	if _, err := db.Exec(`
		PRAGMA foreign_keys = ON;
		CREATE TABLE IF NOT EXISTS commands (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL DEFAULT '',
			icon TEXT NOT NULL DEFAULT '',
			program TEXT NOT NULL,
			args_json TEXT NOT NULL,
			script TEXT NOT NULL DEFAULT '',
			labels_json TEXT NOT NULL DEFAULT '[]',
			last_applied_by TEXT NOT NULL DEFAULT '',
			last_applied_at TEXT NOT NULL DEFAULT '',
			timeout_seconds INTEGER NOT NULL,
			operator_allowed INTEGER NOT NULL DEFAULT 0,
			access_operators INTEGER NOT NULL DEFAULT 0,
			access_all_users INTEGER NOT NULL DEFAULT 0,
			specific_users_json TEXT NOT NULL DEFAULT '[]',
			specific_operators_json TEXT NOT NULL DEFAULT '[]',
			operators_can_run INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS users (
			username TEXT PRIMARY KEY,
			password_hash BLOB NOT NULL,
			role TEXT NOT NULL CHECK(role IN ('admin', 'operator', 'user')),
			created_at TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS command_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL,
			command_name TEXT NOT NULL,
			command_script TEXT NOT NULL DEFAULT '',
			program TEXT NOT NULL DEFAULT '',
			args_json TEXT NOT NULL DEFAULT '[]',
			status TEXT NOT NULL,
			exit_code INTEGER NOT NULL,
			started_at TEXT NOT NULL,
			duration_ms INTEGER NOT NULL,
			stdout TEXT NOT NULL,
			stderr TEXT NOT NULL
		);
		CREATE TABLE IF NOT EXISTS custom_icons (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			svg TEXT NOT NULL,
			created_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_command_runs_started_at ON command_runs(started_at DESC);
	`); err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize SQLite schema: %w", err)
	}
	for _, column := range []struct {
		name       string
		definition string
	}{
		{"access_operators", `INTEGER NOT NULL DEFAULT 0`},
		{"access_all_users", `INTEGER NOT NULL DEFAULT 0`},
		{"specific_users_json", `TEXT NOT NULL DEFAULT '[]'`},
		{"specific_operators_json", `TEXT NOT NULL DEFAULT '[]'`},
		{"operators_can_run", `INTEGER NOT NULL DEFAULT 0`},
		{"script", `TEXT NOT NULL DEFAULT ''`},
		{"labels_json", `TEXT NOT NULL DEFAULT '[]'`},
		{"last_applied_by", `TEXT NOT NULL DEFAULT ''`},
		{"last_applied_at", `TEXT NOT NULL DEFAULT ''`},
		{"icon", `TEXT NOT NULL DEFAULT ''`},
	} {
		added, err := ensureColumn(db, "commands", column.name, column.definition)
		if err != nil {
			db.Close()
			return nil, err
		}
		if added && (column.name == "access_operators" || column.name == "operators_can_run") {
			if _, err := db.Exec(`UPDATE commands SET ` + column.name + ` = operator_allowed`); err != nil {
				db.Close()
				return nil, fmt.Errorf("migrate legacy command permissions: %w", err)
			}
		}
	}
	for _, column := range []struct {
		name       string
		definition string
	}{
		{"command_script", `TEXT NOT NULL DEFAULT ''`},
		{"program", `TEXT NOT NULL DEFAULT ''`},
		{"args_json", `TEXT NOT NULL DEFAULT '[]'`},
	} {
		if _, err := ensureColumn(db, "command_runs", column.name, column.definition); err != nil {
			db.Close()
			return nil, err
		}
	}
	if _, err := db.Exec(`UPDATE commands
		SET last_applied_at = (
			SELECT MAX(command_runs.started_at)
			FROM command_runs
			WHERE command_runs.command_name = commands.name
			  AND command_runs.username = commands.last_applied_by
		)
		WHERE last_applied_at = '' AND last_applied_by <> ''
		  AND EXISTS (
			SELECT 1 FROM command_runs
			WHERE command_runs.command_name = commands.name
			  AND command_runs.username = commands.last_applied_by
		)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("backfill last command run timestamps: %w", err)
	}
	if _, err := db.Exec(`UPDATE command_runs SET exit_code = -1 WHERE status = 'interrupted' AND exit_code <> -1`); err != nil {
		db.Close()
		return nil, fmt.Errorf("normalize interrupted command exit codes: %w", err)
	}
	return &Store{db: db}, nil
}

func ensureColumn(db *sql.DB, table, column, definition string) (bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return false, fmt.Errorf("inspect %s schema: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var ordinal int
		var name, dataType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&ordinal, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, fmt.Errorf("read %s schema: %w", table, err)
		}
		if name == column {
			return false, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("read %s schema: %w", table, err)
	}
	if _, err := db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + definition); err != nil {
		return false, fmt.Errorf("add %s.%s: %w", table, column, err)
	}
	return true, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) SaveCustomIcon(icon CustomIcon) (int64, error) {
	result, err := s.db.Exec(`INSERT INTO custom_icons(name, svg, created_at) VALUES(?, ?, ?)`, icon.Name, icon.SVG, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, fmt.Errorf("save custom icon: %w", err)
	}
	return result.LastInsertId()
}

func (s *Store) ListCustomIcons() ([]CustomIcon, error) {
	rows, err := s.db.Query(`SELECT id, name, svg, created_at FROM custom_icons ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("list custom icons: %w", err)
	}
	defer rows.Close()
	icons := make([]CustomIcon, 0)
	for rows.Next() {
		var icon CustomIcon
		var createdAt string
		if err := rows.Scan(&icon.ID, &icon.Name, &icon.SVG, &createdAt); err != nil {
			return nil, fmt.Errorf("scan custom icon: %w", err)
		}
		icon.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
		if err != nil {
			return nil, fmt.Errorf("parse custom icon timestamp: %w", err)
		}
		icons = append(icons, icon)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read custom icons: %w", err)
	}
	return icons, nil
}

func (s *Store) GetCustomIcon(id int64) (CustomIcon, error) {
	var icon CustomIcon
	var createdAt string
	err := s.db.QueryRow(`SELECT id, name, svg, created_at FROM custom_icons WHERE id = ?`, id).Scan(&icon.ID, &icon.Name, &icon.SVG, &createdAt)
	if err != nil {
		return CustomIcon{}, err
	}
	icon.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt)
	if err != nil {
		return CustomIcon{}, fmt.Errorf("parse custom icon timestamp: %w", err)
	}
	return icon, nil
}

func (s *Store) DeleteCustomIcon(id int64) error {
	var used int
	if err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM commands WHERE icon = ?)`, fmt.Sprintf("custom-%d", id)).Scan(&used); err != nil {
		return fmt.Errorf("check custom icon usage: %w", err)
	}
	if used != 0 {
		return errors.New("custom icon is in use")
	}
	result, err := s.db.Exec(`DELETE FROM custom_icons WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete custom icon: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check deleted custom icon: %w", err)
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) SaveUser(user User) error {
	_, err := s.db.Exec(`INSERT INTO users(username, password_hash, role, created_at) VALUES(?, ?, ?, ?)`,
		user.Username, user.PasswordHash, user.Role, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("save user: %w", err)
	}
	return nil
}

func (s *Store) EnsureUser(user User) error {
	_, err := s.db.Exec(`INSERT INTO users(username, password_hash, role, created_at) VALUES(?, ?, ?, ?)
		ON CONFLICT(username) DO NOTHING`, user.Username, user.PasswordHash, user.Role, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("initialize user: %w", err)
	}
	return nil
}

func (s *Store) UpdateUserRole(username, role string) error {
	if role != "admin" && role != "operator" && role != "user" {
		return fmt.Errorf("invalid user role %q", role)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin user role update: %w", err)
	}
	defer tx.Rollback()
	var currentRole string
	if err := tx.QueryRow(`SELECT role FROM users WHERE username = ?`, username).Scan(&currentRole); err != nil {
		return err
	}
	if currentRole == "admin" && role != "admin" {
		var adminCount int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&adminCount); err != nil {
			return fmt.Errorf("count admin users: %w", err)
		}
		if adminCount <= 1 {
			return ErrLastAdmin
		}
	}
	if _, err := tx.Exec(`UPDATE users SET role = ? WHERE username = ?`, role, username); err != nil {
		return fmt.Errorf("update user role: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit user role update: %w", err)
	}
	return nil
}

func (s *Store) DeleteUser(username, actorUsername string) error {
	if username == actorUsername {
		return ErrCannotDeleteSelf
	}
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin user deletion: %w", err)
	}
	defer tx.Rollback()
	var role string
	if err := tx.QueryRow(`SELECT role FROM users WHERE username = ?`, username).Scan(&role); err != nil {
		return err
	}
	if role == "admin" {
		if actorUsername != "admin" {
			return ErrCannotDeleteAdmin
		}
		var adminCount int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&adminCount); err != nil {
			return fmt.Errorf("count admin users: %w", err)
		}
		if adminCount <= 1 {
			return ErrLastAdmin
		}
	}
	if _, err := tx.Exec(`DELETE FROM users WHERE username = ?`, username); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit user deletion: %w", err)
	}
	return nil
}

func (s *Store) GetUser(username string) (User, error) {
	var user User
	err := s.db.QueryRow(`SELECT username, password_hash, role FROM users WHERE username = ?`, username).
		Scan(&user.Username, &user.PasswordHash, &user.Role)
	if err != nil {
		return User{}, err
	}
	return user, nil
}

func (s *Store) ListUsers(role string) ([]User, error) {
	query := `SELECT username, password_hash, role FROM users`
	args := []any{}
	if role != "" {
		query += ` WHERE role = ?`
		args = append(args, role)
	}
	query += ` ORDER BY username COLLATE NOCASE`
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()
	users := make([]User, 0)
	for rows.Next() {
		var user User
		if err := rows.Scan(&user.Username, &user.PasswordHash, &user.Role); err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read users: %w", err)
	}
	return users, nil
}

func (s *Store) SaveCommand(command Command) (int64, error) {
	command.OperatorsCanRun = command.OperatorsCanRun || command.OperatorAllowed
	command.OperatorAllowed = command.OperatorsCanRun
	argsJSON, err := json.Marshal(command.Args)
	if err != nil {
		return 0, fmt.Errorf("encode command arguments: %w", err)
	}
	specificUsersJSON, err := json.Marshal(command.SpecificUsers)
	if err != nil {
		return 0, fmt.Errorf("encode specific users: %w", err)
	}
	specificOperatorsJSON, err := json.Marshal(command.SpecificOperators)
	if err != nil {
		return 0, fmt.Errorf("encode specific operators: %w", err)
	}
	labelsJSON, err := json.Marshal(command.Labels)
	if err != nil {
		return 0, fmt.Errorf("encode command labels: %w", err)
	}
	updatedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if command.ID == 0 {
		result, err := s.db.Exec(`INSERT INTO commands(name, description, icon, program, args_json, script, labels_json, timeout_seconds, operator_allowed, access_operators, access_all_users, specific_users_json, specific_operators_json, operators_can_run, updated_at)
			VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, command.Name, command.Description, command.Icon, command.Program, string(argsJSON), command.Script, string(labelsJSON), command.TimeoutSeconds,
			command.OperatorsCanRun, command.AccessOperators, command.AccessAllUsers, string(specificUsersJSON), string(specificOperatorsJSON), command.OperatorsCanRun, updatedAt)
		if err != nil {
			return 0, fmt.Errorf("insert command: %w", err)
		}
		return result.LastInsertId()
	}
	result, err := s.db.Exec(`UPDATE commands SET name = ?, description = ?, icon = ?, program = ?, args_json = ?, script = ?, labels_json = ?, timeout_seconds = ?, operator_allowed = ?, access_operators = ?, access_all_users = ?, specific_users_json = ?, specific_operators_json = ?, operators_can_run = ?, updated_at = ? WHERE id = ?`,
		command.Name, command.Description, command.Icon, command.Program, string(argsJSON), command.Script, string(labelsJSON), command.TimeoutSeconds, command.OperatorsCanRun,
		command.AccessOperators, command.AccessAllUsers, string(specificUsersJSON), string(specificOperatorsJSON), command.OperatorsCanRun, updatedAt, command.ID)
	if err != nil {
		return 0, fmt.Errorf("update command: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}
	if rowsAffected == 0 {
		return 0, sql.ErrNoRows
	}
	return command.ID, nil
}

func (s *Store) GetCommand(id int64) (Command, error) {
	row := s.db.QueryRow(`SELECT id, name, description, icon, program, args_json, script, labels_json, timeout_seconds, access_operators, access_all_users, specific_users_json, specific_operators_json, operators_can_run, last_applied_by, last_applied_at, updated_at FROM commands WHERE id = ?`, id)
	return scanCommand(row)
}

func (s *Store) ListCommands(operatorOnly bool) ([]Command, error) {
	query := `SELECT id, name, description, icon, program, args_json, script, labels_json, timeout_seconds, access_operators, access_all_users, specific_users_json, specific_operators_json, operators_can_run, last_applied_by, last_applied_at, updated_at FROM commands`
	if operatorOnly {
		query += ` WHERE operators_can_run = 1`
	}
	query += ` ORDER BY name COLLATE NOCASE`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("list commands: %w", err)
	}
	defer rows.Close()
	commands := make([]Command, 0)
	for rows.Next() {
		command, err := scanCommand(rows)
		if err != nil {
			return nil, err
		}
		commands = append(commands, command)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read commands: %w", err)
	}
	return commands, nil
}

func (s *Store) SetOperatorAllowed(id int64, allowed bool) error {
	result, err := s.db.Exec(`UPDATE commands SET operator_allowed = ?, access_operators = ?, operators_can_run = ?, updated_at = ? WHERE id = ?`, allowed, allowed, allowed, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("update command publication: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) SetLastApplied(id int64, username string) error {
	result, err := s.db.Exec(`UPDATE commands SET last_applied_by = ?, last_applied_at = ? WHERE id = ?`, username, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("update last command user: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteCommand(id int64) error {
	result, err := s.db.Exec(`DELETE FROM commands WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete command: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) SaveRun(run Run) (int64, error) {
	argsJSON, err := json.Marshal(run.Args)
	if err != nil {
		return 0, fmt.Errorf("encode command run arguments: %w", err)
	}
	result, err := s.db.Exec(`INSERT INTO command_runs(username, command_name, command_script, program, args_json, status, exit_code, started_at, duration_ms, stdout, stderr)
		VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, run.Username, run.CommandName, run.CommandScript, run.Program, string(argsJSON), run.Status, run.ExitCode,
		run.StartedAt.UTC().Format(time.RFC3339Nano), run.Duration.Milliseconds(), run.Stdout, run.Stderr)
	if err != nil {
		return 0, fmt.Errorf("save command run: %w", err)
	}
	return result.LastInsertId()
}

func (s *Store) UpdateRun(run Run) error {
	result, err := s.db.Exec(`UPDATE command_runs SET status = ?, exit_code = ?, started_at = ?, duration_ms = ?, stdout = ?, stderr = ? WHERE id = ?`,
		run.Status, run.ExitCode, run.StartedAt.UTC().Format(time.RFC3339Nano), run.Duration.Milliseconds(), run.Stdout, run.Stderr, run.ID)
	if err != nil {
		return fmt.Errorf("update command run: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) GetRun(id int64) (Run, error) {
	row := s.db.QueryRow(`SELECT id, username, command_name, command_script, program, args_json, status, exit_code, started_at, duration_ms, stdout, stderr FROM command_runs WHERE id = ?`, id)
	return scanRun(row)
}

func (s *Store) CountRuns(filter RunFilter) (int, error) {
	conditions, args := runFilterConditions(filter)
	query := `SELECT COUNT(*) FROM command_runs`
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, ` AND `)
	}
	var count int
	if err := s.db.QueryRow(query, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("count command runs: %w", err)
	}
	return count, nil
}

func (s *Store) ListRuns(filter RunFilter, limit, offset int) ([]Run, error) {
	if limit < 1 || limit > 500 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	conditions, args := runFilterConditions(filter)
	query := `SELECT id, username, command_name, command_script, program, args_json, status, exit_code, started_at, duration_ms, stdout, stderr FROM command_runs`
	if len(conditions) > 0 {
		query += ` WHERE ` + strings.Join(conditions, ` AND `)
	}
	query += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list command runs: %w", err)
	}
	defer rows.Close()
	runs := make([]Run, 0)
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read command runs: %w", err)
	}
	return runs, nil
}

func runFilterConditions(filter RunFilter) ([]string, []any) {
	conditions := make([]string, 0, 5)
	args := make([]any, 0, 5)
	if filter.Username != "" {
		if filter.UsernameExact {
			conditions = append(conditions, `username = ?`)
			args = append(args, filter.Username)
		} else {
			conditions = append(conditions, `username LIKE ?`)
			args = append(args, "%"+filter.Username+"%")
		}
	}
	if filter.Command != "" {
		conditions = append(conditions, `command_name LIKE ?`)
		args = append(args, "%"+filter.Command+"%")
	}
	if filter.Status != "" {
		conditions = append(conditions, `status = ?`)
		args = append(args, filter.Status)
	}
	if filter.From != "" {
		conditions = append(conditions, `started_at >= ?`)
		args = append(args, filter.From+`T00:00:00Z`)
	}
	if filter.To != "" {
		conditions = append(conditions, `started_at <= ?`)
		args = append(args, filter.To+`T23:59:59.999999999Z`)
	}
	return conditions, args
}

type scanner interface {
	Scan(dest ...any) error
}

func scanCommand(row scanner) (Command, error) {
	var command Command
	var argsJSON, labelsJSON, specificUsersJSON, specificOperatorsJSON, lastAppliedAt, updatedAt string
	var accessOperators, accessAllUsers, operatorsCanRun int
	if err := row.Scan(&command.ID, &command.Name, &command.Description, &command.Icon, &command.Program, &argsJSON, &command.Script, &labelsJSON, &command.TimeoutSeconds, &accessOperators, &accessAllUsers, &specificUsersJSON, &specificOperatorsJSON, &operatorsCanRun, &command.LastAppliedBy, &lastAppliedAt, &updatedAt); err != nil {
		return Command{}, err
	}
	if err := json.Unmarshal([]byte(argsJSON), &command.Args); err != nil {
		return Command{}, fmt.Errorf("decode command arguments: %w", err)
	}
	if err := json.Unmarshal([]byte(labelsJSON), &command.Labels); err != nil {
		return Command{}, fmt.Errorf("decode command labels: %w", err)
	}
	if err := json.Unmarshal([]byte(specificUsersJSON), &command.SpecificUsers); err != nil {
		return Command{}, fmt.Errorf("decode command user access: %w", err)
	}
	if err := json.Unmarshal([]byte(specificOperatorsJSON), &command.SpecificOperators); err != nil {
		return Command{}, fmt.Errorf("decode command operator access: %w", err)
	}
	command.AccessOperators = accessOperators != 0
	command.AccessAllUsers = accessAllUsers != 0
	command.OperatorsCanRun = operatorsCanRun != 0
	command.OperatorAllowed = command.OperatorsCanRun
	if lastAppliedAt != "" {
		command.LastAppliedAt, _ = time.Parse(time.RFC3339Nano, lastAppliedAt)
	}
	command.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updatedAt)
	return command, nil
}

func scanRun(row scanner) (Run, error) {
	var run Run
	var startedAt, argsJSON string
	var durationMilliseconds int64
	if err := row.Scan(&run.ID, &run.Username, &run.CommandName, &run.CommandScript, &run.Program, &argsJSON, &run.Status, &run.ExitCode, &startedAt, &durationMilliseconds, &run.Stdout, &run.Stderr); err != nil {
		return Run{}, err
	}
	if err := json.Unmarshal([]byte(argsJSON), &run.Args); err != nil {
		return Run{}, fmt.Errorf("decode command run arguments: %w", err)
	}
	if run.Status == "interrupted" {
		run.ExitCode = -1
	}
	parsed, err := time.Parse(time.RFC3339Nano, startedAt)
	if err != nil {
		return Run{}, errors.New("invalid timestamp in command run")
	}
	run.StartedAt = parsed
	run.Duration = time.Duration(durationMilliseconds) * time.Millisecond
	return run, nil
}
