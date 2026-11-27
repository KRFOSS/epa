//go:build linux

package epa

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPersistentCompletion(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home with ' spaces")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("ZDOTDIR", "")
	t.Setenv("SUDO_USER", "")
	bashrc := filepath.Join(home, ".bashrc")
	profile := filepath.Join(home, ".profile")
	zshrc := filepath.Join(home, ".zshrc")
	initial := map[string]string{bashrc: "# custom bash\n", profile: "# custom login without newline", zshrc: "# custom zsh\n"}
	for path, text := range initial {
		if err := os.WriteFile(path, []byte(text), 0640); err != nil {
			t.Fatal(err)
		}
	}
	first := map[string]string{}
	for _, shell := range []string{"bash", "zsh"} {
		result, err := InstallCompletion(shell)
		if err != nil || !reflect.DeepEqual(result, []string{shell}) {
			t.Fatalf("%s: %v %v", shell, result, err)
		}
	}
	for path, text := range initial {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		s := string(data)
		if !strings.HasPrefix(s, text) || strings.Count(s, completionBegin) != 1 || strings.Count(s, completionEnd) != 1 || !strings.Contains(s, "'\"'\"'") {
			t.Errorf("unexpected startup file %s: %q", path, s)
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0640 {
			t.Fatalf("mode: %v %v", info, err)
		}
		first[path] = s
	}
	for _, shell := range []string{"bash", "zsh"} {
		if _, err := InstallCompletion(shell); err != nil {
			t.Fatal(err)
		}
	}
	for path, expected := range first {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != expected {
			t.Fatalf("reinstallation changed %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".bash_profile")); !os.IsNotExist(err) {
		t.Fatal("unexpected shadowing login file")
	}
}

func TestBashLoginPrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SUDO_USER", "")
	for _, name := range []string{".bash_profile", ".bash_login", ".profile"} {
		if err := os.WriteFile(filepath.Join(home, name), []byte("# original\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := InstallCompletion("bash"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".bash_profile", ".bash_login", ".profile"} {
		data, err := os.ReadFile(filepath.Join(home, name))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), completionBegin) != (name == ".bash_profile") {
			t.Errorf("wrong login file: %s", name)
		}
	}
}

func TestCompletionZdotdir(t *testing.T) {
	home := t.TempDir()
	zdir := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZDOTDIR", zdir)
	t.Setenv("SUDO_USER", "")
	if _, err := InstallCompletion("zsh"); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(filepath.Join(zdir, ".zshrc")); err != nil || !strings.Contains(string(data), completionBegin) {
		t.Fatalf("ZDOTDIR: %s %v", data, err)
	}
	if _, err := os.Lstat(filepath.Join(home, ".zshrc")); !os.IsNotExist(err) {
		t.Fatal("unexpected home .zshrc")
	}
}

func TestCompletionRefusesUnsafeFiles(t *testing.T) {
	for _, kind := range []string{"symlink", "hardlink", "writable", "broken-marker", "foreign-loader", "directory-symlink", "writable-directory"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("SUDO_USER", "")
			target := filepath.Join(home, ".bashrc")
			original := []byte("# existing\n")
			switch kind {
			case "symlink", "hardlink":
				outside := filepath.Join(t.TempDir(), "original")
				if err := os.WriteFile(outside, original, 0600); err != nil {
					t.Fatal(err)
				}
				var err error
				if kind == "symlink" {
					err = os.Symlink(outside, target)
				} else {
					err = os.Link(outside, target)
				}
				if err != nil {
					t.Fatal(err)
				}
			case "writable":
				if err := os.WriteFile(target, original, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(target, 0666); err != nil {
					t.Fatal(err)
				}
			case "broken-marker":
				original = []byte(completionBegin + "\n")
				if err := os.WriteFile(target, original, 0600); err != nil {
					t.Fatal(err)
				}
			case "foreign-loader":
				if err := makeCompletionDirectory(home); err != nil {
					t.Fatal(err)
				}
				target = filepath.Join(home, ".local", "share", "epa", "completion", "bash")
				if err := os.WriteFile(target, original, 0600); err != nil {
					t.Fatal(err)
				}
			case "directory-symlink", "writable-directory":
				if err := os.WriteFile(target, original, 0600); err != nil {
					t.Fatal(err)
				}
				dir := filepath.Join(home, ".local")
				if kind == "directory-symlink" {
					if err := os.Symlink(t.TempDir(), dir); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := os.Mkdir(dir, 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.Chmod(dir, 0777); err != nil {
						t.Fatal(err)
					}
				}
			}
			if _, err := InstallCompletion("bash"); err == nil {
				t.Fatal("unsafe configuration accepted")
			}
			data, err := os.ReadFile(target)
			if err != nil || string(data) != string(original) {
				t.Fatalf("unsafe file changed: %q %v", data, err)
			}
		})
	}
}

func TestCompletionInstallCandidates(t *testing.T) {
	for _, style := range Styles {
		if got := Complete(style, []string{"completion", "in"}, Backend{}); !reflect.DeepEqual(got, []string{"install"}) {
			t.Errorf("%s: %v", style, got)
		}
		if got := Complete(style, []string{"completion", "install", ""}, Backend{}); !reflect.DeepEqual(got, []string{"all", "bash", "fish", "zsh"}) {
			t.Errorf("%s: %v", style, got)
		}
	}
}
