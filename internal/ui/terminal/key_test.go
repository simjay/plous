package terminal

import (
	"io"
	"testing"
	"time"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
)

func TestNativeModifiedKeysReachTerminal(t *testing.T) {
	for _, test := range []struct {
		key  uv.KeyPressEvent
		want string
	}{
		{uv.KeyPressEvent{Code: uv.KeyTab, Mod: uv.ModShift}, "\x1b[Z"},
		{uv.KeyPressEvent{Code: uv.KeyEnter, Mod: uv.ModShift}, "\x1b[13;2u"},
		{uv.KeyPressEvent{Code: uv.KeyLeft, Mod: uv.ModCtrl}, "\x1b[1;5D"},
		{uv.KeyPressEvent{Code: 'j', Mod: uv.ModCtrl}, "\n"},
		{uv.KeyPressEvent{Code: 'A', Text: "A", Mod: uv.ModShift}, "A"},
		{uv.KeyPressEvent{Code: '한', Text: "한"}, "한"},
		{uv.KeyPressEvent{Code: uv.KeyLeft, IsRepeat: true}, "\x1b[D"},
		{uv.KeyPressEvent{Code: uv.KeyBackspace, IsRepeat: true}, "\x7f"},
		{uv.KeyPressEvent{Code: 'j', Mod: uv.ModCtrl, IsRepeat: true}, "\n"},
		{uv.KeyPressEvent{Code: uv.KeyTab, Mod: uv.ModScrollLock}, "\t"},
		{uv.KeyPressEvent{Code: uv.KeyDelete, Mod: uv.ModCtrl}, "\x1b[3;5~"},
		{uv.KeyPressEvent{Code: uv.KeyPgUp, Mod: uv.ModShift}, "\x1b[5;2~"},
		{uv.KeyPressEvent{Code: uv.KeyPgDown, Mod: uv.ModAlt}, "\x1b[6;3~"},
		{uv.KeyPressEvent{Code: uv.KeyInsert, Mod: uv.ModShift}, "\x1b[2;2~"},
		{uv.KeyPressEvent{Code: uv.KeyKpEnter, Mod: uv.ModShift}, "\x1b[13;2u"},
		{uv.KeyPressEvent{Code: uv.KeyUp, Mod: uv.ModMeta}, "\x1b[1;9A"},
		{uv.KeyPressEvent{Code: uv.KeyEnter, Mod: uv.ModMeta}, "\x1b[13;33u"},
		{uv.KeyPressEvent{Code: uv.KeyKpLeft}, "\x1b[D"},
		{uv.KeyPressEvent{Code: uv.KeyKpDelete, Mod: uv.ModCtrl}, "\x1b[3;5~"},
		{uv.KeyPressEvent{Code: uv.KeyF2, Mod: uv.ModCtrl}, "\x1b[1;5Q"},
		{uv.KeyPressEvent{Code: uv.KeyF12, Mod: uv.ModShift}, "\x1b[24;2~"},
		{uv.KeyPressEvent{Code: uv.KeyBackspace, Mod: uv.ModCtrl}, "\x1b[127;5u"},
		{uv.KeyPressEvent{Code: 'h', Mod: uv.ModCtrl}, "\x08"},
	} {
		screen := vt.NewSafeEmulator(30, 10)
		pipe := screen.InputPipe().(io.Closer)
		got := make(chan string, 1)
		go func() { p := make([]byte, 64); n, _ := screen.Read(p); got <- string(p[:n]) }()
		sent := make(chan struct{})
		go func() { SendKey(screen, test.key); close(sent) }()
		select {
		case text := <-got:
			if text != test.want {
				t.Errorf("key %s encoded as %q, want %q", test.key.String(), text, test.want)
			}
		case <-time.After(time.Second):
			t.Errorf("key %s was dropped", test.key.String())
		}
		_ = pipe.Close()
		<-sent
	}
}
