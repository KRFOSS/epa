//go:build linux

package epa

import (
	"fmt"
	"os"
	"testing"
)

// Launched by the PTY integration harness. No real packages are changed.
func TestPTYDriver(t *testing.T) {
	scenario := os.Getenv("EPA_PTY_TEST")
	if scenario == "" {
		t.Skip("external PTY harness")
	}
	script := ""
	switch scenario {
	case "prompt":
		script = "printf '\\rReading package lists... 42%%\\r'; sleep 0.2; printf '\\r\\nReplace configuration? [Y/n] '; read answer; printf '\\nAnswer: %s\\n' \"$answer\"; printf 'E: signature verification failed\\n' >&2; exit 100"
	case "apt-cancel":
		script = "printf 'Hit:1 https://mirror.example.test stable InRelease\\r\\n\\rReading package lists... Done\\r\\r\\n\\rCalculating upgrade... Done\\r\\r\\n2 upgraded, 0 newly installed, 0 to remove and 0 not upgraded.\\nDo you want to continue? [Y/n] '; read answer; if [ \"$answer\" = n ]; then printf 'Abort.\\n'; exit 1; fi; exit 100"
	case "success":
		script = "printf '\\rReading package lists... 42%%\\r'; sleep 0.3; printf '\\rProgress: [ 90%%] [######### ]\\r'; sleep 0.2; exit 0"
	case "interrupt":
		script = "trap 'echo Interrupted; exit 130' INT; printf '\\rReading package lists... 42%%\\r'; while :; do sleep 1; done"
	case "full-screen":
		script = "printf '\\033[?1049h\\033[HChoose an option: '; read answer; printf '\\033[?1049l\\nAnswer: %s\\n' \"$answer\"; exit 0"
	case "resize":
		script = "printf '\\rReading package lists... 42%%\\r'; sleep 0.5; stty size; exit 0"
	default:
		os.Exit(2)
	}
	before, err := terminalState(os.Stdin)
	if err != nil {
		os.Exit(3)
	}
	code, started, err := RunInteractive("/bin/sh", []string{"-c", script}, "apt", "upgrade", false)
	after, e := terminalState(os.Stdin)
	if !started || err != nil || e != nil || before != after {
		fmt.Fprintf(os.Stderr, "PTY setup/restore failed: %t %v %v\n", started, err, e)
		os.Exit(4)
	}
	os.Exit(code)
}
