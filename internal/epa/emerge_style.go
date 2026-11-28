package epa

import "strings"

// Translate a deliberately limited subset of emerge options. Arbitrary
// native switches must never be forwarded to a package manager.
func parseEmerge(args []string) (Operation, error) {
	op := Operation{Style: "emerge", Action: Install}
	fail := func() (Operation, error) {
		return op, LocalizedErrorf("지원하지 않거나 안전하게 변환할 수 없는 emerge 명령입니다.", "unsupported or unsafe emerge command")
	}
	var action Action
	var pretend, askSet, deep, newuse, verbose bool
	setAction := func(a Action) bool {
		if action != "" {
			return false
		}
		action = a
		return true
	}
	endOptions := false
	for _, arg := range args {
		if !endOptions && arg == "--" {
			endOptions = true
			continue
		}
		if !endOptions && strings.HasPrefix(arg, "-") {
			switch arg {
			case "--sync":
				if !setAction(Refresh) {
					return fail()
				}
			case "--list-installed":
				if !setAction(List) {
					return fail()
				}
			case "--clean-cache":
				if !setAction(Clean) {
					return fail()
				}
			case "--reinstall-targets":
				if !setAction(Reinstall) {
					return fail()
				}
			case "--search", "-s":
				if !setAction(Search) {
					return fail()
				}
			case "--info":
				if !setAction(Info) {
					return fail()
				}
			case "--update", "-u":
				if !setAction(FullUpgrade) {
					return fail()
				}
			case "--depclean", "-c":
				if !setAction(Autoremove) {
					return fail()
				}
			case "--pretend", "-p":
				if pretend {
					return fail()
				}
				pretend = true
			case "--ask", "--ask=y", "-a":
				if askSet {
					return fail()
				}
				askSet = true
			case "--ask=n":
				if askSet {
					return fail()
				}
				askSet, op.Yes = true, true
			case "--deep", "-D":
				if deep {
					return fail()
				}
				deep = true
			case "--newuse", "-N":
				if newuse {
					return fail()
				}
				newuse = true
			case "--verbose", "-v":
				if verbose {
					return fail()
				}
				verbose = true
			default:
				if strings.HasPrefix(arg, "--") || len(arg) < 3 {
					return fail()
				}
				// Common grouped short flags, e.g. -avuDN @world.
				for _, flag := range arg[1:] {
					switch flag {
					case 'a':
						if askSet {
							return fail()
						}
						askSet = true
					case 'v':
						if verbose {
							return fail()
						}
						verbose = true
					case 'u':
						if !setAction(FullUpgrade) {
							return fail()
						}
					case 'D':
						if deep {
							return fail()
						}
						deep = true
					case 'N':
						if newuse {
							return fail()
						}
						newuse = true
					case 'p':
						if pretend {
							return fail()
						}
						pretend = true
					case 'c':
						if !setAction(Autoremove) {
							return fail()
						}
					default:
						return fail()
					}
				}
			}
			continue
		}
		op.Packages = append(op.Packages, arg)
	}
	if action != "" {
		op.Action = action
	}
	if (deep || newuse || pretend) && op.Action != FullUpgrade {
		return fail()
	}
	if op.Action == FullUpgrade {
		if len(op.Packages) != 1 || op.Packages[0] != "@world" {
			return fail()
		}
		op.Packages = nil
		if pretend {
			op.Action = Outdated
		}
	}
	if op.Action == Autoremove && len(op.Packages) > 0 {
		op.Action = Remove
	}
	if askSet && (!op.Mutates() || op.Action == Refresh || op.Action == Clean) {
		return fail()
	}
	if verbose && (op.Action == Refresh || op.Action == Search || op.Action == Info || op.Action == List || op.Action == Clean || op.Action == Outdated) {
		return fail()
	}
	if deep || newuse || verbose {
		op.PortageOptions = true
	}
	switch op.Action {
	case Install, Remove, Reinstall, Info, Search:
		if len(op.Packages) == 0 {
			return fail()
		}
	default:
		if len(op.Packages) != 0 {
			return fail()
		}
	}
	if op.Action != Search {
		for _, name := range op.Packages {
			if !ValidPackage(name) && !ValidPortageTarget(name) {
				return fail()
			}
		}
	}
	return op, nil
}
