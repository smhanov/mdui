// Package apptest drives the real mdui binary inside a pty so tests can
// exercise the whole program the way a terminal sees it: startup capability
// queries, renderer output, and keystroke handling. It exists to guard
// against the frozen-repaint bug reported on ptys whose slave never answers
// terminal capability queries (script, expect, serial consoles, some CI
// wrappers): the first frame paints and then the screen never updates, even
// though the app keeps accepting input.
//
// The fake terminal is the pty master. Everything the child writes lands in
// the master's read buffer, and everything written to the master shows up on
// the child's stdin — that is how this package injects keyboard input and
// how it replies to capability queries. Which queries get answered is
// selectable per test, so a test can emulate a completely silent terminal
// or answer only a subset while bisecting the trigger.
//
// The package builds the mdui binary once per test run (TestMain) and skips
// itself on platforms without pty support.
package apptest

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
)

const (
	// DefaultRows and DefaultCols match the pty geometry of the reported
	// freeze; 120x40 shows the whole tree plus a full preview.
	DefaultRows = 40
	DefaultCols = 120

	// ChildTerm is the TERM value the child runs under: a plain xterm-like
	// entry with none of the terminal-identity variables a real desktop
	// would set, matching the reported environment.
	ChildTerm = "xterm-256color"
)

// Answers selects which terminal capability queries the fake terminal
// responds to while the child runs. The zero value is a fully silent
// terminal, which is the environment of the reported freeze.
type Answers struct {
	DA1      bool // reply to ESC[c with a primary device attributes report
	OSC11    bool // reply to OSC 11 (ESC]11;?) with a black background
	Mode2026 bool // reply to the DECRQM query for synchronized output (2026)
	Mode2027 bool // reply to the DECRQM query for unicode core (2027)
}

// All returns the answer set of a fully capable terminal.
func All() Answers {
	return Answers{DA1: true, OSC11: true, Mode2026: true, Mode2027: true}
}

// queryTable pairs each capability query the program can write with the
// response a compliant terminal would send back through the pty master.
// The query strings mirror what lipgloss (OSC 11 + DA1) and bubbletea
// (DECRQM 2026/2027) put on the wire at startup.
var queryTable = []struct {
	name     string
	query    string
	response string
	enabled  func(Answers) bool
}{
	{
		name:     "DA1",
		query:    "\x1b[c",
		response: "\x1b[?62;1;2;6;7;8;9;15;18;21;22c",
		enabled:  func(a Answers) bool { return a.DA1 },
	},
	{
		name:     "OSC11",
		query:    "\x1b]11;?\x07",
		response: "\x1b]11;rgb:0000/0000/0000\x07",
		enabled:  func(a Answers) bool { return a.OSC11 },
	},
	{
		name:     "DECRQM-2026",
		query:    "\x1b[?2026$p",
		response: "\x1b[?2026;2$y",
		enabled:  func(a Answers) bool { return a.Mode2026 },
	},
	{
		name:     "DECRQM-2027",
		query:    "\x1b[?2027$p",
		response: "\x1b[?2027;2$y",
		enabled:  func(a Answers) bool { return a.Mode2027 },
	},
}

// Options configures one child session.
type Options struct {
	// Binary is the path to the mdui binary to run (see BuildBinary).
	Binary string
	// Folder is the directory the child opens. Empty creates a fresh
	// fixture folder (see WriteFixtures).
	Folder string
	// Args are extra command-line flags passed to the binary.
	Args []string
	// Rows and Cols size the pty; zero values use the default geometry.
	Rows, Cols int
	// Answers selects which capability queries get answered.
	Answers Answers
	// Trace, when non-empty, is passed to the child as TEA_TRACE so
	// bubbletea logs every sequence it writes to the file.
	Trace string
	// Env, when non-empty, replaces the derived child environment entirely.
	Env []string
}

// Session is one running mdui child attached to a pty.
type Session struct {
	t        *testing.T
	binary   string
	folder   string
	answers  Answers
	cmd      *exec.Cmd
	master   *os.File
	exited   chan error
	stopOnce sync.Once

	mu       sync.Mutex
	buf      bytes.Buffer
	answered map[string]int
}

// Start launches the mdui binary in a new pty and returns the session. The
// child is cleaned up with the test. On platforms without pty support the
// test is skipped.
func Start(t *testing.T, opts Options) *Session {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("pty regression tests are not supported on windows")
	}
	binary := opts.Binary
	if binary == "" {
		t.Fatal("Options.Binary is required; pass TestBinary(t)")
	}
	folder := opts.Folder
	if folder == "" {
		folder = WriteFixtures(t)
	}
	rows, cols := opts.Rows, opts.Cols
	if rows <= 0 {
		rows = DefaultRows
	}
	if cols <= 0 {
		cols = DefaultCols
	}

	master, slave, err := pty.Open()
	if err != nil {
		t.Skipf("could not open a pty: %v", err)
	}
	if err := pty.Setsize(slave, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)}); err != nil {
		t.Skipf("could not size the pty: %v", err)
	}

	s := &Session{
		t:        t,
		binary:   binary,
		folder:   folder,
		master:   master,
		answers:  opts.Answers,
		answered: make(map[string]int),
		exited:   make(chan error, 1),
	}

	cmd := exec.Command(binary, append(opts.Args, folder)...)
	cmd.Dir = folder
	if len(opts.Env) > 0 {
		cmd.Env = opts.Env
	} else {
		cmd.Env = childEnv(t, opts.Trace)
	}
	cmd.Stdin = slave
	cmd.Stdout = slave
	cmd.Stderr = slave
	if err := cmd.Start(); err != nil {
		master.Close()
		slave.Close()
		t.Fatalf("starting %s: %v", binary, err)
	}
	s.cmd = cmd
	// The child owns the slave side now; holding our copy open would keep
	// the master from ever seeing EOF.
	slave.Close()

	go func() {
		err := cmd.Wait()
		s.exited <- err
		// Closing lets Close() unblock even if a test already consumed the
		// result from the channel.
		close(s.exited)
	}()
	go s.readLoop()

	t.Cleanup(func() { s.Close() })
	return s
}

