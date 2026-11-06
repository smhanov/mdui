package render

import (
	"os"
	"testing"
	"time"
)

func TestScanResponses(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantSettled bool
		wantDark    bool
	}{
		{name: "black background with BEL", input: "\x1b]11;rgb:0000/0000/0000\a", wantSettled: true, wantDark: true},
		{name: "black background with ST", input: "\x1b]11;rgb:0000/0000/0000\x1b\\", wantSettled: true, wantDark: true},
		{name: "white background", input: "\x1b]11;rgb:ffff/ffff/ffff\a", wantSettled: true, wantDark: false},
		{name: "mid gray counts as light", input: "\x1b]11;rgb:8080/8080/8080\a", wantSettled: true, wantDark: false},
		{name: "dark gray is dark", input: "\x1b]11;rgb:3333/3333/3333\a", wantSettled: true, wantDark: true},
		{name: "malformed color falls back dark", input: "\x1b]11;not-a-color\a", wantSettled: true, wantDark: true},
		{name: "da1 reply", input: "\x1b[?62;1;2;6;9;15;18c", wantSettled: true, wantDark: true},
		{name: "minimal da1 reply", input: "\x1b[?62;c", wantSettled: true, wantDark: true},
		{name: "decrqm reports do not settle", input: "\x1b[?2026;2$y\x1b[?2027;2$y", wantSettled: false, wantDark: true},
		{name: "keystrokes alone never settle", input: "jjk q", wantSettled: false, wantDark: true},
		{name: "partial osc keeps waiting", input: "\x1b]11;rgb:0000/0", wantSettled: false, wantDark: true},
		{name: "key between replies", input: "j\x1b]11;rgb:0000/0000/0000\a", wantSettled: true, wantDark: true},
		{name: "da1 wins when it comes first", input: "\x1b[?62;c\x1b]11;rgb:ffff/ffff/ffff\a", wantSettled: true, wantDark: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dark, settled := scanResponses([]byte(tt.input))
			if settled != tt.wantSettled {
				t.Fatalf("scanResponses(%q) settled = %v, want %v", tt.input, settled, tt.wantSettled)
			}
			if settled && dark != tt.wantDark {
				t.Fatalf("scanResponses(%q) dark = %v, want %v", tt.input, dark, tt.wantDark)
			}
		})
	}
}

func TestScanResponsesSequencesAcrossChunks(t *testing.T) {
	// The probe re-scans the accumulated stream after every read, so a
	// reply split across two reads must settle once the tail arrives.
	acc := []byte("\x1b]11;rgb:ffff/ffff/ff")
	if _, settled := scanResponses(acc); settled {
		t.Fatal("partial reply settled early")
	}
	dark, settled := scanResponses(append(acc, "ff\a"...))
	if !settled || dark {
		t.Fatalf("completed reply: settled = %v, dark = %v, want true, false", settled, dark)
	}
}

func TestProbeBackgroundNonTerminal(t *testing.T) {
	// Pipes are not terminals: the probe must skip the query entirely and
	// fall back to dark with nothing consumed.
	in, out, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	defer out.Close()

	start := time.Now()
	dark, consumed := ProbeBackground(in, out)
	if !dark {
		t.Fatal("ProbeBackground on pipes returned dark = false, want true")
	}
	if len(consumed) != 0 {
		t.Fatalf("ProbeBackground on pipes consumed %d bytes, want 0", len(consumed))
	}
	if time.Since(start) > 100*time.Millisecond {
		t.Fatalf("ProbeBackground on pipes took %s, want an immediate fallback", time.Since(start).Round(time.Millisecond))
	}
}
