//go:build linux

package epa

import (
	"context"
	"crypto/sha256"
	"debug/elf"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	latestReleaseURL   = "https://api.github.com/repos/KRFOSS/epa/releases/latest"
	releaseDownloadURL = "https://github.com/KRFOSS/epa/releases/download"
	maxReleaseMetadata = 1 << 20
	maxReleaseBinary   = 64 << 20
)

type releaseVersion [3]uint64

var stableVersionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`)

func parseReleaseVersion(s string) (releaseVersion, error) {
	var v releaseVersion
	if !stableVersionPattern.MatchString(s) {
		return v, LocalizedErrorf("EPA 정식 릴리스 버전 형식이 아닙니다: %q", "not a stable EPA release version: %q", s)
	}
	for i, part := range strings.Split(s, ".") {
		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return v, err
		}
		v[i] = n
	}
	return v, nil
}

func (v releaseVersion) newerThan(old releaseVersion) bool {
	for i := range v {
		if v[i] != old[i] {
			return v[i] > old[i]
		}
	}
	return false
}

// Asset naming must match tests/build_release.sh. In particular, a 32-bit
// ARM executable must keep its own GOARM target, not the kernel's uname.
func releaseAsset(arch, goarm string) (string, error) {
	switch arch {
	case "386", "amd64", "arm64", "loong64", "mips", "mipsle", "mips64", "mips64le", "ppc64", "ppc64le", "riscv64", "s390x":
		return "epa-linux-" + arch, nil
	case "arm":
		level, _, _ := strings.Cut(goarm, ",")
		if level == "5" || level == "6" || level == "7" {
			return "epa-linux-armv" + level, nil
		}
	}
	return "", LocalizedErrorf("%s 아키텍처용 릴리스 파일이 없습니다. (GOARM=%s)", "no release asset for %s (GOARM=%s)", arch, goarm)
}

func currentReleaseAsset() (string, error) {
	arm := ""
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "GOARM" {
				arm = s.Value
			}
		}
	}
	return releaseAsset(runtime.GOARCH, arm)
}

// A self-update is never allowed to acquire privileges or replace a
// system-managed installation. Restrict it to the user's default installer
// path; custom and system paths must use the installer/package manager.
type updateTarget struct {
	path string
	info os.FileInfo
	uid  uint32
}

func ownedMode(info os.FileInfo, uid uint32) bool {
	st, ok := info.Sys().(*syscall.Stat_t)
	return ok && st.Uid == uid && info.Mode().Perm()&0022 == 0
}

func checkedTarget(home, exe string, running os.FileInfo, uid uint32) (updateTarget, error) {
	var target updateTarget
	if uid == 0 {
		return target, LocalizedErrorf("관리자 권한 없이 설치한 사용자 전용 EPA만 자동으로 교체할 수 있습니다.", "automatic replacement is restricted to unprivileged user installations")
	}
	if !filepath.IsAbs(home) || !filepath.IsAbs(exe) {
		return target, LocalizedErrorf("실행 파일 또는 홈 디렉터리 경로가 올바르지 않습니다.", "invalid executable or home path")
	}
	resolved, err := filepath.EvalSymlinks(home)
	if err != nil {
		return target, err
	}
	expected := filepath.Join(resolved, ".local", "bin", "epa")
	if filepath.Clean(exe) != expected {
		return target, LocalizedErrorf("자체 업데이트는 사용자가 소유한 ~/.local/bin/epa 설치에만 지원됩니다.", "self-update requires the user-owned ~/.local/bin/epa installation")
	}
	// No symlink or writable directory on the user-controlled part of the path.
	for _, dir := range []string{resolved, filepath.Join(resolved, ".local"), filepath.Dir(expected)} {
		info, err := os.Lstat(dir)
		if err != nil {
			return target, err
		}
		if !info.IsDir() || !ownedMode(info, uid) {
			return target, LocalizedErrorf("안전하게 업데이트할 수 없는 디렉터리입니다: %s", "unsafe update directory: %s", dir)
		}
	}
	info, err := os.Lstat(expected)
	if err != nil {
		return target, err
	}
	if !info.Mode().IsRegular() || !ownedMode(info, uid) || info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 || info.Mode().Perm()&0100 == 0 {
		return target, LocalizedErrorf("안전하게 업데이트할 수 없는 실행 파일입니다: %s", "unsafe update executable: %s", expected)
	}
	st := info.Sys().(*syscall.Stat_t)
	if st.Nlink != 1 || running == nil || !os.SameFile(info, running) {
		return target, LocalizedErrorf("실행 중인 EPA와 설치된 파일이 더 이상 일치하지 않습니다.", "the running EPA executable is no longer the installed file")
	}
	return updateTarget{expected, info, uid}, nil
}

func currentUpdateTarget() (updateTarget, error) {
	if os.Geteuid() == 0 || os.Getuid() != os.Geteuid() || os.Getenv("SUDO_USER") != "" {
		return updateTarget{}, LocalizedErrorf("자체 업데이트는 root 또는 sudo 설치를 교체하지 않습니다. 기존 설치 방식으로 갱신하십시오.", "self-update does not replace root or sudo installations; use the installation method instead")
	}
	account, err := user.Current()
	if err != nil {
		return updateTarget{}, err
	}
	exe, err := os.Executable()
	if err != nil {
		return updateTarget{}, err
	}
	running, err := os.Stat("/proc/self/exe")
	if err != nil {
		return updateTarget{}, err
	}
	return checkedTarget(account.HomeDir, exe, running, uint32(os.Geteuid()))
}

func (t updateTarget) unchanged() error {
	current, err := os.Lstat(t.path)
	if err != nil {
		return err
	}
	if !current.Mode().IsRegular() || !ownedMode(current, t.uid) || current.Mode() != t.info.Mode() || current.Sys().(*syscall.Stat_t).Nlink != 1 ||
		!os.SameFile(current, t.info) || current.Size() != t.info.Size() || !current.ModTime().Equal(t.info.ModTime()) {
		return LocalizedErrorf("자체 업데이트 도중 설치된 EPA가 변경되어 교체를 중단합니다.", "installed EPA changed during self-update; refusing replacement")
	}
	return nil
}

// Eligibility checks are local. No HTTP request happens in a dry run,
// noninteractive command, developer build, disabled mode or failed upgrade.
func SelfUpdateEligible(version string) bool {
	if _, err := parseReleaseVersion(version); err != nil {
		return false
	}
	if _, err := currentReleaseAsset(); err != nil {
		return false
	}
	_, err := currentUpdateTarget()
	return err == nil
}

func ShouldAutoUpdate(action Action, interactive, disabled, dry bool) bool {
	return interactive && !disabled && !dry && (action == Refresh || action == Upgrade || action == FullUpgrade)
}

type releaseSource struct {
	client       *http.Client
	latest, base string
}

func allowedReleaseURL(u *url.URL) bool {
	if u.Scheme != "https" || u.User != nil || (u.Port() != "" && u.Port() != "443") {
		return false
	}
	switch u.Hostname() {
	case "api.github.com", "github.com", "release-assets.githubusercontent.com", "objects.githubusercontent.com", "github-releases.githubusercontent.com":
		return true
	}
	return false
}

func githubReleaseSource() releaseSource {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 10 * time.Second
	transport.TLSHandshakeTimeout = 5 * time.Second
	client := &http.Client{Transport: transport, Timeout: 2 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || !allowedReleaseURL(req.URL) {
			return LocalizedErrorf("안전하지 않은 GitHub 릴리스 리디렉션입니다.", "unsafe GitHub release redirect")
		}
		return nil
	}}
	return releaseSource{client, latestReleaseURL, releaseDownloadURL}
}

func (s releaseSource) get(ctx context.Context, address, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ROKFOSS-EPA")
	req.Header.Set("Accept", accept)
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, LocalizedErrorf("릴리스 서버가 HTTP %d를 반환했습니다.", "release server returned HTTP %d", resp.StatusCode)
	}
	return resp, nil
}

func readBounded(r io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, LocalizedErrorf("릴리스 응답이 허용된 크기를 초과합니다.", "release response exceeds size limit")
	}
	return data, nil
}

func (s releaseSource) latestTag(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	resp, err := s.get(ctx, s.latest, "application/vnd.github+json")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := readBounded(resp.Body, maxReleaseMetadata)
	if err != nil {
		return "", err
	}
	var rel struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err = json.Unmarshal(data, &rel); err != nil {
		return "", err
	}
	if rel.Draft || rel.Prerelease || !strings.HasPrefix(rel.Tag, "v") {
		return "", LocalizedErrorf("최신 릴리스가 정식 버전이 아닙니다.", "latest release is not a stable version")
	}
	if _, err = parseReleaseVersion(rel.Tag[1:]); err != nil {
		return "", err
	}
	return rel.Tag, nil
}

func checksumFor(data []byte, asset string) ([]byte, error) {
	var result []byte
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 || fields[1] != asset {
			continue
		}
		if len(fields) != 2 || result != nil {
			return nil, LocalizedErrorf("%s의 체크섬이 중복되어 확인할 수 없습니다.", "ambiguous checksum for %s", asset)
		}
		digest, err := hex.DecodeString(fields[0])
		if err != nil || len(digest) != sha256.Size {
			return nil, LocalizedErrorf("%s의 SHA-256 체크섬 형식이 올바르지 않습니다.", "invalid SHA-256 for %s", asset)
		}
		result = digest
	}
	if result == nil {
		return nil, LocalizedErrorf("릴리스에 %s의 체크섬이 없습니다.", "release does not contain a checksum for %s", asset)
	}
	return result, nil
}

func (s releaseSource) checksum(ctx context.Context, tag, asset string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	resp, err := s.get(ctx, s.base+"/"+tag+"/SHA256SUMS", "application/octet-stream")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := readBounded(resp.Body, maxReleaseMetadata)
	if err != nil {
		return nil, err
	}
	return checksumFor(data, asset)
}

func verifyELF(current, candidate string) error {
	old, err := elf.Open(current)
	if err != nil {
		return LocalizedErrorf("설치된 실행 파일을 검사할 수 없습니다: %w", "cannot inspect installed executable: %w", err)
	}
	defer old.Close()
	next, err := elf.Open(candidate)
	if err != nil {
		return LocalizedErrorf("릴리스 파일이 ELF 실행 파일이 아닙니다: %w", "release asset is not an ELF executable: %w", err)
	}
	defer next.Close()
	if next.Class != old.Class || next.Data != old.Data || next.Machine != old.Machine ||
		(next.Type != elf.ET_EXEC && next.Type != elf.ET_DYN) {
		return LocalizedErrorf("릴리스 파일의 아키텍처가 설치된 EPA와 일치하지 않습니다.", "release asset architecture does not match installed EPA")
	}
	return nil
}

type UpdateResult struct {
	Version string
	Updated bool
}

// updateFrom is also used with a local test server. Production endpoints
// are fixed, HTTPS-only and pinned to the selected release tag.
func updateFrom(ctx context.Context, s releaseSource, t updateTarget, version, asset string) (UpdateResult, error) {
	result := UpdateResult{Version: version}
	old, err := parseReleaseVersion(version)
	if err != nil {
		return result, err
	}
	dir, err := os.Open(filepath.Dir(t.path))
	if err != nil {
		return result, err
	}
	defer dir.Close()
	// Serialize EPA self-updaters without creating a persistent lock file.
	if err = syscall.Flock(int(dir.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return result, LocalizedErrorf("EPA 설치 파일에 대한 잠금을 획득할 수 없습니다: %w", "cannot lock EPA installation: %w", err)
	}
	defer syscall.Flock(int(dir.Fd()), syscall.LOCK_UN)
	if err = t.unchanged(); err != nil {
		return result, err
	}
	tag, err := s.latestTag(ctx)
	if err != nil {
		return result, err
	}
	next, err := parseReleaseVersion(tag[1:])
	if err != nil {
		return result, err
	}
	if !next.newerThan(old) {
		return result, nil
	}
	expected, err := s.checksum(ctx, tag, asset)
	if err != nil {
		return result, err
	}

	downloadCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	resp, err := s.get(downloadCtx, s.base+"/"+tag+"/"+asset, "application/octet-stream")
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	if resp.ContentLength > maxReleaseBinary {
		return result, LocalizedErrorf("릴리스 바이너리가 허용된 크기를 초과합니다.", "release binary exceeds size limit")
	}
	stage, err := os.CreateTemp(filepath.Dir(t.path), ".epa-update-*")
	if err != nil {
		return result, err
	}
	defer os.Remove(stage.Name())
	defer stage.Close()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(stage, hash), io.LimitReader(resp.Body, maxReleaseBinary+1))
	if err != nil {
		return result, err
	}
	if size == 0 || size > maxReleaseBinary {
		return result, LocalizedErrorf("릴리스 바이너리 크기가 올바르지 않습니다.", "invalid release binary size")
	}
	if !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), hex.EncodeToString(expected)) {
		return result, LocalizedErrorf("릴리스 SHA-256 검증에 실패했습니다. 기존 EPA는 변경하지 않았습니다.", "release SHA-256 verification failed; existing EPA was not changed")
	}
	if err = verifyELF(t.path, stage.Name()); err != nil {
		return result, err
	}
	if err = stage.Chmod(t.info.Mode().Perm()); err != nil {
		return result, err
	}
	if err = stage.Sync(); err != nil {
		return result, err
	}
	if err = stage.Close(); err != nil {
		return result, err
	}
	if err = ctx.Err(); err != nil {
		return result, err
	}
	if err = t.unchanged(); err != nil {
		return result, err
	}
	if err = os.Rename(stage.Name(), t.path); err != nil {
		return result, err
	}
	// The atomic rename is the commit point; directory fsync is best effort.
	_ = dir.Sync()
	return UpdateResult{Version: tag[1:], Updated: true}, nil
}

func SelfUpdate(ctx context.Context, version string) (UpdateResult, error) {
	result := UpdateResult{Version: version}
	if _, err := parseReleaseVersion(version); err != nil {
		return result, err
	}
	target, err := currentUpdateTarget()
	if err != nil {
		return result, err
	}
	asset, err := currentReleaseAsset()
	if err != nil {
		return result, err
	}
	return updateFrom(ctx, githubReleaseSource(), target, version, asset)
}
