// terminalscreen reconstructs a PTY screen for executable UX assertions.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/vt"
)

type event struct {
	Width, Height int
	Data          []byte
}

func main() {
	if err := replay(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// Each batch advances the same screen and returns its current frame.
func replay(input io.Reader, output io.Writer) error {
	decoder, encoder := json.NewDecoder(input), json.NewEncoder(output)
	var terminal *vt.SafeEmulator
	for {
		var events []event
		if err := decoder.Decode(&events); err == io.EOF {
			return nil
		} else if err != nil {
			return err
		}
		if terminal == nil {
			if len(events) == 0 || events[0].Width <= 0 || events[0].Height <= 0 {
				return fmt.Errorf("terminalscreen requires initial dimensions")
			}
			terminal = vt.NewSafeEmulator(events[0].Width, events[0].Height)
			go io.Copy(io.Discard, terminal)
			defer terminal.InputPipe().(io.Closer).Close()
		}
		for _, event := range events {
			if event.Width > 0 && event.Height > 0 {
				terminal.Resize(event.Width, event.Height)
			}
			if _, err := terminal.Write(event.Data); err != nil {
				return err
			}
		}
		if err := encoder.Encode(frame(terminal)); err != nil {
			return err
		}
	}
}

type screen struct {
	Rows []string `json:"rows"`
	ANSI string   `json:"ansi"`
}

func frame(terminal *vt.SafeEmulator) screen {
	rows := make([]string, terminal.Height())
	for y := range rows {
		var row strings.Builder
		for x := 0; x < terminal.Width(); x++ {
			cell := terminal.CellAt(x, y)
			if cell == nil {
				row.WriteByte(' ')
			} else if cell.Width > 0 {
				row.WriteString(cell.Content)
			}
		}
		rows[y] = row.String()
	}
	return screen{rows, terminal.Render()}
}
