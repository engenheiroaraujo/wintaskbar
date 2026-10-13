//go:build windows

package tray

import (
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"unsafe"
)

var (
	gdi            = syscall.NewLazyDLL(filepath.Join(os.Getenv("SystemRoot"), "System32", "gdi32.dll"))
	dwm            = syscall.NewLazyDLL(filepath.Join(os.Getenv("SystemRoot"), "System32", "dwmapi.dll"))
	popupClassOnce sync.Once
	popupClassOK   bool
	popupWindows   sync.Map
	popupCallback  = syscall.NewCallback(popupWndProc)
)

const popupClass = "WinTaskbar.Menu.v1"

type menuRow struct {
	bounds rectangle
	window uintptr
}

type popupMenu struct {
	owner                                               *Icon
	hwnd                                                uintptr
	items                                               []Item
	rows                                                []menuRow
	dpi                                                 int32
	font, iconFont, background, hoverBrush, borderBrush uintptr
	foreground, muted, border                           uint32
	dark                                                bool
	hot, focused                                        int
	keyboard                                            bool
	closing                                             bool
	passive                                             bool
	input                                               popupInput
}

type monitorInfo struct {
	Size          uint32
	Monitor, Work rectangle
	Flags         uint32
}

type paintStruct struct {
	DC                 uintptr
	Erase              int32
	Rect               rectangle
	Restore, IncUpdate int32
	Reserved           [32]byte
}

type drawItem struct {
	Type, ID, ItemID, Action, State uint32
	Window, DC                      uintptr
	Rect                            rectangle
	Data                            uintptr
}

func highContrast() bool {
	var hc struct {
		Size, Flags uint32
		Scheme      uintptr
	}
	hc.Size = uint32(unsafe.Sizeof(hc))
	user.NewProc("SystemParametersInfoW").Call(0x42, uintptr(hc.Size), uintptr(unsafe.Pointer(&hc)), 0)
	return hc.Flags&1 != 0
}

func darkApps() bool {
	var key syscall.Handle
	if syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, ptr(`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`), 0, syscall.KEY_QUERY_VALUE, &key) != nil {
		return false
	}
	defer syscall.RegCloseKey(key)
	var value, kind uint32
	size := uint32(4)
	err := syscall.RegQueryValueEx(key, ptr("AppsUseLightTheme"), nil, &kind, (*byte)(unsafe.Pointer(&value)), &size)
	return err == nil && kind == syscall.REG_DWORD && size == 4 && value == 0
}

func (m *popupMenu) px(n int32) int32 { return (n*m.dpi + 48) / 96 }

func newMenuFont(name string, height int32) uintptr {
	font, _, _ := gdi.NewProc("CreateFontW").Call(uintptr(-height), 0, 0, 0, 400, 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(ptr(name))))
	return font
}

func (i *Icon) openPopup() bool {
	return i.openPopupMode(false)
}

