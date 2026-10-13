package model

import (
	"fmt"
	"regexp"
	"strings"
)

// Problem é um problema em um Theme, com o lugar onde está.
type Problem struct {
	Line    int    // linha de origem, 0 quando desconhecida
	Where   string // ex.: o seletor da regra e a propriedade
	Message string
}

var variableName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Resolve valida o Theme e o transforma na forma que o TAP aplica. Ele
// relata todos os problemas que encontra, não só o primeiro; o resultado só
// faz sentido quando não há nenhum.
func (t Theme) Resolve() (Resolved, []Problem) {
	var problems []Problem
	add := func(line int, where, msg string) {
		problems = append(problems, Problem{Line: line, Where: where, Message: msg})
	}

	if strings.TrimSpace(t.Name) == "" {
		add(0, "name", "o tema precisa de um nome")
	}

	vars := make(map[string]string, len(t.Variables))
	for _, v := range t.Variables {
		where := "variável " + v.Name
		_, defined := vars[v.Name]
		switch {
		case !variableName.MatchString(v.Name):
			add(v.Line, where, "nome inválido: use letras, números e _ (sem começar por número)")
		case defined:
			add(v.Line, where, "variável definida mais de uma vez")
		case strings.Contains(v.Value, "$"):
			add(v.Line, where, "uma variável não pode usar outra variável")
		}
		vars[v.Name] = v.Value
	}

	if len(t.Rules) == 0 {
		add(0, "rules", "o tema não tem regras")
	}

	out := Resolved{Name: t.Name}
	for _, r := range t.Rules {
		where := fmt.Sprintf("regra %q", r.Selector)
		alts, selectorProblems := ParseSelector(r.Selector)
		for _, p := range selectorProblems {
			add(r.Line, where, p)
		}
		if len(r.Styles) == 0 {
			add(r.Line, where, "a regra não tem estilos")
		}

		rr := ResolvedRule{Selector: r.Selector, Alternatives: alts}
		seen := map[string]bool{}
		for _, s := range r.Styles {
			styleWhere := where + ", " + s.Property
			if seen[s.Property] {
				add(s.Line, styleWhere, "propriedade repetida na mesma regra")
				continue
			}
			seen[s.Property] = true

			value, missing := expand(s.Value, vars)
			if len(missing) > 0 {
				add(s.Line, styleWhere, "variável não definida: $"+strings.Join(missing, ", $"))
				continue
			}
			rs, problem := resolveValue(s.Property, value)
			if s.Property == "DockLayoutGap" {
				for _, chain := range alts {
					if len(chain) == 0 || chain[len(chain)-1].Type != "Taskbar.TaskbarFrame" {
						problem = "DockLayoutGap só pode ser aplicado a Taskbar.TaskbarFrame"
					}
				}
			}
			if problem != "" {
				add(s.Line, styleWhere, problem)
				continue
			}
			rr.Styles = append(rr.Styles, rs)
		}
		out.Rules = append(out.Rules, rr)
	}
	return out, problems
}

// expand substitui $Nome pelo valor da variável e relata nomes desconhecidos.
func expand(v string, vars map[string]string) (string, []string) {
	var b strings.Builder
	var missing []string
	for i := 0; i < len(v); i++ {
		if v[i] != '$' {
			b.WriteByte(v[i])
			continue
		}
		j := i + 1
		for j < len(v) && (v[j] == '_' || v[j] >= 'a' && v[j] <= 'z' || v[j] >= 'A' && v[j] <= 'Z' || v[j] >= '0' && v[j] <= '9') {
			j++
		}
		name := v[i+1 : j]
		value, ok := vars[name]
		if name == "" || !ok {
			missing = append(missing, name)
			b.WriteString(v[i:j])
		} else {
			b.WriteString(value)
		}
		i = j - 1
	}
	return b.String(), missing
}
