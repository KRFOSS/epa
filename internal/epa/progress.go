package epa

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// ProgressMode controls presentation only; it must never change the native
// manager's arguments, prompts, signature policy, or transaction behavior.
type ProgressMode string

const (
	ProgressAuto   ProgressMode = "auto"
	ProgressTTY    ProgressMode = "tty"
	ProgressPlain  ProgressMode = "plain"
	ProgressJSON   ProgressMode = "json"
	ProgressNative ProgressMode = "native"
	ProgressQuiet  ProgressMode = "quiet"
)

var ProgressModes = []string{"auto", "tty", "plain", "json", "native", "quiet"}

func ValidProgressMode(s string) bool {
	for _, m := range ProgressModes {
		if s == m {
			return true
		}
	}
	return false
}

type ProgressEvent struct {
	Phase   string `json:"phase"`
	Item    string `json:"item,omitempty"`
	Percent *int   `json:"percent,omitempty"`
	Overall *int   `json:"overall_percent,omitempty"`
	Done    bool   `json:"done,omitempty"`
}

type eventRule struct {
	pattern                 *regexp.Regexp
	phase                   string
	itemGroup, percentGroup int
}

func rule(pattern, phase string, item, percent int) eventRule {
	return eventRule{regexp.MustCompile(pattern), phase, item, percent}
}

// APT's acquisition pulse reports the overall percentage before the bracket
// and the item's percentage inside it. Keep the two scopes separate.
var aptDownload = regexp.MustCompile(`^([0-9]{1,3})%\s*\[[0-9]+\s+([A-Za-z0-9.+_:-]+)\s+[0-9.,]+\s*[kMGT]?B(?:/[0-9.,]+\s*[kMGT]?B)?\s+([0-9]{1,3})%\]$`)

var aptRules = []eventRule{
	rule(`^Reading package lists\.\.\.\s*([0-9]{1,3})%$`, "Reading metadata", 0, 1),
	rule(`^Reading package lists\.\.\.\s*(Done)?$`, "Reading metadata", 0, 0),
	rule(`^Building dependency tree\.\.\.\s*([0-9]{1,3})%$`, "Resolving dependencies", 0, 1),
	rule(`^Building dependency tree\.\.\.\s*(Done)?$`, "Resolving dependencies", 0, 0),
	rule(`^Reading state information\.\.\.\s*([0-9]{1,3})%$`, "Reading installed state", 0, 1),
	rule(`^Reading state information\.\.\.\s*(Done)?$`, "Reading installed state", 0, 0),
	rule(`^Calculating upgrade\.\.\.\s*([0-9]{1,3})%$`, "Calculating upgrade", 0, 1),
	rule(`^Calculating upgrade\.\.\.\s*(Done)?$`, "Calculating upgrade", 0, 0),
	rule(`^Progress:\s*\[\s*([0-9]{1,3})%\s*\]\s*\[[#. =-]*\]$`, "Applying transaction", 0, 1),
	rule(`^([0-9]{1,3})%\s*\[Working\]$`, "Downloading metadata or packages", 0, 1),
}
var pacmanRules = []eventRule{
	rule(`^:: Synchronizing package databases\.\.\.$`, "Reading metadata", 0, 0),
	rule(`^resolving dependencies\.\.\.$`, "Resolving dependencies", 0, 0),
	rule(`^looking for conflicting packages\.\.\.$`, "Checking conflicts", 0, 0),
	rule(`^:: Retrieving packages\.\.\.$`, "Downloading packages", 0, 0),
	rule(`^([A-Za-z0-9.+_:@-]+)\s+[0-9.,]+\s+[KMGT]?i?B\s+[0-9.,]+\s+[KMGT]?i?B/s\s+[0-9:]+\s+\[[# -]*\]\s+([0-9]{1,3})%$`, "Downloading packages", 1, 2),
	rule(`^\(\s*[0-9]+/\s*[0-9]+\)\s+(checking keys in keyring|checking package integrity|loading package files|checking for file conflicts|checking available disk space)\s+\[[# -]*\]\s+([0-9]{1,3})%$`, "Checking packages", 1, 2),
	rule(`^\(\s*[0-9]+/\s*[0-9]+\)\s+(?:installing|upgrading|reinstalling|removing)\s+([A-Za-z0-9.+_:@-]+)\s+\[[# -]*\]\s+([0-9]{1,3})%$`, "Applying transaction", 1, 2),
}
var dnfRules = []eventRule{
	rule(`^Downloading Packages:\s*$`, "Downloading packages", 0, 0),
	rule(`^Running transaction check\s*$`, "Checking transaction", 0, 0),
	rule(`^Running transaction test\s*$`, "Testing transaction", 0, 0),
	rule(`^Running transaction\s*$`, "Applying transaction", 0, 0),
	rule(`^\(\s*[0-9]+/\s*[0-9]+\):\s+([A-Za-z0-9.+_:@-]+\.rpm)\s+([0-9]{1,3})%\s+\[[= >-]*\]\s+[0-9.,]+\s+[kKMGT]?B/s\s*\|\s*[0-9.,]+\s*[kKMGT]?B\s+[0-9:]+(?:\s+ETA)?$`, "Downloading packages", 1, 2),
	rule(`^\[\s*[0-9]+/\s*[0-9]+\]\s+([A-Za-z0-9.+_:@-]+\.rpm)\s+([0-9]{1,3})%\s*\|\s*[0-9.,]+\s*[kKMGT]?i?B/s\s*\|\s*[0-9.,]+\s*[kKMGT]?i?B\s*\|\s*[0-9hms:]+$`, "Downloading packages", 1, 2),
}

