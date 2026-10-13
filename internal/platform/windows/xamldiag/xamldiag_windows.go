//go:build windows

// Package xamldiag encapsula a exportação não documentada InitializeXamlDiagnosticsEx de
// Windows.UI.Xaml.dll, que faz um processo alvo carregar uma DLL TAP (Tool Access
// Point) e lhe entregar um site IXamlDiagnostics.
package xamldiag

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	errs "github.com/engenheiroaraujo/wintaskbar/internal/platform/errors"
)

// CLSIDTap é o CLSID servido pela tap.dll (veja tap/tap.cpp,
// CLSID_WinTaskbarTAP): {7B1A2C54-3F0E-4D6A-9C1B-5A2E8F4D7B10}.
var CLSIDTap = syscall.GUID{
	Data1: 0x7b1a2c54,
	Data2: 0x3f0e,
	Data3: 0x4d6a,
	Data4: [8]byte{0x9c, 0x1b, 0x5a, 0x2e, 0x8f, 0x4d, 0x7b, 0x10},
}

const (
	hresultNotFound     = 0x80070490 // HRESULT_FROM_WIN32(ERROR_NOT_FOUND)
	coinitMultiThreaded = 0x0
	// Quantos nomes de ponto VisualDiagConnectionN testar. O runtime
	// não oferece como saber qual está livre, então o nome é procurado.
	maxConnections = 10000
)

// Error carrega o HRESULT da falha de InitializeXamlDiagnosticsEx.
type Error struct {
	HRESULT uint32
	Name    string
}

func (e *Error) Error() string {
	return fmt.Sprintf("InitializeXamlDiagnosticsEx(%s) failed: HRESULT 0x%08X", e.Name, e.HRESULT)
}

// Inject pede ao processo pid que carregue tapDLL e o anexe ao seu diagnóstico
// XAML. tapDLL deve ser um caminho absoluto para uma DLL com a mesma arquitetura do
// alvo. A chamada deve vir da mesma sessão de usuário do alvo.
func Inject(pid uint32, tapDLL string) error {
	abs, err := filepath.Abs(tapDLL)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); err != nil {
		return errs.Wrap(errs.InvalidInput, "o arquivo do TAP não foi encontrado", err)
	}

	// O estado COM é por thread, então toda a sequência de chamadas fica em uma só.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	ole32 := syscall.NewLazyDLL(filepath.Join(os.Getenv("SystemRoot"), "System32", "ole32.dll"))
	coInit := ole32.NewProc("CoInitializeEx")
	coUninit := ole32.NewProc("CoUninitialize")
	if hr, _, _ := coInit.Call(0, coinitMultiThreaded); int32(hr) < 0 {
		return fmt.Errorf("CoInitializeEx: HRESULT 0x%08X", uint32(hr))
	}
	defer coUninit.Call()

	wux := syscall.NewLazyDLL(filepath.Join(os.Getenv("SystemRoot"), "System32", "Windows.UI.Xaml.dll"))
	initProc := wux.NewProc("InitializeXamlDiagnosticsEx")
	if err := initProc.Find(); err != nil {
		return fmt.Errorf("InitializeXamlDiagnosticsEx not available: %w", err)
	}

	empty, _ := syscall.UTF16PtrFromString("")
	dll, err := syscall.UTF16PtrFromString(abs)
	if err != nil {
		return err
	}

	var lastName string
	for i := 1; i <= maxConnections; i++ {
		lastName = fmt.Sprintf("VisualDiagConnection%d", i)
		name, _ := syscall.UTF16PtrFromString(lastName)

		// HRESULT InitializeXamlDiagnosticsEx(
		//     LPCWSTR endPointName, DWORD pid, LPCWSTR wszDllXamlDiagnostics,
		//     LPCWSTR wszTAPDllName, CLSID tapClsid, LPCWSTR wszInitializationData);
		// Em x64 o CLSID de 16 bytes é passado por referência a uma cópia do chamador.
		clsid := CLSIDTap
		hr, _, _ := initProc.Call(
			uintptr(unsafe.Pointer(name)),
			uintptr(pid),
			uintptr(unsafe.Pointer(empty)),
			uintptr(unsafe.Pointer(dll)),
			uintptr(unsafe.Pointer(&clsid)),
			0,
		)
		runtime.KeepAlive(name)
		runtime.KeepAlive(&clsid)

		switch uint32(hr) {
		case 0:
			return nil
		case hresultNotFound:
			continue // nome de ponto não usado pelo alvo; tenta o próximo
		default:
			return errs.Wrap(errs.Unavailable, "o explorer recusou o TAP", &Error{HRESULT: uint32(hr), Name: lastName})
		}
	}
	return errs.ErrNoDiagConnection
}

// Injector adapta Inject às interfaces dos consumidores.
type Injector struct{}

// Inject implementa a interface do consumidor chamando a função do pacote.
func (Injector) Inject(pid uint32, tapDLL string) error { return Inject(pid, tapDLL) }
