package model

import (
	"encoding/xml"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
)

// propertyKind é o tipo de valor que uma propriedade visual aceita.
type propertyKind int

const (
	kindThickness propertyKind = iota
	kindCornerRadius
	kindLength // número ou Auto
	kindNumber
	kindVisibility
	kindHAlign
	kindVAlign
	kindSizing
	kindText
	kindBrush
	kindTransform // somente objeto XAML
	kindDockLayout
)

// properties é o conjunto de propriedades visuais que o TAP sabe definir, por tipo.
var properties = map[string]propertyKind{
	"DockLayoutGap":       kindDockLayout,
	"Margin":              kindThickness,
	"Padding":             kindThickness,
	"BorderThickness":     kindThickness,
	"CornerRadius":        kindCornerRadius,
	"Width":               kindLength,
	"Height":              kindLength,
	"MinWidth":            kindLength,
	"MinHeight":           kindLength,
	"MaxWidth":            kindLength,
	"MaxHeight":           kindLength,
	"Opacity":             kindNumber,
	"FontSize":            kindNumber,
	"Visibility":          kindVisibility,
	"HorizontalAlignment": kindHAlign,
	"VerticalAlignment":   kindVAlign,
	"BackgroundSizing":    kindSizing,
	"FontFamily":          kindText,
	"Text":                kindText,
	"Background":          kindBrush,
	"BorderBrush":         kindBrush,
	"Foreground":          kindBrush,
	"Fill":                kindBrush,
	"Stroke":              kindBrush,
	"RenderTransform":     kindTransform,
}

const xamlNamespace = "http://schemas.microsoft.com/winfx/2006/xaml/presentation"

// Tinta de reserva quando a tinta do blur vem de um recurso de tema que não
// pode ser resolvido: um cinza escuro.
const blurThemeFallback = 0x202020

// resolveValue classifica um valor já expandido para a propriedade e o valida.
func resolveValue(property, value string) (ResolvedStyle, string) {
	kind, ok := properties[property]
	if !ok {
		return ResolvedStyle{}, fmt.Sprintf("propriedade %q não suportada", property)
	}
	v := strings.TrimSpace(value)
	out := ResolvedStyle{Property: property}

	switch {
	case v == "":
		return out, "valor vazio"

	case strings.HasPrefix(v, "<Blur"):
		if kind != kindBrush {
			return out, fmt.Sprintf("%s não aceita <Blur>; use em Background, BorderBrush, Foreground, Fill ou Stroke", property)
		}
		b, problem := parseBlur(v)
		if problem != "" {
			return out, problem
		}
		out.Kind, out.Blur = KindBlur, b
		return out, ""

	case strings.HasPrefix(v, "<"):
		if kind != kindBrush && kind != kindTransform {
			return out, fmt.Sprintf("%s não aceita um objeto XAML", property)
		}
		x, problem := withXAMLNamespace(v)
		if problem != "" {
			return out, problem
		}
		out.Kind, out.Value = KindXAML, x
		return out, ""
	}

	out.Kind, out.Value = KindPlain, v
	if kind == kindDockLayout {
		gap, err := strconv.ParseFloat(v, 64)
		if err != nil || math.IsNaN(gap) || math.IsInf(gap, 0) || gap < 0 || gap > 1000 {
			return out, "DockLayoutGap espera um número finito entre 0 e 1000"
		}
		out.Kind, out.Value = KindDockLayout, strconv.FormatFloat(gap, 'g', -1, 64)
	}
	return out, validatePlain(property, kind, v)
}

func validatePlain(property string, kind propertyKind, v string) string {
	switch kind {
	case kindThickness:
		if n, ok := numbers(v); !ok || (n != 1 && n != 2 && n != 4) {
			return fmt.Sprintf("%s espera 1, 2 ou 4 números (ex.: 0,0,0,4), não %q", property, v)
		}
	case kindCornerRadius:
		if n, ok := numbers(v); !ok || (n != 1 && n != 4) {
			return fmt.Sprintf("CornerRadius espera 1 ou 4 números, não %q", v)
		}
	case kindLength:
		if !strings.EqualFold(v, "Auto") && !isNumber(v) {
			return fmt.Sprintf("%s espera um número ou Auto, não %q", property, v)
		}
	case kindNumber:
		if !isNumber(v) {
			return fmt.Sprintf("%s espera um número, não %q", property, v)
		}
	case kindVisibility:
		if !oneOf(v, "0", "1", "Visible", "Collapsed") {
			return fmt.Sprintf("Visibility espera Visible ou Collapsed, não %q", v)
		}
	case kindHAlign:
		if !oneOf(v, "Left", "Center", "Right", "Stretch", "0", "1", "2", "3") {
			return fmt.Sprintf("HorizontalAlignment espera Left, Center, Right ou Stretch, não %q", v)
		}
	case kindVAlign:
		if !oneOf(v, "Top", "Center", "Bottom", "Stretch", "0", "1", "2", "3") {
			return fmt.Sprintf("VerticalAlignment espera Top, Center, Bottom ou Stretch, não %q", v)
		}
	case kindSizing:
		if !oneOf(v, "InnerBorderEdge", "OuterBorderEdge") {
			return fmt.Sprintf("BackgroundSizing espera InnerBorderEdge ou OuterBorderEdge, não %q", v)
		}
	case kindBrush:
		if !isColor(v) {
			return fmt.Sprintf("%s espera uma cor (#RRGGBB, #AARRGGBB ou um nome como Transparent) ou um objeto XAML, não %q", property, v)
		}
	case kindTransform:
		return "RenderTransform espera um objeto XAML, como <TranslateTransform X=\"0\" Y=\"2\" />"
	}
	return ""
}