var apkRules = []eventRule{
	rule(`^\([0-9]+/[0-9]+\) (?:Installing|Upgrading|Downgrading|Reinstalling|Purging) ([A-Za-z0-9.+_:-]+) \([^()\r\n]+\)$`, "Applying transaction", 1, 0),
}

var portageRules = []eventRule{
	rule(`^>>> (?:Emerging(?: binary)?|Installing|Unmerging) \([0-9]+ of [0-9]+\) ([A-Za-z0-9.+_:@/-]+)(?:\.\.\.)?$`, "Applying transaction", 1, 0),
	rule(`^>>> Completed \([0-9]+ of [0-9]+\) ([A-Za-z0-9.+_:@/-]+)$`, "Applying transaction", 1, 0),
}

// Accept only complete, recognized progress records. In particular, do not
// interpret a generic number followed by '%' in an error or a prompt.
func ParseProgress(backend, line string) (ProgressEvent, bool) {
	var rules []eventRule
	switch backend {
	case "apt":
		rules = aptRules
	case "dnf":
		rules = dnfRules
	case "pacman":
		rules = pacmanRules
	case "apk":
		rules = apkRules
	case "portage":
		rules = portageRules
	default:
		return ProgressEvent{}, false
	}
	line = strings.TrimSpace(line)
	if backend == "apt" {
		if m := aptDownload.FindStringSubmatch(line); m != nil {
			total, _ := strconv.Atoi(m[1])
			item, _ := strconv.Atoi(m[3])
			if total <= 100 && item <= 100 {
				return ProgressEvent{Phase: "Downloading metadata or packages", Item: m[2], Percent: &item, Overall: &total}, true
			}
			return ProgressEvent{}, false
		}
	}
	for _, r := range rules {
		m := r.pattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		e := ProgressEvent{Phase: r.phase}
		// A 100% bar is not proof of success. Only explicit native
		// completion records can mark an individual phase complete.
		e.Done = backend == "apt" && (line == "Reading package lists... Done" || line == "Building dependency tree... Done" || line == "Reading state information... Done" || line == "Calculating upgrade... Done")
		if r.itemGroup != 0 {
			e.Item = m[r.itemGroup]
		}
		if r.percentGroup != 0 {
			n, err := strconv.Atoi(m[r.percentGroup])
			if err != nil || n < 0 || n > 100 {
				continue
			}
			e.Percent = &n
		}
		return e, true
	}
	return ProgressEvent{}, false
}

var sgr = regexp.MustCompile(`\x1b\[[0-9;]*m`)
var eraseLine = regexp.MustCompile(`\x1b\[(?:0|2)?K`)

// Renderer owns one inline row. Unknown output (including prompts) is
// passed through byte-for-byte after removing the EPA row. Full-screen or
// other cursor-control output disables rewriting for the rest of the run.
type Renderer struct {
	out                                      io.Writer
	backend, action                          string
	width                                    func() int
	color                                    bool
	plain, quiet                             bool
	event                                    ProgressEvent
	active, drawn, passthrough, continuation bool
	endedLine                                bool
	skipLF                                   bool
	aptAwaiting, aptDeclined, aptAborted     bool
	aptAnswer                                string
	spin                                     int
	pending                                  []byte
	lastProgress                             string
	lastPhase                                string
	lastBucket                               int
	started                                  time.Time
}

func NewRenderer(out io.Writer, backend string, width func() int, color, plain bool) *Renderer {
	return &Renderer{out: out, backend: backend, width: width, color: color, plain: plain,
		event: ProgressEvent{Phase: "Running package manager"}, active: true, endedLine: true,
		lastBucket: -1, started: time.Now()}
}

