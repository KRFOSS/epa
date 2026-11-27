package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/KRFOSS/epa/internal/epa"
)

var version = "0.1.0-dev"

const helpKO = `EPA - Everyone's Package Adapter

사용법: epa [--language ko|en] [--style apt|dnf|pacman] [--progress auto|tty|plain|json|native|quiet] [--dry-run] [--no-self-update] 명령 [인수]

기본 APT 스타일 명령:
  update           저장소 정보 갱신
  upgrade          설치된 패키지 업그레이드
  full-upgrade     전체 업그레이드
  install          패키지 설치
  remove           패키지 제거
  search           패키지 검색
  show             패키지 정보 조회
  list             설치된 패키지 목록 조회
  outdated         업그레이드 가능한 패키지 조회
  autoremove       불필요한 의존 패키지 제거
  reinstall        패키지 재설치
  clean            패키지 캐시 정리

EPA 공통 명령 (명령 스타일과 무관):
  style list|current|set NAME|reset  명령 스타일 조회 및 설정
  backend info                      현재 백엔드 확인
  config get progress | set progress MODE
                                    진행 표시 방식 조회 및 설정
  config get self-update | set self-update on|off
                                    자동 자체 업데이트 조회 및 설정
  config get language | set language ko|en
                                    출력 언어 조회 및 설정
  self-update                       EPA 자체 업데이트
  doctor                            환경 진단
  completion bash|zsh               셸 자동완성 스크립트 출력
  completion install [bash|zsh|all] 셸 자동완성 영구 등록
  --help, --version                 도움말 및 버전 확인

공통 옵션:
  --language       이번 실행의 출력 언어 지정 (기본값: ko)
  --style          사용할 명령 스타일 지정
  --progress       진행 표시 방식 지정
  --dry-run        명령을 실행하지 않고 백엔드 명령 확인
  --no-self-update 이번 실행에서 자동 자체 업데이트 생략

옵션은 선택한 명령 스타일에 따라 달라집니다.
지원하지 않거나 안전하게 변환할 수 없는 작업은 실행하지 않습니다.
`

const helpEN = `EPA - Everyone's Package Adapter

Usage: epa [--language ko|en] [--style apt|dnf|pacman] [--progress auto|tty|plain|json|native|quiet] [--dry-run] [--no-self-update] COMMAND [ARGS]

Default APT-style commands:
  update           Refresh repository metadata
  upgrade          Upgrade installed packages
  full-upgrade     Perform a full upgrade
  install          Install packages
  remove           Remove packages
  search           Search for packages
  show             Show package information
  list             List installed packages
  outdated         List upgradable packages
  autoremove       Remove unnecessary dependencies
  reinstall        Reinstall packages
  clean            Clean package caches

EPA commands (independent of style):
  style list|current|set NAME|reset  Query or set command style
  backend info                      Show the active backend
  config get progress | set progress MODE
                                    Query or set progress mode
  config get self-update | set self-update on|off
                                    Query or set automatic self-update
  config get language | set language ko|en
                                    Query or set output language
  self-update                       Update EPA itself
  doctor                            Diagnose the environment
  completion bash|zsh               Print shell completion script
  completion install [bash|zsh|all] Install persistent shell completion
  --help, --version                 Show help or version

Common options:
  --language       Override output language for this invocation (default: ko)
  --style          Select a command style
  --progress       Select a progress mode
  --dry-run        Show the native command without running it
  --no-self-update Skip automatic self-update for this invocation

Options are style-specific.
Unsupported or unsafe operations are not executed.
`

func main() {
	code, err := run(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "epa:", err)
	}
	os.Exit(code)
}

