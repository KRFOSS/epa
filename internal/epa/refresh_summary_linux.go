//go:build linux

package epa

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// UpgradeCandidates queries only local metadata after a successful refresh.
// Pacman must instead use the exact isolated DB held by PacmanRefresh.
func (b Backend) UpgradeCandidates(ctx context.Context) (int, error) {
	binary := b.Binary
	var args []string
	timeout := 30 * time.Second
	switch b.Name {
	case "apt":
		var err error
		binary, err = b.helper("apt")
		if err != nil {
			return 0, err
		}
		args = []string{"-o", "APT::Cmd::Disable-Script-Warning=true", "-o", "APT::Color=false", "list", "--upgradable"}
	case "dnf":
		args = []string{"-q", "--cacheonly", "repoquery", "--upgrades", "--queryformat=%{name}.%{arch}"}
	case "apk":
		args = []string{"--no-network", "--no-progress", "list", "--upgradable"}
	case "portage":
		timeout = 90 * time.Second
		args = []string{"--ignore-default-opts", "--ask=n", "--pretend", "--verbose", "--color=n", "--autounmask-write=n", "--autounmask-continue=n", "--getbinpkg=n", "--update", "--deep", "--newuse", "@world"}
	default:
		return 0, LocalizedErrorf("지원하지 않는 후보 조회 백엔드: %s", "unsupported upgrade-query backend: %s", b.Name)
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	// Do not mistake a skipped repository, invalid cache or solver error for
	// zero upgrades. Keep diagnostics visible, including warnings on success.
	if diagnostic := strings.TrimSpace(stderr.String()); diagnostic != "" {
		return 0, LocalizedErrorf("%s 조회 진단: %s", "%s query diagnostic: %s", b.Name, diagnostic)
	}
	if err != nil {
		return 0, err
	}
	return parseUpgradeCandidates(b.Name, string(out))
}

var aptCandidate = regexp.MustCompile("^([^/\\s]+)/\\S+\\s+\\S+\\s+(\\S+)\\s+\\[upgradable from: \\S+\\]$")
var dnfCandidate = regexp.MustCompile("^[A-Za-z0-9_+][A-Za-z0-9_.+-]*\\.[A-Za-z0-9_]+$")
var apkCandidate = regexp.MustCompile("^\\S+\\s+(\\S+)\\s+.*\\[upgradable from: ([^\\]\\s]+)\\]$")
var portageTotal = regexp.MustCompile("^Total: ([0-9]+) packages?(?: \\([^\\r\\n]*\\))?, Size of downloads:.*$")

func parseUpgradeCandidates(backend, output string) (int, error) {
	invalid := func() (int, error) {
		return 0, LocalizedErrorf("%s 업데이트 후보 출력 형식을 확인할 수 없습니다.", "unrecognized %s upgrade candidate output", backend)
	}
	seen := make(map[string]bool)
	total := -1
	aptHeader := false
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		switch backend {
		case "apt":
			if line == "Listing..." || line == "Listing... Done" {
				aptHeader = true
				continue
			}
			m := aptCandidate.FindStringSubmatch(line)
			if m == nil {
				return invalid()
			}
			seen[m[1]+":"+m[2]] = true
		case "dnf":
			if !dnfCandidate.MatchString(line) {
				return invalid()
			}
			seen[line] = true
		case "apk":
			m := apkCandidate.FindStringSubmatch(line)
			if m == nil {
				return invalid()
			}
			// The installed package-version and architecture identify one
			// candidate even when several repositories offer updates.
			seen[m[2]+":"+m[1]] = true
		case "portage":
			if m := portageTotal.FindStringSubmatch(line); m != nil {
				if total >= 0 {
					return invalid()
				}
				n, err := strconv.Atoi(m[1])
				if err != nil {
					return invalid()
				}
				total = n
			}
		default:
			return invalid()
		}
	}
	if backend == "portage" {
		if total < 0 {
			return invalid()
		}
		return total, nil
	}
	if backend == "apt" && !aptHeader {
		return invalid()
	}
	return len(seen), nil
}
