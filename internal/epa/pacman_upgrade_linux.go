//go:build linux

package epa

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// The caller holds the refresh lock and has successfully published the
// snapshot. Query this exact database rather than falling back to a possibly
// stale system database. -Quq is read-only and does not require elevation.
func (r *PacmanRefresh) UpgradableCount(ctx context.Context) (int, error) {
	binary := systemBinary("pacman")
	if binary == "" {
		return 0, LocalizedErrorf("pacman을 찾을 수 없습니다.", "pacman not found")
	}
	return queryPacmanUpgradeCount(ctx, binary, r.snapshot.path)
}

var pacmanQueryPackage = regexp.MustCompile("^[A-Za-z0-9_+@][A-Za-z0-9@._+-]*$")

func queryPacmanUpgradeCount(ctx context.Context, binary, path string) (int, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "--dbpath", path, "--color", "never", "-Quq")
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	// Pacman returns 1 for an empty match as well as for errors. Only a
	// silent empty result can be interpreted as zero; diagnostics are
	// deliberately never hidden or treated as an empty successful query.
	diagnostic := strings.TrimSpace(stderr.String())
	if diagnostic != "" {
		return 0, LocalizedErrorf("pacman 조회 진단: %s", "pacman query diagnostic: %s", diagnostic)
	}
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 && trimmed == "" {
			return 0, nil
		}
		return 0, err
	}
	if trimmed == "" {
		return 0, nil
	}
	seen := make(map[string]bool)
	for _, line := range strings.Split(trimmed, "\n") {
		name := strings.TrimSpace(line)
		if !pacmanQueryPackage.MatchString(name) || seen[name] {
			return 0, LocalizedErrorf("pacman 업데이트 후보 출력 형식을 확인할 수 없습니다.", "unrecognized pacman upgrade candidate output")
		}
		seen[name] = true
	}
	return len(seen), nil
}
