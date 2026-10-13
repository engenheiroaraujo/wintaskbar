//go:build windows

// Package tray é dono do ícone de notificação nativo e do seu loop de mensagens do Windows.
// Todas as chamadas de UI rodam na thread do SO travada da janela; as atualizações do worker são postadas.
package tray

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const WindowClass = "WinTaskbar.Desktop.v1"
const (
	wmTray   = 0x8001
	wmUpdate = 0x8002
	wmShow   = 0x8003
	wmClose  = 0x0010
)

var (
	user   = syscall.NewLazyDLL(filepath.Join(os.Getenv("SystemRoot"), "System32", "user32.dll"))
	shell  = syscall.NewLazyDLL(filepath.Join(os.Getenv("SystemRoot"), "System32", "shell32.dll"))
	kernel = syscall.NewLazyDLL(filepath.Join(os.Getenv("SystemRoot"), "System32", "kernel32.dll"))
)

func ptr(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }

type point struct{ X, Y int32 }
type message struct {
	Window         uintptr
	ID             uint32
	WParam, LParam uintptr
	Time           uint32
	Point          point
	Private        uint32
}
type windowClass struct {
	Size, Style                        uint32
	Callback                           uintptr
	ClassExtra, WindowExtra            int32
	Instance, Icon, Cursor, Background uintptr
	MenuName, ClassName                *uint16
	SmallIcon                          uintptr
}
type notifyIcon struct {
	Size                uint32
	Window              uintptr
	ID, Flags, Callback uint32
	Icon                uintptr
	Tip                 [128]uint16
	State, StateMask    uint32
	Info                [256]uint16
	Version             uint32
	InfoTitle           [64]uint16
	InfoFlags           uint32
	GUID                syscall.GUID
	BalloonIcon         uintptr
}

type Item struct {
	ID                uint32
	Label             string
	Icon              string
	Toggle            bool
	Checked, Disabled bool
}
type Update struct {
	Status, Notice string
	Error, Quit    bool
	Items          []Item
}

type Icon struct {
	popup          *popupMenu
	hwnd           uintptr
	data           notifyIcon
	taskbarCreated uint32
	items          []Item
	action         func(uint32)
	mu             sync.Mutex
	pending        []Update
}

// Post pode ser chamado pelo worker. As mutações de UI são encaminhadas à thread dela.
func (i *Icon) Post(u Update) {
	i.mu.Lock()
	i.pending = append(i.pending, u)
	i.mu.Unlock()
	user.NewProc("PostMessageW").Call(i.hwnd, wmUpdate, 0, 0)
}

func (i *Icon) notify(op uintptr) bool {
	r, _, _ := shell.NewProc("Shell_NotifyIconW").Call(op, uintptr(unsafe.Pointer(&i.data)))
	return r != 0
}

func (i *Icon) add() {
	i.data.Flags = 1 | 2 | 4 // callback, icon, tooltip
	if !i.notify(0) {
		user.NewProc("SetTimer").Call(i.hwnd, 1, 2000, 0)
		return
	}
	i.data.Version = 4
	i.notify(4) // NIM_SETVERSION
	user.NewProc("KillTimer").Call(i.hwnd, 1)
}

func (i *Icon) menu(passive bool) {
	if i.popup != nil {
		i.popup.close()
		return
	}
	if !highContrast() && i.openPopupMode(passive) {
		return
	}
	i.classicMenu()
}

func (i *Icon) classicMenu() {
	menu, _, _ := user.NewProc("CreatePopupMenu").Call()
	if menu == 0 {
		return
	}
	defer user.NewProc("DestroyMenu").Call(menu)
	for _, item := range i.items {
		flags := uintptr(0)
		if item.Label == "" {
			flags = 0x800
		}
		if item.Checked {
			flags |= 8
		}
		if item.Disabled {
			flags |= 3
		}
		user.NewProc("AppendMenuW").Call(menu, flags, uintptr(item.ID), uintptr(unsafe.Pointer(ptr(item.Label))))
	}
	var p point
	user.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&p)))
	user.NewProc("SetForegroundWindow").Call(i.hwnd)
	selected, _, _ := user.NewProc("TrackPopupMenu").Call(menu, 0x100|2, uintptr(p.X), uintptr(p.Y), 0, i.hwnd, 0)
	user.NewProc("PostMessageW").Call(i.hwnd, 0, 0, 0)
	if selected != 0 {
		i.action(uint32(selected))
	}
}

