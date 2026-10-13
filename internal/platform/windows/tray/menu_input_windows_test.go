//go:build windows

package tray

import (
	"os"
	"strconv"
	"testing"
	"time"
	"unsafe"
)

// Um segundo popup de teste simula o painel que fecha ao perder a ativação.
// Não abre nem manipula o painel real do Explorer.
func TestPassiveMenuPreservesUnderlyingPopup(t *testing.T) {
	if os.Getenv("WINTASKBAR_TEST_TRAY") != "1" {
		t.Skip("set WINTASKBAR_TEST_TRAY=1 for native menu test")
	}
	var selected []uint32
	err := runWithClass(WindowClass+".PassiveTest."+strconv.Itoa(os.Getpid()), func(i *Icon) {
		defer i.Post(Update{Quit: true})
		i.items = []Item{{Label: "Estado", Disabled: true}, {ID: 1, Label: "Aplicar"}, {ID: 2, Label: "Indisponível", Disabled: true}, {ID: 3, Label: "Sair"}}
		panel := &Icon{hwnd: i.hwnd, data: i.data, items: i.items}
		if !panel.openPopup() {
			t.Error("test panel creation failed")
			return
		}
		defer func() {
			if panel.popup != nil {
				panel.popup.close()
			}
		}()
		foreground, _, _ := user.NewProc("GetForegroundWindow").Call()
		if foreground != panel.popup.hwnd {
			t.Error("test panel did not become foreground")
			return
		}
		open := func() bool {
			if !i.openPopupMode(true) {
				t.Error("passive popup creation failed")
				return false
			}
			return true
		}
		if !open() {
			return
		}
		m := i.popup
		activation, _, _ := user.NewProc("SendMessageW").Call(m.hwnd, 0x21, foreground, 0x2010001)
		if activation != 3 {
			t.Error("mouse click would activate popup")
		}
		for n := 0; n < 25; n++ {
			i.Post(Update{Status: "teste", Items: i.items})
			flushMenuMessages()
			time.Sleep(100 * time.Millisecond)
		}
		current, _, _ := user.NewProc("GetForegroundWindow").Call()
		if panel.popup == nil || i.popup != m || current != foreground {
			t.Errorf("opening or refreshing menu dismissed underlying popup or stole focus: foreground=%#x, expected=%#x, menu=%#x, panelAlive=%t, menuAlive=%t", current, foreground, m.hwnd, panel.popup != nil, i.popup == m)
			return
		}
		// Clique completo no controle, sem permitir que BUTTON assuma o foco.
		for _, index := range []int{2, 1} {
			m.filter(&message{Window: m.rows[index].window, ID: 0x201})
			m.filter(&message{Window: m.rows[index].window, ID: 0x202})
		}
		if len(selected) != 1 || selected[0] != 1 || i.popup != nil || panel.popup == nil {
			t.Error("passive click failed or dismissed underlying popup")
			return
		}
		if passivePopup != nil || m.input.mouseHook != 0 || m.input.keyHook != 0 {
			t.Error("input hooks leaked")
		}
		if !open() {
			return
		}
		m = i.popup
		if !m.routeKey(0x100, 0x28) {
			t.Error("Down was not handled")
		}
		flushMenuMessages()
		if m.focused != 3 {
			t.Error("keyboard did not skip disabled item")
		}
		m.routeKey(0x100, 0x0D)
		flushMenuMessages()
		if len(selected) != 2 || selected[1] != 3 || i.popup != nil {
			t.Error("keyboard action failed")
		}
		if !open() {
			return
		}
		m = i.popup
		var bounds rectangle
		user.NewProc("GetWindowRect").Call(m.hwnd, uintptr(unsafe.Pointer(&bounds)))
		m.dismissOutside(point{bounds.Left + 1, bounds.Top + 1})
		flushMenuMessages()
		if i.popup != m {
			t.Error("inside click dismissed menu")
			return
		}
		m.dismissOutside(point{bounds.Left - 1, bounds.Top})
		flushMenuMessages()
		if i.popup != nil || len(selected) != 2 {
			t.Error("outside click failed to dismiss without action")
		}
		if !open() {
			return
		}
		i.popup.routeKey(0x100, 0x1B)
		flushMenuMessages()
		if i.popup != nil || panel.popup == nil {
			t.Error("Escape did not preserve underlying popup")
		}
	}, func(id uint32) { selected = append(selected, id) })
	if err != nil {
		t.Fatal(err)
	}
	if passivePopup != nil {
		t.Error("passive popup leaked after exit")
	}
}
