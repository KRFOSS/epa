//go:build linux

package epa

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func snapshotFixture(t *testing.T) pacmanSnapshot {
	t.Helper()
	root := t.TempDir()
	native := filepath.Join(root, "var/lib/pacman")
	for _, dir := range []string{filepath.Join(root, "var/cache"), filepath.Join(native, "local"), filepath.Join(native, "sync")} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	s, err := newPacmanSnapshot(root, native, uint32(os.Geteuid()))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func fixtureSetup(binary string, args []string) error { return exec.Command(binary, args...).Run() }

func TestPacmanSnapshotLifecycle(t *testing.T) {
	s := snapshotFixture(t)
	if s.queryReady() {
		t.Fatal("uncreated cache marked ready")
	}
	local := filepath.Join(s.native, "local", "fixture")
	system := filepath.Join(s.native, "sync", "core.db")
	for _, p := range []string{local, system} {
		if err := os.WriteFile(p, []byte("original"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	r, err := s.prepare(fixtureSetup)
	if err != nil {
		t.Fatal(err)
	}
	if s.queryReady() {
		t.Fatal("unfinished refresh marked ready")
	}
	if _, err := s.prepare(fixtureSetup); err == nil {
		t.Fatal("concurrent refresh accepted")
	}
	if err := os.Mkdir(filepath.Join(s.path, "sync"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.path, "sync", "core.db"), []byte("new repository"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := r.Finish(fixtureSetup); err != nil {
		t.Fatal(err)
	}
	r.Close()
	if !s.queryReady() {
		t.Fatal("completed cache unavailable")
	}
	for _, p := range []string{local, system} {
		data, err := os.ReadFile(p)
		if err != nil || string(data) != "original" {
			t.Fatalf("native database changed: %q %v", data, err)
		}
	}
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(filepath.Join(s.native, "sync"), future, future); err != nil {
		t.Fatal(err)
	}
	if s.queryReady() {
		t.Fatal("older cache selected over native metadata")
	}
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(filepath.Join(s.native, "sync"), past, past); err != nil {
		t.Fatal(err)
	}
	r, err = s.prepare(fixtureSetup)
	if err != nil {
		t.Fatal(err)
	}
	r.Close() // interrupted or failed refresh: Finish was not called.
	if s.queryReady() {
		t.Fatal("failed refresh advertised as ready")
	}
}

func TestPacmanSnapshotUnsafePaths(t *testing.T) {
	for _, kind := range []string{"directory symlink", "writable parent", "local mismatch", "sync symlink", "marker symlink"} {
		t.Run(kind, func(t *testing.T) {
			s := snapshotFixture(t)
			if err := os.MkdirAll(s.path, 0755); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "directory symlink":
				if err := os.Remove(s.path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(s.native, s.path); err != nil {
					t.Fatal(err)
				}
			case "writable parent":
				if err := os.Chmod(filepath.Dir(s.path), 0777); err != nil {
					t.Fatal(err)
				}
			case "local mismatch":
				if err := os.Symlink(filepath.Join(s.native, "sync"), filepath.Join(s.path, "local")); err != nil {
					t.Fatal(err)
				}
			case "sync symlink":
				if err := os.Symlink(filepath.Join(s.native, "sync"), filepath.Join(s.path, "sync")); err != nil {
					t.Fatal(err)
				}
			case "marker symlink":
				if err := os.Symlink(filepath.Join(s.native, "local"), filepath.Join(s.path, ".epa-ready")); err != nil {
					t.Fatal(err)
				}
			}
			if r, err := s.prepare(fixtureSetup); err == nil {
				r.Close()
				t.Fatal("unsafe snapshot accepted")
			}
			if s.queryReady() {
				t.Fatal("unsafe snapshot selected")
			}
		})
	}
	s := snapshotFixture(t)
	if _, err := newPacmanSnapshot(s.root, filepath.Dir(s.path), s.uid); err == nil {
		t.Fatal("overlapping database accepted")
	}
}
