package epa

import "strings"

// Completion is intentionally conservative. A world upgrade never suggests
// package targets, and removal suggests only installed packages.
func completeEmerge(before []string, prefix string, b Backend) []string {
	var world, depclean, pretend, ask, terminal bool
	var action bool
	for _, word := range before {
		switch word {
		case "--sync", "--list-installed", "--clean-cache":
			terminal = true
		case "--update", "-u":
			world, action = true, true
		case "--depclean", "-c":
			depclean, action = true, true
		case "--reinstall-targets":
			depclean, action = true, true
		case "--search", "-s", "--info":
			action = true
		case "--pretend", "-p":
			pretend = true
		case "--ask", "--ask=y", "--ask=n", "-a":
			ask = true
		case "@world":
			return nil
		default:
			if strings.HasPrefix(word, "-") && !strings.HasPrefix(word, "--") {
				flags := word[1:]
				if strings.ContainsRune(flags, 'u') {
					world, action = true, true
				}
				if strings.ContainsRune(flags, 'c') {
					depclean, action = true, true
				}
				ask = ask || strings.ContainsRune(flags, 'a')
				pretend = pretend || strings.ContainsRune(flags, 'p')
			}
		}
	}
	if terminal {
		return nil
	}
	if strings.HasPrefix(prefix, "-") {
		opts := []string{}
		if !ask && !pretend {
			opts = append(opts, "--ask", "--ask=n", "-a")
		}
		if world {
			opts = append(opts, "--deep", "--newuse", "-D", "-N")
			if !pretend && !ask {
				opts = append(opts, "--pretend", "-p")
			}
		} else if !action {
			opts = append(opts, "--update", "-u")
			if !pretend {
				opts = append(opts, "--depclean", "--search", "--info")
			}
		}
		return matching(opts, prefix)
	}
	if world {
		return matching([]string{"@world"}, prefix)
	}
	if pretend {
		return nil
	}
	return matching(b.packageNames(depclean, prefix), prefix)
}
