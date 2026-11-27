package epa

import (
	"bytes"
	"strings"
	"testing"
)

func TestAPTDownloadScopes(t *testing.T) {
	const line = "62% [1 openssl 40 kB/100 kB 40%]"
	e, ok := ParseProgress("apt", line)
	if !ok || e.Item != "openssl" || e.Percent == nil || *e.Percent != 40 || e.Overall == nil || *e.Overall != 62 || e.Done {
		t.Fatalf("bad progress record: %+v", e)
	}
	for _, invalid := range []string{"162% [1 openssl 40 kB/100 kB 40%]", "62% [1 openssl 40 kB/100 kB 140%]"} {
		if _, ok := ParseProgress("apt", invalid); ok {
			t.Fatalf("accepted %q", invalid)
		}
	}
	var out bytes.Buffer
	r := NewRenderer(&out, "apt", func() int { return 80 }, false, false)
	r.Consume([]byte(line + "\r"))
	if !strings.Contains(out.String(), "40% · total 62%") {
		t.Fatal(out.String())
	}
}

func TestQuietRendererPreservesNativeMessages(t *testing.T) {
	var out bytes.Buffer
	r := NewRenderer(&out, "apt", func() int { return 80 }, false, false)
	r.quiet = true
	r.Paint()
	r.Consume([]byte("Reading package lists... 42%\r\nReplace it? [Y/n] "))
	r.FlushPartial()
	r.Consume([]byte("n\r\nE: signature verification failed\r\n"))
	r.Finish(100)
	if got, want := out.String(), "Replace it? [Y/n] n\r\nE: signature verification failed\r\n"; got != want {
		t.Fatalf("got %q; want %q", got, want)
	}
}

func FuzzParseProgress(f *testing.F) {
	f.Add("apt", "62% [1 openssl 40 kB/100 kB 40%]")
	f.Add("apt", "Replace it? [Y/n]")
	f.Add("pacman", "(1/3) upgrading nginx [#####     ] 50%")
	f.Add("dnf", "Running transaction")
	f.Fuzz(func(t *testing.T, backend, line string) {
		e, ok := ParseProgress(backend, line)
		if !ok {
			return
		}
		if e.Phase == "" {
			t.Fatal("empty phase")
		}
		for _, n := range []*int{e.Percent, e.Overall} {
			if n != nil && (*n < 0 || *n > 100) {
				t.Fatalf("invalid percentage %d", *n)
			}
		}
	})
}