func putText(dst []uint16, s string) {
	clear(dst)
	v := syscall.StringToUTF16(s)
	if len(v) > len(dst) {
		v = v[:len(dst)-1]
	}
	copy(dst, v)
}

func (i *Icon) wndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	switch msg {
	case i.taskbarCreated:
		i.add()
	case 0x113: // WM_TIMER: o Explorer pode não estar pronto no logon.
		i.add()
	case wmTray:
		switch uint16(lparam) {
		case 0x7b: // WM_CONTEXTMENU; coordenadas -1 indicam solicitação por teclado.
			i.menu(uint32(wparam) != 0xffffffff)
		case 0x401: // NIN_KEYSELECT; clique esquerdo não abre.
			i.menu(false)
		}
	case wmShow:
		i.menu(false)
	case wmUpdate:
		i.mu.Lock()
		updates := i.pending
		i.pending = nil
		i.mu.Unlock()
		for _, u := range updates {
			if u.Quit {
				user.NewProc("DestroyWindow").Call(hwnd)
				return 0
			}
			if u.Items != nil {
				i.items = u.Items
				if i.popup != nil {
					i.popup.refresh(u.Items)
				}
			}
			if u.Status != "" {
				putText(i.data.Tip[:], "wintaskbar — "+u.Status)
			}
			i.data.Flags = 4
			if u.Notice != "" {
				i.data.Flags |= 0x10
				putText(i.data.InfoTitle[:], "wintaskbar")
				putText(i.data.Info[:], u.Notice)
				i.data.InfoFlags = 1
				if u.Error {
					i.data.InfoFlags = 3
				}
			}
			i.notify(1)
		}
	case wmClose:
		i.action(0) // restauração assíncrona; depois o worker posta Quit
	case 2: // WM_DESTROY
		if i.popup != nil {
			i.popup.close()
		}
		i.notify(2)
		user.NewProc("PostQuitMessage").Call(0)
	case 0x11: // WM_QUERYENDSESSION: do not restart Explorer during logoff
		return 1
	case 0x16: // WM_ENDSESSION
		if wparam != 0 {
			user.NewProc("DestroyWindow").Call(hwnd)
		}
	default:
		r, _, _ := user.NewProc("DefWindowProcW").Call(hwnd, uintptr(msg), wparam, lparam)
		return r
	}
	return 0
}

// Run mantém um mutex local da sessão, cria o ícone e chama ready apenas para
// a primeira instância. Uma segunda execução abre o menu do ícone existente.
func Run(ready func(*Icon), action func(uint32)) error {
	return runWithClass(WindowClass, ready, action)
}

