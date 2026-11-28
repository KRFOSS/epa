package epa

import (
	"reflect"
	"strings"
	"testing"
)

func TestPortagePlans(t *testing.T) {
	b := Backend{Name: "portage", Binary: "/usr/bin/emerge"}
	mutate := func(yes bool, tail ...string) []string {
		ask := "--ask=y"
		if yes {
			ask = "--ask=n"
		}
		return append([]string{ask, "--verbose", "--autounmask-write=n", "--autounmask-continue=n"}, tail...)
	}
	tests := []struct {
		style               string
		args, want, refresh []string
		privileged          bool
	}{
		{"apt", []string{"update"}, portageSyncArgs(), nil, true},
		{"apt", []string{"install", "app-misc/tree"}, mutate(false, "--update", "--select=y", "--", "app-misc/tree"), nil, true},
		{"apt", []string{"install", "tree", "-y"}, mutate(true, "--update", "--select=y", "--", "tree"), nil, true},
		{"apt", []string{"upgrade"}, mutate(false, portageWorldArgs()...), nil, true},
		{"apt", []string{"full-upgrade", "-y"}, mutate(true, portageWorldArgs()...), nil, true},
		{"dnf", []string{"upgrade", "--refresh", "-y"}, mutate(true, portageWorldArgs()...), portageSyncArgs(), true},
		{"pacman", []string{"-Syu"}, mutate(false, portageWorldArgs()...), portageSyncArgs(), true},
		{"apk", []string{"add", "tree", "--no-interactive"}, mutate(true, "--update", "--select=y", "--", "tree"), nil, true},
		{"apt", []string{"remove", "app-misc/tree"}, mutate(false, "--depclean", "--depclean-lib-check=y", "--", "app-misc/tree"), nil, true},
		{"apt", []string{"autoremove", "-y"}, mutate(true, "--depclean", "--depclean-lib-check=y"), nil, true},
		{"apt", []string{"reinstall", "tree"}, mutate(false, "--selective=n", "--oneshot", "--", "tree"), nil, true},
		{"apt", []string{"search", "tree"}, []string{"--ignore-default-opts", "--search", "--", "tree"}, nil, false},
		{"apt", []string{"show", "dev-util/c++"}, []string{"--ignore-default-opts", "--search", "--fuzzy-search=n", "--", `%@^dev-util/c\+\+$`}, nil, false},
		{"emerge", []string{"--sync"}, portageSyncArgs(), nil, true},
		{"emerge", []string{"-av", "app-misc/tree"}, mutate(false, "--update", "--select=y", "--", "app-misc/tree"), nil, true},
		{"emerge", []string{"--ask=n", "app-misc/tree"}, mutate(true, "--update", "--select=y", "--", "app-misc/tree"), nil, true},
		{"emerge", []string{"-avuDN", "@world"}, mutate(false, portageWorldArgs()...), nil, true},
		{"emerge", []string{"--depclean", "app-misc/tree"}, mutate(false, "--depclean", "--depclean-lib-check=y", "--", "app-misc/tree"), nil, true},
		{"emerge", []string{"--info", "app-misc/tree"}, []string{"--ignore-default-opts", "--info", "--", "app-misc/tree"}, nil, false},
		{"emerge", []string{"-puDN", "@world"}, append([]string{"--ask=n", "--pretend", "--autounmask-write=n", "--autounmask-continue=n"}, portageWorldArgs()...), nil, false},
	}
	for _, c := range tests {
		o, err := Parse(c.style, c.args)
		if err != nil {
			t.Fatalf("%s %v: %v", c.style, c.args, err)
		}
		p, err := b.Build(o)
		if err != nil || !reflect.DeepEqual(p.Args, c.want) || !reflect.DeepEqual(p.RefreshArgs, c.refresh) || p.Privileged != c.privileged || p.RefreshBefore != (c.refresh != nil) {
			t.Errorf("%s %v: %+v %v, want %v / %v", c.style, c.args, p, err, c.want, c.refresh)
		}
	}
}

