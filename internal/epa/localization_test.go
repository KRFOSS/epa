package epa

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"
)

func TestKoreanPhaseLabels(t *testing.T) {
	phases := map[string]bool{"Running package manager": true}
	for _, rules := range [][]eventRule{aptRules, dnfRules, pacmanRules} {
		for _, rule := range rules {
			phases[rule.phase] = true
		}
	}
	for phase := range phases {
		label := phaseLabel(phase)
		if label == phase || !strings.ContainsFunc(label, func(r rune) bool { return unicode.Is(unicode.Hangul, r) }) {
			t.Errorf("untranslated phase %q: %q", phase, label)
		}
	}
	event := ProgressEvent{Phase: "Checking packages", Item: "checking package integrity"}
	if got := itemLabel(event); got != "패키지 무결성 검사" {
		t.Fatalf("untranslated check: %q", got)
	}
}

func TestKoreanDisplayWidth(t *testing.T) {
	if got := displayWidth("한글 ABC"); got != 8 {
		t.Fatalf("width = %d; want 8", got)
	}
	for _, input := range []string{"한글ABC", "패키지 다운로드 중: openssl", "ＡＢ한글", "e\u0301한글"} {
		for n := 0; n <= 40; n++ {
			got := truncate(input, n)
			if !utf8.ValidString(got) || displayWidth(got) > n {
				t.Fatalf("truncate(%q, %d) = %q", input, n, got)
			}
		}
	}
	n, overall := 40, 62
	for width := 8; width <= 120; width++ {
		r := NewRenderer(&bytes.Buffer{}, "apt", func() int { return width }, false, false)
		r.event = ProgressEvent{Phase: "Downloading metadata or packages", Item: "openssl", Percent: &n, Overall: &overall}
		if got := r.text(true); !utf8.ValidString(got) || displayWidth(got) > width-3 {
			t.Fatalf("renderer width %d: %q (%d cells)", width, got, displayWidth(got))
		}
	}
}

func TestLanguageConfigAndCompletion(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := configPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(`{"style":"dnf","progress":"plain","self_update":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig()
	if err != nil || c.Language != "ko" || c.Style != "dnf" || c.SelfUpdate {
		t.Fatalf("legacy config: %+v, %v", c, err)
	}
	if err := SaveLanguage("en"); err != nil {
		t.Fatal(err)
	}
	if err := SaveStyle("apt"); err != nil {
		t.Fatal(err)
	}
	if err := SaveProgress("quiet"); err != nil {
		t.Fatal(err)
	}
	if err := SaveSelfUpdate(true); err != nil {
		t.Fatal(err)
	}
	if err := SaveLanguage("unknown"); err == nil {
		t.Fatal("unsupported language accepted")
	}
	c, err = LoadConfig()
	if err != nil || c.Language != "en" || c.Style != "apt" || c.Progress != "quiet" || !c.SelfUpdate {
		t.Fatalf("saved config: %+v, %v", c, err)
	}
	for _, words := range [][]string{{"--language", ""}, {"config", "set", "language", ""}} {
		if got := Complete("apt", words, Backend{}); !reflect.DeepEqual(got, []string{"en", "ko"}) {
			t.Errorf("completion %v: %v", words, got)
		}
	}
}

func TestEnglishPresentation(t *testing.T) {
	t.Cleanup(func() { SetLanguage("ko") })
	if !SetLanguage("en") || SetLanguage("unsupported") || Language() != "en" {
		t.Fatal("language selection failed")
	}
	if got := phaseLabel("Reading metadata"); got != "Reading metadata" {
		t.Fatalf("English phase: %q", got)
	}
	var out bytes.Buffer
	r := NewRenderer(&out, "apt", func() int { return 80 }, false, false)
	r.Consume([]byte("Reading package lists... 42%\r"))
	r.Finish(0)
	if !strings.Contains(out.String(), "Reading metadata") || !strings.Contains(out.String(), "Completed") {
		t.Fatalf("English renderer: %q", out.String())
	}
	EmitLifecycle(&out, ProgressPlain, "failed", "install", 100, time.Second)
	if !strings.Contains(out.String(), "install: failed (exit 100") {
		t.Fatalf("English lifecycle: %q", out.String())
	}
	cause := errors.New("example")
	err := LocalizedErrorf("실패: %w", "Failed: %w", cause)
	if !errors.Is(err, cause) || err.Error() != "Failed: example" {
		t.Fatalf("English wrapped error: %v", err)
	}
	_, err = Parse("apt", []string{"install"})
	if err == nil || !strings.Contains(err.Error(), "requires a target") {
		t.Fatalf("English operation error: %v", err)
	}
}

func TestLocalizedLifecycleKeepsJSON(t *testing.T) {
	var plain, structured bytes.Buffer
	EmitLifecycle(&plain, ProgressPlain, "started", "refresh", 0, 0)
	EmitLifecycle(&plain, ProgressPlain, "completed", "refresh", 0, time.Second)
	EmitLifecycle(&plain, ProgressPlain, "failed", "install", 100, time.Second)
	for _, want := range []string{"저장소 정보 갱신: 시작", "저장소 정보 갱신: 완료", "패키지 설치: 실패 (종료 코드 100"} {
		if !strings.Contains(plain.String(), want) {
			t.Errorf("missing %q in %q", want, plain.String())
		}
	}
	EmitLifecycle(&structured, ProgressJSON, "failed", "install", 100, time.Second)
	var v map[string]any
	if err := json.Unmarshal(structured.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v["state"] != "failed" || v["action"] != "install" || v["exit_code"] != float64(100) || v["elapsed_ms"] != float64(1000) {
		t.Fatalf("changed JSON output: %s", structured.String())
	}
}
