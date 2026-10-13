//go:build windows

package tray

import (
	"fmt"
	"os"
	"strconv"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

func TestPopupPlacement(t *testing.T) {
	for _, test := range []struct {
		name          string
		anchor, work  rectangle
		width, height int32
	}{
		{"bottom", rectangle{1800, 1040, 1832, 1080}, rectangle{0, 0, 1920, 1040}, 340, 370},
		{"left edge", rectangle{0, 1040, 32, 1080}, rectangle{0, 0, 1920, 1040}, 340, 370},
		{"negative monitor", rectangle{-100, 1040, -68, 1080}, rectangle{-1920, 0, 0, 1040}, 340, 370},
		{"top taskbar", rectangle{100, 0, 132, 40}, rectangle{0, 40, 1920, 1080}, 340, 370},
		{"scaled", rectangle{1800, 1440, 1848, 1500}, rectangle{0, 0, 1920, 1440}, 510, 555},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := placePopup(test.anchor, test.work, test.width, test.height, 8)
			if r.Left < test.work.Left || r.Top < test.work.Top || r.Right > test.work.Right || r.Bottom > test.work.Bottom {
				t.Fatalf("popup outside monitor: %+v", r)
			}
			if r.Right-r.Left != test.width || r.Bottom-r.Top != test.height {
				t.Fatal("popup size changed")
			}
		})
	}
}

func TestMenuNavigationSkipsNonActions(t *testing.T) {
	items := []Item{{Label: "Estado", Disabled: true}, {}, {ID: 1, Label: "Aplicar"}, {ID: 2, Label: "Erro", Disabled: true}, {ID: 3, Label: "Sair"}}
	for _, test := range []struct{ start, step, want int }{{-1, 1, 2}, {2, 1, 4}, {4, 1, 2}, {2, -1, 4}, {0, -1, 4}} {
		if got := nextMenuItem(items, test.start, test.step); got != test.want {
			t.Errorf("%+v: got %d", test, got)
		}
	}
	if nextMenuItem(nil, -1, 1) != -1 || nextMenuItem(items[:2], -1, 1) != -1 {
		t.Fatal("selected a non-action")
	}
	if accessibleLabel(Item{Label: "Iniciar", Toggle: true, Checked: true}) != "Iniciar, ativado" {
		t.Fatal("checked state missing")
	}
	if accessibleLabel(Item{Label: "Iniciar", Toggle: true}) != "Iniciar, desativado" {
		t.Fatal("unchecked state missing")
	}
}

func TestMenuWindowsStructureLayout(t *testing.T) {
	if unsafe.Sizeof(drawItem{}) != 64 || unsafe.Sizeof(paintStruct{}) != 72 || unsafe.Sizeof(monitorInfo{}) != 40 {
		t.Fatal("invalid x64 Win32 menu structures")
	}
}

// Ações locais de teste: nunca executa os comandos reais nem anexa o TAP.
func TestNativePopupMenu(t *testing.T) {
	if os.Getenv("WINTASKBAR_TEST_TRAY") != "1" {
		t.Skip("set WINTASKBAR_TEST_TRAY=1 for native menu test")
	}
	for _, mouse := range []bool{false, true} {
		t.Run(strconv.FormatBool(mouse), func(t *testing.T) { testNativePopupMenu(t, mouse) })
	}
}

