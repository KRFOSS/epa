package epa

import (
	"fmt"
	"regexp"
	"strings"
)

type Action string

const (
	Refresh     Action = "refresh"
	Install     Action = "install"
	Remove      Action = "remove"
	Upgrade     Action = "upgrade"
	FullUpgrade Action = "full-upgrade"
	Search      Action = "search"
	Info        Action = "info"
	List        Action = "list"
	Outdated    Action = "outdated"
	Autoremove  Action = "autoremove"
	Reinstall   Action = "reinstall"
	Clean       Action = "clean"
)

type Operation struct {
	Style                                                      string
	Action                                                     Action
	Packages                                                   []string
	Yes, RefreshFirst, ForceRefresh, Recursive, NoSave, Needed bool
}

var Styles = []string{"apt", "dnf", "pacman"}

func ValidStyle(s string) bool {
	for _, v := range Styles {
		if s == v {
			return true
		}
	}
	return false
}

var aptCommands = map[string]Action{
	"update": Refresh, "install": Install, "remove": Remove,
	"upgrade": Upgrade, "full-upgrade": FullUpgrade, "dist-upgrade": FullUpgrade,
	"search": Search, "show": Info, "info": Info, "list": List, "outdated": Outdated,
	"autoremove": Autoremove, "reinstall": Reinstall, "clean": Clean,
}
var dnfCommands = map[string]Action{
	"makecache": Refresh, "install": Install, "remove": Remove, "erase": Remove,
	"update": FullUpgrade, "upgrade": FullUpgrade, "search": Search, "info": Info,
	"list": List, "check-update": Outdated, "check-upgrade": Outdated,
	"autoremove": Autoremove, "reinstall": Reinstall, "clean": Clean,
}

var packageName = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9.+_:-]*$")

func ValidPackage(s string) bool {
	// Avoid package manager-specific target modifiers, paths, patterns and options.
	return packageName.MatchString(s) && !strings.HasSuffix(s, "-")
}

func Parse(style string, args []string) (Operation, error) {
	op := Operation{Style: style}
	if !ValidStyle(style) {
		return op, fmt.Errorf("unknown style %q", style)
	}
	if len(args) == 0 {
		return op, fmt.Errorf("missing command")
	}
	if style == "pacman" {
		switch args[0] {
		case "-S":
			op.Action = Install
		case "-Ss":
			op.Action = Search
		case "-Si":
			op.Action = Info
		case "-Sy":
			op.Action = Refresh
		case "-Syy":
			op.Action, op.ForceRefresh = Refresh, true
		case "-Su":
			op.Action = FullUpgrade
		case "-Syu", "-Suy":
			op.Action, op.RefreshFirst = FullUpgrade, true
		case "-Syyu", "-Suyy":
			op.Action, op.RefreshFirst, op.ForceRefresh = FullUpgrade, true, true
		case "-R":
			op.Action = Remove
		case "-Rs", "-Rsn", "-Rns":
			op.Action, op.Recursive = Remove, true
			op.NoSave = args[0] != "-Rs"
		case "-Rn":
			op.Action, op.NoSave = Remove, true
		case "-Q":
			op.Action = List
		case "-Qu":
			op.Action = Outdated
		case "-Sc":
			op.Action = Clean
		default:
			return op, fmt.Errorf("unsupported pacman command %q", args[0])
		}
	} else {
		commands := aptCommands
		if style == "dnf" {
			commands = dnfCommands
		}
		var ok bool
		if op.Action, ok = commands[args[0]]; !ok {
			return op, fmt.Errorf("unsupported %s command %q", style, args[0])
		}
	}

	endOptions := false
	for _, arg := range args[1:] {
		if arg == "--" && !endOptions {
			endOptions = true
			continue
		}
		if !endOptions && strings.HasPrefix(arg, "-") {
			switch {
			case style == "apt" && (arg == "-y" || arg == "--yes" || arg == "--assume-yes"):
				op.Yes = true
			case style == "dnf" && (arg == "-y" || arg == "--assumeyes"):
				op.Yes = true
			case style == "pacman" && arg == "--noconfirm":
				op.Yes = true
			case style == "pacman" && arg == "--needed":
				op.Needed = true
			case style == "dnf" && arg == "--refresh":
				op.RefreshFirst = true
			case style == "apt" && arg == "--installed" && op.Action == List:
				// EPA list is the installed-package query.
			case style == "apt" && arg == "--upgradable" && op.Action == List:
				op.Action = Outdated
			default:
				return op, fmt.Errorf("unsupported option %q for %s", arg, style)
			}
			continue
		}
		if style == "dnf" && op.Action == Clean && arg == "all" && len(op.Packages) == 0 {
			continue
		}
		op.Packages = append(op.Packages, arg)
	}
	switch op.Action {
	case Install, Remove, Reinstall, Info, Search:
		if len(op.Packages) == 0 {
			return op, fmt.Errorf("%s requires a target", op.Action)
		}
	case FullUpgrade:
		if len(op.Packages) > 0 && style != "pacman" && style != "dnf" {
			return op, fmt.Errorf("targeted upgrades are not supported in this style")
		}
	default:
		if len(op.Packages) != 0 {
			return op, fmt.Errorf("%s does not accept targets in this version", op.Action)
		}
	}
	if op.Action != Search {
		for _, p := range op.Packages {
			if !ValidPackage(p) {
				return op, fmt.Errorf("unsupported package target %q; use a native package name", p)
			}
		}
	}
	if op.Yes && !op.Mutates() {
		return op, fmt.Errorf("confirmation option is not valid for %s", op.Action)
	}
	if op.Needed && op.Action != Install && op.Action != FullUpgrade {
		return op, fmt.Errorf("--needed is not valid for %s", op.Action)
	}
	if op.RefreshFirst && op.Action != Refresh && op.Action != FullUpgrade {
		return op, fmt.Errorf("--refresh is supported only with makecache or upgrade")
	}
	return op, nil
}

func (o Operation) Mutates() bool {
	switch o.Action {
	case Refresh, Install, Remove, Upgrade, FullUpgrade, Autoremove, Reinstall, Clean:
		return true
	}
	return false
}