func run(args []string) (int, error) {
	if len(args) > 0 && args[0] == "__complete" {
		words := args[1:]
		if len(words) > 0 && words[0] == "--" {
			words = words[1:]
		}
		style, err := epa.LoadStyle()
		if err != nil {
			return 0, nil
		}
		b, _ := epa.Detect()
		for _, v := range epa.Complete(style, words, b) {
			fmt.Println(v)
		}
		return 0, nil
	}
	// Korean is the default, even in the C locale. Do not consume a
	// configuration belonging to a different account under sudo.
	viaSudo := os.Geteuid() == 0 && os.Getenv("SUDO_USER") != ""
	epa.SetLanguage("ko")
	if !viaSudo {
		if c, err := epa.LoadConfig(); err == nil {
			epa.SetLanguage(c.Language)
		}
	}
	languageOverride := false
	style := ""
	progress := ""
	dry := false
	noSelfUpdate := false
	for len(args) > 0 {
		switch args[0] {
		case "--language":
			if len(args) < 2 || !epa.SetLanguage(args[1]) {
				return 2, epa.LocalizedErrorf("--language에 ko 또는 en을 지정해야 합니다.", "--language requires ko or en")
			}
			languageOverride = true
			args = args[2:]
		case "--style":
			if len(args) < 2 || !epa.ValidStyle(args[1]) {
				return 2, epa.LocalizedErrorf("--style에 apt, dnf 또는 pacman을 지정해야 합니다.", "--style requires apt, dnf or pacman")
			}
			style = args[1]
			args = args[2:]
		case "--progress":
			if len(args) < 2 || !epa.ValidProgressMode(args[1]) {
				return 2, epa.LocalizedErrorf("--progress에 auto, tty, plain, json, native 또는 quiet를 지정해야 합니다.", "--progress requires auto, tty, plain, json, native or quiet")
			}
			progress = args[1]
			args = args[2:]
		case "--no-self-update":
			noSelfUpdate = true
			args = args[1:]
		case "--dry-run":
			dry = true
			args = args[1:]
		case "--version":
			if len(args) != 1 {
				return 2, epa.LocalizedErrorf("--version 뒤에는 인수를 지정할 수 없습니다.", "unexpected arguments after --version")
			}
			fmt.Println("epa", version)
			return 0, nil
		case "--help", "-h":
			fmt.Print(epa.Text(helpKO, helpEN))
			return 0, nil
		default:
			goto parsed
		}
	}
parsed:
	if len(args) == 0 {
		fmt.Print(epa.Text(helpKO, helpEN))
		return 0, nil
	}
	switch args[0] {
	case "self-update":
		if len(args) != 1 || dry || noSelfUpdate {
			return 2, epa.LocalizedErrorf("사용법: epa self-update (--dry-run 또는 --no-self-update와 함께 사용할 수 없습니다.)", "usage: epa self-update (not with --dry-run or --no-self-update)")
		}
		return performSelfUpdate(false, false)
	case "config":
		if viaSudo {
			return 2, epa.LocalizedErrorf("sudo를 사용하지 않고 설정을 관리하십시오.", "manage configuration without sudo")
		}
		if len(args) == 3 && args[1] == "get" && args[2] == "language" {
			c, err := epa.LoadConfig()
			if err != nil {
				return 1, err
			}
			fmt.Println(c.Language)
			return 0, nil
		}
		if len(args) == 4 && args[1] == "set" && args[2] == "language" && !dry {
			if err := epa.SaveLanguage(args[3]); err != nil {
				return 2, err
			}
			if !languageOverride {
				epa.SetLanguage(args[3])
			}
			fmt.Println(epa.Text("출력 언어:", "Language:"), args[3])
			return 0, nil
		}
		if len(args) == 3 && args[1] == "get" && args[2] == "self-update" {
			c, err := epa.LoadConfig()
			if err != nil {
				return 1, err
			}
			if c.SelfUpdate {
				fmt.Println("on")
			} else {
				fmt.Println("off")
			}
			return 0, nil
		}
		if len(args) == 4 && args[1] == "set" && args[2] == "self-update" && !dry {
			if args[3] != "on" && args[3] != "off" {
				return 2, epa.LocalizedErrorf("self-update에 on 또는 off를 지정해야 합니다.", "self-update must be on or off")
			}
			if err := epa.SaveSelfUpdate(args[3] == "on"); err != nil {
				return 1, err
			}
			fmt.Println(epa.Text("자체 업데이트:", "Self-update:"), args[3])
			return 0, nil
		}
		if len(args) == 3 && args[1] == "get" && args[2] == "progress" {
			c, err := epa.LoadConfig()
			if err != nil {
				return 1, err
			}
			fmt.Println(c.Progress)
			return 0, nil
		}
		if len(args) == 4 && args[1] == "set" && args[2] == "progress" && !dry {
			if err := epa.SaveProgress(args[3]); err != nil {
				return 2, err
			}
			fmt.Println(epa.Text("진행 표시 방식:", "Progress:"), args[3])
			return 0, nil
		}
		return 2, epa.LocalizedErrorf("사용법: epa config get progress|self-update|language | set progress MODE | set self-update on|off | set language ko|en", "usage: epa config get progress|self-update|language | set progress MODE | set self-update on|off | set language ko|en")
	case "style":
		if len(args) < 2 {
			return 2, epa.LocalizedErrorf("사용법: epa style list|current|set NAME|reset", "usage: epa style list|current|set NAME|reset")
		}
		switch args[1] {
		case "list":
			if len(args) != 2 {
				return 2, epa.LocalizedErrorf("지원하지 않는 추가 인수가 있습니다.", "unexpected arguments")
			}
			fmt.Println(strings.Join(epa.Styles, "\n"))
			return 0, nil
		case "current":
			if len(args) != 2 {
				return 2, epa.LocalizedErrorf("지원하지 않는 추가 인수가 있습니다.", "unexpected arguments")
			}
			if viaSudo {
				return 2, epa.LocalizedErrorf("원래 사용자 계정에서 sudo 없이 명령 스타일을 조회하십시오.", "query the style from the original account without sudo")
			}
			s, err := epa.LoadStyle()
			if err != nil {
				return 1, err
			}
			fmt.Println(s)
			return 0, nil
		case "set", "reset":
			if viaSudo {
				return 2, epa.LocalizedErrorf("EPA를 사용할 계정에서 sudo 없이 명령 스타일을 변경하십시오.", "change the style without sudo, in the account that will use EPA")
			}
			if dry {
				return 2, epa.LocalizedErrorf("명령 스타일 변경과 --dry-run을 함께 사용할 수 없습니다.", "style changes cannot be combined with --dry-run")
			}
			if args[1] == "set" && len(args) == 3 {
				if err := epa.SaveStyle(args[2]); err != nil {
					return 2, err
				}
				fmt.Println(epa.Text("명령 스타일:", "Style:"), args[2])
				return 0, nil
			}
			if args[1] == "reset" && len(args) == 2 {
				if err := epa.ResetStyle(); err != nil {
					return 1, err
				}
				fmt.Println(epa.Text("명령 스타일: apt", "Style: apt"))
				return 0, nil
			}
		}
		return 2, epa.LocalizedErrorf("잘못된 style 명령입니다.", "invalid style command")
	case "completion":
		if len(args) >= 2 && args[1] == "install" {
			if dry || len(args) > 3 {
				return 2, epa.LocalizedErrorf("사용법: epa completion install [bash|zsh|all] (--dry-run 사용 불가)", "usage: epa completion install [bash|zsh|all] (not with --dry-run)")
			}
			shell := "all"
			if len(args) == 3 {
				shell = args[2]
			}
			installed, err := epa.InstallCompletion(shell)
			for _, s := range installed {
				fmt.Println(epa.Text("자동완성 영구 등록:", "Persistent completion installed:"), s)
			}
			if len(installed) > 0 {
				fmt.Println(epa.Text("새 대화형 셸부터 자동완성이 적용됩니다.", "Completion will be enabled in new interactive shells."))
			}
			if err != nil {
				return 1, err
			}
			if len(installed) == 0 {
				fmt.Println(epa.Text("설치된 Bash 또는 Zsh를 찾지 못했습니다.", "No installed Bash or Zsh found."))
			}
			return 0, nil
		}
		if len(args) != 2 {
			return 2, epa.LocalizedErrorf("사용법: epa completion bash|zsh 또는 epa completion install [bash|zsh|all]", "usage: epa completion bash|zsh or epa completion install [bash|zsh|all]")
		}
		s, ok := epa.CompletionScript(args[1])
		if !ok {
			return 2, epa.LocalizedErrorf("지원하지 않는 셸입니다: %q", "unsupported shell %q", args[1])
		}
		fmt.Print(s)
		return 0, nil
	case "backend", "doctor":
		if (args[0] == "backend" && (len(args) != 2 || args[1] != "info")) || (args[0] == "doctor" && len(args) != 1) {
			return 2, epa.LocalizedErrorf("사용법: epa backend info 또는 epa doctor", "usage: epa backend info or epa doctor")
		}
		b, err := epa.Detect()
		if err != nil {
			return 1, err
		}
		fmt.Printf(epa.Text("백엔드: %s (%s)\n", "Backend: %s (%s)\n"), b.Name, b.Binary)
		if args[0] == "doctor" && !viaSudo {
			s, err := epa.LoadStyle()
			if err != nil {
				return 1, err
			}
			fmt.Println(epa.Text("명령 스타일:", "Style:"), s)
		}
		return 0, nil
	}
	if style == "" {
		if viaSudo {
			return 2, epa.LocalizedErrorf("sudo epa를 실행할 때는 명령의 의미가 달라지지 않도록 --style을 지정해야 합니다. 일반적으로 epa를 직접 실행하십시오.", "sudo epa requires --style to avoid changing command meanings; normally run epa directly")
		}
		var err error
		style, err = epa.LoadStyle()
		if err != nil {
			return 1, err
		}
	}
	if progress == "" {
		progress = "auto"
		if !viaSudo {
			c, err := epa.LoadConfig()
			if err != nil {
				return 1, err
			}
			progress = c.Progress
		}
	}
	op, err := epa.Parse(style, args)
	if err != nil {
		return 2, err
	}
	b, err := epa.Detect()
	if err != nil {
		return 1, err
	}
	plan, err := b.Build(op)
	if err != nil {
		return 2, err
	}
	if plan.Note != "" {
		fmt.Fprintln(os.Stderr, "epa:", plan.Note)
	}
	native, argv, err := invocation(plan.Binary, plan.Args, plan.Privileged, dry)
	if err != nil {
		return 1, err
	}
	mode := epa.ProgressMode(progress)
	interactive := epa.IsTerminal(os.Stdin) && epa.IsTerminal(os.Stdout) && epa.IsTerminal(os.Stderr) && os.Getenv("TERM") != "dumb"
	if mode == epa.ProgressAuto {
		if interactive && op.Mutates() {
			mode = epa.ProgressTTY
		} else if op.Mutates() {
			mode = epa.ProgressPlain
		} else {
			mode = epa.ProgressNative
		}
	}
	if mode == epa.ProgressQuiet && !op.Mutates() {
		mode = epa.ProgressNative
	}
	if mode == epa.ProgressTTY && !interactive {
		mode = epa.ProgressPlain
	}
	if plan.RefreshBefore {
		prep, prepArgs, err := invocation(b.Binary, plan.RefreshArgs, true, dry)
		if err != nil {
			return 1, err
		}
		if dry {
			printPlan(prep, prepArgs)
		} else {
			code, err := runCommand(prep, prepArgs, b.Name, "refresh", mode, interactive)
			if code != 0 || err != nil {
				return code, err
			}
		}
	}
	if dry {
		printPlan(native, argv)
		return 0, nil
	}
	// Decide only after the dry-run and refresh-before path. A failed
	// preliminary refresh must not start a self-update.
	autoUpdate := false
	if epa.ShouldAutoUpdate(op.Action, interactive, noSelfUpdate || viaSudo || mode == epa.ProgressJSON, dry) && epa.SelfUpdateEligible(version) {
		if c, cfgErr := epa.LoadConfig(); cfgErr == nil {
			autoUpdate = c.SelfUpdate
		}
	}
	if mode == epa.ProgressNative && !autoUpdate {
		// Native mode keeps original streams, signals and exit status.
		err = syscall.Exec(native, append([]string{native}, argv...), os.Environ())
		return 1, err
	}
	code, err := runCommand(native, argv, b.Name, string(op.Action), mode, interactive)
	if code == 0 && err == nil && autoUpdate {
		// An optional EPA download must never turn a successful package
		// manager operation into a failure or overwrite its exit code.
		_, updateErr := performSelfUpdate(true, mode == epa.ProgressQuiet)
		if updateErr != nil {
			fmt.Fprintln(os.Stderr, epa.Text("epa: 자체 업데이트를 건너뜁니다:", "epa: self-update skipped:"), updateErr)
		}
	}
	return code, err
}

