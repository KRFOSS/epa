package main

import (
	"os"
	"strings"
	"testing"

	"github.com/KRFOSS/epa/internal/epa"
)

func TestKoreanHelp(t *testing.T) {
	for _, want := range []string{"사용법: epa", "기본 APT 스타일 명령:", "EPA 공통 명령", "패키지 설치", "EPA 자체 업데이트", "--no-self-update", "--language ko|en"} {
		if !strings.Contains(helpKO, want) {
			t.Errorf("help is missing %q", want)
		}
	}
}

func TestKoreanCLIError(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	code, err := run([]string{"--style", "unknown"})
	if code != 2 || err == nil || !strings.Contains(err.Error(), "지정해야 합니다") {
		t.Fatalf("unexpected error: code %d, %v", code, err)
	}
}

func TestCLILanguageSelection(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("SUDO_USER", "")
	t.Setenv("LC_ALL", "C")
	t.Cleanup(func() { epa.SetLanguage("ko") })
	check := func(want string, args ...string) {
		t.Helper()
		f, err := os.CreateTemp(t.TempDir(), "stdout-")
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		original := os.Stdout
		defer func() { os.Stdout = original }()
		os.Stdout = f
		code, err := run(args)
		os.Stdout = original
		if code != 0 || err != nil {
			t.Fatalf("run(%v): %d, %v", args, code, err)
		}
		data, err := os.ReadFile(f.Name())
		if err != nil || !strings.Contains(string(data), want) {
			t.Fatalf("run(%v): %q, %v", args, data, err)
		}
	}
	check("사용법: epa", "--help")
	check("ko\n", "config", "get", "language")
	check("Language: en", "config", "set", "language", "en")
	check("Usage: epa", "--help")
	check("사용법: epa", "--language", "ko", "--help")
	check("en\n", "config", "get", "language")
	code, err := run([]string{"--style", "unknown"})
	if code != 2 || err == nil || !strings.Contains(err.Error(), "--style requires") {
		t.Fatalf("English error: %d, %v", code, err)
	}
	check("출력 언어: ko", "config", "set", "language", "ko")
	check("사용법: epa")
}

func TestPacmanUpgradeSummary(t *testing.T) {
	defer epa.SetLanguage("ko")
	for _, test := range []struct {
		lang          string
		count         int
		want, missing string
	}{
		{"ko", 0, "갱신한 저장소 기준 업그레이드 후보가 없습니다", "list --upgradable"},
		{"ko", 3, "업그레이드 후보 3개", ""},
		{"en", 0, "no upgrade candidates", "list --upgradable"},
		{"en", 3, "3 upgrade candidate(s)", ""},
	} {
		epa.SetLanguage(test.lang)
		got := pacmanUpgradeSummary(test.count)
		if !strings.Contains(got, test.want) || test.missing != "" && strings.Contains(got, test.missing) ||
			test.count > 0 && !strings.Contains(got, "epa --style apt list --upgradable") {
			t.Errorf("%s, %d: unexpected summary %q", test.lang, test.count, got)
		}
	}
}