func (i *Icon) openPopupMode(passive bool) bool {
	popupClassOnce.Do(func() {
		instance, _, _ := kernel.NewProc("GetModuleHandleW").Call(0)
		cursor, _, _ := user.NewProc("LoadCursorW").Call(0, 32512)
		class := windowClass{Instance: instance, ClassName: ptr(popupClass), Callback: popupCallback, Cursor: cursor}
		class.Size = uint32(unsafe.Sizeof(class))
		r, _, _ := user.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&class)))
		popupClassOK = r != 0
	})
	if !popupClassOK || len(i.items) == 0 {
		return false
	}

	var anchor rectangle
	var identifier struct {
		Size   uint32
		Window uintptr
		ID     uint32
		GUID   syscall.GUID
	}
	identifier.Size, identifier.Window, identifier.ID = uint32(unsafe.Sizeof(identifier)), i.hwnd, i.data.ID
	hr, _, _ := shell.NewProc("Shell_NotifyIconGetRect").Call(uintptr(unsafe.Pointer(&identifier)), uintptr(unsafe.Pointer(&anchor)))
	if int32(hr) < 0 {
		var cursor point
		user.NewProc("GetCursorPos").Call(uintptr(unsafe.Pointer(&cursor)))
		anchor = rectangle{cursor.X, cursor.Y, cursor.X, cursor.Y}
	}
	monitor, _, _ := user.NewProc("MonitorFromRect").Call(uintptr(unsafe.Pointer(&anchor)), 2)
	info := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if ok, _, _ := user.NewProc("GetMonitorInfoW").Call(monitor, uintptr(unsafe.Pointer(&info))); ok == 0 {
		return false
	}
	m := &popupMenu{owner: i, items: append([]Item(nil), i.items...), dpi: 96, dark: darkApps(), hot: -1, focused: -1, passive: passive}
	// Criar no monitor do ícone antes de consultar a escala da janela.
	instance, _, _ := kernel.NewProc("GetModuleHandleW").Call(0)
	extendedStyle := uintptr(0x80 | 0x8)
	if passive {
		extendedStyle |= 0x08000000 // WS_EX_NOACTIVATE: mantém o painel do Shell ativo.
	}
	hwnd, _, _ := user.NewProc("CreateWindowExW").Call(extendedStyle, uintptr(unsafe.Pointer(ptr(popupClass))), uintptr(unsafe.Pointer(ptr("wintaskbar"))),
		0x80000000|0x40000|0x02000000, uintptr(anchor.Left), uintptr(anchor.Top), 1, 1, i.hwnd, 0, instance, 0)
	if hwnd == 0 {
		return false
	}
	m.hwnd = hwnd
	popupWindows.Store(hwnd, m)
	if dpi, _, _ := user.NewProc("GetDpiForWindow").Call(hwnd); dpi != 0 {
		m.dpi = int32(dpi)
	}
	m.font, m.iconFont = newMenuFont("Segoe UI", m.px(12)), newMenuFont("Segoe Fluent Icons", m.px(12))
	if m.font == 0 || m.iconFont == 0 {
		m.close()
		return false
	}
	m.setPalette()
	if m.background == 0 || m.hoverBrush == 0 || m.borderBrush == 0 {
		m.close()
		return false
	}
	width, height := m.layout()
	if width > info.Work.Right-info.Work.Left-m.px(8) || height > info.Work.Bottom-info.Work.Top-m.px(8) {
		m.close()
		return false // O menu do sistema sabe rolar quando não há espaço vertical.
	}
	position := placePopup(anchor, info.Work, width, height, m.px(8))
	user.NewProc("SetWindowPos").Call(hwnd, 0, uintptr(position.Left), uintptr(position.Top), uintptr(width), uintptr(height), 0x14)
	for n, item := range m.items {
		if item.Label == "" {
			continue
		}
		className, style := "BUTTON", uintptr(0x5001000B) // WS_CHILD|VISIBLE|TABSTOP, BS_OWNERDRAW
		if item.ID == 0 {
			className, style = "STATIC", 0x5000000D
		} // SS_OWNERDRAW, cabeçalho acessível
		r := m.rows[n].bounds
		child, _, _ := user.NewProc("CreateWindowExW").Call(0, uintptr(unsafe.Pointer(ptr(className))), uintptr(unsafe.Pointer(ptr(accessibleLabel(item)))), style,
			uintptr(r.Left), uintptr(r.Top), uintptr(r.Right-r.Left), uintptr(r.Bottom-r.Top), hwnd, uintptr(1000+n), instance, 0)
		if child == 0 {
			m.close()
			return false
		}
		m.rows[n].window = child
		user.NewProc("SendMessageW").Call(child, 0x30, m.font, 0) // WM_SETFONT
		if item.Disabled {
			user.NewProc("EnableWindow").Call(child, 0)
		}
	}
	i.popup = m
	if passive {
		if !m.startInput() {
			m.close()
			return false
		}
		user.NewProc("ShowWindow").Call(hwnd, 4) // SW_SHOWNOACTIVATE
	} else {
		user.NewProc("SetForegroundWindow").Call(hwnd)
		user.NewProc("ShowWindow").Call(hwnd, 5)
	}
	m.focus(nextMenuItem(m.items, -1, 1))
	return true
}

