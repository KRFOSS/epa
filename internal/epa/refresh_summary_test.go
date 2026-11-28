//go:build linux

package epa

import "testing"

func TestRefreshCandidateParsing(t *testing.T) {
	for _, tc := range []struct {
		name, backend, output string
		want                  int
		bad                   bool
	}{
		{"apt empty", "apt", "Listing...\n", 0, false},
		{"apt multiarch", "apt", "Listing... Done\nlibc6/stable 2.37 amd64 [upgradable from: 2.36]\nlibc6/stable 2.37 i386 [upgradable from: 2.36]\n", 2, false},
		{"apt incomplete", "apt", "", 0, true},
		{"apt diagnostic", "apt", "Listing...\nE: cache is invalid", 0, true},
		{"dnf empty", "dnf", "", 0, false},
		{"dnf deduplicated", "dnf", "kernel.x86_64\nkernel.x86_64\nlibfoo.i686\nlibfoo.x86_64\n", 3, false},
		{"dnf error", "dnf", "Error: cache unavailable", 0, true},
		{"apk empty", "apk", "", 0, false},
		{"apk candidates", "apk", "busybox-1.37.0-r19 x86_64 {busybox} (GPL-2.0-only) [upgradable from: busybox-1.37.0-r18]\nbusybox-1.37.0-r20 x86_64 {busybox} (GPL-2.0-only) [upgradable from: busybox-1.37.0-r18]\nssl_client-1.37.0-r20 x86_64 {busybox} (GPL-2.0-only) [upgradable from: ssl_client-1.37.0-r18]\n", 2, false},
		{"apk failure", "apk", "WARNING: no cache", 0, true},
		{"portage none", "portage", "Calculating dependencies... done!\nTotal: 0 packages, Size of downloads: 0 KiB\n", 0, false},
		{"portage singular", "portage", "Total: 1 package (1 upgrade), Size of downloads: 8 KiB\n", 1, false},
		{"portage world", "portage", "[ebuild U ] app-misc/foo-2 [1]\nTotal: 3 packages (2 upgrades, 1 reinstall), Size of downloads: 1 KiB\n", 3, false},
		{"portage no total", "portage", "Calculating dependencies...\n", 0, true},
		{"portage ambiguous", "portage", "Total: 2 packages, Size of downloads: 1 KiB\nTotal: 0 packages, Size of downloads: 0 KiB\n", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseUpgradeCandidates(tc.backend, tc.output)
			if got != tc.want || (err != nil) != tc.bad {
				t.Fatalf("got (%d, %v), want (%d, error=%v)", got, err, tc.want, tc.bad)
			}
		})
	}
}

func TestRefreshStylesNeverAutoUpdate(t *testing.T) {
	for _, tc := range []struct {
		style, command string
		refresh        bool
	}{
		{"apt", "update", true}, {"dnf", "makecache", true},
		{"pacman", "-Sy", true}, {"pacman", "-Syy", true},
		{"apk", "update", true}, {"emerge", "--sync", true},
		{"apt", "upgrade", false}, {"apt", "full-upgrade", false},
		{"dnf", "update", false}, {"dnf", "upgrade", false},
		{"pacman", "-Su", false}, {"pacman", "-Syu", false},
		{"apk", "upgrade", false},
	} {
		op, err := Parse(tc.style, []string{tc.command})
		if err != nil {
			t.Fatalf("%s %s: %v", tc.style, tc.command, err)
		}
		if (op.Action == Refresh) != tc.refresh || ShouldAutoUpdate(op.Action, true, false, false) == tc.refresh {
			t.Errorf("%s %s: unexpected action/automatic update policy: %s", tc.style, tc.command, op.Action)
		}
	}
}
