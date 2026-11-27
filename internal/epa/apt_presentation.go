package epa

import (
	"fmt"
	"regexp"
	"strings"
)

var aptSummary = regexp.MustCompile(`^([0-9]+) upgraded, ([0-9]+) newly installed, ([0-9]+) to remove and ([0-9]+) not upgraded\.$`)
var aptRepository = regexp.MustCompile(`^(Hit|Get):([0-9]+) (.+)$`)

// Translate only recognizable, routine APT output. Never suppress or rewrite
// repository failures, warnings, package names, or maintainer-script prompts.
func aptLineLabel(line string) (string, bool) {
	switch line {
	case "The following packages will be upgraded:":
		return Text("업그레이드할 패키지:", "Packages to upgrade:"), true
	case "The following NEW packages will be installed:":
		return Text("새로 설치할 패키지:", "Packages to install:"), true
	case "The following packages will be REMOVED:":
		return Text("제거할 패키지:", "Packages to remove:"), true
	case "The following packages have been kept back:":
		return Text("업그레이드를 보류한 패키지:", "Packages held back:"), true
	}
	if m := aptSummary.FindStringSubmatch(line); m != nil {
		return fmt.Sprintf(Text("업그레이드 %s개 · 신규 설치 %s개 · 제거 %s개 · 보류 %s개", "Upgrade: %s · Install: %s · Remove: %s · Held back: %s"), m[1], m[2], m[3], m[4]), true
	}
	if m := aptRepository.FindStringSubmatch(line); m != nil {
		label := Text("최신 상태", "Current")
		if m[1] == "Get" {
			label = Text("가져오기", "Fetching")
		}
		return fmt.Sprintf("[%s] %s", label, m[3]), true
	}
	if value, ok := strings.CutPrefix(line, "Need to get "); ok {
		if size, ok := strings.CutSuffix(value, " of archives."); ok && size != "" {
			return fmt.Sprintf(Text("다운로드할 용량: %s", "Download size: %s"), size), true
		}
	}
	if value, ok := strings.CutPrefix(line, "After this operation, "); ok {
		if size, ok := strings.CutSuffix(value, " of additional disk space will be used."); ok && size != "" {
			return fmt.Sprintf(Text("추가 디스크 사용량: %s", "Additional disk space: %s"), size), true
		}
		if size, ok := strings.CutSuffix(value, " disk space will be freed."); ok && size != "" {
			return fmt.Sprintf(Text("확보할 디스크 공간: %s", "Disk space freed: %s"), size), true
		}
	}
	return "", false
}

func aptConfirmation(line string) (string, string, bool) {
	for _, options := range []string{"[Y/n]", "[y/N]"} {
		prefix := "Do you want to continue? " + options + " "
		if answer, ok := strings.CutPrefix(line, prefix); ok {
			switch strings.ToLower(strings.TrimSpace(answer)) {
			case "", "y", "yes", "n", "no":
				return Text("계속하시겠습니까? ", "Continue? ") + options + " " + answer, answer, true
			}
		}
	}
	return "", "", false
}

func aptNegativeAnswer(answer string) bool {
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "n", "no":
		return true
	}
	return false
}

func phaseDoneLabel(phase string) string {
	switch phase {
	case "Reading metadata":
		return Text("저장소 정보 확인 완료", "Repository metadata read")
	case "Resolving dependencies":
		return Text("의존성 확인 완료", "Dependencies resolved")
	case "Reading installed state":
		return Text("설치 상태 확인 완료", "Installed state read")
	case "Calculating upgrade":
		return Text("업그레이드 계획 확인 완료", "Upgrade plan calculated")
	default:
		return phaseLabel(phase)
	}
}