// parseBlur lê <Blur Amount="20" TintColor="..." TintOpacity="0.15" />.
func parseBlur(v string) (Blur, string) {
	var raw struct {
		XMLName     xml.Name
		Amount      string     `xml:"Amount,attr"`
		TintColor   string     `xml:"TintColor,attr"`
		TintOpacity string     `xml:"TintOpacity,attr"`
		Other       []xml.Attr `xml:",any,attr"`
	}
	if err := xml.Unmarshal([]byte(v), &raw); err != nil {
		return Blur{}, fmt.Sprintf("<Blur> mal formado: %v", err)
	}
	if raw.XMLName.Local != "Blur" {
		return Blur{}, fmt.Sprintf("elemento %q não é <Blur>", raw.XMLName.Local)
	}
	if len(raw.Other) > 0 {
		return Blur{}, fmt.Sprintf("<Blur> não tem o atributo %q (use Amount, TintColor e TintOpacity)", raw.Other[0].Name.Local)
	}

	var b Blur
	if raw.Amount != "" {
		a, err := strconv.ParseFloat(raw.Amount, 64)
		if err != nil || math.IsNaN(a) || math.IsInf(a, 0) || a < 0 || a > 250 {
			return Blur{}, fmt.Sprintf("Amount do <Blur> deve estar entre 0 e 250, não %q", raw.Amount)
		}
		b.Amount = a
	}

	alpha, hasOpacity := uint8(0xFF), raw.TintOpacity != ""
	if hasOpacity {
		o, err := strconv.ParseFloat(raw.TintOpacity, 64)
		if err != nil || math.IsNaN(o) || math.IsInf(o, 0) || o < 0 || o > 1 {
			return Blur{}, fmt.Sprintf("TintOpacity do <Blur> deve estar entre 0 e 1, não %q", raw.TintOpacity)
		}
		alpha = uint8(math.Round(o * 255))
	}

	const prefix = "{ThemeResource "
	switch tint := strings.TrimSpace(raw.TintColor); {
	case strings.HasPrefix(tint, prefix) && strings.HasSuffix(tint, "}"):
		b.ThemeKey = strings.TrimSpace(tint[len(prefix) : len(tint)-1])
		b.Alpha = alpha
		b.Fallback = uint32(alpha)<<24 | blurThemeFallback
	case tint == "":
		return Blur{}, "<Blur> precisa de TintColor"
	default:
		c, ok := parseColor(tint)
		if !ok {
			return Blur{}, fmt.Sprintf("TintColor do <Blur> deve ser #RRGGBB, #AARRGGBB ou {ThemeResource Nome}, não %q", tint)
		}
		if hasOpacity {
			c = uint32(alpha)<<24 | c&0xFFFFFF
		}
		b.Alpha = uint8(c >> 24)
		b.Fallback = c
	}
	return b, ""
}

// withXAMLNamespace confere que v é XML bem formado e declara o
// namespace XAML padrão no elemento raiz (o parser XAML do TAP precisa disso).
func withXAMLNamespace(v string) (string, string) {
	d := xml.NewDecoder(strings.NewReader(v))
	roots := 0
	depth := 0
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Sprintf("objeto XAML mal formado: %v", err)
		}
		switch tok.(type) {
		case xml.StartElement:
			if depth == 0 {
				roots++
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
	if roots != 1 {
		return "", "o objeto XAML deve ter exatamente um elemento raiz"
	}
	if strings.Contains(v, "xmlns=") {
		return v, ""
	}
	end := strings.IndexAny(v[1:], " \t\r\n/>") + 1
	return v[:end] + ` xmlns="` + xamlNamespace + `"` + v[end:], ""
}

func numbers(v string) (int, bool) {
	fields := strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == ' ' })
	for _, f := range fields {
		if !isNumber(f) {
			return 0, false
		}
	}
	return len(fields), len(fields) > 0
}

func isNumber(v string) bool {
	_, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	return err == nil
}

func oneOf(v string, options ...string) bool {
	for _, o := range options {
		if strings.EqualFold(v, o) {
			return true
		}
	}
	return false
}

// isColor aceita cores hexadecimais (#RRGGBB) e cores nomeadas (só letras).
func isColor(v string) bool {
	if _, ok := parseColor(v); ok {
		return true
	}
	for _, r := range v {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') {
			return false
		}
	}
	return v != ""
}

// parseColor lê #RRGGBB (opaca) ou #AARRGGBB como ARGB.
func parseColor(v string) (uint32, bool) {
	if !strings.HasPrefix(v, "#") || (len(v) != 7 && len(v) != 9) {
		return 0, false
	}
	n, err := strconv.ParseUint(v[1:], 16, 32)
	if err != nil {
		return 0, false
	}
	if len(v) == 7 {
		return 0xFF000000 | uint32(n), true
	}
	return uint32(n), true
}
