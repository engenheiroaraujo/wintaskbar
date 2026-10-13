// Package repository guarda Themes: decodifica arquivos de Theme (YAML), mantém o
// Default Theme embutido e grava a forma compilada que o TAP lê.
package repository

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/engenheiroaraujo/wintaskbar/internal/modules/theme/model"
	errs "github.com/engenheiroaraujo/wintaskbar/internal/platform/errors"
)

//go:embed default.yaml
var defaultYAML []byte

// DefaultSource nomeia o Default Theme embutido nas mensagens.
const DefaultSource = "tema padrão (embutido)"

// Files localiza o arquivo de Theme do usuário e o arquivo compilado.
type Files struct {
	UserTheme string // ex.: %APPDATA%\wintaskbar\theme.yaml
	Compiled  string // ex.: %LOCALAPPDATA%\wintaskbar\theme.compiled
}

// DefaultFiles devolve os locais por usuário.
func DefaultFiles() (Files, error) {
	roaming, local := os.Getenv("APPDATA"), os.Getenv("LOCALAPPDATA")
	if roaming == "" || local == "" {
		return Files{}, errors.New("APPDATA or LOCALAPPDATA is not set")
	}
	return Files{
		UserTheme: filepath.Join(roaming, "wintaskbar", "theme.yaml"),
		Compiled:  filepath.Join(local, "wintaskbar", "theme.compiled"),
	}, nil
}

// LoadActive carrega o arquivo de Theme do usuário quando existe; senão, o Default
// Theme. Também devolve qual deles carregou.
func (f Files) LoadActive() (model.Theme, string, error) {
	if _, err := os.Stat(f.UserTheme); err == nil {
		t, err := f.Load(f.UserTheme)
		return t, f.UserTheme, err
	} else if !errors.Is(err, fs.ErrNotExist) {
		return model.Theme{}, f.UserTheme, err
	}
	t, err := Decode(defaultYAML)
	return t, DefaultSource, err
}

// Load lê e decodifica o arquivo de Theme em path.
func (f Files) Load(path string) (model.Theme, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return model.Theme{}, errs.Wrap(errs.InvalidInput, "não foi possível ler o arquivo do tema", err)
	}
	return Decode(data)
}

// WriteCompiled grava o Theme compilado de forma atômica, para o TAP nunca ler
// um arquivo pela metade.
func (f Files) WriteCompiled(r model.Resolved) error {
	if err := os.MkdirAll(filepath.Dir(f.Compiled), 0o755); err != nil {
		return err
	}
	tmp := f.Compiled + ".tmp"
	if err := os.WriteFile(tmp, EncodeCompiled(r), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, f.Compiled)
}

// InitUser cria o arquivo de Theme do usuário a partir do Default Theme. created é
// false quando ele já existe (nunca é sobrescrito).
func (f Files) InitUser() (path string, created bool, err error) {
	if _, err := os.Stat(f.UserTheme); err == nil {
		return f.UserTheme, false, nil
	}
	if err := os.MkdirAll(filepath.Dir(f.UserTheme), 0o755); err != nil {
		return f.UserTheme, false, err
	}
	return f.UserTheme, true, os.WriteFile(f.UserTheme, defaultYAML, 0o644)
}

// DefaultYAML é o fonte do Default Theme.
func DefaultYAML() []byte { return defaultYAML }

// Decode interpreta um arquivo de Theme. Problemas de sintaxe e estrutura voltam como um único
// erro invalid_theme listando cada um com sua linha.
func Decode(data []byte) (model.Theme, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return model.Theme{}, errs.WithDetails(errs.InvalidTheme, "o arquivo do tema não é um YAML válido",
			[]errs.Detail{{Where: "YAML", Message: err.Error()}})
	}

	var t model.Theme
	var details []errs.Detail
	problem := func(n *yaml.Node, msg string) {
		details = append(details, errs.Detail{Where: fmt.Sprintf("linha %d", n.Line), Message: msg})
	}

	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return t, errs.WithDetails(errs.InvalidTheme, "o tema tem erros",
			[]errs.Detail{{Where: "linha 1", Message: "o tema deve ter os campos name, variables e rules"}})
	}

	root := doc.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		key, val := root.Content[i], root.Content[i+1]
		switch key.Value {
		case "name":
			if val.Kind != yaml.ScalarNode {
				problem(val, "name deve ser um texto")
				continue
			}
			t.Name = val.Value
		case "variables":
			if val.Kind != yaml.MappingNode {
				problem(val, "variables deve ser uma lista de Nome: valor")
				continue
			}
			for j := 0; j+1 < len(val.Content); j += 2 {
				k, v := val.Content[j], val.Content[j+1]
				if v.Kind != yaml.ScalarNode {
					problem(v, fmt.Sprintf("a variável %s deve ter um valor de texto", k.Value))
					continue
				}
				t.Variables = append(t.Variables, model.Variable{Name: k.Value, Value: v.Value, Line: k.Line})
			}
		case "rules":
			if val.Kind != yaml.SequenceNode {
				problem(val, "rules deve ser uma lista de regras")
				continue
			}
			for _, rn := range val.Content {
				if r, ok := decodeRule(rn, problem); ok {
					t.Rules = append(t.Rules, r)
				}
			}
		default:
			problem(key, fmt.Sprintf("campo desconhecido %q (use name, variables e rules)", key.Value))
		}
	}

	if len(details) > 0 {
		return t, errs.WithDetails(errs.InvalidTheme, "o tema tem erros", details)
	}
	return t, nil
}

func decodeRule(n *yaml.Node, problem func(*yaml.Node, string)) (model.Rule, bool) {
	if n.Kind != yaml.MappingNode {
		problem(n, "cada regra deve ter selector e styles")
		return model.Rule{}, false
	}
	r := model.Rule{Line: n.Line}
	hasSelector := false
	for i := 0; i+1 < len(n.Content); i += 2 {
		key, val := n.Content[i], n.Content[i+1]
		switch key.Value {
		case "selector":
			if val.Kind != yaml.ScalarNode {
				problem(val, "selector deve ser um texto")
				continue
			}
			r.Selector, hasSelector = val.Value, true
		case "styles":
			if val.Kind != yaml.MappingNode {
				problem(val, "styles deve ser uma lista de Propriedade: valor")
				continue
			}
			for j := 0; j+1 < len(val.Content); j += 2 {
				k, v := val.Content[j], val.Content[j+1]
				if v.Kind != yaml.ScalarNode {
					problem(v, fmt.Sprintf("o estilo %s deve ter um valor de texto", k.Value))
					continue
				}
				r.Styles = append(r.Styles, model.Style{Property: k.Value, Value: v.Value, Line: k.Line})
			}
		default:
			problem(key, fmt.Sprintf("campo desconhecido %q na regra (use selector e styles)", key.Value))
		}
	}
	if !hasSelector {
		problem(n, "a regra não tem selector")
		return r, false
	}
	return r, true
}
