//go:build linux

package epa

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
)

func TestReleaseVersion(t *testing.T) {
	for _, s := range []string{"0.1.0", "1.0.0", "1.10.2", "10.0.0"} {
		if _, err := parseReleaseVersion(s); err != nil {
			t.Errorf("%s: %v", s, err)
		}
	}
	for _, s := range []string{"0.1.0-dev", "v1.0.0", "1.0", "1.01.0", "1.0.0/../../x", "1.0.9999999999999999999999999", "1.0.0-rc1", ""} {
		if _, err := parseReleaseVersion(s); err == nil {
			t.Errorf("accepted invalid version %q", s)
		}
	}
	old, _ := parseReleaseVersion("1.9.9")
	next, _ := parseReleaseVersion("1.10.0")
	if !next.newerThan(old) || old.newerThan(next) || old.newerThan(old) {
		t.Fatal("incorrect numeric version comparison")
	}
}

func TestReleaseAsset(t *testing.T) {
	for _, arch := range []string{"386", "amd64", "arm64", "loong64", "mips", "mipsle", "mips64", "mips64le", "ppc64", "ppc64le", "riscv64", "s390x"} {
		got, err := releaseAsset(arch, "")
		if err != nil || got != "epa-linux-"+arch {
			t.Errorf("%s: %s %v", arch, got, err)
		}
	}
	for _, level := range []string{"5", "6", "7"} {
		got, err := releaseAsset("arm", level+",softfloat")
		if err != nil || got != "epa-linux-armv"+level {
			t.Errorf("%s: %s %v", level, got, err)
		}
	}
	for _, arch := range []string{"arm", "wasm", "unknown"} {
		if _, err := releaseAsset(arch, ""); err == nil {
			t.Errorf("accepted unsupported asset %s", arch)
		}
	}
}

func TestReleaseRedirectPolicy(t *testing.T) {
	for _, address := range []string{
		"https://api.github.com/repos/KRFOSS/epa/releases/latest",
		"https://github.com/KRFOSS/epa/releases/download/v1.0.0/epa-linux-amd64",
		"https://release-assets.githubusercontent.com/signed",
		"https://objects.githubusercontent.com/signed",
	} {
		u, _ := url.Parse(address)
		if !allowedReleaseURL(u) {
			t.Errorf("rejected %s", address)
		}
	}
	for _, address := range []string{
		"http://github.com/x", "https://github.com.evil.example/x", "https://127.0.0.1/x",
		"https://user@github.com/x", "https://github.com:444/x", "https://evil.example/x",
	} {
		u, _ := url.Parse(address)
		if allowedReleaseURL(u) {
			t.Errorf("accepted %s", address)
		}
	}
}

func TestChecksumFor(t *testing.T) {
	const name = "epa-linux-amd64"
	sum := strings.Repeat("a", 64)
	if got, err := checksumFor([]byte(sum+"  "+name+"\n"), name); err != nil || len(got) != 32 {
		t.Fatalf("%x %v", got, err)
	}
	for _, manifest := range []string{
		sum + "  epa-linux-arm64\n", "invalid " + name + "\n",
		sum + " " + name + "\n" + sum + " " + name + "\n",
		sum + " " + name + " unexpected\n",
	} {
		if _, err := checksumFor([]byte(manifest), name); err == nil {
			t.Errorf("accepted bad manifest %q", manifest)
		}
	}
}

// The updater never executes the downloaded fixture. Use a native ELF file
// with extra bytes to model a distinct, valid replacement.
func updateFixture(t *testing.T) (updateTarget, []byte, []byte) {
	t.Helper()
	old, err := os.ReadFile("/bin/true")
	if err != nil {
		t.Skipf("native ELF fixture unavailable: %v", err)
	}
	candidate := append(append([]byte(nil), old...), []byte("new EPA fixture")...)
	dir := t.TempDir()
	path := filepath.Join(dir, "epa")
	if err = os.WriteFile(path, old, 0755); err != nil {
		t.Fatal(err)
	}
	st, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	return updateTarget{path, st, uint32(os.Geteuid())}, old, candidate
}

