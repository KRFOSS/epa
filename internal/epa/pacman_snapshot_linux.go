//go:build linux

package epa

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Never use this database for a package-changing transaction. The native
// system sync databases remain untouched until an explicit full upgrade.
const PacmanSnapshotPath = "/var/cache/epa/pacman"

type pacmanSnapshot struct {
	root, path, native string
	uid                uint32
}

func snapshotContains(base, path string) bool {
	rel, err := filepath.Rel(base, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func newPacmanSnapshot(root, native string, uid uint32) (pacmanSnapshot, error) {
	s := pacmanSnapshot{root: root, path: filepath.Join(root, "var/cache/epa/pacman"), uid: uid}
	if !filepath.IsAbs(native) {
		return s, LocalizedErrorf("pacman DBPath가 절대 경로가 아닙니다.", "pacman DBPath is not absolute")
	}
	resolved, err := filepath.EvalSymlinks(native)
	if err != nil {
		return s, err
	}
	if snapshotContains(s.path, resolved) || snapshotContains(resolved, s.path) {
		return s, LocalizedErrorf("EPA 조회 캐시와 시스템 패키지 DB가 겹칩니다.", "EPA query cache overlaps the system package database")
	}
	// A custom pacman configuration may use symlinked sync/local
	// subdirectories. Refuse aliases into the EPA cache as well.
	for _, name := range []string{"local", "sync"} {
		actual, err := filepath.EvalSymlinks(filepath.Join(resolved, name))
		if errors.Is(err, os.ErrNotExist) && name == "sync" {
			continue
		}
		if err != nil {
			return s, err
		}
		if snapshotContains(s.path, actual) || snapshotContains(actual, s.path) {
			return s, LocalizedErrorf("EPA 조회 캐시와 시스템 패키지 DB가 겹칩니다.", "EPA query cache overlaps the system package database")
		}
	}
	s.native = resolved
	info, err := os.Stat(filepath.Join(resolved, "local"))
	if err != nil {
		return s, err
	}
	if !info.IsDir() {
		return s, LocalizedErrorf("pacman 설치 DB가 디렉터리가 아닙니다.", "pacman local database is not a directory")
	}
	return s, nil
}

func nativePacmanDB() (string, error) {
	binary := systemBinary("pacman-conf")
	if binary == "" {
		return "", LocalizedErrorf("pacman-conf를 찾을 수 없습니다.", "pacman-conf not found")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, binary, "DBPath").Output()
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(string(out))
	if !filepath.IsAbs(path) || strings.ContainsAny(path, "\r\n\x00") {
		return "", LocalizedErrorf("pacman DBPath가 올바르지 않습니다.", "invalid pacman DBPath")
	}
	return path, nil
}

func (s pacmanSnapshot) directory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || !ownedMode(info, s.uid) {
		return LocalizedErrorf("안전하지 않은 pacman 조회 캐시 디렉터리: %s", "unsafe pacman query cache directory: %s", path)
	}
	return nil
}

// Check every component before asking sudo to create anything. Only the
// EPA subdirectories may be missing; no existing directory is chmod'ed.
func (s pacmanSnapshot) directories() (bool, error) {
	for _, rel := range []string{"", "var", "var/cache", "var/cache/epa", "var/cache/epa/pacman"} {
		path := filepath.Join(s.root, rel)
		err := s.directory(path)
		if errors.Is(err, os.ErrNotExist) && (rel == "var/cache/epa" || rel == "var/cache/epa/pacman") {
			return false, nil
		}
		if err != nil {
			return false, err
		}
	}
	return true, nil
}

func (s pacmanSnapshot) localLink() (bool, error) {
	path := filepath.Join(s.path, "local")
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != s.uid || info.Mode()&os.ModeSymlink == 0 {
		return false, LocalizedErrorf("EPA 조회 캐시의 local 링크가 안전하지 않습니다: %s", "unsafe local link in EPA query cache: %s", path)
	}
	dest, err := os.Readlink(path)
	if err != nil {
		return false, err
	}
	if !filepath.IsAbs(dest) || filepath.Clean(dest) != filepath.Join(s.native, "local") {
		return false, LocalizedErrorf("EPA 조회 캐시의 local 링크가 현재 설치 DB와 다릅니다: %s", "EPA query cache local link differs from the current installed database: %s", path)
	}
	return true, nil
}

func (s pacmanSnapshot) readyMarker() (os.FileInfo, error) {
	path := filepath.Join(s.path, ".epa-ready")
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || st.Nlink != 1 || !info.Mode().IsRegular() || !ownedMode(info, s.uid) {
		return nil, LocalizedErrorf("EPA 조회 캐시의 완료 표시 파일이 안전하지 않습니다: %s", "unsafe EPA query cache completion marker: %s", path)
	}
	return info, nil
}

func (s pacmanSnapshot) optionalSync() error {
	err := s.directory(filepath.Join(s.path, "sync"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Run privileged setup using only fixed, trusted system executables and
// arguments derived from the checked system pacman configuration.
type privilegedSetup func(binary string, args []string) error

func runSnapshotSetup(run privilegedSetup, name string, args ...string) error {
	binary := systemBinary(name)
	if binary == "" {
		return LocalizedErrorf("필수 도구 %s를 찾을 수 없습니다.", "required tool %s not found", name)
	}
	return run(binary, args)
}

type PacmanRefresh struct {
	snapshot pacmanSnapshot
	lock     *os.File
}

func (r *PacmanRefresh) Close() {
	_ = syscall.Flock(int(r.lock.Fd()), syscall.LOCK_UN)
	_ = r.lock.Close()
}

func (s pacmanSnapshot) prepare(run privilegedSetup) (*PacmanRefresh, error) {
	exists, err := s.directories()
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := runSnapshotSetup(run, "install", "-d", "-m", "0755", "--", s.path); err != nil {
			return nil, err
		}
		exists, err = s.directories()
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, os.ErrNotExist
		}
	}
	lock, err := os.Open(s.path)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, LocalizedErrorf("다른 EPA 저장소 갱신 작업이 진행 중입니다: %w", "another EPA repository refresh is running: %w", err)
	}
	r := &PacmanRefresh{snapshot: s, lock: lock}
	ok := false
	defer func() {
		if !ok {
			r.Close()
		}
	}()
	linked, err := s.localLink()
	if err != nil {
		return nil, err
	}
	if !linked {
		if err := runSnapshotSetup(run, "ln", "-sT", "--", filepath.Join(s.native, "local"), filepath.Join(s.path, "local")); err != nil {
			return nil, err
		}
		linked, err = s.localLink()
		if err != nil {
			return nil, err
		}
		if !linked {
			return nil, os.ErrNotExist
		}
	}
	if err := s.optionalSync(); err != nil {
		return nil, err
	}
	// A failed or interrupted refresh must not be advertised as current.
	if _, err := s.readyMarker(); err == nil {
		if err := runSnapshotSetup(run, "rm", "-f", "--", filepath.Join(s.path, ".epa-ready")); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	ok = true
	return r, nil
}

func PreparePacmanRefresh(run privilegedSetup) (*PacmanRefresh, error) {
	native, err := nativePacmanDB()
	if err != nil {
		return nil, err
	}
	s, err := newPacmanSnapshot("/", native, 0)
	if err != nil {
		return nil, err
	}
	return s.prepare(run)
}

// Call only after the native refresh exits successfully. No completion
// marker is written on error or cancellation.
func (r *PacmanRefresh) Finish(run privilegedSetup) error {
	if err := r.snapshot.directory(filepath.Join(r.snapshot.path, "sync")); err != nil {
		return err
	}
	return runSnapshotSetup(run, "install", "-m", "0644", "--", "/dev/null", filepath.Join(r.snapshot.path, ".epa-ready"))
}

func (s pacmanSnapshot) queryReady() bool {
	exists, err := s.directories()
	if err != nil || !exists {
		return false
	}
	linked, err := s.localLink()
	if err != nil || !linked {
		return false
	}
	marker, err := s.readyMarker()
	if err != nil {
		return false
	}
	if err := s.directory(filepath.Join(s.path, "sync")); err != nil {
		return false
	}
	// Once a real system upgrade refreshes newer metadata, prefer the
	// native DB instead of the older isolated query snapshot.
	if nativeSync, err := os.Stat(filepath.Join(s.native, "sync")); err == nil && nativeSync.ModTime().After(marker.ModTime()) {
		return false
	}
	return true
}

// Only read-only search, info and upgrade-list callers use this function.
// Shell completion does not create caches, elevate privileges or use network.
func PacmanQueryArgs(args []string) []string {
	if _, err := os.Lstat(filepath.Join(PacmanSnapshotPath, ".epa-ready")); err != nil {
		return args
	}
	native, err := nativePacmanDB()
	if err != nil {
		return args
	}
	s, err := newPacmanSnapshot("/", native, 0)
	if err != nil || !s.queryReady() {
		return args
	}
	return append([]string{"--dbpath", s.path}, args...)
}
