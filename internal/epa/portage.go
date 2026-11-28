package epa

import (
	"regexp"
	"strings"
)

// Portage package atoms may contain exactly one category separator. Do not
// forward local files, version constraints, slots, repos, sets or USE flags.
var portageTarget = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9+_.-]*(/[A-Za-z0-9][A-Za-z0-9+_.-]*)?$`)
var portageVersionSuffix = regexp.MustCompile(`-[0-9]+(?:\.[0-9]+)*[a-z]?(?:_(?:alpha|beta|pre|rc|p)[0-9]*)*(?:-r[0-9]+)?$`)

func ValidPortageTarget(name string) bool {
	if !portageTarget.MatchString(name) || strings.HasSuffix(name, "-") {
		return false
	}
	parts := strings.Split(name, "/")
	for _, part := range parts {
		if strings.HasSuffix(part, "-") {
			return false
		}
	}
	pkg := strings.ToLower(parts[len(parts)-1])
	for _, suffix := range []string{".ebuild", ".tbz2", ".xpak", ".gpkg", ".gpkg.tar", ".tar", ".tar.gz", ".tar.xz", ".rpm", ".deb", ".apk"} {
		if strings.HasSuffix(pkg, suffix) {
			return false
		}
	}
	return !portageVersionSuffix.MatchString(pkg)
}

// Use the installed Portage API, not filenames or a network lookup. A fixed
// isolated Python program prints only actual category/package names and
// unambiguous short names. The caller supplies a read-only database name.
const portageNamesScript = `import sys
import portage
from collections import Counter
tree, prefix = sys.argv[1:3]
db = portage.db[portage.settings["EROOT"]][tree].dbapi
if tree == "porttree" and "/" in prefix:
    category = prefix.split("/", 1)[0]
    names = sorted(set(db.cp_all(categories=[category]))) if category in db.settings.categories else []
else:
    names = sorted(set(db.cp_all()))
short = Counter(n.rsplit("/", 1)[-1] for n in names)
for name in names:
    base = name.rsplit("/", 1)[-1]
    if name.startswith(prefix):
        print(name)
    if "/" not in prefix and short[base] == 1 and base.startswith(prefix):
        print(base)
`

func portageSyncArgs() []string { return []string{"--ask=n", "--sync"} }

func portageWorldArgs() []string {
	return []string{"--update", "--deep", "--newuse", "@world"}
}

func buildPortage(b Backend, o Operation, unsupported func(string) (Plan, error)) (Plan, error) {
	p := Plan{Binary: b.Binary, Privileged: o.Mutates()}
	for _, name := range o.Packages {
		if o.Action != Search && !ValidPortageTarget(name) {
			return unsupported(Text("Gentoo의 실제 패키지 이름 또는 category/package를 사용하십시오. 파일, 버전, 슬롯 및 저장소 지정자는 지원하지 않습니다.", "use a native Gentoo package name or category/package; files, versions, slots and repository qualifiers are unsupported"))
		}
	}
	switch o.Action {
	case Refresh:
		p.Args = portageSyncArgs()
		if o.ForceRefresh {
			p.Note = Text("Portage 저장소를 다시 동기화합니다. 변경되지 않은 파일의 재다운로드는 보장하지 않습니다.", "Resynchronizing Portage repositories; unchanged files may be reused.")
		}
	case Install:
		// Update existing targets if needed, and record explicit selections.
		p.Args = []string{"--update", "--select=y"}
	case Upgrade, FullUpgrade:
		if len(o.Packages) != 0 {
			return unsupported(Text("특정 패키지만 갱신하는 기능은 아직 지원하지 않습니다.", "targeted upgrades are not yet supported"))
		}
		p.Args = portageWorldArgs()
		p.Note = Text("Gentoo의 @world 전체를 갱신하며 USE 플래그 변경에 따른 재빌드도 포함할 수 있습니다.", "Updating Gentoo @world, including rebuilds needed for changed USE flags.")
		if o.RefreshFirst {
			p.RefreshBefore = true
			p.RefreshArgs = portageSyncArgs()
		}
	case Remove:
		if o.Style == "pacman" {
			return unsupported(Text("Portage depclean은 Pacman 제거와 다른 의존성 및 world 정책을 사용합니다.", "Portage depclean has different dependency and world semantics from Pacman removal"))
		}
		// Never translate to --unmerge: it ignores reverse dependencies.
		p.Args = []string{"--depclean", "--depclean-lib-check=y"}
		p.Note = Text("Portage가 역의존성을 검사합니다. 다른 패키지가 요구하는 대상은 남겨둘 수 있습니다.", "Portage checks reverse dependencies and may retain required packages.")
	case Autoremove:
		p.Args = []string{"--depclean", "--depclean-lib-check=y"}
		p.Note = Text("Portage가 @world의 전체 의존성을 검사합니다. 제거할 패키지 목록을 확인하십시오.", "Portage checks @world dependencies. Review the removal list.")
	case Reinstall:
		// Rebuild the target without promoting it into the world set.
		p.Args = []string{"--selective=n", "--oneshot"}
	case Search:
		p.Args = []string{"--ignore-default-opts", "--search"}
	case Info:
		if o.Style == "emerge" {
			p.Args = []string{"--ignore-default-opts", "--info"}
		} else {
			// emerge --info is primarily system/build diagnostics. Exact
			// native search records provide package metadata for common show.
			p.Args = []string{"--ignore-default-opts", "--search", "--fuzzy-search=n", "--"}
			for _, name := range o.Packages {
				prefix := "%^"
				if strings.Contains(name, "/") {
					prefix = "%@^"
				}
				p.Args = append(p.Args, prefix+regexp.QuoteMeta(name)+"$")
			}
			return p, nil
		}
	case List:
		var err error
		p.Binary, err = b.helper("portageq")
		if err != nil {
			return Plan{}, err
		}
		p.Args = []string{"match", "/", ""}
	case Outdated:
		p.Args = append([]string{"--ask=n", "--pretend", "--autounmask-write=n", "--autounmask-continue=n"}, portageWorldArgs()...)
	case Clean:
		var err error
		p.Binary, err = b.helper("eclean")
		if err != nil {
			return unsupported(Text("캐시 정리에는 app-portage/gentoolkit의 eclean이 필요합니다. emerge --clean은 캐시 정리 명령이 아닙니다.", "cache cleanup requires eclean from app-portage/gentoolkit; emerge --clean is not cache cleanup"))
		}
		p.Args = []string{"distfiles"}
		p.Note = Text("eclean을 사용해 오래된 소스 아카이브만 정리합니다. 설치된 패키지는 제거하지 않습니다.", "Using eclean to remove obsolete source archives, not installed packages.")
	default:
		return unsupported(Text("지원하지 않는 작업입니다.", "unsupported action"))
	}
	switch o.Action {
	case Install, Remove, Autoremove, Reinstall, Upgrade, FullUpgrade:
		ask := "--ask=y"
		if o.Yes {
			ask = "--ask=n"
		}
		p.Args = append([]string{ask, "--verbose", "--autounmask-write=n", "--autounmask-continue=n"}, p.Args...)
	}
	if len(o.Packages) != 0 {
		p.Args = append(p.Args, "--")
		p.Args = append(p.Args, o.Packages...)
	}
	return p, nil
}