func (r *Renderer) Clear() {
	if r.drawn {
		fmt.Fprint(r.out, "\r\x1b[2K")
		r.drawn = false
	}
}

func (r *Renderer) Paint() {
	if r.quiet || !r.active || r.passthrough || !r.endedLine {
		return
	}
	if r.plain || r.width() < 8 {
		r.Clear()
		bucket := -1
		if r.event.Percent != nil {
			bucket = *r.event.Percent / 10
		}
		if r.lastPhase == r.event.Phase && r.lastProgress == r.event.Item && r.lastBucket == bucket {
			return
		}
		r.lastPhase, r.lastProgress, r.lastBucket = r.event.Phase, r.event.Item, bucket
		fmt.Fprintf(r.out, "[EPA] %s\r\n", r.text(false))
		return
	}
	frames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	mark := frames[r.spin%len(frames)]
	r.spin++
	if r.color {
		mark = "\x1b[36m" + mark + "\x1b[0m"
	}
	r.Clear()
	fmt.Fprintf(r.out, "\r%s %s", mark, r.text(true))
	r.drawn = true
}

// Translate presentation labels only. Keep parser rules, event identifiers,
// JSON values and unrecognized backend output unchanged.
func phaseLabel(phase string) string {
	if Language() == "en" {
		return phase
	}
	switch phase {
	case "Running package manager":
		return "패키지 관리자 실행 중"
	case "Reading metadata":
		return "저장소 정보 읽는 중"
	case "Resolving dependencies":
		return "의존성 확인 중"
	case "Reading installed state":
		return "설치 상태 확인 중"
	case "Calculating upgrade":
		return "업그레이드 계획 확인 중"
	case "Applying transaction":
		return "패키지 변경 적용 중"
	case "Downloading metadata or packages":
		return "저장소 정보 또는 패키지 다운로드 중"
	case "Checking conflicts":
		return "패키지 충돌 확인 중"
	case "Downloading packages":
		return "패키지 다운로드 중"
	case "Checking packages":
		return "패키지 검사 중"
	case "Checking transaction":
		return "패키지 변경 검사 중"
	case "Testing transaction":
		return "패키지 변경 사전 검사 중"
	default:
		return phase
	}
}

func itemLabel(event ProgressEvent) string {
	if Language() == "en" {
		return event.Item
	}
	if event.Phase == "Checking packages" {
		switch event.Item {
		case "checking keys in keyring":
			return "키링의 키 확인"
		case "checking package integrity":
			return "패키지 무결성 검사"
		case "loading package files":
			return "패키지 파일 불러오기"
		case "checking for file conflicts":
			return "파일 충돌 검사"
		case "checking available disk space":
			return "사용 가능한 디스크 공간 확인"
		}
	}
	return event.Item
}

func (r *Renderer) text(fit bool) string {
	text := phaseLabel(r.event.Phase)
	if r.event.Item != "" {
		text += ": " + itemLabel(r.event)
	}
	suffix := ""
	if r.event.Percent != nil {
		n := *r.event.Percent
		cells := 10
		if fit {
			cells = min(24, max(0, (r.width()-12)/3))
		}
		if cells > 0 {
			bar := n * cells / 100
			suffix = fmt.Sprintf(" [%s%s] %3d%%", strings.Repeat("=", bar), strings.Repeat(" ", cells-bar), n)
		} else {
			suffix = fmt.Sprintf(" %d%%", n)
		}
	}
	if r.event.Overall != nil {
		total := fmt.Sprintf(Text(" · 전체 %d%%", " · total %d%%"), *r.event.Overall)
		if !fit || displayWidth(suffix+total)+2 < r.width()-3 {
			suffix += total
		}
	}
	if fit {
		limit := r.width() - 3 // no last-column wrap
		if limit < 1 {
			limit = 1
		}
		if displayWidth(suffix) >= limit {
			suffix = ""
		}
		text = truncate(text, limit-displayWidth(suffix))
	}
	return text + suffix
}

// Korean syllables and common East Asian full-width characters occupy two
// terminal cells. Combining marks and format characters occupy no cells.
// Treat ambiguous-width punctuation as narrow, as most terminals do.
func cellWidth(r rune) int {
	if unicode.IsControl(r) || unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r) || unicode.Is(unicode.Cf, r) {
		return 0
	}
	if (r >= 0x1100 && r <= 0x115f) || r == 0x2329 || r == 0x232a ||
		(r >= 0x2e80 && r <= 0xa4cf && r != 0x303f) ||
		(r >= 0xac00 && r <= 0xd7a3) || (r >= 0xf900 && r <= 0xfaff) ||
		(r >= 0xfe10 && r <= 0xfe19) || (r >= 0xfe30 && r <= 0xfe6f) ||
		(r >= 0xff00 && r <= 0xff60) || (r >= 0xffe0 && r <= 0xffe6) ||
		(r >= 0x20000 && r <= 0x3fffd) {
		return 2
	}
	return 1
}

