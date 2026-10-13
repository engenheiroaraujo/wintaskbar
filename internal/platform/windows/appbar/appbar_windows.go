//go:build windows

// Package appbar lê e define o estado "Ocultar automaticamente a barra de
// tarefas" do Windows pela API documentada SHAppBarMessage.
package appbar

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

const (
	abmGetState = 0x00000004
	abmSetState = 0x0000000A

	// ABS_* state flags.
	stateAutoHide    = 0x1
	stateAlwaysOnTop = 0x2
)

// appBarData mirrors APPBARDATA on 64-bit Windows (48 bytes).
type appBarData struct {
	CbSize           uint32
	_                uint32 // preenchimento antes do HWND, que tem o tamanho de um ponteiro
	HWnd             uintptr
	UCallbackMessage uint32
	UEdge            uint32
	Rc               [4]int32
	LParam           uintptr
}

var (
	system32            = filepath.Join(os.Getenv("SystemRoot"), "System32")
	procSHAppBarMessage = syscall.NewLazyDLL(filepath.Join(system32, "shell32.dll")).NewProc("SHAppBarMessage")
	procFindWindow      = syscall.NewLazyDLL(filepath.Join(system32, "user32.dll")).NewProc("FindWindowW")

	// ErrNoTaskbar indica que a janela da barra de tarefas não foi encontrada.
	ErrNoTaskbar = errors.New("appbar: Shell_TrayWnd window not found")
)

func trayWindow() (uintptr, error) {
	class, _ := syscall.UTF16PtrFromString("Shell_TrayWnd")
	hwnd, _, _ := procFindWindow.Call(uintptr(unsafe.Pointer(class)), 0)
	if hwnd == 0 {
		return 0, ErrNoTaskbar
	}
	return hwnd, nil
}

func state(hwnd uintptr) uint32 {
	d := appBarData{HWnd: hwnd}
	d.CbSize = uint32(unsafe.Sizeof(d))
	r, _, _ := procSHAppBarMessage.Call(abmGetState, uintptr(unsafe.Pointer(&d)))
	return uint32(r)
}

// AutoHide informa se "Ocultar automaticamente a barra de tarefas" está ligado.
func AutoHide() (bool, error) {
	hwnd, err := trayWindow()
	if err != nil {
		return false, err
	}
	return state(hwnd)&stateAutoHide != 0, nil
}

// SetAutoHide liga ou desliga "Ocultar automaticamente a barra de tarefas", preservando os
// outros indicadores de estado.
func SetAutoHide(on bool) error {
	hwnd, err := trayWindow()
	if err != nil {
		return err
	}
	s := state(hwnd)
	if on {
		s |= stateAutoHide
	} else {
		s &^= stateAutoHide
	}
	d := appBarData{HWnd: hwnd, LParam: uintptr(s)}
	d.CbSize = uint32(unsafe.Sizeof(d))
	procSHAppBarMessage.Call(abmSetState, uintptr(unsafe.Pointer(&d)))
	return nil
}

// Toggle inverte o estado e devolve o novo valor de AutoHide.
func Toggle() (bool, error) {
	cur, err := AutoHide()
	if err != nil {
		return false, err
	}
	if err := SetAutoHide(!cur); err != nil {
		return cur, err
	}
	return !cur, nil
}

var _ = stateAlwaysOnTop // mantido para documentar o layout dos indicadores

// Settings adapta as funções do pacote às interfaces dos consumidores.
type Settings struct{}

// AutoHide implementa a interface do consumidor chamando a função do pacote.
func (Settings) AutoHide() (bool, error) { return AutoHide() }

// SetAutoHide implementa a interface do consumidor chamando a função do pacote.
func (Settings) SetAutoHide(on bool) error { return SetAutoHide(on) }

// ToggleAutoHide implementa a interface do consumidor chamando Toggle.
func (Settings) ToggleAutoHide() (bool, error) { return Toggle() }
