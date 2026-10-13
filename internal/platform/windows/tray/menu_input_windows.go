//go:build windows

package tray

import (
	"syscall"
	"unsafe"
)

const wmPopupKey = 0x8004

// Ativos apenas enquanto o menu aberto pelo mouse não assume o foco do Shell.
// As callbacks rodam na thread do menu e somente encaminham mensagens, sem
// executar comandos do Agent, registrar teclas ou bloquear cliques externos.
var (
	passivePopup   *popupMenu
	popupMouseProc = syscall.NewCallback(popupMouseHook)
	popupKeyProc   = syscall.NewCallback(popupKeyHook)
)

type popupInput struct {
	mouseHook, keyHook, foreground uintptr
	pressed                        int
}

func (m *popupMenu) startInput() bool {
	m.input.pressed = -1
	m.input.foreground, _, _ = user.NewProc("GetForegroundWindow").Call()
	instance, _, _ := kernel.NewProc("GetModuleHandleW").Call(0)
	passivePopup = m
	m.input.mouseHook, _, _ = user.NewProc("SetWindowsHookExW").Call(14, popupMouseProc, instance, 0)
	m.input.keyHook, _, _ = user.NewProc("SetWindowsHookExW").Call(13, popupKeyProc, instance, 0)
	return m.input.mouseHook != 0 && m.input.keyHook != 0
}

func (m *popupMenu) stopInput() {
	if passivePopup == m {
		passivePopup = nil
	}
	for _, hook := range []uintptr{m.input.mouseHook, m.input.keyHook} {
		if hook != 0 {
			user.NewProc("UnhookWindowsHookEx").Call(hook)
		}
	}
	m.input.mouseHook, m.input.keyHook = 0, 0
}

func popupMouseHook(code int32, event, data uintptr) uintptr {
	if code >= 0 && passivePopup != nil {
		switch event {
		case 0x201, 0x204, 0x207, 0x20B: // Clique de qualquer botão fora fecha, sem consumir.
			var position point // Primeiro campo de MSLLHOOKSTRUCT.
			kernel.NewProc("RtlMoveMemory").Call(uintptr(unsafe.Pointer(&position)), data, unsafe.Sizeof(position))
			passivePopup.dismissOutside(position)
		}
	}
	r, _, _ := user.NewProc("CallNextHookEx").Call(0, uintptr(code), event, data)
	return r
}

func (m *popupMenu) dismissOutside(position point) {
	var bounds rectangle
	user.NewProc("GetWindowRect").Call(m.hwnd, uintptr(unsafe.Pointer(&bounds)))
	if position.X < bounds.Left || position.X >= bounds.Right || position.Y < bounds.Top || position.Y >= bounds.Bottom {
		user.NewProc("PostMessageW").Call(m.hwnd, wmClose, 0, 0)
	}
}

func popupKeyHook(code int32, event, data uintptr) uintptr {
	if code >= 0 && passivePopup != nil {
		var key uint32 // Primeiro campo de KBDLLHOOKSTRUCT.
		kernel.NewProc("RtlMoveMemory").Call(uintptr(unsafe.Pointer(&key)), data, unsafe.Sizeof(key))
		if passivePopup.routeKey(event, uintptr(key)) {
			return 1
		}
	}
	r, _, _ := user.NewProc("CallNextHookEx").Call(0, uintptr(code), event, data)
	return r
}

func (m *popupMenu) routeKey(event, key uintptr) bool {
	foreground, _, _ := user.NewProc("GetForegroundWindow").Call()
	if foreground != m.input.foreground {
		user.NewProc("PostMessageW").Call(m.hwnd, wmClose, 0, 0)
		return false
	}
	switch key {
	case 0x1B, 0x0D, 0x20, 0x26, 0x28, 0x24, 0x23, 0x09:
		if event == 0x100 { // WM_KEYDOWN; o Shell não deve receber a mesma navegação.
			user.NewProc("PostMessageW").Call(m.hwnd, wmPopupKey, key, 0)
		}
		return true
	case 0x10, 0xA0, 0xA1: // Shift continua disponível para Shift+Tab.
		return false
	default:
		if event == 0x100 || event == 0x104 { // Outros atalhos continuam no aplicativo ativo.
			user.NewProc("PostMessageW").Call(m.hwnd, wmClose, 0, 0)
		}
		return false
	}
}