func testNativePopupMenu(t *testing.T, mouse bool) {
	className := WindowClass + ".MenuTest." + strconv.Itoa(os.Getpid())
	var icon *Icon
	var selected []uint32
	var failure string
	done := make(chan struct{})
	defer close(done)
	err := runWithClass(className, func(i *Icon) {
		icon = i
		i.items = []Item{{Label: "wintaskbar — teste", Disabled: true}, {}, {ID: 1, Label: "Aplicar", Icon: "\uE72C"}, {ID: 2, Label: "Indisponível", Disabled: true}, {ID: 3, Label: "Iniciar", Toggle: true, Checked: true}}
		// Notificações v4: clique esquerdo não abre; contexto e teclado abrem.
		for _, event := range []uintptr{0x201, 0x202, 0x203, 0x400, 0x205} {
			i.wndProc(i.hwnd, wmTray, 0, 1<<16|event)
			if i.popup != nil {
				failure = "non-context mouse event opened popup"
				i.Post(Update{Quit: true})
				return
			}
		}
		i.wndProc(i.hwnd, wmTray, 0, 1<<16|0x7b)
		if i.popup == nil {
			failure = "context event did not open popup"
			i.Post(Update{Quit: true})
			return
		}
		m := i.popup
		// O texto do controle nativo também expõe o estado para leitores de tela.
		var label [128]uint16
		user.NewProc("GetWindowTextW").Call(m.rows[4].window, uintptr(unsafe.Pointer(&label[0])), 128)
		if syscall.UTF16ToString(label[:]) != "Iniciar, ativado" {
			failure = "accessible state missing"
		}
		m.activate(3)
		if i.popup == nil {
			failure = "disabled item was activated"
			i.Post(Update{Quit: true})
			return
		}
		m.filter(&message{Window: m.hwnd, ID: 0x100, WParam: 0x1B})
		if i.popup != nil {
			failure = "Escape did not close popup"
			i.Post(Update{Quit: true})
			return
		}
		i.wndProc(i.hwnd, wmTray, 0, 1<<16|0x401)
		if i.popup == nil {
			failure = "keyboard activation after Escape failed"
			i.Post(Update{Quit: true})
			return
		}
		process, _, _ := kernel.NewProc("GetCurrentProcess").Call()
		// Aquecer caches de fonte/tema do Windows antes de contar recursos.
		for n := 0; n < 5; n++ {
			i.popup.close()
			if !i.openPopup() {
				failure = "warmup reopen failed"
				i.Post(Update{Quit: true})
				return
			}
			flushMenuMessages()
		}
		before, _, _ := user.NewProc("GetGuiResources").Call(process, 0)
		// Perda de foco fecha sem disparar ação; repetir exercita criação e limpeza.
		for n := 0; n < 5; n++ {
			user.NewProc("SendMessageW").Call(i.popup.hwnd, 0x06, 0, 0)
			if i.popup != nil {
				failure = "deactivation did not close popup"
				break
			}
			if !i.openPopup() {
				failure = "popup reopen failed"
				break
			}
			flushMenuMessages()
		}
		if failure != "" {
			i.Post(Update{Quit: true})
			return
		}
		after, _, _ := user.NewProc("GetGuiResources").Call(process, 0)
		if after > before+2 {
			failure = fmt.Sprintf("GDI objects grew after repeated close/reopen: %d -> %d", before, after)
			i.Post(Update{Quit: true})
			return
		}
		m = i.popup
		// Atualizações periódicas preservam a janela aberta e atualizam o nome acessível.
		items := append([]Item(nil), i.items...)
		items[4].Checked = false
		m.refresh(items)
		user.NewProc("GetWindowTextW").Call(m.rows[4].window, uintptr(unsafe.Pointer(&label[0])), 128)
		if syscall.UTF16ToString(label[:]) != "Iniciar, desativado" {
			failure = "live accessible state not updated"
		}
		// Mensagens apenas à janela de teste, sem injetar teclas no desktop.
		go func(window, button uintptr) {
			time.Sleep(100 * time.Millisecond)
			if mouse {
				user.NewProc("PostMessageW").Call(button, 0xF5, 0, 0) // BM_CLICK somente no controle de teste.
				return
			}
			user.NewProc("PostMessageW").Call(window, 0x100, 0x28, 0)
			user.NewProc("PostMessageW").Call(window, 0x100, 0x0D, 0)
			user.NewProc("PostMessageW").Call(window, 0x100, 0x0D, 0)
		}(m.hwnd, m.rows[4].window)
		go func() {
			select {
			case <-time.After(3 * time.Second):
				i.Post(Update{Quit: true})
			case <-done:
			}
		}()
	}, func(id uint32) {
		selected = append(selected, id)
		icon.Post(Update{Quit: true})
	})
	if err != nil {
		t.Fatal(err)
	}
	if failure != "" {
		t.Fatal(failure)
	}
	if len(selected) != 1 || selected[0] != 3 {
		t.Fatalf("actions = %v, want exactly [3]", selected)
	}
	if icon.popup != nil {
		t.Fatal("popup leaked after exit")
	}
	popupWindows.Range(func(_, _ any) bool { t.Error("popup registry retained a destroyed window"); return false })
}

func flushMenuMessages() {
	var msg message
	for n := 0; n < 100; n++ {
		available, _, _ := user.NewProc("PeekMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, 1)
		if available == 0 {
			return
		}
		user.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&msg)))
		user.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&msg)))
	}
}
