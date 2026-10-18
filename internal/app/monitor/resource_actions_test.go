package monitor

import (
	"context"
	"io"
	"strings"
	"testing"

	kubecommon "github.com/simjay/plous/internal/integrations/kubernetes/common"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestExecArgumentsAreParsedWithoutLocalShellExpansion(t *testing.T) {
	args, err := SplitExecCommand(`/bin/sh -c 'echo "$HOME"; printf "hello world"'`)
	if err != nil || len(args) != 3 || args[2] != `echo "$HOME"; printf "hello world"` {
		t.Fatalf("arguments lost quoting: %#v %v", args, err)
	}
	args, err = SplitExecCommand(`echo $(touch /tmp/never)`)
	if err != nil || strings.Join(args, " ") != `echo $(touch /tmp/never)` {
		t.Fatal("command substitution was interpreted locally")
	}
	for _, input := range []string{"", `sh 'unterminated`, `sh trailing\`} {
		if _, err := SplitExecCommand(input); err == nil {
			t.Fatalf("invalid command accepted: %q", input)
		}
	}
}

func TestTerminalBufferCancellationUnblocksRead(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	input := &TerminalInput{Ctx: ctx, Chunks: make(chan []byte, 2)}
	_, _ = input.Write([]byte("hello"))
	buf := make([]byte, 2)
	if n, _ := input.Read(buf); n != 2 || string(buf) != "he" {
		t.Fatal("input lost partial read")
	}
	buf = make([]byte, 4)
	if n, _ := input.Read(buf); string(buf[:n]) != "llo" {
		t.Fatal("input remainder lost")
	}
	cancel()
	if _, err := input.Read(buf); err != io.EOF {
		t.Fatal("cancelled terminal input still blocked")
	}
}

func TestRevisionValueCopiesOnlyImmutableSnapshot(t *testing.T) {
	object := unstructured.Unstructured{Object: map[string]any{"metadata": map[string]any{"name": "revision"}, "data": map[string]any{"spec": map[string]any{"image": "checkout:v1"}}}}
	values := ResourceValues(kubecommon.Resource{Name: "controllerrevisions", Group: "apps"}, object)
	if len(values) != 1 || !strings.Contains(values[0].Text, "checkout:v1") || strings.Contains(values[0].Text, "metadata") {
		t.Fatalf("unexpected revision snapshot: %#v", values)
	}
}
