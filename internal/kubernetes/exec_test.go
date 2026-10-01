package kubernetes

import (
	"bytes"
	"context"
	"fmt"
	"k8s.io/client-go/rest"
	"net/http"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestExecStreamsInputOutputAndPreservesCommandArguments(t *testing.T) {
	var input string
	client := testClient(t, func(w http.ResponseWriter, request *http.Request) {
		if !strings.HasSuffix(request.URL.Path, "/exec") {
			pod := testPod("pod", "uid")
			pod["status"] = map[string]any{"containerStatuses": []any{map[string]any{"name": "app", "state": map[string]any{"running": map[string]any{}}}}}
			reply(w, pod)
			return
		}
		if request.Method != "GET" || request.URL.Query().Get("container") != "app" || !reflect.DeepEqual(request.URL.Query()["command"], []string{"/bin/sh", "-c", "printf 'literal argument'"}) {
			t.Errorf("incorrect exec request: %s %s", request.Method, request.URL)
		}
		upgrader := websocket.Upgrader{Subprotocols: []string{"v5.channel.k8s.io"}}
		connection, err := upgrader.Upgrade(w, request, nil)
		if err != nil {
			t.Error(err)
			return
		}
		defer connection.Close()
		for {
			_, message, err := connection.ReadMessage()
			if err != nil {
				t.Error(err)
				return
			}
			if len(message) > 0 && message[0] == 0 {
				input += string(message[1:])
			}
			if len(message) == 2 && message[0] == 255 && message[1] == 0 {
				break
			}
		}
		_ = connection.WriteMessage(websocket.BinaryMessage, append([]byte{1}, []byte("output\n")...))
		_ = connection.WriteMessage(websocket.BinaryMessage, append([]byte{2}, []byte("warning\n")...))
		_ = connection.WriteMessage(websocket.BinaryMessage, append([]byte{3}, []byte(`{"status":"Success"}`)...))
		_ = connection.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var stdout, stderr bytes.Buffer
	err := client.Exec(ctx, ExecOptions{Namespace: "shop", Pod: "pod", UID: "uid", Container: "app", Command: []string{"/bin/sh", "-c", "printf 'literal argument'"}, Stdin: strings.NewReader("input\n"), Stdout: &stdout, Stderr: &stderr})
	if err != nil || input != "input\n" || stdout.String() != "output\n" || stderr.String() != "warning\n" {
		t.Fatalf("stream: input=%q stdout=%q stderr=%q err=%v", input, stdout.String(), stderr.String(), err)
	}
}

func TestExecRejectsReplacedOrStoppedContainerBeforeConnecting(t *testing.T) {
	for _, scenario := range []string{"replaced", "stopped"} {
		t.Run(scenario, func(t *testing.T) {
			var connections atomic.Int32
			client := testClient(t, func(w http.ResponseWriter, request *http.Request) {
				if strings.HasSuffix(request.URL.Path, "/exec") {
					connections.Add(1)
					return
				}
				uid := "uid"
				if scenario == "replaced" {
					uid = "new-uid"
				}
				reply(w, testPod("pod", uid))
			})
			err := client.Exec(context.Background(), ExecOptions{Namespace: "shop", Pod: "pod", UID: "uid", Container: "app", Command: []string{"sh"}})
			if err == nil || connections.Load() != 0 {
				t.Fatalf("exec reached an invalid target: %v", err)
			}
		})
	}
}

func TestLogsApplyRangeAndTimestampOptions(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, request *http.Request) {
		query := request.URL.Query()
		if query.Get("timestamps") != "false" || query.Get("sinceSeconds") != "600" || query.Get("tailLines") != "50" || query.Get("follow") != "true" {
			t.Errorf("wrong log options: %s", request.URL)
		}
		_, _ = w.Write([]byte("entry\n"))
	})
	since, tail, timestamps := int64(600), int64(50), false
	err := client.Logs(context.Background(), LogOptions{Namespace: "shop", Pod: "pod", Container: "app", Follow: true, SinceSeconds: &since, TailLines: &tail, Timestamps: &timestamps}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
}

func TestLogsReportsConnectedBeforeReceivingOutput(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, request *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		<-request.Context().Done()
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	connected, done := make(chan struct{}), make(chan error, 1)
	go func() {
		done <- client.Logs(ctx, LogOptions{Namespace: "shop", Pod: "pod", Container: "app", Follow: true, OnConnect: func() { close(connected) }}, func(string) { t.Error("idle stream emitted output") })
	}()
	select {
	case <-connected:
	case <-ctx.Done():
		t.Fatal("idle stream never reported its established connection")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("cancellation did not stop the idle stream")
	}
}

func TestTimeRangeDoesNotSilentlyAddDefaultTailLimit(t *testing.T) {
	client := testClient(t, func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Query().Get("sinceSeconds") != "3600" || request.URL.Query().Has("tailLines") {
			t.Errorf("time range unexpectedly truncated: %s", request.URL)
		}
	})
	since := int64(3600)
	if err := client.Logs(context.Background(), LogOptions{Namespace: "shop", Pod: "pod", Container: "app", SinceSeconds: &since}, func(string) {}); err != nil {
		t.Fatal(err)
	}
}

func TestLogsVerifyCapturedPodBeforeStartingStream(t *testing.T) {
	for _, uid := range []string{"captured", "replacement"} {
		t.Run(uid, func(t *testing.T) {
			var streamRequests atomic.Int32
			client := testClient(t, func(w http.ResponseWriter, request *http.Request) {
				if request.URL.Path == "/api/v1/namespaces/shop/pods/pod" {
					reply(w, testPod("pod", uid))
					return
				}
				if request.URL.Path != "/api/v1/namespaces/shop/pods/pod/log" {
					t.Errorf("unexpected log target: %s", request.URL)
				}
				streamRequests.Add(1)
				_, _ = w.Write([]byte("captured log\n"))
			})
			var text strings.Builder
			err := client.Logs(context.Background(), LogOptions{Namespace: "shop", Pod: "pod", Container: "app", UID: "captured", Previous: true}, func(line string) { text.WriteString(line) })
			if uid == "replacement" {
				if err == nil || streamRequests.Load() != 0 || text.Len() != 0 {
					t.Fatalf("replacement Pod was streamed: %q %v", text.String(), err)
				}
			} else if err != nil || streamRequests.Load() != 1 || text.String() != "captured log\n" {
				t.Fatalf("captured Pod logs unavailable: %q %v", text.String(), err)
			}
		})
	}
}

func TestExecDiagnosticsKeepCauseWithoutTransportCredentials(t *testing.T) {
	client := &Client{config: &rest.Config{BearerToken: "private-token", Username: "user", Password: "private-password"}}
	err := fmt.Errorf("POST https://user:private-password@cluster/exec?token=private-token: executable not found\nAuthorization: Bearer private-token\nmore detail")
	text := client.ExecErrorText(err)
	for _, secret := range []string{"private-token", "private-password", "token=", "user:"} {
		if strings.Contains(text, secret) {
			t.Fatalf("credential leaked: %q", text)
		}
	}
	if !strings.Contains(text, "executable not found") || !strings.Contains(text, "more detail") {
		t.Fatal("cause was replaced with a generic error")
	}
	if client.ExecErrorText(nil) != "" {
		t.Fatal("successful Exec gained an error")
	}
}
