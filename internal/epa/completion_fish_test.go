//go:build linux

package epa

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestFishPersistentCompletion(t *testing.T) {
	home := t.TempDir()
	cfg := filepath.Join(home, "custom config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("SUDO_USER", "")
	t.Setenv("SUDO_COMMAND", "")
	for i := 0; i < 2; i++ {
		got, err := InstallCompletion("fish")
		if err != nil || !reflect.DeepEqual(got, []string{"fish"}) {
			t.Fatalf("install=%v err=%v", got, err)
		}
	}
	path := filepath.Join(cfg, "fish", "completions", "epa.fish")
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "command epa completion fish | source") {
		t.Fatalf("loader=%q err=%v", data, err)
	}
	if _, err := os.Stat(filepath.Join(cfg, "fish", "config.fish")); !os.IsNotExist(err) {
		t.Fatal("fish config was unexpectedly modified")
	}
	original := []byte("# user-defined fish completion\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := InstallCompletion("fish"); err == nil {
		t.Fatal("custom file overwritten")
	}
	data, err = os.ReadFile(path)
	if err != nil || string(data) != string(original) {
		t.Fatalf("custom file changed: %q %v", data, err)
	}
}

func TestFishCompletionCandidates(t *testing.T) {
	script, ok := CompletionScript("fish")
	if !ok || !strings.Contains(script, "__complete --") {
		t.Fatal("Fish completion script unavailable")
	}
	if got := Complete("apt", []string{"completion", "install", "f"}, Backend{}); !reflect.DeepEqual(got, []string{"fish"}) {
		t.Fatal(got)
	}
}
