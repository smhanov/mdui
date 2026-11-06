package apptest

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Timing budget for driving the real binary. The values are generous on
// purpose: the silent-terminal case includes a two-second probe timeout
// before the first frame, and repaint detection must stay robust on loaded
// CI machines.
const (
	// firstFrameTimeout covers app startup plus the background probe that
	// a silent terminal leaves to time out.
	firstFrameTimeout = 10 * time.Second
	// settleQuiet is how long the output must stay still before a frame
	// counts as settled, settleMax caps that wait.
	settleQuiet = 250 * time.Millisecond
	settleMax   = 3 * time.Second
	// keyGap spaces scripted keystrokes like a human typing.
	keyGap = 500 * time.Millisecond
	// repaintTimeout is the budget for a frame-changing key to produce
	// visible output.
	repaintTimeout = 1500 * time.Millisecond
	// minRepaintBytes is the smallest output growth that still counts as a
	// repaint; a real frame rewrite at 120x40 is several kilobytes.
	minRepaintBytes = 512
	// quitTimeout is how long q gets to end the process cleanly.
	quitTimeout = 5 * time.Second
)

// TestRepaintWithoutQueryResponses encodes the reported freeze: in a pty
// whose slave never answers capability queries, the first frame paints and
// then a frame-changing key must still produce a repaint, and q must still
// quit cleanly.
func TestRepaintWithoutQueryResponses(t *testing.T) {
	driveRepaint(t, Answers{})
}

// TestRepaintWithAllQueriesAnswered runs the same script against a fully
// responsive fake terminal, guarding the healthy path.
func TestRepaintWithAllQueriesAnswered(t *testing.T) {
	driveRepaint(t, All())
}

// driveRepaint starts mdui with the given answer set, presses j until the
// second file is previewed, and asserts a repaint happened. It logs the byte
// accounting so failures show the frozen-vs-healthy shape directly.
func driveRepaint(t *testing.T, answers Answers) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping pty test in -short mode")
	}

	s := Start(t, Options{Binary: BuildBinary(t), Answers: answers})

	// The initial frame lists the fixture files in the tree.
	if ok, elapsed := s.WaitFor("alpha.md", firstFrameTimeout); !ok {
		t.Fatalf("initial frame never appeared within %s (wrote %d bytes)",
			firstFrameTimeout, s.Size())
	} else {
		t.Logf("initial frame after %s (%d bytes)", elapsed.Round(time.Millisecond), s.Size())
	}
	frame := s.Size()
	if strings.Contains(string(s.Output()), "Beta document") {
		t.Fatalf("unexpected beta preview before any key press; test markers are wrong")
	}
	if size := s.Settle(settleQuiet, settleMax); size != frame {
		t.Logf("frame kept growing to %d bytes before settling", size)
		frame = size
	}

	s.Write("j")
	time.Sleep(keyGap)
	s.Write("j")

	found, elapsed := s.WaitFor("Beta document", repaintTimeout+settleMax)
	growth := s.Size() - frame
	t.Logf("post-key output growth: %d bytes in %s (beta preview found: %v)",
		growth, elapsed.Round(time.Millisecond), found)
	if !found {
		t.Fatalf("beta preview never rendered after j j within %s: "+
			"frame=%d bytes, final=%d bytes, growth=%d bytes — display looks frozen",
			repaintTimeout+settleMax, frame, s.Size(), growth)
	}
	if growth < minRepaintBytes {
		t.Fatalf("repaint suspiciously small: %d bytes after j j (want >= %d)",
			growth, minRepaintBytes)
	}

	s.Write("q")
	if err := s.Wait(quitTimeout); err != nil {
		t.Fatalf("q did not exit cleanly: %v", err)
	}
	t.Logf("clean exit after q; total output %d bytes", s.Size())
}

