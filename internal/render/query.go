package render

import (
	"bytes"
	"image/color"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"github.com/lucasb-eyer/go-colorful"
	"github.com/muesli/cancelreader"
)

// probeTimeout bounds how long the background probe may delay startup and
// hold the terminal input. A terminal that answers capability queries
// responds in milliseconds; a silent pty (script, expect, serial consoles,
// some CI wrappers) gets the dark fallback after this budget instead of
// blocking for the two-second lipgloss query, which runs twice and used to
// swallow every keystroke typed during startup.
const probeTimeout = 500 * time.Millisecond

// backgroundQuery asks the terminal for its background color (OSC 11) and,
// as lipgloss does, for its primary device attributes (DA1), whose reply
// also ends the probe when no color report arrives.
var backgroundQuery = ansi.RequestBackgroundColor + ansi.RequestPrimaryDeviceAttributes

// ProbeBackground asks the terminal whether its background is dark, using
// the same query and fallback rules as lipgloss.HasDarkBackground but with
// a bounded wait. It returns the darkness verdict and every input byte the
// probe consumed from the terminal while waiting: those bytes can contain
// keystrokes typed before the program started reading input, so callers
// replay them into the program's input reader instead of dropping them.
// When nothing answers within probeTimeout the verdict is dark, which is
// the fallback lipgloss uses as well.
func ProbeBackground(in, out *os.File) (dark bool, consumed []byte) {
	dark = true

	if os.Getenv("TERM") == "dumb" || !term.IsTerminal(in.Fd()) || !term.IsTerminal(out.Fd()) {
		return dark, nil
	}

	state, err := term.MakeRaw(in.Fd())
	if err != nil {
		return dark, nil
	}
	defer term.Restore(in.Fd(), state) //nolint:errcheck

	if _, err := out.WriteString(backgroundQuery); err != nil {
		return dark, nil
	}

	reader, err := cancelreader.NewReader(in)
	if err != nil {
		return dark, nil
	}

	var (
		mu  sync.Mutex
		acc bytes.Buffer
	)
	appendRead := func(b []byte) {
		mu.Lock()
		defer mu.Unlock()
		acc.Write(b)
	}
	snapshot := func() []byte {
		mu.Lock()
		defer mu.Unlock()
		return bytes.Clone(acc.Bytes())
	}

	settled := make(chan bool, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		chunk := make([]byte, 256)
		for {
			n, err := reader.Read(chunk)
			if n > 0 {
				appendRead(chunk[:n])
				mu.Lock()
				verdict, complete := scanResponses(acc.Bytes())
				mu.Unlock()
				if complete {
					settled <- verdict
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	select {
	case verdict := <-settled:
		return verdict, snapshot()
	case <-time.After(probeTimeout):
		// Stop the read loop first, then take the snapshot after the
		// goroutine has flushed its last read, so no consumed byte is lost.
		_ = reader.Cancel()
		<-done
		return true, snapshot()
	}
}

// scanResponses walks the accumulated probe input and reports the first
// decisive reply, mirroring lipgloss's interpretation rules: a well-formed
// OSC 11 background report decides by the color's HSL lightness, a DA1
// reply ends the probe with the dark fallback, and anything else (typed
// keystrokes, partial replies) keeps the probe waiting. settled is false
// while the probe is still waiting for a usable reply.
func scanResponses(acc []byte) (dark, settled bool) {
	dark = true
	i := 0
	for i < len(acc) {
		if acc[i] != '\x1b' || i+1 >= len(acc) {
			i++ // typed keystrokes and noise between replies
			continue
		}
		switch acc[i+1] {
		case ']': // OSC reply, BEL or ST terminated
			end := bytes.IndexByte(acc[i:], '\a')
			st := bytes.Index(acc[i:], []byte("\x1b\\"))
			switch {
			case st >= 0 && (end == -1 || st < end):
				if ok := oscReport(acc[i+2:i+st], &dark); ok {
					return dark, true
				}
				i += st + 2
			case end >= 0:
				if ok := oscReport(acc[i+2:i+end], &dark); ok {
					return dark, true
				}
				i += end + 1
			default:
				return dark, false // reply still arriving
			}
		case '[': // CSI reply; DA1 reports are private-mode sequences ending in c
			params := acc[i+2:]
			j := 0
			for j < len(params) && isCSIParam(params[j]) {
				j++
			}
			if j >= len(params) {
				return dark, false // reply still arriving
			}
			if params[j] == 'c' && len(params) > 0 && params[0] == '?' {
				return true, true // DA1: capabilities without a color report
			}
			i += 2 + j + 1
		default:
			i += 2 // two-byte escape we do not care about
		}
	}
	return dark, false
}

// oscReport interprets one OSC payload (between the introducer and its
// terminator). A background report with exactly one field decides the
// verdict; malformed or unrelated reports are skipped. The ok result
// reports whether the probe is settled.
func oscReport(payload []byte, dark *bool) bool {
	parts := bytes.SplitN(payload, []byte(";"), 2)
	if len(parts) != 2 || !bytes.Equal(parts[0], []byte("11")) {
		return false
	}
	c := ansi.XParseColor(string(parts[1]))
	if c == nil {
		return true // malformed color: keep lipgloss's dark fallback
	}
	*dark = isDarkColor(c)
	return true
}

// isCSIParam reports whether b is a byte that can appear inside a CSI
// sequence before its final byte: parameter bytes 0x30-0x3F (digits, : ;,
// < = > ?) or intermediate bytes 0x20-0x2F.
func isCSIParam(b byte) bool {
	switch {
	case b >= '0' && b <= '?':
		return true
	case b >= ' ' && b <= '/':
		return true
	}
	return false
}

// isDarkColor applies lipgloss's rule: a color is dark when its HSL
// lightness is below one half.
func isDarkColor(c color.Color) bool {
	col, ok := colorful.MakeColor(c)
	if !ok {
		return true
	}
	_, _, l := col.Hsl()
	return l < 0.5 //nolint:mnd
}