// Write sends keys to the pty master; the child reads them as keyboard input.
func (s *Session) Write(keys string) {
	s.t.Helper()
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.master.WriteString(keys); err != nil {
		s.t.Fatalf("writing %q to the pty: %v", keys, err)
	}
}

// Output returns a copy of everything the child has written so far.
func (s *Session) Output() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]byte, s.buf.Len())
	copy(out, s.buf.Bytes())
	return out
}

// Size returns the number of bytes written to the pty so far.
func (s *Session) Size() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Len()
}

// WaitFor polls until the output contains marker and reports whether it
// appeared and how long that took.
func (s *Session) WaitFor(marker string, timeout time.Duration) (bool, time.Duration) {
	start := time.Now()
	deadline := start.Add(timeout)
	for time.Now().Before(deadline) {
		if bytes.Contains(s.Output(), []byte(marker)) {
			return true, time.Since(start)
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false, timeout
}

// Settle waits until the output has been quiet for quiet and returns the
// stable byte size, so callers can snapshot "the initial frame" before
// provoking a repaint. max caps the wait.
func (s *Session) Settle(quiet, max time.Duration) int {
	deadline := time.Now().Add(max)
	size := s.Size()
	for time.Now().Before(deadline) {
		time.Sleep(quiet)
		next := s.Size()
		if next == size {
			return size
		}
		size = next
	}
	return size
}

// Wait blocks until the child exits and returns its wait error, or an error
// mentioning the timeout if the child was still running.
func (s *Session) Wait(timeout time.Duration) error {
	select {
	case err := <-s.exited:
		return err
	case <-time.After(timeout):
		return fmt.Errorf("child did not exit within %s", timeout)
	}
}

// Close kills the child if needed and releases the pty. It is idempotent and
// is registered with the test by Start.
func (s *Session) Close() {
	s.stopOnce.Do(func() {
		// Killing an already-exited process reports an error that does not
		// matter; the exited channel then yields the wait result promptly.
		_ = s.cmd.Process.Kill()
		<-s.exited
		_ = s.master.Close()
	})
}

// readLoop copies child output into the session buffer and answers any
// pending capability queries. When the child exits, its slave side closes
// and the master read returns EOF (or EIO on Linux), ending the loop.
func (s *Session) readLoop() {
	chunk := make([]byte, 8192)
	for {
		n, err := s.master.Read(chunk)
		if n > 0 {
			s.mu.Lock()
			s.buf.Write(chunk[:n])
			s.answerQueries()
			s.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// answerQueries writes the configured response for every query occurrence
// not answered yet. Called with s.mu held.
func (s *Session) answerQueries() {
	for _, qr := range queryTable {
		if !qr.enabled(s.answers) {
			continue
		}
		seen := bytes.Count(s.buf.Bytes(), []byte(qr.query))
		for i := s.answered[qr.name]; i < seen; i++ {
			if _, err := s.master.WriteString(qr.response); err != nil {
				return
			}
		}
		if seen > s.answered[qr.name] {
			s.answered[qr.name] = seen
		}
	}
}

// childEnv builds the child's environment: a stable TERM with no terminal
// identity inherited from the test runner, and XDG paths that keep the child
// away from the user's real mdui configuration and session state.
func childEnv(t *testing.T, trace string) []string {
	t.Helper()
	home := t.TempDir()
	stripped := map[string]bool{
		"TERM":                 true,
		"TERM_PROGRAM":         true,
		"TERM_PROGRAM_VERSION": true,
		"SSH_TTY":              true,
		"WT_SESSION":           true,
		"COLORFGBG":            true,
		"TEA_TRACE":            true,
		"XDG_CONFIG_HOME":      true,
		"XDG_STATE_HOME":       true,
		"XDG_DATA_HOME":        true,
		"HOME":                 true,
	}
	env := make([]string, 0, 16)
	for _, kv := range os.Environ() {
		name, _, ok := strings.Cut(kv, "=")
		if ok && !stripped[name] {
			env = append(env, kv)
		}
	}
	env = append(env,
		"TERM="+ChildTerm,
		"HOME="+home,
		"XDG_CONFIG_HOME="+filepath.Join(home, "config"),
		"XDG_STATE_HOME="+filepath.Join(home, "state"),
		"XDG_DATA_HOME="+filepath.Join(home, "data"),
	)
	if trace != "" {
		env = append(env, "TEA_TRACE="+trace)
	}
	return env
}

// WriteFixtures creates a folder of small markdown documents whose preview
// text is unique per file, so a repaint that lands on beta.md is detectable
// by the "Beta document" heading. Filenames sort the tree: alpha.md first,
// then beta.md, then gamma.md.
func WriteFixtures(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"alpha.md": "# Alpha document\n\nAlpha body text.\n",
		"beta.md":  "# Beta document\n\nBeta body text.\n",
		"gamma.md": "# Gamma document\n\nGamma body text.\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("writing fixture %s: %v", name, err)
		}
	}
	return dir
}

// repoRoot returns the repository root. This package lives two levels below
// it, so the result is stable no matter which directory runs the tests.
func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}
