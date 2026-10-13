//go:build windows

package tray

import (
	"os"
	"strconv"
	"testing"
	"time"
	"unsafe"
)

func TestWindowsStructureLayout(t *testing.T) {
	if unsafe.Sizeof(uintptr(0)) != 8 {
		t.Skip("x64 application")
	}
	if unsafe.Sizeof(notifyIcon{}) != 976 {
		t.Fatalf("NOTIFYICONDATAW size: %d", unsafe.Sizeof(notifyIcon{}))
	}
	if unsafe.Sizeof(windowClass{}) != 80 {
		t.Fatalf("WNDCLASSEXW size: %d", unsafe.Sizeof(windowClass{}))
	}
	if unsafe.Sizeof(message{}) != 48 {
		t.Fatalf("MSG size: %d", unsafe.Sizeof(message{}))
	}
}

// Verificação de integração opcional: cria um ícone temporário, exercita o loop de
// mensagens da janela e o remove. Nunca anexa um TAP nem reinicia o Explorer.
func TestNativeTrayLoop(t *testing.T) {
	if os.Getenv("WINTASKBAR_TEST_TRAY") != "1" {
		t.Skip("set WINTASKBAR_TEST_TRAY=1 for native UI smoke test")
	}
	className := WindowClass + ".Test." + strconv.Itoa(os.Getpid())
	desktopWindow := findWindow()
	var icon *Icon
	closed := false
	err := runWithClass(className, func(i *Icon) {
		icon = i
		i.Post(Update{Status: "teste", Items: []Item{{ID: 1, Label: "Teste"}}})
		go func() {
			time.Sleep(150 * time.Millisecond)
			user.NewProc("PostMessageW").Call(i.hwnd, wmClose, 0, 0)
		}()
	}, func(id uint32) {
		if id == 0 {
			closed = true
			icon.Post(Update{Quit: true})
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if !closed || findWindowClass(className) != 0 {
		t.Fatal("tray window did not shut down cleanly")
	}
	if findWindow() != desktopWindow {
		t.Fatal("test interfered with the running desktop app")
	}
}