// TestLiveReloadUpdatesTreeAndViewer drives the real binary: writing the open
// document must repaint it in place, and creating a markdown file must add it
// to the tree without any key press.
func TestLiveReloadUpdatesTreeAndViewer(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping pty test in -short mode")
	}

	root := WriteFixtures(t)
	s := Start(t, Options{Binary: BuildBinary(t), Folder: root, Answers: All()})
	if ok, elapsed := s.WaitFor("alpha.md", firstFrameTimeout); !ok {
		t.Fatalf("initial frame never appeared within %s", firstFrameTimeout)
	} else {
		t.Logf("initial frame after %s", elapsed.Round(time.Millisecond))
	}

	s.Write("j")
	if ok, elapsed := s.WaitFor("Alpha document", repaintTimeout+settleMax); !ok {
		t.Fatalf("alpha preview never rendered after j within %s", repaintTimeout+settleMax)
	} else {
		t.Logf("alpha preview after %s", elapsed.Round(time.Millisecond))
	}

	alpha := filepath.Join(root, "alpha.md")
	file, err := os.OpenFile(alpha, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("opening alpha.md: %v", err)
	}
	if _, err := file.WriteString("\n\nliveupdate marker text\n"); err != nil {
		file.Close()
		t.Fatalf("appending to alpha.md: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("closing alpha.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "delta.md"), []byte("# Delta document\n\nDelta body text.\n"), 0o644); err != nil {
		t.Fatalf("creating delta.md: %v", err)
	}

	if ok, elapsed := s.WaitFor("liveupdate", 3*time.Second); !ok {
		t.Fatalf("open document did not re-render after a write within 3s (wrote %d bytes)", s.Size())
	} else {
		t.Logf("document refreshed after %s", elapsed.Round(time.Millisecond))
	}
	if ok, elapsed := s.WaitFor("delta.md", 3*time.Second); !ok {
		t.Fatalf("new markdown file did not appear in the tree within 3s (wrote %d bytes)", s.Size())
	} else {
		t.Logf("tree refreshed after %s", elapsed.Round(time.Millisecond))
	}

	s.Write("q")
	if err := s.Wait(quitTimeout); err != nil {
		t.Fatalf("q did not exit cleanly after live updates: %v", err)
	}
}

// TestKeysTypedDuringStartupSurvive drives the reported experience: on a
// silent pty, keystrokes typed while mdui is still starting up (a scripted
// or serial session types before the first frame exists) must not vanish.
// Before the bounded background probe, every key typed during the first
// seconds was consumed by the terminal query and the screen froze on the
// initial frame — the shape of GitHub issue #1.
func TestKeysTypedDuringStartupSurvive(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping pty test in -short mode")
	}

	s := Start(t, Options{Binary: BuildBinary(t), Answers: Answers{}})

	// Type like a script feeding a silent pty: the first key lands while
	// startup queries are still in flight, the rest after the first frame.
	time.Sleep(150 * time.Millisecond)
	s.Write("j")
	time.Sleep(1400 * time.Millisecond)
	s.Write("j")
	time.Sleep(1200 * time.Millisecond)
	s.Write("q")

	found, elapsed := s.WaitFor("Beta document", firstFrameTimeout)
	if !found {
		t.Fatalf("keys typed during startup were lost: beta preview never "+
			"rendered within %s (wrote %d bytes)", elapsed, s.Size())
	}
	if err := s.Wait(quitTimeout); err != nil {
		t.Fatalf("q did not exit cleanly: %v (wrote %d bytes)", err, s.Size())
	}
	t.Logf("keys survived startup; total output %d bytes", s.Size())
}

// TestExplicitStyleSkipsStartupQueries pins the cheap path: with an explicit
// style there is nothing to detect, so mdui must not send any background or
// capability query and the first frame appears without waiting on answers.
func TestExplicitStyleSkipsStartupQueries(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping pty test in -short mode")
	}

	s := Start(t, Options{
		Binary:  BuildBinary(t),
		Args:    []string{"--style", "dark"},
		Answers: Answers{},
	})
	if ok, elapsed := s.WaitFor("alpha.md", firstFrameTimeout); !ok {
		t.Fatalf("initial frame never appeared within %s", firstFrameTimeout)
	} else if elapsed > 400*time.Millisecond {
		t.Fatalf("explicit style still delayed startup for %s, want < 400ms", elapsed.Round(time.Millisecond))
	}
	if out := string(s.Output()); strings.Contains(out, "\x1b]11;?\a") || strings.Contains(out, "\x1b[c") {
		t.Fatal("explicit style still sent a background or DA1 query on the wire")
	}
	s.Write("q")
	if err := s.Wait(quitTimeout); err != nil {
		t.Fatalf("q did not exit cleanly: %v", err)
	}
}

// TestQueriesOnStartup records which capability queries mdui writes on a
// silent terminal, so the trigger analysis in the plan stays checkable.
func TestQueriesOnStartup(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping pty test in -short mode")
	}
	s := Start(t, Options{Binary: BuildBinary(t), Answers: All()})
	// Let the app reach its first frame: before that, the terminal is not
	// in raw mode yet and a queued q would be swallowed by the startup
	// probe instead of quitting the app.
	if ok, _ := s.WaitFor("alpha.md", firstFrameTimeout); !ok {
		t.Fatalf("initial frame never appeared within %s", firstFrameTimeout)
	}
	s.Write("q")
	if err := s.Wait(quitTimeout); err != nil {
		t.Fatalf("q did not exit cleanly: %v", err)
	}
	out := string(s.Output())
	for _, qr := range queryTable {
		n := strings.Count(out, qr.query)
		t.Logf("%-12s sent %dx", qr.name, n)
		if n == 0 {
			t.Errorf("expected at least one %s query on the wire", qr.name)
		}
	}
}
