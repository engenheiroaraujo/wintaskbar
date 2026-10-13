// Package model é o vocabulário do Theme: Theme, Rule, Selector, Style e
// Variable, e as regras puras que validam um Theme e o resolvem na forma que o
// TAP aplica (variáveis expandidas, seletores decompostos, valores
// classificados). Nada aqui toca em arquivos, no Windows ou no TAP.
package model

// Theme é um conjunto nomeado de Rules e Variables que define o visual da Taskbar.
type Theme struct {
	Name      string
	Variables []Variable
	Rules     []Rule
}

// Variable é um valor nomeado reutilizado por várias Rules, referenciado como $Nome.
type Variable struct {
	Name  string
	Value string
	Line  int // linha de origem, para mensagens de erro; 0 quando desconhecida
}

// Rule é um Selector com os Styles aplicados aos elementos que ele identifica.
type Rule struct {
	Selector string
	Styles   []Style
	Line     int
}

// Style atribui um valor a uma propriedade visual dos elementos casados. Value
// é como foi escrito: pode referenciar Variables.
type Style struct {
	Property string
	Value    string
	Line     int
}

// Chain é uma alternativa de um Selector: o ancestral mais externo primeiro e
// o elemento casado por último; cada passo é filho direto do anterior.
type Chain []Compound

// Compound identifica um elemento por classe, nome e atributos.
type Compound struct {
	Type  string // como escrito: curto ("Grid") ou completo ("Windows.UI.Xaml.Controls.Grid")
	Name  string // x:Name; vazio para qualquer um
	Attrs []Attr
}

// Attr é uma condição de atributo, como [Text=...].
type Attr struct {
	Name  string
	Value string
}

// ValueKind diz como o TAP deve transformar um valor resolvido em valor de propriedade.
type ValueKind string

const (
	KindPlain      ValueKind = "plain"       // interpretado pelo tipo da propriedade (espessura, número, enum, cor, texto)
	KindXAML       ValueKind = "xaml"        // um objeto XAML, como um pincel de gradiente ou uma transformação
	KindBlur       ValueKind = "blur"        // o pincel de desfoque do produto
	KindDockLayout ValueKind = "dock-layout" // limite dinâmico da Dock antes da Tray
)

// Blur é o pincel de fundo desfocado e com tinta.
type Blur struct {
	Amount float64
	// ThemeKey nomeia o recurso de cor do tema usado como tinta; vazio quando
	// a tinta é uma cor literal.
	ThemeKey string
	// Alpha é a opacidade da tinta (0-255).
	Alpha uint8
	// Fallback é a cor ARGB usada quando o recurso do tema não pode ser
	// resolvido ou os efeitos avançados estão desligados.
	Fallback uint32
}

// Resolved é um Theme validado, pronto para o TAP.
type Resolved struct {
	Name  string
	Rules []ResolvedRule
}

// ResolvedRule é uma Rule com o seletor decomposto e os valores resolvidos.
type ResolvedRule struct {
	Selector     string // como escrito, para logs
	Alternatives []Chain
	Styles       []ResolvedStyle
}

// ResolvedStyle é um Style com variáveis expandidas e o valor classificado.
type ResolvedStyle struct {
	Property string
	Kind     ValueKind
	Value    string // texto simples, ou XAML com o namespace padrão declarado
	Blur     Blur   // preenchido quando Kind é KindBlur
}
