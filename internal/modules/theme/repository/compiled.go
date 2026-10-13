package repository

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/engenheiroaraujo/wintaskbar/internal/modules/theme/model"
)

// O Theme compilado é o contrato com o TAP (veja tap/theme_file.h e
// ADR 0001). Um registro por linha, campos separados por tabulação; dentro de um
// campo, barra invertida, tab, CR e LF são escapados como \\ \t \r \n.
//
//	wintaskbar-theme	2
//	name	<nome do tema>
//	rule	<seletor como escrito>
//	alt                                   um por alternativa do seletor
//	c	<classe|classe|...>	<x:Name ou vazio>   elemento mais externo primeiro
//	a	<atributo>	<valor>               condição do c anterior
//	style	<propriedade>	plain	<valor>
//	style	<propriedade>	xaml	<xaml com namespace>
//	style	DockLayoutGap	dock-layout	<folga em pixels lógicos>
//	style	<propriedade>	blur	<quantidade>	<chave do tema ou vazio>	<alpha 0-255>	<fallback AARRGGBB>
//	end
const (
	CompiledMagic   = "wintaskbar-theme"
	CompiledVersion = 2
)

// EncodeCompiled gera o formato compilado de um Theme resolvido.
func EncodeCompiled(r model.Resolved) []byte {
	var b strings.Builder
	line := func(fields ...string) {
		for i, f := range fields {
			if i > 0 {
				b.WriteByte('\t')
			}
			b.WriteString(escape(f))
		}
		b.WriteByte('\n')
	}

	line(CompiledMagic, strconv.Itoa(CompiledVersion))
	line("name", r.Name)
	for _, rule := range r.Rules {
		line("rule", rule.Selector)
		for _, chain := range rule.Alternatives {
			line("alt")
			for _, c := range chain {
				line("c", strings.Join(c.ClassNames(), "|"), c.Name)
				for _, a := range c.Attrs {
					line("a", a.Name, a.Value)
				}
			}
		}
		for _, s := range rule.Styles {
			switch s.Kind {
			case model.KindBlur:
				line("style", s.Property, string(s.Kind),
					strconv.FormatFloat(s.Blur.Amount, 'g', -1, 64),
					s.Blur.ThemeKey,
					strconv.Itoa(int(s.Blur.Alpha)),
					fmt.Sprintf("%08X", s.Blur.Fallback))
			default:
				line("style", s.Property, string(s.Kind), s.Value)
			}
		}
	}
	line("end")
	return []byte(b.String())
}

var escaper = strings.NewReplacer(`\`, `\\`, "\t", `\t`, "\r", `\r`, "\n", `\n`)

func escape(s string) string { return escaper.Replace(s) }
