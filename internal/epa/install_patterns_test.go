package epa

import (
	"fmt"
	"reflect"
	"testing"
)

func TestInstallPatternParsing(t *testing.T) {
	for _, c := range []struct {
		style string
		args  []string
	}{
		{"apt", []string{"install", "zsh*", "-y"}},
		{"dnf", []string{"install", "zsh?"}},
		{"pacman", []string{"-S", "zsh*", "--noconfirm"}},
		{"apk", []string{"add", "zsh*"}},
		{"emerge", []string{"--ask=n", "app-shells/zsh*"}},
	} {
		op, err := Parse(c.style, c.args)
		if err != nil || !op.HasInstallPatterns() {
			t.Errorf("%s %v: %+v %v", c.style, c.args, op, err)
		}
	}
	for _, args := range [][]string{
		{"install", "*"}, {"install", "?"},
		{"install", "../zsh*"}, {"install", "zsh[ab]"},
		{"install", "zsh*:amd64"}, {"install", "--", "-zsh*"},
		{"install", "zsh*;rm"}, {"remove", "zsh*"},
		{"reinstall", "zsh*"}, {"upgrade", "zsh*"},
	} {
		if _, err := Parse("apt", args); err == nil {
			t.Errorf("unexpectedly accepted %v", args)
		}
	}
}

func TestInstallPatternExpansion(t *testing.T) {
	for _, backend := range []string{"apt", "dnf", "pacman", "apk"} {
		op, err := Parse("apt", []string{"install", "zsh*", "zsh-doc", "curl", "zsh?"})
		if err != nil {
			t.Fatal(err)
		}
		got, err := expandInstallPatterns(backend, op, []string{"zsh-doc", "zsh", "zsh", "zsh9", "zsh:virtual", "zsh-", "bash"})
		want := []string{"zsh", "zsh-doc", "zsh9", "curl"}
		if err != nil || !reflect.DeepEqual(got.Packages, want) || got.HasInstallPatterns() {
			t.Errorf("%s: %+v %v; want %v", backend, got, err, want)
		}
		plan, err := (Backend{Name: backend, Binary: "/usr/bin/native"}).Build(got)
		if err != nil || !reflect.DeepEqual(plan.Args[len(plan.Args)-len(want):], want) {
			t.Errorf("%s native plan: %+v %v", backend, plan, err)
		}
		if _, err := (Backend{Name: backend, Binary: "/usr/bin/native"}).Build(op); err == nil {
			t.Errorf("%s accepted unexpanded patterns", backend)
		}
	}
	op, _ := Parse("emerge", []string{"--ask=n", "zsh*", "app-misc/tree?"})
	got, err := expandInstallPatterns("portage", op, []string{"app-shells/zsh", "app-shells/zsh-completions", "app-misc/tree1", "other/zsh", "zsh"})
	want := []string{"app-shells/zsh", "app-shells/zsh-completions", "other/zsh", "app-misc/tree1"}
	if err != nil || !reflect.DeepEqual(got.Packages, want) {
		t.Fatalf("portage: %+v %v; want %v", got, err, want)
	}
}

func TestInstallPatternRefusals(t *testing.T) {
	op, _ := Parse("apt", []string{"install", "zsh*"})
	if _, err := expandInstallPatterns("pacman", op, []string{"bash"}); err == nil {
		t.Fatal("empty match accepted")
	}
	op, _ = Parse("apt", []string{"install", "app-shells/zsh*"})
	if _, err := expandInstallPatterns("pacman", op, []string{"zsh"}); err == nil {
		t.Fatal("Portage category accepted on Pacman")
	}
	op, _ = Parse("apt", []string{"install", "pkg*"})
	names := make([]string, maxPatternTargets+1)
	for i := range names {
		names[i] = fmt.Sprintf("pkg%d", i)
	}
	if _, err := expandInstallPatterns("apt", op, names); err == nil {
		t.Fatal("unbounded match accepted")
	}
}

func TestCleanAcrossStyles(t *testing.T) {
	styles := []struct {
		name string
		args []string
	}{
		{"apt", []string{"clean"}},
		{"dnf", []string{"clean", "all"}},
		{"pacman", []string{"-Sc"}},
		{"apk", []string{"cache", "clean"}},
		{"emerge", []string{"--clean-cache"}},
	}
	for _, style := range styles {
		op, err := Parse(style.name, style.args)
		if err != nil || op.Action != Clean {
			t.Fatalf("%s: %+v %v", style.name, op, err)
		}
		for _, backend := range []struct {
			name string
			args []string
		}{
			{"apt", []string{"clean"}},
			{"dnf", []string{"clean", "packages"}},
			{"pacman", []string{"-Sc"}},
			{"apk", []string{"cache", "clean"}},
			{"portage", []string{"distfiles"}},
		} {
			if backend.name == "dnf" && style.name == "dnf" {
				backend.args[1] = "all"
			}
			plan, err := (Backend{Name: backend.name, Binary: "/usr/bin/native"}).Build(op)
			if backend.name == "portage" && systemBinary("eclean") == "" {
				if err == nil {
					t.Errorf("%s accepted absent eclean", style.name)
				}
				continue
			}
			if err != nil || !reflect.DeepEqual(plan.Args, backend.args) || !plan.Privileged {
				t.Errorf("%s on %s: %+v %v; want %v", style.name, backend.name, plan, err, backend.args)
			}
		}
	}
}
