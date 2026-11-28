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
		if o.Style != "apk" {
			return unsupported(Text("APK cache clean은 오래되거나 불필요한 캐시만 정리합니다. APK 스타일로 직접 실행하십시오.", "APK cache clean removes only obsolete or unnecessary cached files; use APK style explicitly"))
		}
		p.Args = []string{"cache", "clean"}
	case Autoremove:
		return unsupported(Text("APK는 제거 과정에서 불필요한 의존성을 자동 정리하므로 별도 autoremove를 지원하지 않습니다.", "APK cleans unused dependencies during removals; separate autoremove is unavailable"))
	default:
		return unsupported(Text("지원하지 않는 작업입니다.", "unsupported action"))
	}
	switch o.Action {
	case Install, Remove, Reinstall, Upgrade, FullUpgrade:
		if o.Yes {
			p.Args = append([]string{"--no-interactive"}, p.Args...)
		} else if o.Interactive || o.Style != "apk" {
			p.Args = append([]string{"--interactive"}, p.Args...)
		}
	}
	return p, nil
}
