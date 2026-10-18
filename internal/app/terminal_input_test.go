package app

import (
	"bytes"
	uv "github.com/charmbracelet/ultraviolet"
	"io"
	"os"
	"testing"
	"time"
	"unicode/utf8"
)

func TestUTF8TerminalInputPreservesSplitUTF8(t *testing.T) {
	text := []byte("\x1b[200~UPPER café 界 🌍 bb cc :quit\n\x1b[201~")
	for split := 1; split < len(text); split++ {
		r, w, err := os.Pipe()
		if err != nil {
			t.Fatal(err)
		}
		input := &utf8TerminalInput{File: r}
		var got []byte
		for _, part := range [][]byte{text[:split], text[split:]} {
			if _, err := w.Write(part); err != nil {
				t.Fatal(err)
			}
			buf := make([]byte, 4096)
			n, err := input.Read(buf)
			if err != nil || !utf8.Valid(buf[:n]) {
				t.Fatalf("split %d: invalid input %q, error %v", split, buf[:n], err)
			}
			got = append(got, buf[:n]...)
		}
		r.Close()
		w.Close()
		if !bytes.Equal(got, text) {
			t.Fatalf("split %d: got %q, want %q", split, got, text)
		}
	}
}

func TestUTF8TerminalInputPendingRuneKeepsCancellation(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	input := &utf8TerminalInput{File: r}
	cancellable, err := uv.NewCancelReader(input)
	if err != nil {
		t.Fatal(err)
	}
	defer cancellable.Close()
	if _, err := w.Write([]byte{0xc3}); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4096)
	if n, err := cancellable.Read(buf); n != 0 || err != nil {
		t.Fatalf("partial rune escaped: %d, %v", n, err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := cancellable.Read(buf)
		done <- err
	}()
	if !cancellable.Cancel() {
		t.Fatal("terminal input lost cancellation support")
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("read was not cancelled")
		}
	case <-time.After(time.Second):
		t.Fatal("read remained blocked on a partial character")
	}
}

func TestUTF8TerminalInputEOFAndShortBuffers(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	input := &utf8TerminalInput{File: r}
	if n, err := input.Read(nil); n != 0 || err != nil {
		t.Fatalf("empty read: %d, %v", n, err)
	}
	if _, err := input.Read(make([]byte, 1)); err != io.ErrShortBuffer {
		t.Fatalf("short buffer: %v", err)
	}
	w.Close()
	if n, err := input.Read(make([]byte, 4096)); n != 0 || err != io.EOF {
		t.Fatalf("EOF: %d, %v", n, err)
	}
}
