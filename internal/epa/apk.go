package epa

import "strings"

// Alpine uses the world constraint set rather than APT's installed/automatic
// distinction. Keep native resolution and refuse destructive approximations.
func buildAPK(b Backend, o Operation, unsupported func(string) (Plan, error)) (Plan, error) {
	p := Plan{Binary: b.Binary, Privileged: o.Mutates()}
	if o.Action != Search {
		for _, name := range o.Packages {
			if strings.Contains(name, ":") {
				return unsupported(Text("APK 가상 제약 대신 실제 패키지 이름을 사용하십시오.", "use a real package name rather than an APK virtual constraint"))
			}
		}
	}
	switch o.Action {
	case Refresh:
		p.Args = []string{"update"}
		if o.ForceRefresh {
			p.Args = append([]string{"--force-refresh"}, p.Args...)
		}
	case Install:
		p.Args = []string{"add"}
	case Remove:
		if o.Style == "pacman" {
			return unsupported(Text("APK는 world 제약과 불필요한 의존성을 함께 변경하므로 Pacman 제거 방식을 보장할 수 없습니다.", "APK changes world constraints and unused dependencies; Pacman removal semantics cannot be guaranteed"))
		}
		p.Args = []string{"del"}
		p.Note = Text("APK는 불필요한 의존 패키지도 제거하며 다른 패키지가 요구하는 대상은 남겨둘 수 있습니다.", "APK also removes unused dependencies and may retain a target required by other packages.")
	case Upgrade, FullUpgrade:
		if len(o.Packages) != 0 {
			return unsupported(Text("특정 패키지만 업그레이드하는 기능은 아직 지원하지 않습니다.", "targeted upgrades are not yet supported"))
		}
		p.Args = []string{"upgrade"}
		// Do not use --available: it may force downgrades to repository versions.
		if o.RefreshFirst {
			p.RefreshBefore = true
			p.RefreshArgs = []string{"update"}
			if o.ForceRefresh {
				p.RefreshArgs = append([]string{"--force-refresh"}, p.RefreshArgs...)
			}
		}
		p.Note = Text("Alpine의 네이티브 업그레이드 정책을 적용합니다.", "Applying Alpine's native upgrade policy.")
	case Search:
		p.Args = []string{"search"}
	case Info:
		p.Args = []string{"info", "--all"}
	case List:
		p.Args = []string{"info"}
	case Outdated:
		p.Args = []string{"list", "--upgradable"}
	case Reinstall:
		p.Args = []string{"fix", "--reinstall"}
	case Clean:
		p.Args = []string{"cache", "clean"}
		if o.Style != "apk" {
			p.Note = Text("APK의 네이티브 캐시 정리를 수행합니다. 오래되거나 불필요한 캐시 파일만 정리합니다.", "Using native APK cache cleanup, which removes only obsolete or unnecessary cached files.")
		}
	case Autoremove:
		return Plan{Skip: true, Note: Text("APK는 패키지 제거 시 불필요한 의존성을 정리합니다. 별도의 자동 제거 명령이 없어 작업을 건너뜁니다. 패키지를 제거하지 않았습니다.", "APK prunes unused dependencies during removal. No separate autoremove command is available; skipping this operation without removing packages.")}, nil
	default:
		return unsupported(Text("지원하지 않는 작업입니다.", "unsupported action"))
	}
	switch o.Action {
	case Install, Remove, Reinstall, Upgrade, FullUpgrade:
		if o.Yes {
			p.Args = append([]string{"--no-interactive"}, p.Args...)
		} else {
			p.Args = append([]string{"--interactive"}, p.Args...)
		}
	}
	return p, nil
}
