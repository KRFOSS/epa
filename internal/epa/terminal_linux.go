//go:build linux

package epa

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
	"time"
	"unsafe"
)

type windowSize struct{ Rows, Cols, X, Y uint16 }

func ioctl(fd uintptr, request uintptr, arg unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, request, uintptr(arg))
	if errno != 0 {
		return errno
	}
	return nil
}

func terminalState(f *os.File) (syscall.Termios, error) {
	var state syscall.Termios
	err := ioctl(f.Fd(), syscall.TCGETS, unsafe.Pointer(&state))
	return state, err
}

func IsTerminal(f *os.File) bool { _, err := terminalState(f); return err == nil }

func getSize(f *os.File) windowSize {
	var w windowSize
	_ = ioctl(f.Fd(), syscall.TIOCGWINSZ, unsafe.Pointer(&w))
	if w.Cols == 0 {
		w.Cols = 80
	}
	if w.Rows == 0 {
		w.Rows = 24
	}
	return w
}

func setSize(f *os.File, w windowSize) error {
	return ioctl(f.Fd(), syscall.TIOCSWINSZ, unsafe.Pointer(&w))
}

func rawState(original syscall.Termios) syscall.Termios {
	r := original
	r.Iflag &^= syscall.BRKINT | syscall.ICRNL | syscall.INPCK | syscall.ISTRIP | syscall.IXON
	r.Oflag &^= syscall.OPOST
	r.Cflag &^= syscall.CSIZE | syscall.PARENB
	r.Cflag |= syscall.CS8
	r.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.IEXTEN | syscall.ISIG
	r.Cc[syscall.VMIN] = 1
	r.Cc[syscall.VTIME] = 0
	return r
}

func openPTY() (*os.File, *os.File, error) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		return nil, nil, err
	}
	var unlock int32
	if err = ioctl(master.Fd(), syscall.TIOCSPTLCK, unsafe.Pointer(&unlock)); err != nil {
		master.Close()
		return nil, nil, err
	}
	var number uint32
	if err = ioctl(master.Fd(), syscall.TIOCGPTN, unsafe.Pointer(&number)); err != nil {
		master.Close()
		return nil, nil, err
	}
	slave, err := os.OpenFile(fmt.Sprintf("/dev/pts/%d", number), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		master.Close()
		return nil, nil, err
	}
	return master, slave, nil
}