func displayWidth(s string) int {
	n := 0
	for _, r := range s {
		n += cellWidth(r)
	}
	return n
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if displayWidth(s) <= n {
		return s
	}
	width := 0
	for i, r := range s {
		width += cellWidth(r)
		if width > n-1 {
			return s[:i] + "…"
		}
	}
	return s
}

func (r *Renderer) native(data []byte) {
	if len(data) == 0 {
		return
	}
	if strings.TrimSpace(string(data)) != "" {
		r.aptDeclined, r.aptAborted = false, false
	}
	r.Clear()
	r.active = false
	r.out.Write(data)
	last := data[len(data)-1]
	// A bare CR is not a completed prompt or output line.
	r.endedLine = last == '\n'
}

func (r *Renderer) Consume(data []byte) {
	if r.passthrough {
		r.native(data)
		return
	}
	for len(data) > 0 {
		i := bytes.IndexAny(data, "\r\n")
		if i < 0 {
			r.pending = append(r.pending, data...)
			if len(r.pending) > 16384 {
				r.FlushPartial()
			}
			return
		}
		r.pending = append(r.pending, data[:i+1]...)
		data = data[i+1:]
		// If the native output emits CRLF, do not output an extra blank row
		// after a recognized progress record.
		r.record(r.pending)
		r.pending = r.pending[:0]
		if r.passthrough {
			r.native(data)
			return
		}
	}
}

func (r *Renderer) record(frame []byte) {
	if r.continuation {
		awaiting := r.aptAwaiting
		declined := awaiting && aptNegativeAnswer(r.aptAnswer+strings.TrimRight(string(frame), "\r\n"))
		r.aptAwaiting = false
		r.aptAnswer = ""
		r.native(frame)
		if awaiting {
			r.aptDeclined = declined
		}
		r.continuation = false
		return
	}
	if len(frame) == 1 && r.endedLine {
		// APT prefixes rewritten progress records with bare carriage
		// returns, even after completed lines. They must not turn an
		// empty row into a partial native prompt.
		if frame[0] == '\r' && r.backend == "apt" {
			return
		}
		if r.active || (frame[0] == '\n' && r.skipLF) {
			r.skipLF = false
			return
		}
	}
	r.skipLF = false
	clean := strings.TrimRight(string(frame), "\r\n")
	clean = sgr.ReplaceAllString(clean, "")
	clean = eraseLine.ReplaceAllString(clean, "")
	// Preserve native interactive/full-screen output rather than attempting
	// to emulate arbitrary terminal escape sequences.
	for _, c := range clean {
		if (c < 32 && c != '\t') || c == 127 {
			r.passthrough = true
			r.native(frame)
			return
		}
	}
	if r.backend == "apt" && !r.quiet && r.endedLine {
		if label, answer, ok := aptConfirmation(clean); ok {
			r.native([]byte(label + string(frame[len(strings.TrimRight(string(frame), "\r\n")):])))
			r.aptDeclined = aptNegativeAnswer(answer)
			return
		}
		if clean == "Abort." && r.aptDeclined {
			r.native([]byte(Text("작업 취소.", "Aborted.") + string(frame[len(strings.TrimRight(string(frame), "\r\n")):])))
			r.aptAborted = true
			return
		}
		if strings.TrimSpace(clean) != "" {
			r.aptDeclined, r.aptAborted = false, false
		}
		if label, ok := aptLineLabel(clean); ok {
			r.native([]byte(label + string(frame[len(strings.TrimRight(string(frame), "\r\n")):])))
			return
		}
	}
	e, ok := ParseProgress(r.backend, clean)
	if !ok {
		r.native(frame)
		return
	}
	if !r.endedLine {
		// A prompt or native partial line is still visible. Do not overwrite it.
		r.native(frame)
		return
	}
	r.event = e
	if r.quiet {
		r.active = true
		return
	}
	if e.Done {
		r.Clear()
		mark := "✓"
		if r.color {
			mark = "\x1b[32m" + mark + "\x1b[0m"
		}
		fmt.Fprintf(r.out, "%s %s\r\n", mark, phaseDoneLabel(e.Phase))
		r.skipLF = frame[len(frame)-1] == '\r'
		r.active = false
		return
	}
	r.active = true
	r.Paint()
}

