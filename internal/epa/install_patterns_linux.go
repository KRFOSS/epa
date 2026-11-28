//go:build linux

package epa

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"
)

const maxPatternTargets = 256

var installPattern = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9.+_?*-]*(/[A-Za-z0-9][A-Za-z0-9.+_?*-]*)?$")

func ValidInstallPattern(s string) bool {
	return len(s) <= 256 && strings.ContainsAny(s, "*?") && installPattern.MatchString(s)
}

func (o Operation) HasInstallPatterns() bool {
	if o.Action != Install {
		return false
	}
	for _, target := range o.Packages {
		if ValidInstallPattern(target) {
			return true
		}
	}
	return false
}

func (b Backend) ExpandInstallPatterns(ctx context.Context, op Operation) (Operation, error) {
	if !op.HasInstallPatterns() {
		return op, nil
	}
	names, err := b.installCatalog(ctx)
	if err != nil {
		return op, err
	}
	return expandInstallPatterns(b.Name, op, names)
}

func catalogName(backend, name string) bool {
	if backend == "portage" {
		return strings.Contains(name, "/") && ValidPortageTarget(name)
	}
	return ValidPackage(name) && !strings.Contains(name, ":")
}

func expandInstallPatterns(backend string, op Operation, names []string) (Operation, error) {
	if !op.HasInstallPatterns() {
		return op, nil
	}
	catalog := make([]string, 0, len(names))
	for _, name := range names {
		if catalogName(backend, name) {
			catalog = append(catalog, name)
		}
	}
	sort.Strings(catalog)
	seen := map[string]bool{}
	matched := map[string]bool{}
	var targets []string
	add := func(name string) {
		if !seen[name] {
			targets = append(targets, name)
			seen[name] = true
		}
	}
	for _, target := range op.Packages {
		if !ValidInstallPattern(target) {
			add(target)
			continue
		}
		if backend != "portage" && strings.Contains(target, "/") {
			return op, LocalizedErrorf("이 백엔드에서는 category/package 패턴을 사용할 수 없습니다: %q", "category/package patterns require Portage: %q", target)
		}
		found := false
		for _, name := range catalog {
			subject := name
			if backend == "portage" && !strings.Contains(target, "/") {
				subject = path.Base(name)
			}
			ok, err := path.Match(target, subject)
			if err != nil {
				return op, err
			}
			if !ok {
				continue
			}
			found = true
			matched[name] = true
			if len(matched) > maxPatternTargets {
				return op, LocalizedErrorf("설치 패턴의 일치 대상이 %d개를 초과합니다. 패턴을 좁혀 주십시오.", "install patterns match more than %d packages; narrow the patterns", maxPatternTargets)
			}
			add(name)
		}
		if !found {
			return op, LocalizedErrorf("설치용 저장소에서 패턴과 일치하는 패키지를 찾지 못했습니다: %q", "no package in the installation repositories matches %q", target)
		}
	}
	op.Packages = targets
	return op, nil
}

// Use the same local metadata that native installation uses, not the
// isolated Pacman query snapshot. Never refresh repositories here.
func (b Backend) installCatalog(ctx context.Context) ([]string, error) {
	binary := b.Binary
	var args []string
	switch b.Name {
	case "apt":
		binary = systemBinary("apt-cache")
		args = []string{"dumpavail"}
	case "dnf":
		args = []string{"-q", "--cacheonly", "repoquery", "--available", "--queryformat=%{name}"}
	case "pacman":
		args = []string{"-Slq"}
	case "apk":
		args = []string{"--no-network", "search", "-q"}
	case "portage":
		binary = systemBinary("python3")
		args = []string{"-I", "-c", portageNamesScript, "porttree", ""}
	default:
		return nil, LocalizedErrorf("지원하지 않는 백엔드: %s", "unsupported backend: %s", b.Name)
	}
	if binary == "" {
		return nil, LocalizedErrorf("설치 대상 조회 도구가 없습니다.", "installation catalog tool is unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	scanner := bufio.NewScanner(out)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	var names []string
	seen := make(map[string]bool)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if b.Name == "apt" {
			if !strings.HasPrefix(line, "Package: ") {
				continue
			}
			line = strings.TrimSpace(strings.TrimPrefix(line, "Package: "))
		}
		if line == "" {
			continue
		}
		// Portage's completion query also emits unqualified aliases.
		if b.Name == "portage" && !strings.Contains(line, "/") {
			continue
		}
		if catalogName(b.Name, line) && !seen[line] {
			names = append(names, line)
			seen[line] = true
		}
	}
	scanErr := scanner.Err()
	if scanErr != nil {
		_ = cmd.Process.Kill()
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if scanErr != nil {
		return nil, scanErr
	}
	if diagnostic := strings.TrimSpace(stderr.String()); diagnostic != "" {
		return nil, LocalizedErrorf("%s 설치 대상 조회 진단: %s", "%s installation catalog diagnostic: %s", b.Name, diagnostic)
	}
	if waitErr != nil {
		return nil, waitErr
	}
	return names, nil
}
