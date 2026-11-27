package epa

import (
	"bytes"
	"strings"
	"testing"
)

func TestAPTDownloadScopes(t *testing.T) {
	const line = "62% [1 openssl 40 kB/100 kB 40%]"
	e, ok := ParseProgress("apt", line)
	if !ok || e.Item != "openssl" || e.Percent == nil || *e.Percent != 40 || e.Overall == nil || *e.Overall != 62 || e.Done {
		t.Fatalf("bad progress record: %+v", e)
	}
	for _, invalid := range []string{"162% [1 openssl 40 kB/100 kB 40%]", "62% [1 openssl 40 kB/100 kB 140%]"} {
		if _, ok := ParseProgress("apt", invalid); ok {
			t.Fatalf("accepted %q", invalid)
		}
	}
	var out bytes.Buffer
	r := NewRenderer(&out, "apt", func() int { return 80 }, false, false)
	r.Consume([]byte(line + "\r"))
	if !strings.Contains(out.String(), "40% · 전체 62%") {
		t.Fatal(out.String())
	}
}

func TestQuietRendererPreservesNativeMessages(t *testing.T) {
	var out bytes.Buffer
	r := NewRenderer(&out, "apt", func() int { return 80 }, false, false)
	r.quiet = true
	r.Paint()
	r.Consume([]byte("Reading package lists... 42%\r\nReplace it? [Y/n] "))
	r.FlushPartial()
	r.Consume([]byte("n\r\nE: signature verification failed\r\n"))
	r.Finish(100)
	if got, want := out.String(), "Replace it? [Y/n] n\r\nE: signature verification failed\r\n"; got != want {
		t.Fatalf("got %q; want %q", got, want)
	}
}

func FuzzParseProgress(f *testing.F) {
	f.Add("apt", "62% [1 openssl 40 kB/100 kB 40%]")
	f.Add("apt", "Replace it? [Y/n]")
	f.Add("pacman", "(1/3) upgrading nginx [#####     ] 50%")
	f.Add("dnf", "Running transaction")
	f.Fuzz(func(t *testing.T, backend, line string) {
		e, ok := ParseProgress(backend, line)
		if !ok {
			return
		}
		if e.Phase == "" {
			t.Fatal("empty phase")
		}
		for _, n := range []*int{e.Percent, e.Overall} {
			if n != nil && (*n < 0 || *n > 100) {
				t.Fatalf("invalid percentage %d", *n)
			}
		}
	})
}

func TestAPTUpgradePresentationAndCancellation(t *testing.T) {
	SetLanguage("ko")
	var out bytes.Buffer
	r := NewRenderer(&out, "apt", func() int { return 100 }, false, false)
	r.Consume([]byte("Hit:1 https://mirror.example.test stable InRelease\r\n\rReading package lists... 0%\r\rReading package lists... Done\r\r\n\rBuilding dependency tree... Done\r\r\n\rReading state information... Done\r\r\n\rCalculating upgrade... Done\r\r\n"))
	r.Consume([]byte("The following packages will be upgraded:\r\n  nginx openssl\r\n2 upgraded, 0 newly installed, 0 to remove and 0 not upgraded.\r\nNeed to get 200 kB/18.9 MB of archives.\r\nAfter this operation, 20.5 kB of additional disk space will be used.\r\nDo you want to continue? [Y/n] "))
	r.FlushPartial()
	r.Consume([]byte("n"))
	r.FlushPartial()
	r.Consume([]byte("\r\nAbort.\r\n"))
	r.Finish(1)
	got := out.String()
	for _, want := range []string{"[최신 상태] https://mirror.example.test stable InRelease", "저장소 정보 확인 완료", "의존성 확인 완료", "설치 상태 확인 완료", "업그레이드 계획 확인 완료", "업그레이드할 패키지:", "nginx openssl", "업그레이드 2개 · 신규 설치 0개 · 제거 0개 · 보류 0개", "다운로드할 용량: 200 kB/18.9 MB", "추가 디스크 사용량: 20.5 kB", "계속하시겠습니까? [Y/n] n", "작업 취소 (종료 코드 1)"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	for _, raw := range []string{"Reading package lists...", "Building dependency tree...", "Reading state information...", "Calculating upgrade...", "Do you want to continue?", "실패 (종료 코드 1"} {
		if strings.Contains(got, raw) {
			t.Errorf("unexpected %q in %q", raw, got)
		}
	}
}

func TestAPTAbortDoesNotHideRealError(t *testing.T) {
	for _, scenario := range []struct {
		name, output string
		code         int
	}{
		{"unrelated abort", "Abort.\r\n", 1},
		{"affirmative answer", "Do you want to continue? [Y/n] y\r\nAbort.\r\n", 1},
		{"error after refusal", "Do you want to continue? [Y/n] n\r\nAbort.\r\nE: actual failure\r\n", 1},
		{"non-cancellation status", "Do you want to continue? [Y/n] n\r\nAbort.\r\nE: actual failure\r\n", 100},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var out bytes.Buffer
			r := NewRenderer(&out, "apt", func() int { return 80 }, false, false)
			r.Consume([]byte(scenario.output))
			r.Finish(scenario.code)
			if !strings.Contains(out.String(), "실패 (종료 코드") || strings.Contains(out.String(), "작업 취소 (종료 코드") {
				t.Fatal(out.String())
			}
			if strings.Contains(scenario.output, "E: actual failure") && !strings.Contains(out.String(), "E: actual failure") {
				t.Fatal("error hidden")
			}
		})
	}
}

func TestAPTEnglishPresentation(t *testing.T) {
	SetLanguage("en")
	defer SetLanguage("ko")
	var out bytes.Buffer
	r := NewRenderer(&out, "apt", func() int { return 80 }, false, false)
	r.Consume([]byte("Reading package lists... Done\r\nDo you want to continue? [Y/n] n\r\nAbort.\r\n"))
	r.Finish(1)
	for _, want := range []string{"Repository metadata read", "Continue? [Y/n] n", "Cancelled (exit 1)"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in %q", want, out.String())
		}
	}
}
