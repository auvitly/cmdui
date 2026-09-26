package config

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestLoadConfigHashesPasswordsAndAppliesDefaults(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	configData := `{"users":[{"username":"admin","password":"correct-horse-battery","role":"admin"},{"username":"ops","password":"operator-password","role":"operator"}]}`
	if err := os.WriteFile(configPath, []byte(configData), 0600); err != nil {
		t.Fatal(err)
	}

	config, err := Load(configPath)
	if err != nil {
		t.Fatalf("loadConfig() error = %v", err)
	}
	if config.Address != "127.0.0.1:8080" {
		t.Errorf("Address = %q, want default address", config.Address)
	}
	if config.Users["admin"].Role != "admin" || config.Users["ops"].Role != "operator" {
		t.Fatal("configured roles were not loaded")
	}
	if err := bcrypt.CompareHashAndPassword(config.Users["admin"].PasswordHash, []byte("correct-horse-battery")); err != nil {
		t.Errorf("admin password hash does not verify: %v", err)
	}
	if string(config.Users["admin"].PasswordHash) == "correct-horse-battery" {
		t.Fatal("password was stored in plaintext")
	}
}

func TestLoadConfigRequiresAdmin(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{"users":[{"username":"ops","password":"operator-password","role":"operator"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(configPath); err == nil {
		t.Fatal("loadConfig() accepted a configuration without an admin")
	}
}

func TestLoadConfigAcceptsFourCharacterPassword(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	configData := `{"users":[{"username":"admin","password":"abcd","role":"admin"}]}`
	if err := os.WriteFile(configPath, []byte(configData), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(configPath); err != nil {
		t.Fatalf("loadConfig() rejected a four-character password: %v", err)
	}
}

func TestLoadConfigRejectsCommandShells(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	configData := `{"allowed_executables":["C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe"],"users":[{"username":"admin","password":"correct-horse-battery","role":"admin"}]}`
	if err := os.WriteFile(configPath, []byte(configData), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(configPath); err == nil {
		t.Fatal("loadConfig() accepted a command shell in the executable allowlist")
	}
}
