package epa

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type Backend struct{ Name, Binary string }
type Plan struct {
	Binary          string
	Args            []string
	Privileged      bool
	RefreshBefore   bool
	IsolatedRefresh bool
	RefreshArgs     []string
	Note            string
	Skip            bool // no native transaction is required
}

func systemBinary(name string) string {
	for _, dir := range []string{"/usr/bin", "/bin", "/usr/sbin", "/sbin"} {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() && st.Mode().Perm()&0111 != 0 {
			return p
		}
	}
	return ""
}

func osRelease() map[string]string {
	values := map[string]string{}
	for _, path := range []string{"/etc/os-release", "/usr/lib/os-release"} {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			k, v, ok := strings.Cut(scanner.Text(), "=")
			if ok {
				values[k] = strings.Trim(v, "\"'")
			}
		}
		f.Close()
		return values
	}
	return values
}

func Detect() (Backend, error) {
	data := osRelease()
	for _, id := range append([]string{data["ID"]}, strings.Fields(data["ID_LIKE"])...) {
		var name string
		var binaries []string
		switch id {
		case "debian", "ubuntu", "linuxmint", "pop", "kali", "raspbian":
			name, binaries = "apt", []string{"apt-get"}
		case "rhel", "fedora", "centos", "rocky", "almalinux", "ol", "amzn":
			name, binaries = "dnf", []string{"dnf", "dnf5"}
		case "gentoo":
			name, binaries = "portage", []string{"emerge"}
		case "alpine":
			name, binaries = "apk", []string{"apk"}
		case "arch", "archlinux", "manjaro", "cachyos", "endeavouros":
			name, binaries = "pacman", []string{"pacman"}
		}
		if name == "" {
			continue
		}
		for _, n := range binaries {
			if p := systemBinary(n); p != "" {
				return Backend{name, p}, nil
			}
		}
		return Backend{}, LocalizedErrorf("%s 배포판을 감지했으나 지원되는 패키지 관리자를 찾을 수 없습니다.", "%s detected, but its supported package manager is unavailable", id)
	}
	return Backend{}, LocalizedErrorf("지원하지 않는 배포판입니다: %q (지원 백엔드: apt, dnf, pacman, apk, portage)", "unsupported distribution %q (supported backends: apt, dnf, pacman, apk, portage)", data["ID"])
}

func (b Backend) helper(name string) (string, error) {
	p := systemBinary(name)
	if p == "" {
		return "", LocalizedErrorf("필수 백엔드 도구 %s를 찾을 수 없습니다.", "required backend tool %s not found", name)
	}
	return p, nil
}

