package epa

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Backend struct{ Name, Binary string }
type Plan struct {
	Binary        string
	Args          []string
	Privileged    bool
	RefreshBefore bool
	Note          string
}

func systemBinary(name string) string {
	for _, dir := range []string{"/usr/bin", "/bin", "/usr/sbin", "/sbin"} {
		p := filepath.Join(dir, name)
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() && st.Mode().Perm()&0111 != 0 {
			return p
		}
	}
	return ""
}

func osRelease() map[string]string {
	values := map[string]string{}
	for _, path := range []string{"/etc/os-release", "/usr/lib/os-release"} {
		f, err := os.Open(path)
		if err != nil {
			continue
		}
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			k, v, ok := strings.Cut(scanner.Text(), "=")
			if ok {
				values[k] = strings.Trim(v, "\"'")
			}
		}
		f.Close()
		return values
	}
	return values
}

func Detect() (Backend, error) {
	data := osRelease()
	for _, id := range append([]string{data["ID"]}, strings.Fields(data["ID_LIKE"])...) {
		var name string
		var binaries []string
		switch id {
		case "debian", "ubuntu", "linuxmint", "pop", "kali", "raspbian":
			name, binaries = "apt", []string{"apt-get"}
		case "rhel", "fedora", "centos", "rocky", "almalinux", "ol", "amzn":
			name, binaries = "dnf", []string{"dnf", "dnf5"}
		case "arch", "archlinux", "manjaro", "cachyos", "endeavouros":
			name, binaries = "pacman", []string{"pacman"}
		}
		if name == "" {
			continue
		}
		for _, n := range binaries {
			if p := systemBinary(n); p != "" {
				return Backend{name, p}, nil
			}
		}
		return Backend{}, fmt.Errorf("%s detected, but its supported package manager is unavailable", id)
	}
	return Backend{}, fmt.Errorf("unsupported distribution %q (supported backends: apt, dnf, pacman)", data["ID"])
}

func (b Backend) helper(name string) (string, error) {
	p := systemBinary(name)
	if p == "" {
		return "", fmt.Errorf("required backend tool %s not found", name)
	}
	return p, nil
}