func (m *popupMenu) setPalette() {
	for _, brush := range []uintptr{m.background, m.hoverBrush, m.borderBrush} {
		if brush != 0 {
			gdi.NewProc("DeleteObject").Call(brush)
		}
	}
	background, hover := uint32(0xF9F9F9), uint32(0xEBEBEB)
	m.foreground, m.muted, m.border = 0x202020, 0x777777, 0xD5D5D5
	if m.dark {
		background, hover, m.foreground, m.muted, m.border = 0x292929, 0x383838, 0xF5F5F5, 0xA0A0A0, 0x505050
	}
	m.background, _, _ = gdi.NewProc("CreateSolidBrush").Call(uintptr(background))
	m.hoverBrush, _, _ = gdi.NewProc("CreateSolidBrush").Call(uintptr(hover))
	m.borderBrush, _, _ = gdi.NewProc("CreateSolidBrush").Call(uintptr(m.border))
	corner, dark := uint32(2), uint32(0)
	if m.dark {
		dark = 1
	}
	for attribute, value := range map[uintptr]*uint32{33: &corner, 20: &dark, 34: &m.border} {
		dwm.NewProc("DwmSetWindowAttribute").Call(m.hwnd, attribute, uintptr(unsafe.Pointer(value)), 4)
	}
}

func (m *popupMenu) layout() (int32, int32) {
	width := m.px(160)
	dc, _, _ := user.NewProc("GetDC").Call(m.hwnd)
	old, _, _ := gdi.NewProc("SelectObject").Call(dc, m.font)
	for _, item := range m.items {
		r := rectangle{}
		user.NewProc("DrawTextW").Call(dc, uintptr(unsafe.Pointer(ptr(item.Label))), ^uintptr(0), uintptr(unsafe.Pointer(&r)), 0x400|0x800|0x20)
		width = max(width, r.Right+m.px(36))
	}
	gdi.NewProc("SelectObject").Call(dc, old)
	user.NewProc("ReleaseDC").Call(m.hwnd, dc)
	y := m.px(4)
	for _, item := range m.items {
		height := m.px(24)
		if item.Label == "" {
			height = m.px(7)
		} else if item.ID == 0 {
			height = m.px(28)
		}
		m.rows = append(m.rows, menuRow{bounds: rectangle{m.px(4), y, width - m.px(4), y + height}})
		y += height
	}
	return width, y + m.px(4)
}

func (m *popupMenu) close() {
	if m.hwnd != 0 && !m.closing {
		m.closing = true
		user.NewProc("DestroyWindow").Call(m.hwnd)
	}
}

func (m *popupMenu) release() {
	m.stopInput()
	popupWindows.Delete(m.hwnd)
	m.hwnd = 0
	if m.owner.popup == m {
		m.owner.popup = nil
	}
	for _, object := range []uintptr{m.font, m.iconFont, m.background, m.hoverBrush, m.borderBrush} {
		if object != 0 {
			gdi.NewProc("DeleteObject").Call(object)
		}
	}
}

func (m *popupMenu) activate(n int) {
	if m.hwnd == 0 || n < 0 || n >= len(m.items) {
		return
	}
	item := m.items[n]
	if item.ID == 0 || item.Disabled || item.Label == "" {
		return
	}
	m.close() // Fecha antes da ação; impede execução duplicada por Enter/click.
	m.owner.action(item.ID)
}

func (m *popupMenu) focus(n int) {
	if n < 0 {
		return
	}
	m.focused = n
	if m.passive {
		user.NewProc("InvalidateRect").Call(m.hwnd, 0, 0)
		for _, row := range m.rows {
			if row.window != 0 {
				user.NewProc("InvalidateRect").Call(row.window, 0, 0)
			}
		}
	} else {
		user.NewProc("SetFocus").Call(m.rows[n].window)
	}
}

func (m *popupMenu) refresh(items []Item) {
	if len(items) != len(m.items) {
		m.close()
		return
	}
	for n, item := range items {
		if item.ID != m.items[n].ID || (item.Label == "") != (m.items[n].Label == "") {
			m.close()
			return
		}
		if item == m.items[n] {
			continue
		}
		m.items[n] = item
		if hwnd := m.rows[n].window; hwnd != 0 {
			user.NewProc("SetWindowTextW").Call(hwnd, uintptr(unsafe.Pointer(ptr(accessibleLabel(item)))))
			enabled := uintptr(1)
			if item.Disabled {
				enabled = 0
			}
			user.NewProc("EnableWindow").Call(hwnd, enabled)
			user.NewProc("InvalidateRect").Call(hwnd, 0, 0)
		}
	}
	if m.focused >= 0 && m.items[m.focused].Disabled {
		m.focus(nextMenuItem(m.items, m.focused, 1))
	}
}

