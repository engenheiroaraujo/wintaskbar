//go:build windows

package tray

// Captura opcional apenas da janela de teste, para revisão do desenho real.
import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"unsafe"
)

func captureTestMenu(m *popupMenu, directory string) error {
	if err := os.MkdirAll(directory, 0755); err != nil {
		return err
	}
	previousDark := m.dark
	defer func() { m.dark = previousDark; m.setPalette() }()
	for _, dark := range []bool{false, true} {
		m.dark = dark
		m.setPalette()
		user.NewProc("RedrawWindow").Call(m.hwnd, 0, 0, 0x181) // invalidar filhos e pintar já
		name := "menu-light.png"
		if dark {
			name = "menu-dark.png"
		}
		if err := captureTestWindow(m.hwnd, filepath.Join(directory, name)); err != nil {
			return err
		}
	}
	return nil
}

func captureTestWindow(hwnd uintptr, path string) error {
	var rect rectangle
	user.NewProc("GetClientRect").Call(hwnd, uintptr(unsafe.Pointer(&rect)))
	width, height := int(rect.Right), int(rect.Bottom)
	if width <= 0 || height <= 0 {
		return fmt.Errorf("invalid capture bounds")
	}
	dc, _, _ := user.NewProc("GetDC").Call(hwnd)
	defer user.NewProc("ReleaseDC").Call(hwnd, dc)
	memory, _, _ := gdi.NewProc("CreateCompatibleDC").Call(dc)
	if memory == 0 {
		return fmt.Errorf("create capture DC failed")
	}
	defer gdi.NewProc("DeleteDC").Call(memory)
	var bitmapInfo struct {
		Size                        uint32
		Width, Height               int32
		Planes, Bits                uint16
		Compression, ImageSize      uint32
		XPixels, YPixels            int32
		ColorsUsed, ColorsImportant uint32
	}
	bitmapInfo.Size, bitmapInfo.Width, bitmapInfo.Height = 40, int32(width), -int32(height)
	bitmapInfo.Planes, bitmapInfo.Bits = 1, 32
	var pixels unsafe.Pointer
	bitmap, _, _ := gdi.NewProc("CreateDIBSection").Call(dc, uintptr(unsafe.Pointer(&bitmapInfo)), 0, uintptr(unsafe.Pointer(&pixels)), 0, 0)
	if bitmap == 0 {
		return fmt.Errorf("create capture bitmap failed")
	}
	defer gdi.NewProc("DeleteObject").Call(bitmap)
	old, _, _ := gdi.NewProc("SelectObject").Call(memory, bitmap)
	defer gdi.NewProc("SelectObject").Call(memory, old)
	if ok, _, _ := user.NewProc("PrintWindow").Call(hwnd, memory, 3); ok == 0 {
		return fmt.Errorf("PrintWindow failed")
	}
	source := unsafe.Slice((*byte)(pixels), width*height*4)
	output := image.NewRGBA(image.Rect(0, 0, width, height))
	for n := 0; n < len(source); n += 4 {
		output.Pix[n], output.Pix[n+1], output.Pix[n+2], output.Pix[n+3] = source[n+2], source[n+1], source[n], 255
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return png.Encode(file, output)
}

func TestNativeMenuAppearance(t *testing.T) {
	directory := os.Getenv("WINTASKBAR_MENU_CAPTURE_DIR")
	if directory == "" {
		t.Skip("set WINTASKBAR_MENU_CAPTURE_DIR for native menu captures")
	}
	var failure error
	err := runWithClass(WindowClass+".AppearanceTest", func(i *Icon) {
		i.items = []Item{
			{Label: "wintaskbar — tema ativo", Disabled: true}, {},
			{ID: 1, Label: "Aplicar / recarregar tema", Icon: "\uE72C"},
			{ID: 2, Label: "Desativar tema (restaurar barra)", Icon: "\uE71A"},
			{ID: 3, Label: "Editar tema…", Icon: "\uE70F"}, {},
			{ID: 4, Label: "Ocultar automaticamente", Icon: "\uE76C", Toggle: true},
			{ID: 5, Label: "Iniciar com o Windows", Icon: "\uE768", Toggle: true, Checked: true}, {},
			{ID: 6, Label: "Abrir pasta de logs", Icon: "\uE8B7"},
			{ID: 7, Label: "Ver último erro…", Icon: "\uE946", Disabled: true},
			{ID: 100, Label: "Sair e restaurar a barra", Icon: "\uE7E8"},
		}
		if !i.openPopup() {
			failure = fmt.Errorf("preview menu creation failed")
		} else {
			failure = captureTestMenu(i.popup, directory)
		}
		i.Post(Update{Quit: true})
	}, func(uint32) {})
	if err != nil {
		t.Fatal(err)
	}
	if failure != nil {
		t.Fatal(failure)
	}
}
