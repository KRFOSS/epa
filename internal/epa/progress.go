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
	"unicode/utf8"
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
)

var ProgressModes = []string{"auto", "tty", "plain", "json", "native"}

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

var aptRules = []eventRule{
	rule(`^Reading package lists\.\.\.\s*([0-9]{1,3})%$`, "Reading metadata", 0, 1),
	rule(`^Reading package lists\.\.\.\s*(Done)?$`, "Reading metadata", 0, 0),
	rule(`^Building dependency tree\.\.\.\s*([0-9]{1,3})%$`, "Resolving dependencies", 0, 1),
	rule(`^Building dependency tree\.\.\.\s*(Done)?$`, "Resolving dependencies", 0, 0),
	rule(`^Reading state information\.\.\.\s*(Done)?$`, "Reading installed state", 0, 0),
	rule(`^Progress:\s*\[\s*([0-9]{1,3})%\s*\]\s*\[[#. =-]*\]$`, "Applying transaction", 0, 1),
	rule(`^([0-9]{1,3})%\s*\[Working\]$`, "Downloading metadata or packages", 0, 1),
	rule(`^([0-9]{1,3})%\s*\[[0-9]+\s+([A-Za-z0-9.+_:-]+)\s+[0-9.,]+\s*[kMGT]?B(?:/[0-9.,]+\s*[kMGT]?B)?\s+[0-9]{1,3}%\]$`, "Downloading metadata or packages", 2, 1),
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
	default:
		return ProgressEvent{}, false
	}
	line = strings.TrimSpace(line)
	for _, r := range rules {
		m := r.pattern.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		e := ProgressEvent{Phase: r.phase}
		// A 100% bar is not proof of success. Only explicit native
		// completion records can mark an individual phase complete.
		e.Done = backend == "apt" && (line == "Reading package lists... Done" || line == "Building dependency tree... Done" || line == "Reading state information... Done")
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
	backend                                  string
	width                                    func() int
	color                                    bool
	plain                                    bool
	event                                    ProgressEvent
	active, drawn, passthrough, continuation bool
	endedLine                                bool
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
	if !r.active || r.passthrough || !r.endedLine {
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

func (r *Renderer) text(fit bool) string {
	text := r.event.Phase
	if r.event.Item != "" {
		text += ": " + r.event.Item
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
	if fit {
		limit := r.width() - 3 // no last-column wrap
		if limit < 1 {
			limit = 1
		}
		if utf8.RuneCountInString(suffix) >= limit {
			suffix = ""
		}
		text = truncate(text, limit-utf8.RuneCountInString(suffix))
	}
	return text + suffix
}

func truncate(s string, n int) string {
	if n <= 0 {
		return ""
	}
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	if n == 1 {
		return string(rs[:1])
	}
	return string(rs[:n-1]) + "…"
}

func (r *Renderer) native(data []byte) {
	if len(data) == 0 {
		return
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
		r.native(frame)
		r.continuation = false
		return
	}
	if len(frame) == 1 && (frame[0] == '\r' || frame[0] == '\n') && r.active && r.endedLine {
		return
	}
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
	if e.Done {
		r.Clear()
		mark := "✓"
		if r.color {
			mark = "\x1b[32m" + mark + "\x1b[0m"
		}
		fmt.Fprintf(r.out, "%s %s\r\n", mark, e.Phase)
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
	r.native(r.pending)
	r.pending = r.pending[:0]
	r.continuation = true
}

func (r *Renderer) Finish(code int) {
	r.FlushPartial()
	r.Clear()
	// A full-screen native application owns terminal restoration. Do not
	// inject cursor operations while its state may still be active.
	if !r.endedLine || r.passthrough {
		fmt.Fprint(r.out, "\r\n")
	}
	mark, msg := "✓", "Completed"
	if code != 0 {
		mark, msg = "×", fmt.Sprintf("Failed (exit %d; last observed stage: %s)", code, r.event.Phase)
	}
	if r.color {
		c := "32"
		if code != 0 {
			c = "31"
		}
		mark = "\x1b[" + c + "m" + mark + "\x1b[0m"
	}
	fmt.Fprintf(r.out, "%s %s · %s\r\n", mark, msg, time.Since(r.started).Round(100*time.Millisecond))
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
			fmt.Fprintf(out, "[EPA] %s: started\n", action)
		} else {
			fmt.Fprintf(out, "[EPA] %s: %s (exit %d, %s)\n", action, state, code, elapsed.Round(100*time.Millisecond))
		}
	}
}