func (m *popupMenu) filter(msg *message) bool {
	inside := msg.Window == m.hwnd
	index := -1
	for n, row := range m.rows {
		if row.window != 0 && row.window == msg.Window {
			inside, index = true, n
			break
		}
	}
	if !inside {
		return false
	}
	if m.passive {
		switch msg.ID {
		case 0x201: // Não entregar ao BUTTON: seu SetFocus ativaria o popup.
			m.input.pressed = index
			return true
		case 0x202:
			pressed := m.input.pressed
			m.input.pressed = -1
			if index >= 0 && index == pressed {
				m.activate(index)
			}
			return true
		case 0x203, 0x204, 0x205, 0x206, 0x207, 0x208, 0x209:
			return true
		}
	}
	if msg.ID == 0x200 || msg.ID == 0x2A3 { // WM_MOUSEMOVE / WM_MOUSELEAVE
		if msg.ID == 0x2A3 || index < 0 || m.items[index].Disabled {
			index = -1
		}
		if m.hot != index {
			old := m.hot
			m.hot = index
			for _, n := range []int{old, index} {
				if n >= 0 {
					user.NewProc("InvalidateRect").Call(m.rows[n].window, 0, 0)
				}
			}
		}
		if index >= 0 {
			var track struct {
				Size, Flags uint32
				Window      uintptr
				Time        uint32
			}
			track.Size, track.Flags, track.Window = uint32(unsafe.Sizeof(track)), 2, msg.Window
			user.NewProc("TrackMouseEvent").Call(uintptr(unsafe.Pointer(&track)))
		}
	}
	if msg.ID != 0x100 {
		return false
	} // WM_KEYDOWN; Space continua com o botão nativo.
	if index >= 0 {
		m.focused = index
	}
	m.keyboard = true
	switch msg.WParam {
	case 0x1B:
		m.close()
	case 0x0D:
		m.activate(m.focused)
	case 0x20:
		if !m.passive {
			return false
		}
		m.activate(m.focused)
	case 0x26:
		m.focus(nextMenuItem(m.items, m.focused, -1))
	case 0x28:
		m.focus(nextMenuItem(m.items, m.focused, 1))
	case 0x24:
		m.focus(nextMenuItem(m.items, -1, 1))
	case 0x23:
		m.focus(nextMenuItem(m.items, 0, -1))
	case 0x09:
		shift, _, _ := user.NewProc("GetKeyState").Call(0x10)
		if m.passive {
			shift, _, _ = user.NewProc("GetAsyncKeyState").Call(0x10)
		}
		step := 1
		if int16(shift) < 0 {
			step = -1
		}
		m.focus(nextMenuItem(m.items, m.focused, step))
	default:
		return false
	}
	return true
}

func popupWndProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) uintptr {
	if msg == 0x83 {
		return 0
	} // WM_NCCALCSIZE: mantém a sombra DWM sem moldura de redimensionamento.
	if msg == 0x84 {
		return 1
	} // WM_NCHITTEST: não redimensionar o popup.
	if value, ok := popupWindows.Load(hwnd); ok {
		m := value.(*popupMenu)
		switch msg {
		case wmPopupKey:
			m.filter(&message{Window: hwnd, ID: 0x100, WParam: wparam})
			return 0
		case 0x21: // WM_MOUSEACTIVATE: receber o clique sem desativar o painel do Shell.
			if m.passive {
				return 3 // MA_NOACTIVATE
			}
		case 0x86: // WM_NCACTIVATE: deixa o DWM processar a ativação.
		case 0x06:
			if uint16(wparam) == 0 {
				m.close()
			}
			return 0
		case 0x10:
			m.close()
			return 0
		case 0x82:
			m.release() // WM_NCDESTROY: os controles filhos já foram destruídos.
		case 0x111:
			if wparam>>16 == 0 {
				n := int(uint16(wparam)) - 1000
				if n >= 0 && n < len(m.rows) && lparam == m.rows[n].window {
					m.activate(n)
				}
			}
			return 0
		case 0x2B:
			// Copia a estrutura nativa sem tratar outros LPARAMs (inteiros) como
			// ponteiros Go durante callbacks, GC ou crescimento da pilha.
			var draw drawItem
			kernel.NewProc("RtlMoveMemory").Call(uintptr(unsafe.Pointer(&draw)), lparam, unsafe.Sizeof(draw))
			n := int(draw.ID) - 1000
			if n >= 0 && n < len(m.items) {
				m.drawRow(n, &draw)
			}
			return 1
		case 0x14:
			return 1 // WM_ERASEBKGND: pintura completa em WM_PAINT.
		case 0x0F:
			var paint paintStruct
			dc, _, _ := user.NewProc("BeginPaint").Call(hwnd, uintptr(unsafe.Pointer(&paint)))
			user.NewProc("FillRect").Call(dc, uintptr(unsafe.Pointer(&paint.Rect)), m.background)
			for n, item := range m.items {
				if item.Label == "" {
					r := m.rows[n].bounds
					r.Left += m.px(8)
					r.Right -= m.px(8)
					r.Top += (r.Bottom - r.Top) / 2
					r.Bottom = r.Top + 1
					user.NewProc("FillRect").Call(dc, uintptr(unsafe.Pointer(&r)), m.borderBrush)
				}
			}
			user.NewProc("EndPaint").Call(hwnd, uintptr(unsafe.Pointer(&paint)))
			return 0
		case 0x1A, 0x31A, 0x02E0:
			m.close()
			return 0 // Configuração, tema ou DPI: recriar ao abrir.
		}
	}
	r, _, _ := user.NewProc("DefWindowProcW").Call(hwnd, uintptr(msg), wparam, lparam)
	return r
}

func (m *popupMenu) drawRow(n int, draw *drawItem) {
	item, dc, r := m.items[n], draw.DC, draw.Rect
	focused := draw.State&0x10 != 0 || m.passive && m.focused == n
	user.NewProc("FillRect").Call(dc, uintptr(unsafe.Pointer(&r)), m.background)
	if !item.Disabled && (m.hot == n || draw.State&1 != 0 || m.keyboard && focused) {
		oldBrush, _, _ := gdi.NewProc("SelectObject").Call(dc, m.hoverBrush)
		nullPen, _, _ := gdi.NewProc("GetStockObject").Call(8)
		oldPen, _, _ := gdi.NewProc("SelectObject").Call(dc, nullPen)
		gdi.NewProc("RoundRect").Call(dc, 0, 0, uintptr(r.Right), uintptr(r.Bottom), uintptr(m.px(8)), uintptr(m.px(8)))
		gdi.NewProc("SelectObject").Call(dc, oldPen)
		gdi.NewProc("SelectObject").Call(dc, oldBrush)
	}
	color := m.foreground
	if item.Disabled {
		color = m.muted
	}
	gdi.NewProc("SetTextColor").Call(dc, uintptr(color))
	gdi.NewProc("SetBkMode").Call(dc, 1)
	oldFont, _, _ := gdi.NewProc("SelectObject").Call(dc, m.font)
	text := r
	text.Left += m.px(8)
	text.Right -= m.px(20)
	if item.ID == 0 {
		text.Left = m.px(8)
		text.Right = r.Right - m.px(8)
	}
	drawMenuText(dc, item.Label, &text)
	gdi.NewProc("SelectObject").Call(dc, m.iconFont)
	if item.Checked {
		check := r
		check.Left = r.Right - m.px(16)
		check.Right = r.Right - m.px(4)
		drawMenuText(dc, "\uE73E", &check)
	}
	gdi.NewProc("SelectObject").Call(dc, oldFont)
	if m.keyboard && focused {
		focus := r
		focus.Left++
		focus.Top++
		focus.Right--
		focus.Bottom--
		user.NewProc("DrawFocusRect").Call(dc, uintptr(unsafe.Pointer(&focus)))
	}
}

func drawMenuText(dc uintptr, text string, rect *rectangle) {
	user.NewProc("DrawTextW").Call(dc, uintptr(unsafe.Pointer(ptr(text))), ^uintptr(0), uintptr(unsafe.Pointer(rect)), 0x20|0x04|0x800|0x8000)
}
