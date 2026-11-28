package epa

import (
	"context"
	_ "embed"
	"os/exec"
	"sort"
	"strings"
	"time"
)

//go:embed completion.bash
var bashCompletion string

//go:embed completion.zsh
var zshCompletion string

//go:embed completion.fish
var fishCompletion string

func CompletionScript(shell string) (string, bool) {
	switch shell {
	case "bash":
		return bashCompletion, true
	case "zsh":
		return zshCompletion, true
	case "fish":
		return fishCompletion, true
	}
	return "", false
}

func commandsFor(style string) []string {
	list := []string{"style", "backend", "doctor", "completion", "config", "self-update", "--style", "--language", "--progress", "--dry-run", "--no-self-update", "--help", "--version"}
	switch style {
	case "apt":
		for cmd := range aptCommands {
			list = append(list, cmd)
		}
	case "dnf":
		for cmd := range dnfCommands {
			list = append(list, cmd)
		}
	case "apk":
		for cmd := range apkCommands {
			list = append(list, cmd)
		}
	case "emerge":
		list = append(list, "--sync", "--ask", "--ask=n", "-av", "-avuDN", "--update", "--depclean", "--search", "--info", "--pretend", "--list-installed", "--clean-cache", "--reinstall-targets")
	case "pacman":
		list = append(list, "-S", "-Ss", "-Si", "-Sy", "-Syy", "-Su", "-Syu", "-Syyu", "-R", "-Rs", "-Rn", "-Rns", "-Q", "-Qu", "-Sc")
	}
	return list
}

// Complete consumes all arguments up to the cursor, including the incomplete word.
func Complete(style string, words []string, backend Backend) []string {
	if len(words) == 0 {
		words = []string{""}
	}
	prefix := words[len(words)-1]
	before := words[:len(words)-1]
	for len(before) > 0 {
		switch before[0] {
		case "--style":
			if len(before) == 1 {
				return matching(Styles, prefix)
			}
			if !ValidStyle(before[1]) {
				return nil
			}
			style = before[1]
			before = before[2:]
		case "--language":
			if len(before) == 1 {
				return matching(Languages, prefix)
			}
			if !ValidLanguage(before[1]) {
				return nil
			}
			before = before[2:]
		case "--progress":
			if len(before) == 1 {
				return matching(ProgressModes, prefix)
			}
			if !ValidProgressMode(before[1]) {
				return nil
			}
			before = before[2:]
		case "--dry-run", "--no-self-update":
			before = before[1:]
		default:
			goto parsed
		}
	}
parsed:
	if len(before) == 0 {
		candidates := commandsFor(style)
		if style == "emerge" && prefix != "" && !strings.HasPrefix(prefix, "-") {
			candidates = append(candidates, backend.packageNames(false, prefix)...)
		}
		return matching(candidates, prefix)
	}
	if before[0] == "config" {
		if len(before) == 1 {
			return matching([]string{"get", "set"}, prefix)
		}
		if len(before) == 2 && (before[1] == "get" || before[1] == "set") {
			return matching([]string{"progress", "self-update", "language"}, prefix)
		}
		if len(before) == 3 && before[1] == "set" && before[2] == "self-update" {
			return matching([]string{"on", "off"}, prefix)
		}
		if len(before) == 3 && before[1] == "set" && before[2] == "language" {
			return matching(Languages, prefix)
		}
		if len(before) == 3 && before[1] == "set" && before[2] == "progress" {
			return matching(ProgressModes, prefix)
		}
		return nil
	}
	if before[0] == "style" {
		if len(before) == 1 {
			return matching([]string{"list", "set", "current", "reset"}, prefix)
		}
		if before[1] == "set" && len(before) == 2 {
			return matching(Styles, prefix)
		}
		return nil
	}
	if before[0] == "backend" && len(before) == 1 {
		return matching([]string{"info"}, prefix)
	}
	if before[0] == "completion" {
		if len(before) == 1 {
			return matching([]string{"bash", "zsh", "fish", "install"}, prefix)
		}
		if len(before) == 2 && before[1] == "install" {
			return matching([]string{"all", "bash", "zsh", "fish"}, prefix)
		}
		return nil
	}
	if style == "emerge" {
		return completeEmerge(before, prefix, backend)
	}
	cmd := before[0]
	if style == "apk" {
		switch cmd {
		case "cache":
			if len(before) == 1 {
				return matching([]string{"clean"}, prefix)
			}
			return nil
		case "list":
			if len(before) == 1 {
				return matching([]string{"--installed", "--upgradable", "-I", "-u"}, prefix)
			}
			return nil
		case "info":
			return matching(backend.packageNames(false, prefix), prefix)
		}
	}
	// Use the parser's operation table, with a placeholder only for target-required actions.
	var op Operation
	var err error
	op, err = Parse(style, []string{cmd})
	if err != nil {
		op, err = Parse(style, []string{cmd, "placeholder"})
	}
	if err != nil {
		return nil
	}
	if strings.HasPrefix(prefix, "-") {
		var opts []string
		if op.Mutates() {
			switch style {
			case "apt":
				opts = append(opts, "-y", "--yes", "--assume-yes")
			case "dnf":
				opts = append(opts, "-y", "--assumeyes")
			case "apk":
				if op.Action == Install || op.Action == Remove || op.Action == Reinstall || op.Action == FullUpgrade {
					opts = append(opts, "--no-interactive")
					if backend.Name == "apk" {
						opts = append(opts, "-i", "--interactive")
					}
				}
				if op.Action == Refresh && backend.Name == "apk" {
					opts = append(opts, "--force-refresh")
				}
				if op.Action == Reinstall {
					opts = append(opts, "-r", "--reinstall")
				}
			case "pacman":
				opts = append(opts, "--noconfirm")
			}
		}
		if style == "pacman" && backend.Name == "pacman" && (op.Action == Install || op.Action == FullUpgrade) {
			opts = append(opts, "--needed")
		}
		if style == "dnf" && (op.Action == Refresh || op.Action == FullUpgrade) {
			opts = append(opts, "--refresh")
		}
		if style == "apt" && op.Action == List {
			opts = append(opts, "--installed", "--upgradable")
		}
		return matching(opts, prefix)
	}
	if style == "dnf" && op.Action == Clean {
		return matching([]string{"all"}, prefix)
	}
	switch op.Action {
	case Install, Search, Info:
		return matching(backend.packageNames(false, prefix), prefix)
	case Remove, Reinstall:
		return matching(backend.packageNames(true, prefix), prefix)
	case FullUpgrade:
		if style != "apt" && style != "apk" {
			return matching(backend.packageNames(true, prefix), prefix)
		}
	}
	return nil
}