func TestUpdateFromRelease(t *testing.T) {
	for _, tt := range []struct {
		name, tag                                                string
		prerelease, badHash, badELF, missing, changed, cancelled bool
		wantUpdated, wantError                                   bool
	}{
		{name: "success", tag: "v0.2.0", wantUpdated: true},
		{name: "same", tag: "v0.1.0"},
		{name: "older", tag: "v0.0.9"},
		{name: "prerelease", tag: "v0.2.0", prerelease: true, wantError: true},
		{name: "bad-tag", tag: "v0.2.0/../../x", wantError: true},
		{name: "checksum-mismatch", tag: "v0.2.0", badHash: true, wantError: true},
		{name: "not-elf", tag: "v0.2.0", badELF: true, wantError: true},
		{name: "missing", tag: "v0.2.0", missing: true, wantError: true},
		{name: "concurrent-change", tag: "v0.2.0", changed: true, wantError: true},
		{name: "cancelled", tag: "v0.2.0", cancelled: true, wantError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			target, old, candidate := updateFixture(t)
			if tt.badELF {
				candidate = []byte("not an executable")
			}
			digest := sha256.Sum256(candidate)
			if tt.badHash {
				digest = sha256.Sum256([]byte("wrong"))
			}
			const asset = "epa-linux-amd64"
			var requests atomic.Int32
			concurrent := []byte("another installation")
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				switch r.URL.Path {
				case "/latest":
					fmt.Fprintf(w, `{"tag_name":%q,"draft":false,"prerelease":%t}`, tt.tag, tt.prerelease)
				case "/download/" + tt.tag + "/SHA256SUMS":
					if tt.missing {
						http.NotFound(w, r)
						return
					}
					fmt.Fprintf(w, "%x  %s\n", digest, asset)
				case "/download/" + tt.tag + "/" + asset:
					if tt.changed {
						stage := target.path + ".concurrent"
						if err := os.WriteFile(stage, concurrent, 0755); err != nil {
							t.Error(err)
						}
						if err := os.Rename(stage, target.path); err != nil {
							t.Error(err)
						}
					}
					_, _ = w.Write(candidate)
				default:
					t.Errorf("unexpected release URL: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.cancelled {
				cancel()
			}
			result, err := updateFrom(ctx, releaseSource{srv.Client(), srv.URL + "/latest", srv.URL + "/download"}, target, "0.1.0", asset)
			if (err != nil) != tt.wantError || result.Updated != tt.wantUpdated {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			want := old
			if tt.changed {
				want = concurrent
			}
			if tt.wantUpdated {
				want = candidate
				if result.Version != "0.2.0" {
					t.Fatal(result)
				}
			}
			got, err := os.ReadFile(target.path)
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("unexpected installed bytes, err=%v", err)
			}
			st, err := os.Stat(target.path)
			if err != nil || st.Mode().Perm() != 0755 {
				t.Fatalf("mode changed: %v %v", st, err)
			}
			files, err := filepath.Glob(filepath.Join(filepath.Dir(target.path), ".epa-update-*"))
			if err != nil || len(files) != 0 {
				t.Fatalf("staging files remain: %v %v", files, err)
			}
			if (tt.name == "same" || tt.name == "older") && requests.Load() != 1 {
				t.Fatalf("download attempted despite no newer release: %d", requests.Load())
			}
			if tt.cancelled && requests.Load() != 0 {
				t.Fatal("request made after cancellation")
			}
		})
	}
}