// RunInteractive uses a PTY so the native manager keeps a controlling
// terminal, including sudo, debconf and package-maintainer prompts.
// Any setup failure before Start is returned without starting a transaction.
func RunInteractive(binary string, args []string, backend string, plain bool, quiet ...bool) (int, bool, error) {
	original, err := terminalState(os.Stdin)
	if err != nil {
		return 1, false, err
	}
	master, slave, err := openPTY()
	if err != nil {
		return 1, false, err
	}
	defer master.Close()
	defer slave.Close()
	if err = ioctl(slave.Fd(), syscall.TCSETS, unsafe.Pointer(&original)); err != nil {
		return 1, false, err
	}
	if err = setSize(slave, getSize(os.Stdout)); err != nil {
		return 1, false, err
	}

	raw := rawState(original)
	if err = ioctl(os.Stdin.Fd(), syscall.TCSETS, unsafe.Pointer(&raw)); err != nil {
		return 1, false, err
	}
	defer ioctl(os.Stdin.Fd(), syscall.TCSETS, unsafe.Pointer(&original))
	signals := make(chan os.Signal, 8)
	signal.Notify(signals, syscall.SIGWINCH, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGTSTP, syscall.SIGQUIT)
	defer signal.Stop(signals)

	cmd := exec.Command(binary, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = slave, slave, slave
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err = cmd.Start(); err != nil {
		return 1, false, err
	}
	_ = slave.Close()

	r := NewRenderer(os.Stdout, backend, func() int { return int(getSize(os.Stdout).Cols) }, os.Getenv("NO_COLOR") == "" && !plain, plain)
	r.quiet = len(quiet) > 0 && quiet[0]
	r.Paint()
	done := make(chan struct{})
	outCh := make(chan []byte, 16)
	var output <-chan []byte = outCh
	go func() {
		defer close(outCh)
		buf := make([]byte, 8192)
		for {
			n, e := master.Read(buf)
			if n > 0 {
				select {
				case outCh <- append([]byte(nil), buf[:n]...):
				case <-done:
					return
				}
			}
			if e != nil {
				return
			}
		}
	}()
	inputDone := make(chan struct{})
	suspend := make(chan struct{}, 1)
	inputNotice := make(chan struct{}, 1)
	go func() {
		defer close(inputDone)
		buf := make([]byte, 4096)
		for {
			select {
			case <-done:
				return
			default:
			}
			// stdin is descriptor zero. Poll first instead of depending on
			// VTIME, which is not implemented consistently by every PTY.
			var readers syscall.FdSet
			readers.Bits[0] = 1
			timeout := syscall.NsecToTimeval(int64(100 * time.Millisecond))
			ready, e := syscall.Select(1, &readers, nil, nil, &timeout)
			if e != nil {
				if errors.Is(e, syscall.EINTR) {
					continue
				}
				return
			}
			if ready == 0 {
				continue
			}
			n, e := syscall.Read(0, buf)
			if n > 0 {
				select {
				case inputNotice <- struct{}{}:
				default:
				}
				for _, ch := range buf[:n] {
					// Ctrl-Z is handled by the outer shell's job control when
					// the child terminal has signal processing enabled.
					if ch == 26 {
						if st, e := terminalState(master); e == nil && st.Lflag&syscall.ISIG != 0 {
							select {
							case suspend <- struct{}{}:
							default:
							}
							continue
						}
					}
					if _, e := master.Write([]byte{ch}); e != nil {
						return
					}
				}
			}
			if e != nil && !errors.Is(e, syscall.EINTR) && !errors.Is(e, syscall.EAGAIN) {
				return
			}
		}
	}()
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	ticker := time.NewTicker(90 * time.Millisecond)
	defer ticker.Stop()
	var waitErr error
	exited := false
	var drain <-chan time.Time
	lastData := time.Now()
	stop := func() {
		r.FlushPartial()
		r.Clear()
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGTSTP)
		_ = ioctl(os.Stdin.Fd(), syscall.TCSETS, unsafe.Pointer(&original))
		// SIGSTOP also works for a foreground group whose parent does not
		// honor SIGTSTP. Resume with the shell's fg command.
		_ = syscall.Kill(os.Getpid(), syscall.SIGSTOP)
		_ = ioctl(os.Stdin.Fd(), syscall.TCSETS, unsafe.Pointer(&raw))
		_ = setSize(master, getSize(os.Stdout))
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGCONT)
	}
	for !exited || output != nil {
		select {
		case b, ok := <-output:
			if !ok {
				output = nil
				continue
			}
			r.Consume(b)
			lastData = time.Now()
		case <-ticker.C:
			if time.Since(lastData) >= 90*time.Millisecond {
				r.FlushPartial()
			}
			r.Paint()
		case <-inputNotice:
			r.FlushPartial()
			r.Clear()
			r.active = false
		case <-suspend:
			if !exited {
				stop()
			}
		case s := <-signals:
			if s == syscall.SIGWINCH {
				r.Clear()
				_ = setSize(master, getSize(os.Stdout))
				r.Paint()
			} else if !exited {
				if s == syscall.SIGTSTP {
					stop()
				} else if sig, ok := s.(syscall.Signal); ok {
					_ = syscall.Kill(-cmd.Process.Pid, sig)
				}
			}
		case waitErr = <-waited:
			exited = true
			// Allow the PTY reader to drain after the native process exits.
			// Descendants must not keep EPA blocked indefinitely.
			drain = time.After(2 * time.Second)
		case <-drain:
			_ = master.Close()
			output = nil
			drain = nil
		}
	}
	close(done)
	_ = master.Close()
	<-inputDone
	code := ExitCode(waitErr)
	r.Finish(code)
	return code, true, nil
}

// RunPlain preserves native streams, or routes both to stderr for JSON.
// No noninteractive flags are added. SIGINT is delivered by the original
// terminal to both processes; external signals are forwarded explicitly.
func RunPlain(binary string, args []string, mode ProgressMode, action string) (int, error) {
	start := time.Now()
	var out io.Writer = os.Stderr
	if mode == ProgressJSON {
		out = os.Stdout
	}
	EmitLifecycle(out, mode, "started", action, 0, 0)
	cmd := exec.Command(binary, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if mode == ProgressJSON {
		cmd.Stdout = os.Stderr
	}
	sigs := make(chan os.Signal, 8)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	defer signal.Stop(sigs)
	if err := cmd.Start(); err != nil {
		EmitLifecycle(out, mode, "failed", action, 1, time.Since(start))
		return 1, err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	for {
		select {
		case err := <-done:
			code := ExitCode(err)
			state := "completed"
			if code != 0 {
				state = "failed"
			}
			EmitLifecycle(out, mode, state, action, code, time.Since(start))
			return code, nil
		case s := <-sigs:
			_ = cmd.Process.Signal(s)
		}
	}
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if status, ok := exit.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return 128 + int(status.Signal())
		}
		if c := exit.ExitCode(); c >= 0 {
			return c
		}
	}
	return 1
}
