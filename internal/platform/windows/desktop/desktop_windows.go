//go:build windows

// Package desktop fornece configurações de inicialização por usuário e integração com o shell.
package desktop

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"
)

const RunKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const UninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\wintaskbar`

// SettingsKey guarda as escolhas do usuário. O desinstalador a remove.
const SettingsKey = `Software\wintaskbar`

var advapi = syscall.NewLazyDLL(filepath.Join(os.Getenv("SystemRoot"), "System32", "advapi32.dll"))

func wide(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }

// SetString grava um valor em HKCU; nunca exige direitos de administrador.
func SetString(key, name, value string) error {
	var handle syscall.Handle
	r, _, _ := advapi.NewProc("RegCreateKeyExW").Call(uintptr(syscall.HKEY_CURRENT_USER), uintptr(unsafe.Pointer(wide(key))), 0, 0, 0, syscall.KEY_SET_VALUE, 0, uintptr(unsafe.Pointer(&handle)), 0)
	if r != 0 {
		return syscall.Errno(r)
	}
	defer syscall.RegCloseKey(handle)
	v, err := syscall.UTF16FromString(value)
	if err != nil {
		return err
	}
	r, _, _ = advapi.NewProc("RegSetValueExW").Call(uintptr(handle), uintptr(unsafe.Pointer(wide(name))), 0, syscall.REG_SZ, uintptr(unsafe.Pointer(&v[0])), uintptr(len(v)*2))
	if r != 0 {
		return syscall.Errno(r)
	}
	return nil
}

func ReadString(key, name string) (string, error) {
	var handle syscall.Handle
	err := syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, wide(key), 0, syscall.KEY_QUERY_VALUE, &handle)
	if err == syscall.ERROR_FILE_NOT_FOUND {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer syscall.RegCloseKey(handle)
	var kind, size uint32
	err = syscall.RegQueryValueEx(handle, wide(name), nil, &kind, nil, &size)
	if err == syscall.ERROR_FILE_NOT_FOUND {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if kind != syscall.REG_SZ || size > 65536 {
		return "", fmt.Errorf("invalid registry string")
	}
	v := make([]uint16, size/2+1)
	if err := syscall.RegQueryValueEx(handle, wide(name), nil, &kind, (*byte)(unsafe.Pointer(&v[0])), &size); err != nil {
		return "", err
	}
	return syscall.UTF16ToString(v), nil
}

func DeleteValue(key, name string) error {
	var handle syscall.Handle
	err := syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, wide(key), 0, syscall.KEY_SET_VALUE, &handle)
	if err == syscall.ERROR_FILE_NOT_FOUND {
		return nil
	}
	if err != nil {
		return err
	}
	defer syscall.RegCloseKey(handle)
	r, _, _ := advapi.NewProc("RegDeleteValueW").Call(uintptr(handle), uintptr(unsafe.Pointer(wide(name))))
	if r != 0 && r != 2 {
		return syscall.Errno(r)
	}
	return nil
}

func DeleteKey(key string) error {
	r, _, _ := advapi.NewProc("RegDeleteKeyW").Call(uintptr(syscall.HKEY_CURRENT_USER), uintptr(unsafe.Pointer(wide(key))))
	if r != 0 && r != 2 {
		return syscall.Errno(r)
	}
	return nil
}

// startupTaskName é a tarefa agendada que inicia o wintaskbar ao fazer logon.
const startupTaskName = "wintaskbar"

func hidden(cmd *exec.Cmd) *exec.Cmd {
	// Não criar console: ocultar a janela ainda permite que o host de console
	// interfira no foco durante a consulta periódica da tarefa de logon.
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	return cmd
}

func system32(name string) string {
	return filepath.Join(os.Getenv("SystemRoot"), "System32", name)
}

func taskExists(name string) bool {
	return hidden(exec.Command(system32("schtasks.exe"), "/Query", "/TN", name)).Run() == nil
}

func deleteTask(name string) {
	_ = hidden(exec.Command(system32("schtasks.exe"), "/Delete", "/TN", name, "/F")).Run()
}

// registerTask cria (ou substitui) a tarefa de logon do usuário atual para exe.
// O XML vai por -EncodedCommand para que nenhum caractere do caminho seja
// tratado como código PowerShell.
func registerTask(name, exe string) error {
	who, err := user.Current()
	if err != nil {
		return err
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	script := "$ErrorActionPreference='Stop'; Register-ScheduledTask -TaskName " + quote(name) +
		" -Xml " + quote(StartupTaskXML(exe, who.Username)) + " -Force | Out-Null"
	units := utf16.Encode([]rune(script))
	raw := make([]byte, 0, len(units)*2)
	for _, u := range units {
		raw = append(raw, byte(u), byte(u>>8))
	}
	cmd := hidden(exec.Command(system32(filepath.Join("WindowsPowerShell", "v1.0", "powershell.exe")),
		"-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(raw)))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("register startup task: %w: %s", err, out)
	}
	return nil
}

// StartupEnabled informa se o wintaskbar inicia com o Windows: pela tarefa
// agendada ou, em instalações anteriores, pela chave Run.
func StartupEnabled() (bool, error) {
	if taskExists(startupTaskName) {
		return true, nil
	}
	v, err := ReadString(RunKey, "wintaskbar")
	return v != "", err
}

// SetStartup liga ou desliga a inicialização com o Windows usando o exe atual.
func SetStartup(enabled bool) error {
	if !enabled {
		deleteTask(startupTaskName)
		return DeleteValue(RunKey, "wintaskbar")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return SetStartupFor(exe)
}

// SetStartupFor liga a inicialização com o Windows apontando para exe. Prefere a
// tarefa agendada, que roda ao logon sem o atraso da chave Run; se ela não puder
// ser criada, usa a chave Run, que funciona em qualquer conta.
func SetStartupFor(exe string) error {
	if err := registerTask(startupTaskName, exe); err != nil {
		return SetString(RunKey, "wintaskbar", `"`+exe+`"`)
	}
	return DeleteValue(RunKey, "wintaskbar") // migra instalações anteriores
}

func Open(path string) error {
	shell := syscall.NewLazyDLL(filepath.Join(os.Getenv("SystemRoot"), "System32", "shell32.dll"))
	r, _, _ := shell.NewProc("ShellExecuteW").Call(0, uintptr(unsafe.Pointer(wide("open"))), uintptr(unsafe.Pointer(wide(path))), 0, 0, 1)
	if r <= 32 {
		return fmt.Errorf("open %s: shell error %d", path, r)
	}
	return nil
}

func Edit(path string) error {
	cmd := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "notepad.exe"), path)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// Shortcut uses Windows' built-in COM shell through a hidden PowerShell process.
// As aspas literais evitam que caminhos sejam interpretados como código PowerShell.
func Shortcut(path, target, args string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	script := "$ErrorActionPreference='Stop'; $link=(New-Object -ComObject WScript.Shell).CreateShortcut(" + quote(path) + "); $link.TargetPath=" + quote(target) + "; $link.Arguments=" + quote(args) + "; $link.WorkingDirectory=" + quote(filepath.Dir(target)) + "; $link.Save()"
	cmd := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create shortcut: %w: %s", err, out)
	}
	return nil
}
