package terminal

import (
	"fmt"

	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/vt"
)

// SendKey supplements vt's legacy encoder with modified terminal keys.
func SendKey(screen *vt.SafeEmulator, key uv.KeyPressEvent) {
	key.Mod &^= uv.ModCapsLock | uv.ModNumLock | uv.ModScrollLock
	// The legacy encoder compares complete events and cannot represent repeats.
	key.IsRepeat = false
	if key.Text != "" && key.Mod&^uv.ModShift == 0 {
		screen.SendText(key.Text)
		return
	}
	if key.Code == uv.KeyTab && key.Mod == uv.ModShift {
		screen.SendText("\x1b[Z")
		return
	}
	// Kitty distinguishes keypad navigation; legacy terminal programs do not.
	if code, ok := map[rune]rune{
		uv.KeyKpUp: uv.KeyUp, uv.KeyKpDown: uv.KeyDown,
		uv.KeyKpLeft: uv.KeyLeft, uv.KeyKpRight: uv.KeyRight,
		uv.KeyKpHome: uv.KeyHome, uv.KeyKpEnd: uv.KeyEnd,
		uv.KeyKpPgUp: uv.KeyPgUp, uv.KeyKpPgDown: uv.KeyPgDown,
		uv.KeyKpInsert: uv.KeyInsert, uv.KeyKpDelete: uv.KeyDelete,
		uv.KeyKpBegin: uv.KeyBegin,
	}[key.Code]; ok {
		key.Code = code
	}
	modifier := 1
	if key.Mod&uv.ModShift != 0 {
		modifier += 1
	}
	if key.Mod&uv.ModAlt != 0 {
		modifier += 2
	}
	if key.Mod&uv.ModCtrl != 0 {
		modifier += 4
	}
	if key.Mod&uv.ModSuper != 0 {
		modifier += 8
	}
	if key.Mod&uv.ModHyper != 0 {
		modifier += 16
	}
	if key.Mod&uv.ModMeta != 0 {
		modifier += 32
	}
	if modifier > 1 {
		navigationModifier := modifier
		if key.Mod&uv.ModMeta != 0 && key.Mod&(uv.ModSuper|uv.ModHyper) == 0 {
			// Legacy arrows use bit 3 for Meta; CSI-u uses bit 5.
			navigationModifier = modifier - 32 + 8
		}
		suffix := map[rune]string{
			uv.KeyUp: "A", uv.KeyDown: "B", uv.KeyRight: "C", uv.KeyLeft: "D",
			uv.KeyHome: "H", uv.KeyEnd: "F",
			uv.KeyF1: "P", uv.KeyF2: "Q", uv.KeyF3: "R", uv.KeyF4: "S",
		}[key.Code]
		if suffix != "" {
			screen.SendText(fmt.Sprintf("\x1b[1;%d%s", navigationModifier, suffix))
			return
		}
		if code := map[rune]int{
			uv.KeyInsert: 2, uv.KeyDelete: 3, uv.KeyPgUp: 5, uv.KeyPgDown: 6,
			uv.KeyF5: 15, uv.KeyF6: 17, uv.KeyF7: 18, uv.KeyF8: 19,
			uv.KeyF9: 20, uv.KeyF10: 21, uv.KeyF11: 23, uv.KeyF12: 24,
		}[key.Code]; code != 0 {
			screen.SendText(fmt.Sprintf("\x1b[%d;%d~", code, navigationModifier))
			return
		}
		if key.Code == uv.KeyEnter || key.Code == uv.KeyKpEnter {
			screen.SendText(fmt.Sprintf("\x1b[13;%du", modifier))
			return
		}
		// Keep distinct modified Backspace, Tab, and Escape events.
		controlSpecial := key.Mod&uv.ModCtrl != 0 && (key.Code == uv.KeyBackspace || key.Code == uv.KeyTab || key.Code == uv.KeyEscape)
		if (controlSpecial || key.Mod&^(uv.ModAlt|uv.ModCtrl) != 0) && key.Code > 0 && key.Code <= 0x10ffff {
			screen.SendText(fmt.Sprintf("\x1b[%d;%du", key.Code, modifier))
			return
		}
	}
	key.BaseCode, key.ShiftedCode = 0, 0
	screen.SendKey(key)
}
