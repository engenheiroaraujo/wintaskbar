//go:build windows

package xamldiag

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"unsafe"
)

// TestTapComWiring carrega a tap.dll neste processo (que não é o explorer) e confere a
// ligação COM: DllGetClassObject serve o CLSID, a factory cria o
// objeto, e ele implementa IObjectWithSite. Nunca toca no explorer.exe.
func TestTapComWiring(t *testing.T) {
	dll := filepath.Join("..", "..", "..", "..", "..", "tap", "build", "tap.dll")
	if override := os.Getenv("WINTASKBAR_TEST_TAP"); override != "" {
		dll = override
	}
	abs, _ := filepath.Abs(dll)
	if _, err := os.Stat(abs); err != nil {
		t.Skipf("tap.dll not built (%s); run scripts\\build-tap.ps1", abs)
	}

	ole32 := syscall.NewLazyDLL("ole32.dll")
	if hr, _, _ := ole32.NewProc("CoInitializeEx").Call(0, 0); int32(hr) < 0 {
		t.Fatalf("CoInitializeEx: 0x%08X", uint32(hr))
	}
	defer ole32.NewProc("CoUninitialize").Call()

	lib, err := syscall.LoadLibrary(abs)
	if err != nil {
		t.Fatalf("LoadLibrary: %v", err)
	}
	defer syscall.FreeLibrary(lib)

	getClassObject, err := syscall.GetProcAddress(lib, "DllGetClassObject")
	if err != nil {
		t.Fatalf("export DllGetClassObject: %v", err)
	}
	if _, err := syscall.GetProcAddress(lib, "DllCanUnloadNow"); err != nil {
		t.Fatalf("export DllCanUnloadNow: %v", err)
	}

	// IID_IClassFactory {00000001-0000-0000-C000-000000000046}
	iidFactory := syscall.GUID{Data1: 1, Data4: [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	// IID_IObjectWithSite {FC4801A3-2BA9-11CF-A229-00AA003D7352}
	iidSite := syscall.GUID{Data1: 0xFC4801A3, Data2: 0x2BA9, Data3: 0x11CF,
		Data4: [8]byte{0xA2, 0x29, 0x00, 0xAA, 0x00, 0x3D, 0x73, 0x52}}

	clsid := CLSIDTap
	type comObject struct{ VTable *[5]uintptr }
	var factory *comObject
	hr, _, _ := syscall.SyscallN(getClassObject,
		uintptr(unsafe.Pointer(&clsid)), uintptr(unsafe.Pointer(&iidFactory)), uintptr(unsafe.Pointer(&factory)))
	if hr != 0 || factory == nil {
		t.Fatalf("DllGetClassObject: hr=0x%08X factory=%p", uint32(hr), factory)
	}

	// IClassFactory vtable: QI(0) AddRef(1) Release(2) CreateInstance(3) LockServer(4)

	var site *comObject
	hr, _, _ = syscall.SyscallN(factory.VTable[3], uintptr(unsafe.Pointer(factory)), 0,
		uintptr(unsafe.Pointer(&iidSite)), uintptr(unsafe.Pointer(&site)))
	if hr != 0 || site == nil {
		t.Fatalf("CreateInstance(IObjectWithSite): hr=0x%08X obj=%p", uint32(hr), site)
	}

	// IObjectWithSite vtable: ... SetSite(3) GetSite(4). SetSite(NULL) deve ter sucesso.
	if hr, _, _ = syscall.SyscallN(site.VTable[3], uintptr(unsafe.Pointer(site)), 0); hr != 0 {
		t.Fatalf("SetSite(NULL): hr=0x%08X", uint32(hr))
	}

	syscall.SyscallN(site.VTable[2], uintptr(unsafe.Pointer(site)))       // Release object
	syscall.SyscallN(factory.VTable[2], uintptr(unsafe.Pointer(factory))) // Release factory

	// Um CLSID desconhecido deve ser recusado.
	other := syscall.GUID{Data1: 0xdeadbeef}
	var none uintptr
	hr, _, _ = syscall.SyscallN(getClassObject,
		uintptr(unsafe.Pointer(&other)), uintptr(unsafe.Pointer(&iidFactory)), uintptr(unsafe.Pointer(&none)))
	if uint32(hr) != 0x80040111 { // CLASS_E_CLASSNOTAVAILABLE
		t.Fatalf("foreign CLSID: hr=0x%08X, want CLASS_E_CLASSNOTAVAILABLE", uint32(hr))
	}
}
