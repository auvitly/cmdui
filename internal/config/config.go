package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cmdui/internal/domain"

	"golang.org/x/crypto/bcrypt"
)

type User = domain.User

type Config struct {
	Address               string
	DatabasePath          string
	SessionTTL            time.Duration
	MaxCommandTimeout     time.Duration
	MaxOutputBytes        int
	MaxConcurrentCommands int
	CookieSecure          bool
	AllowedExecutables    map[string]struct{}
	ScriptInterpreter     string
	Users                 map[string]User
}

type fileConfig struct {
	Address                  string           `json:"address"`
	DatabasePath             string           `json:"database_path"`
	SessionTTLMinutes        int              `json:"session_ttl_minutes"`
	MaxCommandTimeoutSeconds int              `json:"max_command_timeout_seconds"`
	MaxOutputBytes           int              `json:"max_output_bytes"`
	MaxConcurrentCommands    int              `json:"max_concurrent_commands"`
	CookieSecure             bool             `json:"cookie_secure"`
	AllowedExecutables       []string         `json:"allowed_executables"`
	ScriptInterpreter        string           `json:"script_interpreter"`
	Users                    []configuredUser `json:"users"`
}

type configuredUser struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}

	var file fileConfig
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&file); err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}

	if file.Address == "" {
		file.Address = "127.0.0.1:8080"
	}
	if file.DatabasePath == "" {
		file.DatabasePath = filepath.Join("data", "cmdui.sqlite")
	}
	if file.SessionTTLMinutes == 0 {
		file.SessionTTLMinutes = 480
	}
	if file.MaxCommandTimeoutSeconds == 0 {
		file.MaxCommandTimeoutSeconds = 30
	}
	if file.MaxOutputBytes == 0 {
		file.MaxOutputBytes = 64 * 1024
	}
	if file.MaxConcurrentCommands == 0 {
		file.MaxConcurrentCommands = 4
	}
	if file.SessionTTLMinutes < 1 || file.MaxCommandTimeoutSeconds < 1 || file.MaxOutputBytes < 1024 || file.MaxConcurrentCommands < 1 {
		return Config{}, errors.New("session TTL, command timeout and concurrent command limit must be positive (output limit at least 1024 bytes)")
	}

	databasePath, err := filepath.Abs(file.DatabasePath)
	if err != nil {
		return Config{}, fmt.Errorf("resolve database path: %w", err)
	}

	users := make(map[string]User, len(file.Users))
	adminFound := false
	for _, entry := range file.Users {
		username := strings.TrimSpace(entry.Username)
		if username == "" || len(entry.Password) < 4 {
			return Config{}, errors.New("each user needs a username and a password of at least 4 characters")
		}
		if entry.Role != "admin" && entry.Role != "operator" && entry.Role != "user" {
			return Config{}, fmt.Errorf("user %q has invalid role %q", username, entry.Role)
		}
		if _, exists := users[username]; exists {
			return Config{}, fmt.Errorf("duplicate username %q", username)
		}
		passwordHash, err := bcrypt.GenerateFromPassword([]byte(entry.Password), bcrypt.DefaultCost)
		if err != nil {
			return Config{}, fmt.Errorf("hash password for user %q: %w", username, err)
		}
		users[username] = User{Username: username, Role: entry.Role, PasswordHash: passwordHash}
		adminFound = adminFound || entry.Role == "admin"
	}
	if !adminFound {
		return Config{}, errors.New("configuration must define at least one admin user")
	}

	allowedExecutables := make(map[string]struct{}, len(file.AllowedExecutables))
	for _, executable := range file.AllowedExecutables {
		if !filepath.IsAbs(executable) {
			return Config{}, fmt.Errorf("allowed executable path must be absolute: %q", executable)
		}
		baseName := strings.ToLower(strings.TrimSuffix(filepath.Base(executable), filepath.Ext(executable)))
		switch baseName {
		case "cmd", "powershell", "pwsh", "sh", "bash", "zsh", "fish":
			return Config{}, fmt.Errorf("command shell cannot be added to allowed executables: %q", executable)
		}
		allowedExecutables[filepath.Clean(executable)] = struct{}{}
	}
	if file.ScriptInterpreter != "" && !filepath.IsAbs(file.ScriptInterpreter) {
		return Config{}, errors.New("script_interpreter must be an absolute path")
	}

	return Config{
		Address:               file.Address,
		DatabasePath:          databasePath,
		SessionTTL:            time.Duration(file.SessionTTLMinutes) * time.Minute,
		MaxCommandTimeout:     time.Duration(file.MaxCommandTimeoutSeconds) * time.Second,
		MaxOutputBytes:        file.MaxOutputBytes,
		MaxConcurrentCommands: file.MaxConcurrentCommands,
		CookieSecure:          file.CookieSecure,
		AllowedExecutables:    allowedExecutables,
		ScriptInterpreter:     file.ScriptInterpreter,
		Users:                 users,
	}, nil
}
