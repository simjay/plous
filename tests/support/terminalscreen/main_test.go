package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestReplayKeepsScreenAcrossBatches(t *testing.T) {
	batches := [][]event{
		{{Width: 20, Height: 6}, {Data: []byte("\x1b[?1049h\x1b[2J\x1b[Hobsolete\x1b[2;1Hchart-00")}},
		{{Data: []byte("\x1b[2;7H12\x1b[1;1H\x1b[2")}},
		{{Data: []byte("KReady \xe7\x95")}},
		{{Data: []byte("\x8c")}, {Width: 24, Height: 8}, {Data: []byte("\x1b[8;1Hfooter")}},
	}
	var input, output bytes.Buffer
	for _, batch := range batches {
		if err := json.NewEncoder(&input).Encode(batch); err != nil {
			t.Fatal(err)
		}
	}
	if err := replay(&input, &output); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&output)
	var got screen
	for range batches {
		if err := decoder.Decode(&got); err != nil {
			t.Fatal(err)
		}
	}
	if len(got.Rows) != 8 || strings.TrimRight(got.Rows[0], " ") != "Ready 界" ||
		strings.TrimRight(got.Rows[1], " ") != "chart-12" || strings.TrimRight(got.Rows[7], " ") != "footer" {
		t.Fatalf("lost screen state between batches: %#v", got.Rows)
	}
	if strings.Contains(got.ANSI, "obsolete") {
		t.Fatal("erased output remained visible")
	}
}

func TestReplayRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{"[]", `[{'Width':20}]`, `[{"Data":"eA=="}]`} {
		if err := replay(strings.NewReader(input), &bytes.Buffer{}); err == nil {
			t.Errorf("accepted invalid input %q", input)
		}
	}
}