// This explicit opt-in test downloads a real public release into a
// disposable directory; it never replaces the user's installed EPA.
func TestLiveReleaseUpdate(t *testing.T) {
	if os.Getenv("EPA_TEST_LIVE_RELEASE") != "1" {
		t.Skip("set EPA_TEST_LIVE_RELEASE=1 for the public release integration test")
	}
	target, _, _ := updateFixture(t)
	asset, err := currentReleaseAsset()
	if err != nil {
		t.Fatal(err)
	}
	result, err := updateFrom(context.Background(), githubReleaseSource(), target, "0.0.0", asset)
	if err != nil || !result.Updated {
		t.Fatalf("update result=%+v error=%v", result, err)
	}
	out, err := exec.Command(target.path, "--version").Output()
	if err != nil || strings.TrimSpace(string(out)) != "epa "+result.Version {
		t.Fatalf("replacement version=%q error=%v", out, err)
	}
	t.Logf("Verified public release %s (%s); replacement runs successfully", result.Version, asset)
}

func TestCheckedUpdateTarget(t *testing.T) {
	uid := uint32(os.Geteuid())
	if uid == 0 {
		t.Skip("user-owned path test requires non-root")
	}
	home := t.TempDir()
	path := filepath.Join(home, ".local", "bin", "epa")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("test"), 0755); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = checkedTarget(home, path, info, uid); err != nil {
		t.Fatal(err)
	}
	if _, err = checkedTarget(home, path, info, 0); err == nil {
		t.Fatal("root target accepted")
	}
	if _, err = checkedTarget(home, path+".other", info, uid); err == nil {
		t.Fatal("custom target accepted")
	}
	if err = os.Link(path, path+".hardlink"); err != nil {
		t.Fatal(err)
	}
	if _, err = checkedTarget(home, path, info, uid); err == nil {
		t.Fatal("hard-linked target accepted")
	}
	if err = os.Remove(path + ".hardlink"); err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(filepath.Dir(path), 0777); err != nil {
		t.Fatal(err)
	}
	if _, err = checkedTarget(home, path, info, uid); err == nil {
		t.Fatal("writable directory accepted")
	}
	if err = os.Chmod(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(path, path+".real"); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(path+".real", path); err != nil {
		t.Fatal(err)
	}
	if _, err = checkedTarget(home, path, info, uid); err == nil {
		t.Fatal("symlink target accepted")
	}
}

func TestShouldAutoUpdate(t *testing.T) {
	for _, action := range []Action{Refresh, Upgrade, FullUpgrade, Install, Remove, Search, Outdated} {
		want := action == Refresh || action == Upgrade || action == FullUpgrade
		if ShouldAutoUpdate(action, true, false, false) != want {
			t.Errorf("incorrect policy for %s", action)
		}
		if ShouldAutoUpdate(action, false, false, false) || ShouldAutoUpdate(action, true, true, false) || ShouldAutoUpdate(action, true, false, true) {
			t.Errorf("unsafe automatic update for %s", action)
		}
	}
}

func TestSelfUpdateConfigAndCompletion(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	path, err := configPath()
	if err != nil {
		t.Fatal(err)
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, []byte(`{"style":"apt","progress":"auto"}`), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig()
	if err != nil || !c.SelfUpdate {
		t.Fatalf("old config: %+v %v", c, err)
	}
	if err = SaveSelfUpdate(false); err != nil {
		t.Fatal(err)
	}
	if err = SaveStyle("pacman"); err != nil {
		t.Fatal(err)
	}
	if err = SaveProgress("plain"); err != nil {
		t.Fatal(err)
	}
	c, err = LoadConfig()
	if err != nil || c.SelfUpdate || c.Style != "pacman" || c.Progress != "plain" {
		t.Fatalf("lost preference: %+v %v", c, err)
	}
	if got := Complete("apt", []string{"config", "set", "self-update", ""}, Backend{}); !reflect.DeepEqual(got, []string{"off", "on"}) {
		t.Fatalf("completion: %v", got)
	}
	if got := Complete("apt", []string{"--no-self-update", "up"}, Backend{}); !reflect.DeepEqual(got, []string{"update", "upgrade"}) {
		t.Fatalf("completion: %v", got)
	}
}
