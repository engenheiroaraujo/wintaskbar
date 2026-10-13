//go:build windows

package tray

// Geometria e navegação sem chamadas ao Windows, compartilhadas pelos testes.
type rectangle struct{ Left, Top, Right, Bottom int32 }

func placePopup(anchor, work rectangle, width, height, gap int32) rectangle {
	x, y := anchor.Right-width, anchor.Top-height-gap
	if y < work.Top {
		y = anchor.Bottom + gap
	}
	x = max(work.Left, min(x, work.Right-width))
	y = max(work.Top, min(y, work.Bottom-height))
	return rectangle{x, y, x + width, y + height}
}

func nextMenuItem(items []Item, start, step int) int {
	for n := 0; n < len(items); n++ {
		start = (start + step + len(items)) % len(items)
		if items[start].ID != 0 && items[start].Label != "" && !items[start].Disabled {
			return start
		}
	}
	return -1
}

func accessibleLabel(item Item) string {
	if item.Toggle {
		if item.Checked {
			return item.Label + ", ativado"
		}
		return item.Label + ", desativado"
	}
	return item.Label
}
