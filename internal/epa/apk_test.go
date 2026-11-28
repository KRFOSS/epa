package epa

import (
	"reflect"
	"testing"
)

func TestAPKPlans(t *testing.T) {
	b := Backend{Name: "apk", Binary: "/sbin/apk"}
	cases := []struct {
		style               string
		args, want, refresh []string
		privileged          bool
	}{
		{"apt", []string{"update"}, []string{"update"}, nil, true},
		{"apt", []string{"install", "tree"}, []string{"--interactive", "add", "--", "tree"}, nil, true},
		{"apt", []string{"install", "tree", "-y"}, []string{"--no-interactive", "add", "--", "tree"}, nil, true},
		{"apt", []string{"remove", "tree", "-y"}, []string{"--no-interactive", "del", "--", "tree"}, nil, true},
		{"apt", []string{"reinstall", "tree", "-y"}, []string{"--no-interactive", "fix", "--reinstall", "--", "tree"}, nil, true},
		{"apt", []string{"full-upgrade", "-y"}, []string{"--no-interactive", "upgrade"}, nil, true},
		{"dnf", []string{"update", "--refresh", "-y"}, []string{"--no-interactive", "upgrade"}, []string{"update"}, true},
		{"pacman", []string{"-Syyu", "--noconfirm"}, []string{"--no-interactive", "upgrade"}, []string{"--force-refresh", "update"}, true},
		{"apk", []string{"add", "tree"}, []string{"add", "--", "tree"}, nil, true},
		{"apk", []string{"del", "tree", "--interactive"}, []string{"--interactive", "del", "--", "tree"}, nil, true},
		{"apk", []string{"fix", "tree", "-r"}, []string{"fix", "--reinstall", "--", "tree"}, nil, true},
		{"apk", []string{"update", "--force-refresh"}, []string{"--force-refresh", "update"}, nil, true},
		{"apk", []string{"cache", "clean"}, []string{"cache", "clean"}, nil, true},
		{"apk", []string{"info"}, []string{"info"}, nil, false},
		{"apk", []string{"info", "tree"}, []string{"info", "--all", "--", "tree"}, nil, false},
		{"apk", []string{"list", "--installed"}, []string{"info"}, nil, false},
		{"apk", []string{"list", "--upgradable"}, []string{"list", "--upgradable"}, nil, false},
		{"apk", []string{"search", "tree"}, []string{"search", "--", "tree"}, nil, false},
	}
	for _, c := range cases {
		op, err := Parse(c.style, c.args)
		if err != nil {
			t.Fatalf("%s %v: %v", c.style, c.args, err)
		}
		p, err := b.Build(op)
		if err != nil || !reflect.DeepEqual(p.Args, c.want) || !reflect.DeepEqual(p.RefreshArgs, c.refresh) || p.RefreshBefore != (c.refresh != nil) || p.Privileged != c.privileged {
			t.Errorf("%s %v: %+v, %v; want %v refresh %v", c.style, c.args, p, err, c.want, c.refresh)
		}
	}
}

func TestAPKRefusals(t *testing.T) {
	b := Backend{Name: "apk", Binary: "/sbin/apk"}
	for _, c := range []struct {
		style string
		args  []string
	}{
		{"apt", []string{"autoremove"}},
		{"apt", []string{"clean"}},
		{"pacman", []string{"-R", "tree"}},
		{"pacman", []string{"-S", "tree", "--needed"}},
		{"apk", []string{"upgrade", "tree"}},
		{"apk", []string{"add", "cmd:tree"}},
		{"apk", []string{"add", "tree", "--allow-untrusted"}},
		{"apk", []string{"add", "tree", "--interactive", "--no-interactive"}},
		{"apk", []string{"cache", "download"}},
		{"apk", []string{"list"}},
		{"apk", []string{"list", "--installed", "--upgradable"}},
		{"apk", []string{"fix"}},
		{"apk", []string{"update", "--no-interactive"}},
	} {
		op, err := Parse(c.style, c.args)
		if err == nil {
			_, err = b.Build(op)
		}
		if err == nil {
			t.Errorf("unexpectedly allowed %s %v", c.style, c.args)
		}
	}
}

func TestAPKCompletionAndProgress(t *testing.T) {
	b := Backend{Name: "apk"}
	for _, c := range []struct{ words, want []string }{
		{[]string{"ad"}, []string{"add"}},
		{[]string{"cache", ""}, []string{"clean"}},
		{[]string{"list", "--up"}, []string{"--upgradable"}},
		{[]string{"add", "--no"}, []string{"--no-interactive"}},
		{[]string{"add", "--int"}, []string{"--interactive"}},
		{[]string{"update", "--fo"}, []string{"--force-refresh"}},
	} {
		if got := Complete("apk", c.words, b); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%v: %v, want %v", c.words, got, c.want)
		}
	}
	for _, line := range []string{"(1/2) Installing tree (2.2.1-r0)", "(2/2) Purging tree (2.2.1-r0)"} {
		e, ok := ParseProgress("apk", line)
		if !ok || e.Item != "tree" || e.Percent != nil || e.Done {
			t.Fatalf("bad event: %+v (%v)", e, ok)
		}
	}
	if _, ok := ParseProgress("apk", "ERROR: unable to select packages:"); ok {
		t.Fatal("recognized error as progress")
	}
}
