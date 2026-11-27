//go:build linux

package epa

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const completionBegin = "# >>> EPA shell completion >>>"
const completionEnd = "# <<< EPA shell completion <<<"
const completionLoaderHeader = "# EPA-managed shell completion loader\n"
const maxStartupFile = 2 << 20

// Only modify regular files owned by this account, without following symbolic
// links or altering shared writable files. Do not change the user's other
// startup configuration or override their existing completion registration.
func safeCompletionDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || !ownedMode(info, uint32(os.Geteuid())) {
		return LocalizedErrorf("안전하게 수정할 수 없는 디렉터리입니다: %s", "unsafe completion directory: %s", path)
	}
	return nil
}

func makeCompletionDirectory(home string) error {
	// Check every component beneath the home directory, including ones
	// that already exist. Never traverse a symlink or shared writable
	// directory to install shell code.
	path := home
	for _, component := range []string{".local", "share", "epa", "completion"} {
		path = filepath.Join(path, component)
		err := safeCompletionDirectory(path)
		if errors.Is(err, os.ErrNotExist) {
			if err = os.Mkdir(path, 0700); err != nil && !errors.Is(err, os.ErrExist) {
				return err
			}
			err = safeCompletionDirectory(path)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func safeCompletionFile(info os.FileInfo) bool {
	return info.Mode().IsRegular() && ownedMode(info, uint32(os.Geteuid())) &&
		info.Sys().(*syscall.Stat_t).Nlink == 1 &&
		info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) == 0
}

func readCompletionFile(f *os.File, path string) ([]byte, error) {
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !safeCompletionFile(info) {
		return nil, LocalizedErrorf("안전하게 수정할 수 없는 파일입니다: %s", "unsafe completion file: %s", path)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxStartupFile+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxStartupFile || bytes.IndexByte(data, 0) >= 0 {
		return nil, LocalizedErrorf("지원하지 않는 시작 파일입니다: %s", "unsupported startup file: %s", path)
	}
	return data, nil
}

func appendCompletionBlock(path, block string) error {
	if err := safeCompletionDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_APPEND|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return err
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	data, err := readCompletionFile(f, path)
	if err != nil {
		return err
	}
	// Match whole lines. Do not modify a malformed or custom managed block.
	lines := strings.Split(string(data), "\n")
	begin, end := 0, 0
	for _, line := range lines {
		if line == completionBegin {
			begin++
		}
		if line == completionEnd {
			end++
		}
	}
	if begin != 0 || end != 0 {
		if begin == 1 && end == 1 && strings.Contains(string(data), block) {
			return nil
		}
		return LocalizedErrorf("기존 EPA 자동완성 블록을 먼저 확인하십시오: %s", "inspect existing EPA completion block before reinstalling: %s", path)
	}
	// A complete, ordinary shell startup file can be extended by a single
	// managed block. Preserve inode, permissions, contents and other metadata.
	extra := "\n" + block + "\n"
	if len(data) == 0 {
		extra = block + "\n"
	}
	if _, err := io.WriteString(f, extra); err != nil {
		return err
	}
	return f.Sync()
}

func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'" }

// Keep the loader small and stable across EPA releases. The installed binary
// generates the current completion script when the interactive shell starts.
// The separate loader also lets .profile remain valid POSIX shell syntax.
func completionLoader(shell string) string {
	if shell == "bash" {
		return completionLoaderHeader + `if command -v epa >/dev/null 2>&1 && ! complete -p epa >/dev/null 2>&1; then
    source <(command epa completion bash) || :
fi
`
	}
	return completionLoaderHeader + `if (( $+commands[epa] )); then
    if (( ! $+functions[compdef] )); then
        autoload -Uz compinit || return 0
        # Ignore insecure completion directories; never bypass compaudit.
        compinit -i || return 0
    fi
    if (( $+functions[compdef] )) && [[ -z "${_comps[epa]-}" ]]; then
        source <(command epa completion zsh) || :
    fi
fi
`
}

func installCompletionLoader(path, content string) error {
	if err := safeCompletionDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	// Write the complete loader to a temporary file and publish it with an
	// exclusive hard link. An interrupted write cannot leave a partial
	// shell script, and no existing user file is replaced.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".epa-completion-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err := io.WriteString(tmp, content); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Link(tmp.Name(), path); err == nil {
		return os.Remove(tmp.Name())
	} else if !errors.Is(err, os.ErrExist) {
		return err
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := readCompletionFile(f, path)
	if err != nil {
		return err
	}
	if string(data) != content {
		return LocalizedErrorf("기존 자동완성 로더를 확인하십시오: %s", "inspect existing completion loader: %s", path)
	}
	return nil
}

func completionBlock(shell, loader string) string {
	quote := shellQuote(loader)
	// Use portable syntax in .profile. Bash only parses the Bash-specific
	// loader after the guards confirm this is an interactive Bash process.
	guard := "BASH_VERSION"
	if shell == "zsh" {
		guard = "ZSH_VERSION"
	}
	return fmt.Sprintf("%s\nif [ -n \"${%s:-}\" ]; then\n    case \"$-\" in\n        *i*) [ ! -r %s ] || . %s || : ;;\n    esac\nfi\n%s", completionBegin, guard, quote, quote, completionEnd)
}

func bashLoginFile(home string) (string, error) {
	// Bash selects the first of these files. Do not create a .bash_profile
	// that would shadow an existing .profile or .bash_login.
	for _, name := range []string{".bash_profile", ".bash_login", ".profile"} {
		p := filepath.Join(home, name)
		if _, err := os.Lstat(p); err == nil {
			return p, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	return filepath.Join(home, ".profile"), nil
}

// InstallCompletion adds a persistent, account-local shell startup hook. It
// does not edit system files or run a shell. With "all", only available shells
// are registered; an explicit shell can be configured before installing it.
func InstallCompletion(shell string) ([]string, error) {
	if os.Getuid() != os.Geteuid() || DirectSudo() {
		return nil, LocalizedErrorf("자동완성은 사용할 계정에서 sudo 없이 등록하십시오.", "install completion from the intended account without sudo")
	}
	if shell != "all" && shell != "bash" && shell != "zsh" && shell != "fish" {
		return nil, LocalizedErrorf("지원하지 않는 셸입니다: %q", "unsupported shell %q", shell)
	}
	home, err := accountHome()
	if err != nil {
		return nil, err
	}
	home = filepath.Clean(home)
	if !filepath.IsAbs(home) {
		return nil, LocalizedErrorf("홈 디렉터리는 절대 경로여야 합니다.", "home directory must be absolute")
	}
	if err := safeCompletionDirectory(home); err != nil {
		return nil, err
	}
	// Use a stable home-relative directory even if XDG_CONFIG_HOME changes
	// between the installer and a login shell.
	dir := filepath.Join(home, ".local", "share", "epa", "completion")
	if strings.ContainsAny(dir, "\r\n") {
		return nil, LocalizedErrorf("자동완성 경로에 줄바꿈을 사용할 수 없습니다.", "completion path contains a newline")
	}
	shells := []string{shell}
	if shell == "all" {
		shells = nil
		for _, s := range []string{"bash", "zsh", "fish"} {
			if _, err := exec.LookPath(s); err == nil {
				shells = append(shells, s)
			}
		}
	}
	var installed []string
	var failures []error
	for _, s := range shells {
		if err := installShellCompletion(home, dir, s); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", s, err))
		} else {
			installed = append(installed, s)
		}
	}
	return installed, errors.Join(failures...)
}

func ensureCompletionDirectory(path string) error {
	err := safeCompletionDirectory(path)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return err
	}
	if err := ensureCompletionDirectory(parent); err != nil {
		return err
	}
	if err := os.Mkdir(path, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return err
	}
	return safeCompletionDirectory(path)
}

func installFishCompletion() error {
	dir, err := accountConfigDir()
	if err != nil {
		return err
	}
	if err := ensureCompletionDirectory(dir); err != nil {
		return err
	}
	for _, part := range []string{"fish", "completions"} {
		dir = filepath.Join(dir, part)
		if err := ensureCompletionDirectory(dir); err != nil {
			return err
		}
	}
	loader := completionLoaderHeader + "if type -q epa\n    command epa completion fish | source\nend\n"
	return installCompletionLoader(filepath.Join(dir, "epa.fish"), loader)
}

func installShellCompletion(home, dir, shell string) error {
	if shell == "fish" {
		return installFishCompletion()
	}
	var files []string
	if shell == "bash" {
		login, err := bashLoginFile(home)
		if err != nil {
			return err
		}
		files = []string{filepath.Join(home, ".bashrc"), login}
	} else {
		zdotdir := os.Getenv("ZDOTDIR")
		if inheritedRootEnvironment() {
			zdotdir = home
		}
		if zdotdir == "" {
			zdotdir = home
		}
		if !filepath.IsAbs(zdotdir) {
			return LocalizedErrorf("ZDOTDIR에는 절대 경로를 지정하십시오.", "ZDOTDIR must be an absolute path")
		}
		files = []string{filepath.Join(zdotdir, ".zshrc")}
	}
	if err := makeCompletionDirectory(home); err != nil {
		return err
	}
	loader := filepath.Join(dir, shell)
	if err := installCompletionLoader(loader, completionLoader(shell)); err != nil {
		return err
	}
	block := completionBlock(shell, loader)
	for _, path := range files {
		if err := appendCompletionBlock(path, block); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
	}
	return nil
}
