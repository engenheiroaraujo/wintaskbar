//go:build windows

// Package explorer localiza o processo explorer.exe que possui a Taskbar.
package explorer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"

	errs "github.com/engenheiroaraujo/wintaskbar/internal/platform/errors"
)

// TaskbarPID devolve o ID do processo que possui a janela da Taskbar da
// sessão atual. Procurar por Shell_TrayWnd é exato mesmo quando
// existem vários processos explorer.exe (ex.: janelas de pasta em processo separado).
func TaskbarPID() (uint32, error) {
	user32 := syscall.NewLazyDLL(filepath.Join(os.Getenv("SystemRoot"), "System32", "user32.dll"))
	findWindow := user32.NewProc("FindWindowW")
	getWindowThreadProcessID := user32.NewProc("GetWindowThreadProcessId")

	class, _ := syscall.UTF16PtrFromString("Shell_TrayWnd")
	hwnd, _, _ := findWindow.Call(uintptr(unsafe.Pointer(class)), 0)
	if hwnd == 0 {
		return 0, errs.ErrNoTaskbar // o explorer ainda não subiu, ou está reiniciando
	}

	var pid uint32
	tid, _, callErr := getWindowThreadProcessID.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if tid == 0 || pid == 0 {
		return 0, fmt.Errorf("explorer: GetWindowThreadProcessId: %v", callErr)
	}
	return pid, nil
}

const (
	processTerminate = 0x0001
	// Quanto tempo o Windows tem para subir um novo explorer sozinho antes de Restart
	// subir um.
	autoRestartWait = 8 * time.Second
)

// Restart encerra o explorer que possui a Taskbar e devolve seu PID.
// O Windows sobe um novo sozinho (a Taskbar volta com o visual original);
// se nenhum aparecer em poucos segundos, Restart o inicia.
func Restart() (uint32, error) {
	pid, err := TaskbarPID()
	if err != nil {
		return 0, err
	}

	kernel32 := syscall.NewLazyDLL(filepath.Join(os.Getenv("SystemRoot"), "System32", "kernel32.dll"))
	h, _, callErr := kernel32.NewProc("OpenProcess").Call(processTerminate, 0, uintptr(pid))
	if h == 0 {
		return pid, fmt.Errorf("explorer: OpenProcess: %v", callErr)
	}
	defer kernel32.NewProc("CloseHandle").Call(h)
	if ok, _, callErr := kernel32.NewProc("TerminateProcess").Call(h, 1); ok == 0 {
		return pid, fmt.Errorf("explorer: TerminateProcess: %v", callErr)
	}

	deadline := time.Now().Add(autoRestartWait)
	for time.Now().Before(deadline) {
		if newPID, err := TaskbarPID(); err == nil && newPID != pid {
			return pid, nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	if err := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "explorer.exe")).Start(); err != nil {
		return pid, fmt.Errorf("explorer: start: %w", err)
	}
	return pid, nil
}

// Shell adapta Restart às interfaces dos consumidores.
type Shell struct{}

// RestartExplorer implementa a interface do consumidor chamando Restart.
func (Shell) RestartExplorer() (uint32, error) { return Restart() }

// Locator adapta TaskbarPID às interfaces dos consumidores.
type Locator struct{}

// TaskbarPID implementa a interface do consumidor chamando a função do pacote.
func (Locator) TaskbarPID() (uint32, error) { return TaskbarPID() }