func (b Backend) Build(o Operation) (Plan, error) {
	p := Plan{Binary: b.Binary, Privileged: o.Mutates()}
	unsupported := func(reason string) (Plan, error) {
		return Plan{}, LocalizedErrorf("%s에서 %s 작업을 안전하게 수행할 수 없습니다: %s", "%s cannot safely perform %s: %s", b.Name, o.Action, reason)
	}
	if o.Action == FullUpgrade && len(o.Packages) > 0 && o.Style != b.Name {
		return unsupported(Text("특정 패키지 업그레이드와 결합된 업그레이드는 백엔드마다 동작 방식이 다릅니다.", "targeted and combined upgrades have different semantics across backends"))
	}
	if o.PortageOptions && b.Name != "portage" {
		return unsupported(Text("명시적인 Portage 옵션은 Portage 백엔드에서만 지원합니다.", "explicit Portage options require the Portage backend"))
	}
	if b.Name != "portage" && o.Action != Search {
		for _, name := range o.Packages {
			if !ValidPackage(name) {
				return unsupported(Text("category/package 형식은 Portage 백엔드에서만 지원합니다.", "category/package targets require the Portage backend"))
			}
		}
	}
	if o.Interactive && b.Name != "apk" {
		return unsupported(Text("APK의 명시적 확인 옵션은 APK 백엔드에서만 지원합니다.", "explicit APK interaction is supported only on the APK backend"))
	}
	if o.ForceRefresh && b.Name != "apk" && o.Style == "apk" {
		return unsupported(Text("APK 강제 갱신과 동일한 동작을 보장할 수 없습니다.", "cannot guarantee equivalent APK forced refresh"))
	}
	if o.Action == Clean && o.Style == "apk" && b.Name != "apk" {
		return unsupported(Text("APK 캐시 정리 방식은 다른 백엔드에서 지원하지 않습니다.", "APK cache cleanup semantics are not supported on other backends"))
	}
	if o.NoSave && b.Name != "pacman" {
		return unsupported(Text("요청한 구성 파일 제거 방식을 지원하지 않습니다.", "the requested configuration removal semantics are unavailable"))
	}
	if o.Needed && b.Name != "pacman" {
		return unsupported(Text("--needed와 동일하게 동작하는 검증된 옵션이 없습니다.", "--needed has no verified equivalent"))
	}
	if o.Recursive && b.Name == "apt" {
		return unsupported(Text("재귀적 제거 동작 방식이 다릅니다.", "recursive removal semantics differ"))
	}
	if o.RefreshFirst && o.Action == FullUpgrade && b.Name == "apt" {
		p.RefreshBefore = true
		p.RefreshArgs = aptRefreshArgs(o.ForceRefresh)
	}

	switch b.Name {
	case "apt":
		switch o.Action {
		case Refresh:
			p.Args = aptRefreshArgs(o.ForceRefresh)
		case Install:
			p.Args = []string{"install", "--no-remove"}
		case Remove:
			for _, target := range o.Packages {
				if strings.HasSuffix(target, "+") {
					return unsupported(Text("APT는 패키지 이름 끝의 + 기호를 설치 지시로 해석합니다.", "APT interprets a trailing plus as an installation modifier"))
				}
			}
			p.Args = []string{"remove"}
		case Upgrade:
			p.Args = []string{"upgrade", "--with-new-pkgs", "--no-remove"}
		case FullUpgrade:
			if len(o.Packages) > 0 {
				return unsupported(Text("특정 패키지만 업그레이드하는 기능을 지원하지 않습니다.", "targeted upgrades are not supported"))
			}
			// A non-APT upgrade style does not imply permission for APT dist-upgrade.
			if o.Style != "apt" {
				p.Args = []string{"upgrade", "--with-new-pkgs", "--no-remove"}
				p.Note = Text("패키지를 제거하지 않고 업그레이드합니다.", "Upgrading without removing packages.")
			} else {
				p.Args = []string{"dist-upgrade"}
			}
		case Search, Info:
			var err error
			p.Binary, err = b.helper("apt-cache")
			if err != nil {
				return Plan{}, err
			}
			if o.Action == Search {
				p.Args = []string{"search"}
			} else {
				p.Args = []string{"show"}
			}
		case List, Outdated:
			var err error
			p.Binary, err = b.helper("apt")
			if err != nil {
				return Plan{}, err
			}
			if o.Action == List {
				p.Args = []string{"list", "--installed"}
			} else {
				p.Args = []string{"list", "--upgradable"}
			}
		case Autoremove:
			p.Args = []string{"autoremove"}
		case Reinstall:
			p.Args = []string{"install", "--reinstall", "--no-remove"}
		case Clean:
			if o.Style != "apt" {
				return unsupported(Text("APT의 캐시 정리 동작 방식이 다릅니다.", "APT cache cleanup semantics differ"))
			}
			p.Args = []string{"clean"}
		default:
			return unsupported(Text("지원하지 않는 작업입니다.", "unsupported action"))
		}
		if o.Yes {
			p.Args = append(p.Args, "-y")
		}
	case "dnf":
		switch o.Action {
		case Refresh:
			p.Args = []string{"--refresh", "makecache"}
		case Install:
			p.Args = []string{"install"}
		case Remove:
			p.Args = []string{"remove"}
			if o.Style != "dnf" && !o.Recursive {
				p.Args = append(p.Args, "--noautoremove")
			}
			if o.Style == "pacman" {
				return unsupported(Text("DNF는 pacman -R이 제거를 거부할 의존 패키지까지 제거할 수 있습니다.", "DNF may remove dependent packages that pacman -R would refuse to remove"))
			}
		case Upgrade:
			p.Args = []string{"--setopt=obsoletes=False", "upgrade"}
			p.Note = Text("DNF의 설치 전용 패키지 수 제한을 포함한 기본 트랜잭션 정책을 적용합니다.", "DNF applies its native transaction policy, including install-only package limits.")
		case FullUpgrade:
			p.Args = []string{"upgrade"}
			if o.Style == "apt" {
				p.Note = Text("DNF의 기본 업그레이드 정책을 적용합니다.", "DNF applies its native upgrade policy.")
			}
		case Search:
			p.Args = []string{"search"}
		case Info:
			p.Args = []string{"info"}
		case List:
			p.Args = []string{"list", "--installed"}
		case Outdated:
			p.Args = []string{"check-update"}
			resolved, _ := filepath.EvalSymlinks(b.Binary)
			if filepath.Base(b.Binary) == "dnf5" || filepath.Base(resolved) == "dnf5" {
				p.Args[0] = "check-upgrade"
			}
		case Autoremove:
			p.Args = []string{"autoremove"}
		case Reinstall:
			p.Args = []string{"reinstall"}
		case Clean:
			if o.Style == "pacman" {
				return unsupported(Text("pacman의 캐시 정리 동작 방식이 다릅니다.", "pacman cache cleanup semantics differ"))
			}
			p.Args = []string{"clean", "packages"}
			if o.Style == "dnf" {
				p.Args[1] = "all"
			}
		default:
			return unsupported(Text("지원하지 않는 작업입니다.", "unsupported action"))
		}
		if o.RefreshFirst && o.Action == FullUpgrade {
			p.Args = append([]string{"--refresh"}, p.Args...)
		}
		if o.Yes {
			p.Args = append([]string{"-y"}, p.Args...)
		}
	case "portage":
		return buildPortage(b, o, unsupported)
	case "apk":
		var err error
		p, err = buildAPK(b, o, unsupported)
		if err != nil {
			return Plan{}, err
		}
	case "pacman":
		switch o.Action {
		case Refresh:
			p.IsolatedRefresh = true
			p.Args = []string{"--dbpath", PacmanSnapshotPath, "--logfile", "/dev/null", "-Sy"}
			if o.ForceRefresh {
				p.Args[len(p.Args)-1] = "-Syy"
			}
			p.Note = Text("설치용 DB는 변경하지 않고 조회용 저장소 정보만 갱신합니다.", "Refreshing query-only repository metadata without changing the transaction database.")
		case Upgrade, FullUpgrade:
			// The isolated refresh never updates the transaction DB.
			// Always refresh the native DB together with a system upgrade.
			p.Args = []string{"-Syu"}
			if o.ForceRefresh {
				p.Args[0] = "-Syyu"
			}
			if o.Action == Upgrade {
				p.Note = Text("Arch에서는 전체 시스템 업그레이드를 수행합니다. 패키지 교체 및 제거가 필요할 수 있습니다.", "Arch performs a full system upgrade, which may require replacing or removing packages.")
			} else if o.Style != "pacman" || !o.RefreshFirst {
				p.Note = Text("저장소 정보를 갱신하고 시스템 전체를 업그레이드합니다.", "Refreshing repository metadata and upgrading the entire system.")
			}
		case Install:
			p.Args = []string{"-S"}
			p.Note = Text("Arch 시스템을 최신 상태로 유지하십시오. 설치는 조회용 캐시가 아닌 시스템 패키지 DB를 사용합니다.", "Use a fully upgraded Arch system; installation uses the system database, not the query cache.")
		case Remove:
			if o.Style != "pacman" {
				p.Note = Text("Pacman은 의존성이 깨지는 패키지 제거를 거부합니다.", "Pacman refuses removals that would break dependencies.")
			}
			p.Args = []string{"-R"}
			if o.Recursive && o.NoSave {
				p.Args[0] = "-Rns"
			} else if o.Recursive {
				p.Args[0] = "-Rs"
			} else if o.NoSave {
				p.Args[0] = "-Rn"
			}
		case Search:
			p.Args = []string{"-Ss"}
		case Info:
			p.Args = []string{"-Si"}
		case List:
			p.Args = []string{"-Q"}
		case Outdated:
			p.Args = []string{"-Qu"}
		case Clean:
			if o.Style != "pacman" {
				return unsupported(Text("pacman의 캐시 정리 동작 방식이 다릅니다.", "pacman cache cleanup semantics differ"))
			}
			p.Args = []string{"-Sc"}
		case Reinstall:
			p.Args = []string{"-S"}
		case Autoremove:
			return unsupported(Text("고아 패키지를 제거하려면 대상 패키지를 직접 선택해야 합니다.", "orphan removal requires an explicit package selection"))
		default:
			return unsupported(Text("지원하지 않는 작업입니다.", "unsupported action"))
		}
		if o.Yes {
			p.Args = append(p.Args, "--noconfirm")
		}
		if o.Needed {
			p.Args = append(p.Args, "--needed")
		}
	default:
		return unsupported(Text("알 수 없는 백엔드입니다.", "unknown backend"))
	}
	if o.ForceRefresh && b.Name == "apt" {
		note := Text("HTTP(S) 캐시를 우회해 저장소를 다시 확인합니다. 변경되지 않은 인덱스는 APT가 재사용할 수 있습니다.", "Rechecking repositories without HTTP(S) proxy cache reuse. APT may reuse unchanged indexes.")
		if p.Note != "" {
			p.Note += " "
		}
		p.Note += note
	}
	if len(o.Packages) > 0 {
		p.Args = append(p.Args, "--")
		p.Args = append(p.Args, o.Packages...)
	}
	return p, nil
}

// APT has no pacman -Syy equivalent that unconditionally downloads every
// index. Revalidate through HTTP(S) without proxy cache reuse, leaving
// APT's local index management, signature checks and locks intact.
func aptRefreshArgs(force bool) []string {
	if force {
		return []string{"-o", "Acquire::http::No-Cache=true", "-o", "Acquire::https::No-Cache=true", "update"}
	}
	return []string{"update"}
}
