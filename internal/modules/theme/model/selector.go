package model

import (
	"fmt"
	"strings"
)

// Condições de atributo que o TAP sabe avaliar.
var supportedAttrs = map[string]bool{
	"Text":                              true,
	"AutomationProperties.AutomationId": true,
}

// ParseSelector decompõe um seletor como
// "Taskbar.TaskbarFrame > Grid#RootGrid, StackPanel#Tray" em suas
// alternativas. Devolve todos os problemas encontrados, em português.
func ParseSelector(text string) ([]Chain, []string) {
	var problems []string
	if strings.TrimSpace(text) == "" {
		return nil, []string{"seletor vazio"}
	}
	if strings.Contains(text, "@") {
		problems = append(problems, "estados visuais (@) não são suportados")
	}
	depth := 0
	for _, ch := range text {
		if ch == '[' {
			depth++
		}
		if ch == ']' {
			depth--
		}
		if depth < 0 || depth > 1 {
			return nil, append(problems, "colchetes desbalanceados ou aninhados")
		}
	}
	if depth != 0 {
		return nil, append(problems, "colchetes desbalanceados")
	}

	var alts []Chain
	for _, altText := range splitTop(text, ',') {
		if altText == "" {
			problems = append(problems, "alternativa vazia entre vírgulas")
			continue
		}
		var chain Chain
		for _, part := range splitTop(altText, '>') {
			c, ps := parseCompound(part)
			problems = append(problems, ps...)
			chain = append(chain, c)
		}
		alts = append(alts, chain)
	}
	return alts, problems
}

func parseCompound(s string) (Compound, []string) {
	var c Compound
	var problems []string
	if s == "" {
		return c, []string{"elemento vazio entre '>'"}
	}

	i := 0
	for i < len(s) && s[i] != '#' && s[i] != '[' {
		i++
	}
	c.Type = strings.TrimSpace(s[:i])
	if i < len(s) && s[i] == '#' {
		j := i + 1
		for j < len(s) && s[j] != '[' {
			j++
		}
		c.Name = strings.TrimSpace(s[i+1 : j])
		if c.Name == "" {
			problems = append(problems, fmt.Sprintf("nome vazio depois de '#' em %q", s))
		}
		i = j
	}
	for i < len(s) {
		if s[i] != '[' {
			problems = append(problems, fmt.Sprintf("texto inesperado %q em %q", s[i:], s))
			break
		}
		end := strings.IndexByte(s[i:], ']')
		if end < 0 {
			return c, append(problems, "condição sem fechamento ']'")
		}
		inner := s[i+1 : i+end]
		name, value, ok := strings.Cut(inner, "=")
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("condição %q sem '='", inner))
		case !supportedAttrs[name]:
			problems = append(problems, fmt.Sprintf("condição [%s=...] não suportada", name))
		default:
			c.Attrs = append(c.Attrs, Attr{Name: name, Value: value})
		}
		i += end + 1
	}

	if c.Type == "" {
		problems = append(problems, fmt.Sprintf("tipo do elemento ausente em %q", s))
	} else if strings.ContainsAny(c.Type, " \t") {
		problems = append(problems, fmt.Sprintf("tipo %q contém espaço (faltou um '>'?)", c.Type))
	}
	return c, problems
}

// ClassNames são os nomes de classe em tempo de execução que o Type do compound aceita: um nome completo
// como está, um nome curto em cada namespace XAML em que possa estar.
func (c Compound) ClassNames() []string {
	if strings.Contains(c.Type, ".") {
		return []string{c.Type}
	}
	return []string{
		"Windows.UI.Xaml.Controls." + c.Type,
		"Windows.UI.Xaml.Shapes." + c.Type,
		"Windows.UI.Xaml.Controls.Primitives." + c.Type,
		"Windows.UI.Xaml." + c.Type,
	}
}

// splitTop divide em sep fora de [...] e apara cada parte.
func splitTop(s string, sep byte) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
		case sep:
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	return append(out, strings.TrimSpace(s[start:]))
}