// Uma classe e um mutex separados permitem que os testes nativos convivam com o app de desktop.
func runWithClass(className string, ready func(*Icon), action func(uint32)) error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	// Escala física por monitor apenas nesta thread de UI.
	previousDPI, _, _ := user.NewProc("SetThreadDpiAwarenessContext").Call(^uintptr(3))
	if previousDPI != 0 {
		defer user.NewProc("SetThreadDpiAwarenessContext").Call(previousDPI)
	}
	mutex, _, err := kernel.NewProc("CreateMutexW").Call(0, 0, uintptr(unsafe.Pointer(ptr(`Local\`+className))))
	if mutex == 0 {
		return fmt.Errorf("create instance mutex: %w", err)
	}
	defer kernel.NewProc("CloseHandle").Call(mutex)
	if err == syscall.Errno(183) {
		for n := 0; n < 20; n++ {
			if hwnd := findWindowClass(className); hwnd != 0 {
				user.NewProc("PostMessageW").Call(hwnd, wmShow, 0, 0)
				return nil
			}
			time.Sleep(100 * time.Millisecond)
		}
		return fmt.Errorf("o wintaskbar já está iniciando; tente novamente em instantes")
	}
	i := &Icon{action: action}
	created, _, _ := user.NewProc("RegisterWindowMessageW").Call(uintptr(unsafe.Pointer(ptr("TaskbarCreated"))))
	i.taskbarCreated = uint32(created)
	instance, _, _ := kernel.NewProc("GetModuleHandleW").Call(0)
	icon, _, _ := user.NewProc("LoadIconW").Call(instance, 1)
	if icon == 0 {
		icon, _, _ = user.NewProc("LoadIconW").Call(0, 32512)
	}
	// A bandeja usa o ícone no tamanho pequeno do sistema: reduzir o de 32 px
	// deixa a imagem borrada. Sem o recurso, cai no ícone grande.
	small := icon
	cx, _, _ := user.NewProc("GetSystemMetrics").Call(49)                              // SM_CXSMICON
	cy, _, _ := user.NewProc("GetSystemMetrics").Call(50)                              // SM_CYSMICON
	if h, _, _ := user.NewProc("LoadImageW").Call(instance, 1, 1, cx, cy, 0); h != 0 { // IMAGE_ICON
		small = h
	}
	class := windowClass{Instance: instance, ClassName: ptr(className), Callback: syscall.NewCallback(i.wndProc), Icon: icon, SmallIcon: small}
	class.Size = uint32(unsafe.Sizeof(class))
	if r, _, err := user.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&class))); r == 0 {
		return fmt.Errorf("register window: %w", err)
	}
	defer user.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(class.ClassName)), instance)
	hwnd, _, err := user.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(class.ClassName)), uintptr(unsafe.Pointer(ptr("wintaskbar"))), 0, 0, 0, 0, 0, 0, 0, instance, 0)
	if hwnd == 0 {
		return fmt.Errorf("create tray window: %w", err)
	}
	i.hwnd = hwnd
	i.data = notifyIcon{Window: hwnd, ID: 1, Callback: wmTray, Icon: small}
	i.data.Size = uint32(unsafe.Sizeof(i.data))
	putText(i.data.Tip[:], "wintaskbar — iniciando")
	i.add()
	ready(i)
	var msg message
	for {
		r, _, err := user.NewProc("GetMessageW").Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) == -1 {
			return fmt.Errorf("read window message: %w", err)
		}
		if r == 0 {
			return nil
		}
		if i.popup != nil && i.popup.filter(&msg) {
			continue
		}
		user.NewProc("TranslateMessage").Call(uintptr(unsafe.Pointer(&msg)))
		user.NewProc("DispatchMessageW").Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func findWindow() uintptr {
	return findWindowClass(WindowClass)
}

func findWindowClass(className string) uintptr {
	h, _, _ := user.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(ptr(className))), 0)
	return h
}

// Stop pede à instância de desktop em execução que restaure a barra e encerre.
func Stop() error {
	h := findWindow()
	if h == 0 {
		return nil
	}
	var pid uint32
	user.NewProc("GetWindowThreadProcessId").Call(h, uintptr(unsafe.Pointer(&pid)))
	process, _, err := kernel.NewProc("OpenProcess").Call(0x00100000, 0, uintptr(pid)) // SYNCHRONIZE
	if process == 0 {
		return fmt.Errorf("open desktop process: %w", err)
	}
	defer kernel.NewProc("CloseHandle").Call(process)
	user.NewProc("PostMessageW").Call(h, wmClose, 0, 0)
	// Espera o processo encerrar, e não só a janela ser destruída: o instalador precisa
	// poder substituir o executável logo depois que isto retorna.
	result, _, _ := kernel.NewProc("WaitForSingleObject").Call(process, 45000)
	if result == 0 {
		return nil
	}
	return fmt.Errorf("não foi possível encerrar o wintaskbar; use Sair no menu da bandeja")
}

func Message(text string, failure bool) {
	flags := uintptr(0x40)
	if failure {
		flags = 0x10
	}
	user.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(ptr(text))), uintptr(unsafe.Pointer(ptr("wintaskbar"))), flags)
}

func Confirm(text string) bool {
	r, _, _ := user.NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(ptr(text))), uintptr(unsafe.Pointer(ptr("wintaskbar"))), 0x24)
	return r == 6
}