// FlushPartial is called on an idle timer. This makes prompts without a
// trailing newline visible immediately and disables animation until a new
// complete progress record is seen.
func (r *Renderer) FlushPartial() {
	if len(r.pending) == 0 {
		return
	}
	if bytes.ContainsAny(r.pending, "\x1b\x08") {
		r.passthrough = true
	}
	if r.backend == "apt" && !r.quiet && !r.passthrough && !r.continuation && r.endedLine {
		if label, answer, ok := aptConfirmation(string(r.pending)); ok {
			r.native([]byte(label))
			r.aptAwaiting = true
			r.aptAnswer = answer
			r.aptDeclined, r.aptAborted = false, false
			r.pending = r.pending[:0]
			r.continuation = true
			return
		}
	}
	if r.aptAwaiting {
		if len(r.aptAnswer)+len(r.pending) <= 8 {
			r.aptAnswer += string(r.pending)
		} else {
			r.aptAwaiting = false
			r.aptAnswer = ""
		}
	}
	r.native(r.pending)
	r.pending = r.pending[:0]
	r.continuation = true
}

func (r *Renderer) Finish(code int) {
	r.FlushPartial()
	r.Clear()
	if r.quiet {
		return
	}
	// A full-screen native application owns terminal restoration. Do not
	// inject cursor operations while its state may still be active.
	if !r.endedLine || r.passthrough {
		fmt.Fprint(r.out, "\r\n")
	}
	mark, msg := "✓", Text("완료", "Completed")
	if r.action != "" {
		msg = fmt.Sprintf(Text("%s 완료", "%s completed"), actionLabel(r.action))
	}
	if code != 0 {
		mark, msg = "×", fmt.Sprintf(Text("실패 (종료 코드 %d, 마지막 확인 단계: %s)", "Failed (exit %d; last observed stage: %s)"), code, phaseLabel(r.event.Phase))
	}
	cancelled := code == 130 || (code == 1 && r.aptAborted)
	if cancelled {
		mark, msg = "-", fmt.Sprintf(Text("작업 취소 (종료 코드 %d)", "Cancelled (exit %d)"), code)
	}
	if r.color {
		c := "32"
		if cancelled {
			c = "33"
		} else if code != 0 {
			c = "31"
		}
		mark = "\x1b[" + c + "m" + mark + "\x1b[0m"
	}
	fmt.Fprintf(r.out, "%s %s · %s\r\n", mark, msg, time.Since(r.started).Round(100*time.Millisecond))
}

func actionLabel(action string) string {
	if Language() == "en" {
		return action
	}
	switch Action(action) {
	case Refresh:
		return "저장소 정보 갱신"
	case Install:
		return "패키지 설치"
	case Remove:
		return "패키지 제거"
	case Upgrade:
		return "패키지 업그레이드"
	case FullUpgrade:
		return "전체 업그레이드"
	case Search:
		return "패키지 검색"
	case Info:
		return "패키지 정보 조회"
	case List:
		return "패키지 목록 조회"
	case Outdated:
		return "업그레이드 후보 조회"
	case Autoremove:
		return "불필요한 패키지 제거"
	case Reinstall:
		return "패키지 재설치"
	case Clean:
		return "패키지 캐시 정리"
	default:
		return action
	}
}

func EmitLifecycle(out io.Writer, mode ProgressMode, state, action string, code int, elapsed time.Duration) {
	if mode == ProgressJSON {
		// Keep native output on stderr in JSON mode.
		e := struct {
			State     string `json:"state"`
			Action    string `json:"action"`
			ExitCode  int    `json:"exit_code"`
			ElapsedMS int64  `json:"elapsed_ms"`
		}{state, action, code, elapsed.Milliseconds()}
		_ = json.NewEncoder(out).Encode(e)
	} else if mode == ProgressPlain {
		if state == "started" {
			fmt.Fprintf(out, Text("[EPA] %s: 시작\n", "[EPA] %s: started\n"), actionLabel(action))
		} else {
			label := state
			if state == "completed" {
				label = Text("완료", "completed")
			} else if state == "failed" {
				label = Text("실패", "failed")
			} else if state == "skipped" {
				label = Text("건너뜀", "skipped")
			}
			fmt.Fprintf(out, Text("[EPA] %s: %s (종료 코드 %d, %s)\n", "[EPA] %s: %s (exit %d, %s)\n"), actionLabel(action), label, code, elapsed.Round(100*time.Millisecond))
		}
	}
}
