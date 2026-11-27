//go:build linux

package epa

import (
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// DirectSudo distinguishes the usual "sudo epa" invocation from a root
// shell which merely inherited sudo's environment. This is a CLI policy,
// not an authentication or privilege boundary. Never use SUDO_USER to
// choose another account's configuration or installation path.
func DirectSudo() bool {
	exe, _ := os.Executable()
	return directSudo(os.Geteuid(), os.Getenv("SUDO_USER"), os.Getenv("SUDO_COMMAND"), exe)
}

func directSudo(euid int, sudoUser, command, executable string) bool {
	if euid != 0 || sudoUser == "" {
		return false
	}
	fields := strings.Fields(command)
	if len(fields) == 0 {
		// Inherited SUDO_USER alone does not identify a direct invocation.
		return false
	}
	name := filepath.Base(fields[0])
	return name == "epa" || fields[0] == executable || name == filepath.Base(executable)
}

func inheritedRootEnvironment() bool {
	return os.Geteuid() == 0 && os.Getenv("SUDO_USER") != ""
}

// A root shell may retain HOME, XDG_CONFIG_HOME or ZDOTDIR from the
// invoking account. Resolve the actual account rather than trusting them.
func accountHome() (string, error) {
	if !inheritedRootEnvironment() {
		return os.UserHomeDir()
	}
	account, err := user.LookupId("0")
	if err != nil {
		return "", err
	}
	return account.HomeDir, nil
}
