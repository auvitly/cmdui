package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLinuxNameUsesSystemOSRelease(t *testing.T) {
	path := filepath.Join(t.TempDir(), "os-release")
	if err := os.WriteFile(path, []byte("NAME=Debian GNU/Linux\nPRETTY_NAME=\"Debian GNU/Linux 12 (bookworm)\"\nID=debian\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := linuxName(path); got != "Debian GNU/Linux 12 (bookworm)" {
		t.Fatalf("linuxName() = %q", got)
	}
}

func TestLinuxNameFallsBackToDistributionID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "os-release")
	if err := os.WriteFile(path, []byte("ID=fedora\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := linuxName(path); got != "Linux · fedora" {
		t.Fatalf("linuxName() = %q", got)
	}
}

func TestWindowsVersionIncludesDisplayVersionAndBuild(t *testing.T) {
	got := formatWindowsVersion("Windows 10 Pro", "24H2", "26100")
	if got != "Windows 11 Pro · 24H2 · Сборка 26100" {
		t.Fatalf("formatWindowsVersion() = %q", got)
	}
}

func TestMacOSNameReadsProductVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "SystemVersion.plist")
	plist := `<?xml version="1.0" encoding="UTF-8"?><plist><dict><key>ProductName</key><string>macOS</string><key>ProductVersion</key><string>15.4</string></dict></plist>`
	if err := os.WriteFile(path, []byte(plist), 0600); err != nil {
		t.Fatal(err)
	}
	if got := macOSName(path); got != "macOS 15.4" {
		t.Fatalf("macOSName() = %q", got)
	}
}
