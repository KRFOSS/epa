package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"

	"github.com/KRFOSS/epa/internal/epa"
)

var version = "0.1.0-dev"
const help = "EPA - Everyone's Package Adapter\n\nUsage: epa [--style apt|dnf|pacman] [--progress auto|tty|plain|json|native|quiet] [--dry-run] COMMAND [ARGS]\n\nDefault APT-style commands:\n  update, upgrade, full-upgrade, install, remove, search, show,\n  list, outdated, autoremove, reinstall, clean\n\nEPA commands (independent of style):\n  style list|current|set NAME|reset\n  backend info\n  config get progress | set progress MODE\n  doctor\n  completion bash|zsh\n  --help, --version\n\nUse --dry-run before COMMAND to print the native command without running it.\nOptions are style-specific. Unsupported options and operations fail closed.\n"

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
	style := ""
	progress := ""
	dry := false
	for len(args) > 0 {
		switch args[0] {
		case "--style":
			if len(args) < 2 || !epa.ValidStyle(args[1]) {
				return 2, fmt.Errorf("--style requires apt, dnf or pacman")
			}
			style = args[1]
			args = args[2:]
		case "--progress":
			if len(args) < 2 || !epa.ValidProgressMode(args[1]) {
				return 2, fmt.Errorf("--progress requires auto, tty, plain, json, native or quiet")
			}
			progress = args[1]
			args = args[2:]
		case "--dry-run":
			dry = true
			args = args[1:]
		case "--version":
			if len(args) != 1 {
				return 2, fmt.Errorf("unexpected arguments after --version")
			}
			fmt.Println("epa", version)
			return 0, nil
		case "--help", "-h":
			fmt.Print(help)
			return 0, nil
		default:
			goto parsed
		}
	}
parsed:
	if len(args) == 0 {
		fmt.Print(help)
		return 0, nil
	}
	viaSudo := os.Geteuid() == 0 && os.Getenv("SUDO_USER") != ""
	switch args[0] {
	case "config":
		if viaSudo {
			return 2, fmt.Errorf("manage configuration without sudo")
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
			fmt.Println("Progress:", args[3])
			return 0, nil
		}
		return 2, fmt.Errorf("usage: epa config get progress | set progress MODE")
	case "style":
		if len(args) < 2 {
			return 2, fmt.Errorf("usage: epa style list|current|set NAME|reset")
		}
		switch args[1] {
		case "list":
			if len(args) != 2 {
				return 2, fmt.Errorf("unexpected arguments")
			}
			fmt.Println(strings.Join(epa.Styles, "\n"))
			return 0, nil
		case "current":
			if len(args) != 2 {
				return 2, fmt.Errorf("unexpected arguments")
			}
			if viaSudo {
				return 2, fmt.Errorf("query the style from the original account without sudo")
			}
			s, err := epa.LoadStyle()
			if err != nil {
				return 1, err
			}
			fmt.Println(s)
			return 0, nil
		case "set", "reset":
			if viaSudo {
				return 2, fmt.Errorf("change the style without sudo, in the account that will use EPA")
			}
			if dry {
				return 2, fmt.Errorf("style changes cannot be combined with --dry-run")
			}
			if args[1] == "set" && len(args) == 3 {
				if err := epa.SaveStyle(args[2]); err != nil {
					return 2, err
				}
				fmt.Println("Style:", args[2])
				return 0, nil
			}
			if args[1] == "reset" && len(args) == 2 {
				if err := epa.ResetStyle(); err != nil {
					return 1, err
				}
				fmt.Println("Style: apt")
				return 0, nil
			}
		}
		return 2, fmt.Errorf("invalid style command")
	case "completion":
		if len(args) != 2 {
			return 2, fmt.Errorf("usage: epa completion bash|zsh")
		}
		s, ok := epa.CompletionScript(args[1])
		if !ok {
			return 2, fmt.Errorf("unsupported shell %q", args[1])
		}
		fmt.Print(s)
		return 0, nil
	case "backend", "doctor":
		if (args[0] == "backend" && (len(args) != 2 || args[1] != "info")) || (args[0] == "doctor" && len(args) != 1) {
			return 2, fmt.Errorf("usage: epa backend info or epa doctor")
		}
		b, err := epa.Detect()
		if err != nil {
			return 1, err
		}
		fmt.Printf("Backend: %s (%s)\n", b.Name, b.Binary)
		if args[0] == "doctor" && !viaSudo {
			s, err := epa.LoadStyle()
			if err != nil {
				return 1, err
			}
			fmt.Println("Style:", s)
		}
		return 0, nil
	}
	if style == "" {
		if viaSudo {
			return 2, fmt.Errorf("sudo epa requires --style to avoid changing command meanings; normally run epa directly")
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
		prep, prepArgs, err := invocation(b.Binary, []string{"update"}, true, dry)
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
	if mode == epa.ProgressNative {
		// Native mode keeps original streams, signals and exit status.
		err = syscall.Exec(native, append([]string{native}, argv...), os.Environ())
		return 1, err
	}
	return runCommand(native, argv, b.Name, string(op.Action), mode, interactive)
}

func runCommand(binary string, args []string, backend, action string, mode epa.ProgressMode, interactive bool) (int, error) {
	if interactive && (mode == epa.ProgressTTY || mode == epa.ProgressPlain || mode == epa.ProgressQuiet) {
		code, started, err := epa.RunInteractive(binary, args, backend, mode == epa.ProgressPlain, mode == epa.ProgressQuiet)
		if started {
			return code, err
		}
		fmt.Fprintln(os.Stderr, "epa: terminal renderer unavailable; using unfiltered backend output:", err)
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
	return "", nil, fmt.Errorf("sudo is not installed; run as root with an explicit --style")
}

func printPlan(binary string, args []string) {
	fmt.Printf("%q", binary)
	for _, arg := range args {
		fmt.Printf(" %q", arg)
	}
	fmt.Println()
}
