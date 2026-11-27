//go:build linux

package epa

import (
	"os"
	"os/user"
	"path/filepath"
	"testing"
)

func TestDirectSudo(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		uid                    int
		sudoUser, command, exe string
		want                   bool
	}{
		{"regular account", 1000, "operator", "/usr/local/bin/epa update", "/usr/local/bin/epa", false},
		{"direct root", 0, "", "", "/usr/local/bin/epa", false},
		{"login shell", 0, "operator", "/bin/bash", "/usr/local/bin/epa", false},
		{"zsh shell", 0, "operator", "/usr/bin/zsh", "/usr/local/bin/epa", false},
		{"fish shell", 0, "operator", "/usr/bin/fish", "/usr/local/bin/epa", false},
		{"su shell", 0, "operator", "/usr/bin/su -", "/usr/local/bin/epa", false},
		{"inherited user only", 0, "operator", "", "/usr/local/bin/epa", false},
		{"direct sudo", 0, "operator", "/usr/local/bin/epa style set apt", "/usr/local/bin/epa", true},
		{"direct short name", 0, "operator", "epa update", "/usr/local/bin/epa", true},
		{"renamed executable", 0, "operator", "/tmp/epa-test --version", "/tmp/epa-test", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := directSudo(tc.uid, tc.sudoUser, tc.command, tc.exe); got != tc.want {
				t.Fatalf("DirectSudo = %v, want %v", got, tc.want)
			}
		})
	}
}

// Only reads the actual root account's path. Executed in the disposable
// root container; never writes to the host account's configuration.
func TestRootAccountPaths(t *testing.T) {
	if os.Getenv("EPA_TEST_ROOT_INSTALL") != "1" || os.Geteuid() != 0 {
		t.Skip("requires disposable root container")
	}
	account, err := user.LookupId("0")
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("SUDO_USER", "operator")
	t.Setenv("SUDO_COMMAND", "/usr/bin/fish")
	if DirectSudo() {
		t.Fatal("root shell detected as direct sudo")
	}
	home, err := accountHome()
	if err != nil || home != account.HomeDir {
		t.Fatalf("home=%q err=%v", home, err)
	}
	cfg, err := configPath()
	if err != nil || cfg != filepath.Join(account.HomeDir, ".config", "epa", "config.json") {
		t.Fatalf("config=%q err=%v", cfg, err)
	}
}