func TestPortageRefusals(t *testing.T) {
	b := Backend{Name: "portage", Binary: "/usr/bin/emerge"}
	for _, c := range []struct {
		style string
		args  []string
	}{
		{"apt", []string{"install", "/tmp/tree.ebuild"}},
		{"apt", []string{"install", "app-misc/tree.ebuild"}},
		{"apt", []string{"install", "app-misc/tree-1.0.gpkg.tar"}},
		{"apt", []string{"install", "tree.tbz2"}},
		{"apt", []string{"install", "app-misc/tree-1.0"}},
		{"apt", []string{"install", "app-misc/tree:0"}},
		{"apt", []string{"install", "app-misc/tree::gentoo"}},
		{"apt", []string{"install", "app-misc/../tree"}},
		{"apt", []string{"install", "@world"}},
		{"apt", []string{"install", "--", "--nodeps"}},
		{"apt", []string{"install", "tree", "--nodeps"}},
		{"emerge", []string{"--unmerge", "tree"}},
		{"emerge", []string{"--clean"}},
		{"emerge", []string{"--ask=n", "--ask=y", "tree"}},
		{"emerge", []string{"--update", "tree"}},
		{"emerge", []string{"-uD", "@system"}},
		{"emerge", []string{"--pretend", "tree"}},
		{"emerge", []string{"--depclean", "--ignore-world", "tree"}},
		{"emerge", []string{"--sync", "tree"}},
		{"emerge", []string{"--sync", "--depclean"}},
		{"pacman", []string{"-R", "tree"}},
	} {
		o, err := Parse(c.style, c.args)
		if err == nil {
			_, err = b.Build(o)
		}
		if err == nil {
			t.Errorf("unexpectedly accepted %s %v", c.style, c.args)
		}
	}
	for _, name := range []string{"apt", "dnf", "pacman", "apk"} {
		o, err := Parse("apt", []string{"install", "app-misc/tree"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = (Backend{Name: name, Binary: "/usr/bin/native"}).Build(o); err == nil {
			t.Errorf("Portage category target leaked to %s", name)
		}
	}
	o, err := Parse("emerge", []string{"-avuDN", "@world"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = (Backend{Name: "apt", Binary: "/usr/bin/apt-get"}).Build(o); err == nil {
		t.Fatal("Portage-specific options accepted by APT")
	}
	// Bare package names must not collide with EPA's explicit extensions.
	o, err = Parse("emerge", []string{"clean"})
	if err != nil || o.Action != Install {
		t.Fatalf("bare package alias: %+v %v", o, err)
	}
}

func TestPortageCompletionAndProgress(t *testing.T) {
	b := Backend{Name: "portage"}
	for _, c := range []struct{ words, want []string }{
		{[]string{"--sy"}, []string{"--sync"}},
		{[]string{"-avuDN", "@"}, []string{"@world"}},
		{[]string{"--pretend", "--update", "@"}, []string{"@world"}},
		{[]string{"--sync", ""}, nil},
		{[]string{"--update", "@world", ""}, nil},
	} {
		if got := Complete("emerge", c.words, b); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%v: %v want %v", c.words, got, c.want)
		}
	}
	for _, line := range []string{
		">>> Emerging (1 of 2) app-misc/tree-2.1.1::gentoo",
		">>> Emerging binary (1 of 2) app-misc/tree-2.1.1::gentoo",
		">>> Installing (1 of 2) app-misc/tree-2.1.1::gentoo",
		">>> Unmerging (1 of 2) app-misc/tree-2.1.1...",
	} {
		e, ok := ParseProgress("portage", line)
		if !ok || !strings.HasPrefix(e.Item, "app-misc/tree-2.1.1") || e.Percent != nil || e.Done {
			t.Fatalf("invalid event %q: %+v %v", line, e, ok)
		}
	}
	if _, ok := ParseProgress("portage", ">>> Failed app-misc/tree-2.1.1"); ok {
		t.Fatal("failure consumed as progress")
	}
}
