package epa

import (
	"reflect"
	"testing"
)

func TestStyleSemantics(t *testing.T) {
	tests := []struct {
		style  string
		args   []string
		action Action
		yes    bool
	}{
		{"apt", []string{"update"}, Refresh, false},
		{"dnf", []string{"update"}, FullUpgrade, false},
		{"apt", []string{"install", "httpd", "-y"}, Install, true},
		{"pacman", []string{"-Syu", "--noconfirm"}, FullUpgrade, true},
		{"pacman", []string{"-Rns", "nginx"}, Remove, false},
		{"apt", []string{"list", "--upgradable"}, Outdated, false},
	}
	for _, tt := range tests {
		op, err := Parse(tt.style, tt.args)
		if err != nil || op.Action != tt.action || op.Yes != tt.yes {
			t.Errorf("%s %v: %+v, %v", tt.style, tt.args, op, err)
		}
	}
}
func TestPlan(t *testing.T) {
	tests := []struct {
		style, backend string
		args, want     []string
		fail, refresh  bool
	}{
		{"apt", "dnf", []string{"install", "httpd", "-y"}, []string{"-y", "install", "--", "httpd"}, false, false},
		{"apt", "dnf", []string{"update"}, []string{"--refresh", "makecache"}, false, false},
		{"dnf", "apt", []string{"update"}, []string{"upgrade", "--with-new-pkgs", "--no-remove"}, false, false},
		{"pacman", "apt", []string{"-Syu"}, []string{"upgrade", "--with-new-pkgs", "--no-remove"}, false, true},
		{"apt", "pacman", []string{"update"}, nil, true, false},
		{"apt", "pacman", []string{"upgrade"}, nil, true, false},
		{"apt", "pacman", []string{"full-upgrade"}, []string{"-Syu"}, false, false},
		{"apt", "pacman", []string{"remove", "nginx"}, []string{"-R", "--", "nginx"}, false, false},
		{"pacman", "dnf", []string{"-R", "nginx"}, nil, true, false},
		{"pacman", "pacman", []string{"-Rns", "nginx"}, []string{"-Rns", "--", "nginx"}, false, false},
		{"apt", "apt", []string{"remove", "nginx+"}, nil, true, false},
		{"pacman", "apt", []string{"-Syyu"}, nil, true, false},
		{"pacman", "dnf", []string{"-Syu", "nginx"}, nil, true, false},
		{"dnf", "pacman", []string{"upgrade", "nginx"}, nil, true, false},
		{"apt", "dnf", []string{"clean"}, []string{"clean", "packages"}, false, false},
	}
	for _, tt := range tests {
		op, err := Parse(tt.style, tt.args)
		if err != nil {
			t.Fatal(err)
		}
		plan, err := (Backend{Name: tt.backend, Binary: "/usr/bin/native"}).Build(op)
		if tt.fail {
			if err == nil {
				t.Errorf("expected refusal for %s %v on %s", tt.style, tt.args, tt.backend)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(plan.Args, tt.want) || plan.RefreshBefore != tt.refresh {
			t.Errorf("%s %v on %s: %+v, %v; want %v", tt.style, tt.args, tt.backend, plan, err, tt.want)
		}
	}
}
func TestInvalid(t *testing.T) {
	for _, args := range [][]string{
		{"install"}, {"install", "nginx", "--allow-unauthenticated"}, {"install", "--", "-o"},
		{"install", "../../foo"}, {"install", "foo-"}, {"install", "foo;rm"},
		{"update", "nginx"}, {"search", "nginx", "-y"},
	} {
		if _, err := Parse("apt", args); err == nil {
			t.Errorf("unexpectedly accepted %v", args)
		}
	}
}
func TestStyleConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	s, err := LoadStyle()
	if err != nil || s != "apt" {
		t.Fatalf("%s %v", s, err)
	}
	if err := SaveStyle("pacman"); err != nil {
		t.Fatal(err)
	}
	s, err = LoadStyle()
	if err != nil || s != "pacman" {
		t.Fatalf("%s %v", s, err)
	}
	if err := ResetStyle(); err != nil {
		t.Fatal(err)
	}
	s, err = LoadStyle()
	if err != nil || s != "apt" {
		t.Fatalf("%s %v", s, err)
	}
}
func TestCompletion(t *testing.T) {
	b := Backend{Name: "dnf"}
	for _, tt := range []struct {
		style       string
		words, want []string
	}{
		{"apt", []string{"ins"}, []string{"install"}},
		{"apt", []string{"style", "set", "p"}, []string{"pacman"}},
		{"apt", []string{"--style", "pacman", "-Sy"}, []string{"-Sy", "-Syu", "-Syy", "-Syyu"}},
		{"apt", []string{"install", "httpd", "-y"}, []string{"-y"}},
		{"dnf", []string{"install", "httpd", "--ass"}, []string{"--assumeyes"}},
	} {
		if got := Complete(tt.style, tt.words, b); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%v: %v != %v", tt.words, got, tt.want)
		}
	}
}
func FuzzParse(f *testing.F) {
	f.Add("apt", "install", "httpd", "-y")
	f.Add("pacman", "-Syu", "", "--noconfirm")
	f.Fuzz(func(t *testing.T, style, a, b, c string) { _, _ = Parse(style, []string{a, b, c}) })
}