func matching(values []string, prefix string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, v := range values {
		if strings.HasPrefix(v, prefix) && !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	sort.Strings(result)
	return result
}

func (b Backend) packageNames(installed bool, prefix string) []string {
	var binary string
	var args []string
	switch b.Name {
	case "apt":
		if installed {
			binary = systemBinary("dpkg-query")
			args = []string{"-W", "-f=${db:Status-Abbrev} ${binary:Package}\\n"}
		} else {
			binary = systemBinary("apt-cache")
			args = []string{"pkgnames"}
			if prefix != "" && !strings.HasPrefix(prefix, "-") {
				args = append(args, prefix)
			}
		}
	case "dnf":
		if installed {
			// Query the RPM database directly: DNF can still initialize
			// repositories or plugins even with --cacheonly --installed.
			binary = systemBinary("rpm")
			args = []string{"-qa", "--qf", "%{NAME}\\n"}
		} else {
			binary = b.Binary
			args = []string{"-q", "--cacheonly", "repoquery", "--qf=%{name}\n", "--available"}
		}
	case "portage":
		binary = systemBinary("python3")
		tree := "porttree"
		if installed {
			tree = "vartree"
		}
		args = []string{"-I", "-c", portageNamesScript, tree, prefix}
	case "apk":
		binary = b.Binary
		if installed {
			args = []string{"--no-network", "info"}
		} else {
			args = []string{"--no-network", "search", "-q"}
		}
	case "pacman":
		binary = b.Binary
		if installed {
			args = []string{"-Qq"}
		} else {
			args = PacmanQueryArgs([]string{"-Slq"})
		}
	default:
		return nil
	}
	if binary == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, binary, args...).Output()
	if err != nil {
		return nil
	}
	var result []string
	for _, line := range strings.Split(string(output), "\n") {
		if b.Name == "apt" && installed {
			if !strings.HasPrefix(line, "ii ") {
				continue
			}
			line = strings.TrimSpace(line[3:])
		}
		if ValidPackage(line) || (b.Name == "portage" && ValidPortageTarget(line)) {
			result = append(result, line)
		}
	}
	return result
}
