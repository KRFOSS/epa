package epa

import (
	"bytes"
	"strings"
	"testing"
)

func TestProgressParsing(t *testing.T) {
	cases := []struct {
		backend, line, phase string
		percent              int
	}{
		{"apt", "Reading package lists... 42%", "Reading metadata", 42},
		{"apt", "Progress: [ 50%] [#####     ]", "Applying transaction", 50},
		{"apt", "65% [Working]", "Downloading metadata or packages", 65},
		{"pacman", "(1/3) upgrading nginx [#####     ] 50%", "Applying transaction", 50},
		{"pacman", "nginx  1.2 MiB  2.0 MiB/s 00:00 [####  ] 66%", "Downloading packages", 66},
		{"dnf", "(1/3): nginx-1.2.rpm  52% [====  ] 1 MB/s | 2 MB 00:02 ETA", "Downloading packages", 52},
		{"dnf", "Running transaction test", "Testing transaction", -1},
	}
	for _, tt := range cases {
		e, ok := ParseProgress(tt.backend, tt.line)
		if !ok || e.Phase != tt.phase || (tt.percent >= 0 && (e.Percent == nil || *e.Percent != tt.percent)) {
			t.Errorf("%s %q => %+v %t", tt.backend, tt.line, e, ok)
		}
	}
	for _, line := range []string{"Reading package lists... 150%", "Progress: [ 50%] [####] Error: signature invalid", "Do you want to continue? [Y/n]", "Error: package signature check failed (100%)"} {
		if _, ok := ParseProgress("apt", line); ok {
			t.Errorf("accepted unsafe record %q", line)
		}
	}
}

func TestRendererPreservesPromptAndError(t *testing.T) {
	var out bytes.Buffer
	r := NewRenderer(&out, "apt", func() int { return 80 }, false, false)
	r.Paint()
	r.Consume([]byte("\rReading package lists... 42%\r"))
	r.Consume([]byte("Configuration file '/etc/example'\r\nReplace it? [Y/n] "))
	r.FlushPartial()
	before := out.String()
	r.Paint()
	if out.String() != before {
		t.Fatal("paint overwrote pending prompt")
	}
	r.Consume([]byte("n\r\nE: signature verification failed\r\n"))
	r.Finish(100)
	s := out.String()
	for _, part := range []string{"저장소 정보 읽는 중", "42%", "Replace it? [Y/n] ", "signature verification failed", "종료 코드 100"} {
		if !strings.Contains(s, part) {
			t.Errorf("missing %q in %q", part, s)
		}
	}
}
func TestRendererUnknownControl(t *testing.T) {
	var out bytes.Buffer
	r := NewRenderer(&out, "apt", func() int { return 40 }, false, false)
	r.Consume([]byte("\x1b[?1049h\x1b[Hdialog\r\n"))
	r.Consume([]byte("Choose an option: "))
	r.FlushPartial()
	before := out.String()
	r.Paint()
	if out.String() != before || !strings.Contains(before, "\x1b[?1049h\x1b[Hdialog") || !strings.Contains(before, "Choose an option: ") {
		t.Fatalf("native fallback failed: %q", before)
	}
}
func TestProgressConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := SaveProgress("plain"); err != nil {
		t.Fatal(err)
	}
	if err := SaveStyle("pacman"); err != nil {
		t.Fatal(err)
	}
	if err := ResetStyle(); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig()
	if err != nil || c.Style != "apt" || c.Progress != "plain" {
		t.Fatalf("%+v %v", c, err)
	}
}