func performSelfUpdate(automatic, quiet bool) (int, error) {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer cancel()
	if !quiet {
		fmt.Fprintln(os.Stderr, epa.Text("EPA: 새로운 정식 릴리스를 확인하는 중...", "EPA: checking for a new release..."))
	}
	result, err := epa.SelfUpdate(ctx, version)
	if err != nil {
		return 1, err
	}
	if result.Updated {
		fmt.Fprintf(os.Stderr, epa.Text("EPA: %s -> %s 업데이트 완료. 다음 실행부터 새 버전을 사용합니다.\n", "EPA: %s -> %s; the new version will run next time.\n"), version, result.Version)
	} else if !automatic || !quiet {
		fmt.Fprintln(os.Stderr, epa.Text("EPA: 설치된 버전보다 새로운 정식 릴리스가 없습니다.", "EPA: no newer stable release."))
	}
	return 0, nil
}

func runCommand(binary string, args []string, backend, action string, mode epa.ProgressMode, interactive bool) (int, error) {
	if interactive && (mode == epa.ProgressTTY || mode == epa.ProgressPlain || mode == epa.ProgressQuiet) {
		code, started, err := epa.RunInteractive(binary, args, backend, action, mode == epa.ProgressPlain, mode == epa.ProgressQuiet)
		if started {
			return code, err
		}
		fmt.Fprintln(os.Stderr, epa.Text("epa: 터미널 진행 표시를 사용할 수 없어 백엔드 원본 출력을 표시합니다:", "epa: terminal renderer unavailable; using unfiltered backend output:"), err)
		if mode != epa.ProgressQuiet {
			mode = epa.ProgressPlain
		}
	}
	return epa.RunPlain(binary, args, mode, action)
}

func invocation(binary string, args []string, privileged, dry bool) (string, []string, error) {
	if !privileged || os.Geteuid() == 0 {
		return binary, args, nil
	}
	for _, p := range []string{"/usr/bin/sudo", "/bin/sudo"} {
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() && st.Mode().Perm()&0111 != 0 {
			return p, append([]string{"--", binary}, args...), nil
		}
	}
	if dry {
		return "/usr/bin/sudo", append([]string{"--", binary}, args...), nil
	}
	return "", nil, epa.LocalizedErrorf("sudo가 설치되어 있지 않습니다. root 계정에서 --style을 명시하여 실행하십시오.", "sudo is not installed; run as root with an explicit --style")
}

func printPlan(binary string, args []string) {
	fmt.Printf("%q", binary)
	for _, arg := range args {
		fmt.Printf(" %q", arg)
	}
	fmt.Println()
}
