//go:build linux

package epa

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPacmanUpgradeCount(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		count      int
		wantErr    bool
	}{
		{"multiple", "printf 'flex\\npython-shtab\\nxl2tpd\\n'", 3, false},
		{"single", "printf 'lib32-test@2\\n'", 1, false},
		{"zero exit one", "exit 1", 0, false},
		{"zero exit zero", "exit 0", 0, false},
		{"database failure", "echo 'error: database missing' >&2; exit 1", 0, true},
		{"warning without result", "echo 'warning: database invalid' >&2; exit 1", 0, true},
		{"partial result", "printf 'flex\\n'; echo 'error: database missing' >&2; exit 1", 0, true},
		{"diagnostic on success", "echo 'warning: stale database' >&2; printf 'flex\\n'", 0, true},
		{"unexpected exit", "exit 2", 0, true},
		{"partial silent failure", "printf 'flex\\n'; exit 1", 0, true},
		{"unexpected format", "printf 'flex 1.0 -> 2.0\\n'", 0, true},
		{"duplicate", "printf 'flex\\nflex\\n'", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			db := filepath.Join(dir, "query db")
			path := filepath.Join(dir, "pacman")
			body := "#!/bin/sh\n" +
				"test \"$#\" = 5 && test \"$1\" = --dbpath && test \"$2\" = '" + db + "' && test \"$3\" = --color && test \"$4\" = never && test \"$5\" = -Quq && test \"$LC_ALL\" = C || exit 99\n" +
				tc.body + "\n"
			if err := os.WriteFile(path, []byte(body), 0700); err != nil {
				t.Fatal(err)
			}
			got, err := queryPacmanUpgradeCount(context.Background(), path, db)
			if got != tc.count || (err != nil) != tc.wantErr {
				t.Fatalf("count = %d, err = %v; want %d, error %t", got, err, tc.count, tc.wantErr)
			}
		})
	}
}

func TestPacmanUpgradeCountCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if n, err := queryPacmanUpgradeCount(ctx, "/bin/true", "/unused"); err == nil || n != 0 {
		t.Fatalf("canceled query: %d, %v", n, err)
	}
}