func (b Backend) Build(o Operation) (Plan, error) {
	p := Plan{Binary: b.Binary, Privileged: o.Mutates()}
	unsupported := func(reason string) (Plan, error) {
		return Plan{}, fmt.Errorf("%s cannot safely perform %s: %s", b.Name, o.Action, reason)
	}
	if o.Action == FullUpgrade && len(o.Packages) > 0 && o.Style != b.Name {
		return unsupported("targeted and combined upgrades have different semantics across backends")
	}
	if o.NoSave && b.Name != "pacman" {
		return unsupported("the requested configuration removal semantics are unavailable")
	}
	if o.Needed && b.Name != "pacman" {
		return unsupported("--needed has no verified equivalent")
	}
	if o.Recursive && b.Name == "apt" {
		return unsupported("recursive removal semantics differ")
	}
	if o.ForceRefresh && b.Name != "pacman" {
		return unsupported("forced refresh has no verified equivalent")
	}
	if o.RefreshFirst && o.Action == FullUpgrade && b.Name == "apt" {
		p.RefreshBefore = true
	}

	switch b.Name {
	case "apt":
		switch o.Action {
		case Refresh:
			p.Args = []string{"update"}
		case Install:
			p.Args = []string{"install", "--no-remove"}
		case Remove:
			for _, target := range o.Packages {
				if strings.HasSuffix(target, "+") {
					return unsupported("APT interprets a trailing plus as an installation modifier")
				}
			}
			p.Args = []string{"remove"}
		case Upgrade:
			p.Args = []string{"upgrade", "--with-new-pkgs", "--no-remove"}
		case FullUpgrade:
			if len(o.Packages) > 0 {
				return unsupported("targeted upgrades are not supported")
			}
			// A non-APT upgrade style does not imply permission for APT dist-upgrade.
			if o.Style != "apt" {
				p.Args = []string{"upgrade", "--with-new-pkgs", "--no-remove"}
				p.Note = "Using conservative APT upgrade semantics."
			} else {
				p.Args = []string{"dist-upgrade"}
			}
		case Search, Info:
			var err error
			p.Binary, err = b.helper("apt-cache")
			if err != nil {
				return Plan{}, err
			}
			if o.Action == Search {
				p.Args = []string{"search"}
			} else {
				p.Args = []string{"show"}
			}
		case List, Outdated:
			var err error
			p.Binary, err = b.helper("apt")
			if err != nil {
				return Plan{}, err
			}
			if o.Action == List {
				p.Args = []string{"list", "--installed"}
			} else {
				p.Args = []string{"list", "--upgradable"}
			}
		case Autoremove:
			p.Args = []string{"autoremove"}
		case Reinstall:
			p.Args = []string{"install", "--reinstall", "--no-remove"}
		case Clean:
			if o.Style != "apt" {
				return unsupported("APT cache cleanup semantics differ")
			}
			p.Args = []string{"clean"}
		default:
			return unsupported("unsupported action")
		}
		if o.Yes {
			p.Args = append(p.Args, "-y")
		}
	case "dnf":
		switch o.Action {
		case Refresh:
			p.Args = []string{"--refresh", "makecache"}
		case Install:
			p.Args = []string{"install"}
		case Remove:
			p.Args = []string{"remove"}
			if o.Style != "dnf" && !o.Recursive {
				p.Args = append(p.Args, "--noautoremove")
			}
			if o.Style == "pacman" {
				return unsupported("DNF may remove dependent packages that pacman -R would refuse to remove")
			}
		case Upgrade:
			p.Args = []string{"--setopt=obsoletes=False", "upgrade"}
			p.Note = "DNF applies its native transaction policy, including install-only package limits."
		case FullUpgrade:
			p.Args = []string{"upgrade"}
			if o.Style == "apt" {
				p.Note = "DNF applies its native upgrade policy."
			}
		case Search:
			p.Args = []string{"search"}
		case Info:
			p.Args = []string{"info"}
		case List:
			p.Args = []string{"list", "--installed"}
		case Outdated:
			p.Args = []string{"check-update"}
			resolved, _ := filepath.EvalSymlinks(b.Binary)
			if filepath.Base(b.Binary) == "dnf5" || filepath.Base(resolved) == "dnf5" {
				p.Args[0] = "check-upgrade"
			}
		case Autoremove:
			p.Args = []string{"autoremove"}
		case Reinstall:
			p.Args = []string{"reinstall"}
		case Clean:
			if o.Style == "pacman" {
				return unsupported("pacman cache cleanup semantics differ")
			}
			p.Args = []string{"clean", "packages"}
			if o.Style == "dnf" {
				p.Args[1] = "all"
			}
		default:
			return unsupported("unsupported action")
		}
		if o.RefreshFirst && o.Action == FullUpgrade {
			p.Args = append([]string{"--refresh"}, p.Args...)
		}
		if o.Yes {
			p.Args = append([]string{"-y"}, p.Args...)
		}
	case "pacman":
		switch o.Action {
		case Refresh:
			return unsupported("standalone refresh can lead to a partial upgrade; use epa --style pacman -Syu")
		case Upgrade:
			return unsupported("APT upgrade cannot guarantee no removals; use full-upgrade or epa --style pacman -Syu")
		case FullUpgrade:
			if o.Style == "pacman" && !o.RefreshFirst {
				p.Args = []string{"-Su"}
			} else if o.ForceRefresh {
				p.Args = []string{"-Syyu"}
			} else {
				p.Args = []string{"-Syu"}
				if o.Style != "pacman" {
					p.Note = "A full Arch upgrade refreshes repositories and upgrades the entire system."
				}
			}
		case Install:
			p.Args = []string{"-S"}
			p.Note = "Use a fully upgraded Arch system; EPA never refreshes metadata alone before installing."
		case Remove:
			if o.Style != "pacman" {
				p.Note = "Pacman refuses removals that would break dependencies."
			}
			p.Args = []string{"-R"}
			if o.Recursive && o.NoSave {
				p.Args[0] = "-Rns"
			} else if o.Recursive {
				p.Args[0] = "-Rs"
			} else if o.NoSave {
				p.Args[0] = "-Rn"
			}
		case Search:
			p.Args = []string{"-Ss"}
		case Info:
			p.Args = []string{"-Si"}
		case List:
			p.Args = []string{"-Q"}
		case Outdated:
			p.Args = []string{"-Qu"}
		case Clean:
			if o.Style != "pacman" {
				return unsupported("pacman cache cleanup semantics differ")
			}
			p.Args = []string{"-Sc"}
		case Reinstall:
			p.Args = []string{"-S"}
		case Autoremove:
			return unsupported("orphan removal requires an explicit package selection")
		default:
			return unsupported("unsupported action")
		}
		if o.Yes {
			p.Args = append(p.Args, "--noconfirm")
		}
		if o.Needed {
			p.Args = append(p.Args, "--needed")
		}
	default:
		return unsupported("unknown backend")
	}
	if len(o.Packages) > 0 {
		p.Args = append(p.Args, "--")
		p.Args = append(p.Args, o.Packages...)
	}
	return p, nil
}
