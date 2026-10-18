package cli

import (
	"strings"
	"testing"
)

const missingSession = noConversation + "b0a60826-a83b-46da-a8a9-85bc027fbcb2"

func TestResumeDiagnosticAcrossTerminalChunks(t *testing.T) {
	// Claude emits terminal cleanup controls around its startup error.
	for _, raw := range []string{
		"\x1b7\x1b[r\x1b8\x1b[?25h\x1b[38;2;255;107;128m" + missingSession + "\x1b[39m\r\r\n\x1b[?25h\x1b(B\x0f\x1b[?1016l\x1b[?1006l\x1b[?2004l\x1b[<u",
		"No\x1b[4Gconversation\x1b[17Gfound\x1b[23Gwith\x1b[28Gsession\r\nID:\r\nb0a60826-a83b-46da-a8a9-85bc027fbcb2\r\n\x1b]0;\x07\x1b]0;\x1b\\",
	} {
		for size := 1; size <= len(raw); size++ {
			d := resumeDiagnostic{enabled: true, expected: missingSession}
			for start := 0; start < len(raw); start += size {
				chunk := raw[start:min(start+size, len(raw))]
				if n, err := d.Write([]byte(chunk)); err != nil || n != len(chunk) {
					t.Fatalf("chunk size %d: n=%d err=%v", size, n, err)
				}
			}
			if !d.enabled || d.text() != compactDiagnostic(missingSession) {
				t.Fatalf("chunk size %d lost the startup diagnostic: %q", size, d.text())
			}
			if d.missingConversation(nil) {
				t.Fatal("a successful process exit requested recovery")
			}
		}
	}
}

func TestResumeDiagnosticDiscardsOtherOutput(t *testing.T) {
	for _, chunks := range [][]string{
		{"Claude Code\n", missingSession},
		{"Authentication required\n", missingSession},
		{missingSession, "\nOther failure"},
		{noConversation + "e4a60826-a83b-46da-a8a9-85bc027fbcb2"},
		{strings.Repeat(" ", 4097), missingSession},
	} {
		d := resumeDiagnostic{enabled: true, expected: missingSession}
		for _, chunk := range chunks {
			_, _ = d.Write([]byte(chunk))
		}
		if d.enabled || len(d.data) != 0 {
			t.Fatal("retained output beyond the missing-conversation diagnostic")
		}
	}
	d := resumeDiagnostic{}
	_, _ = d.Write([]byte(missingSession))
	if len(d.data) != 0 {
		t.Fatal("fresh launches retained resume diagnostics")
	}
}
