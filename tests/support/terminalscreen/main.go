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
	var events []event
	if err := json.NewDecoder(os.Stdin).Decode(&events); err != nil || len(events) == 0 {
		fmt.Fprintln(os.Stderr, "terminalscreen requires terminal events")
		os.Exit(1)
	}
	terminal := vt.NewSafeEmulator(events[0].Width, events[0].Height)
	go io.Copy(io.Discard, terminal)
	for _, event := range events {
		if event.Width > 0 && event.Height > 0 {
			terminal.Resize(event.Width, event.Height)
		}
		if _, err := terminal.Write(event.Data); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
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
	if err := json.NewEncoder(os.Stdout).Encode(struct {
		Rows []string `json:"rows"`
		ANSI string   `json:"ansi"`
	}{rows, terminal.Render()}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
