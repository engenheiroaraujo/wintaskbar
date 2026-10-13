package model

import (
	"strings"
	"testing"
)

func TestDockLayoutGapValidation(t *testing.T) {
	for _, value := range []string{"-1", "1001", "NaN", "+Inf", "Auto", "12px", "<Grid />"} {
		if _, problem := resolveValue("DockLayoutGap", value); problem == "" {
			t.Errorf("accepted invalid gap %q", value)
		}
	}
	for _, value := range []string{"0", "1.5", "12", "1000"} {
		style, problem := resolveValue("DockLayoutGap", value)
		if problem != "" || style.Kind != KindDockLayout || style.Value != value {
			t.Errorf("gap %q: %+v, %s", value, style, problem)
		}
	}
	for _, selector := range []string{"Taskbar.TaskbarFrame", "Grid", "Taskbar.TaskbarFrame, Grid"} {
		theme := Theme{Name: "layout", Variables: []Variable{{Name: "Gap", Value: "12"}},
			Rules: []Rule{{Selector: selector, Styles: []Style{{Property: "DockLayoutGap", Value: "$Gap"}}}}}
		resolved, problems := theme.Resolve()
		if selector == "Taskbar.TaskbarFrame" {
			if len(problems) != 0 || resolved.Rules[0].Styles[0].Value != "12" {
				t.Fatalf("valid layout: %+v, %+v", resolved, problems)
			}
		} else if len(problems) == 0 {
			t.Errorf("accepted layout target %q", selector)
		}
	}
}

func TestBlurRejectsValuesTheTAPCannotAccept(t *testing.T) {
	for _, amount := range []string{"NaN", "+Inf", "-1", "251"} {
		if _, problem := parseBlur(`<Blur Amount="` + amount + `" TintColor="#202020" />`); problem == "" {
			t.Errorf("accepted blur amount %s", amount)
		}
	}
	if _, problem := parseBlur(`<Blur Amount="250" TintColor="#202020" />`); problem != "" {
		t.Fatal(problem)
	}
	if _, problem := parseBlur(`<Blur TintColor="#202020" TintOpacity="NaN" />`); problem == "" {
		t.Fatal("accepted NaN opacity")
	}
}

func TestParseSelectorDecomposesAlternativesAndChains(t *testing.T) {
	alts, problems := ParseSelector("Taskbar.TaskbarFrame > Grid#RootGrid, StackPanel#Tray[Text=x]")
	if len(problems) != 0 {
		t.Fatalf("problems = %v", problems)
	}
	if len(alts) != 2 || len(alts[0]) != 2 || len(alts[1]) != 1 {
		t.Fatalf("alts = %+v, want [2 steps] and [1 step]", alts)
	}
	if got := alts[0][1]; got.Type != "Grid" || got.Name != "RootGrid" {
		t.Fatalf("second step = %+v", got)
	}
	if got := alts[1][0].Attrs; len(got) != 1 || got[0] != (Attr{"Text", "x"}) {
		t.Fatalf("attrs = %+v", got)
	}
}

func TestParseSelectorReportsEveryProblem(t *testing.T) {
	_, problems := ParseSelector("Grid#Root@CommonStates > , #NoType[Foo=1]")
	want := []string{"estados visuais", "elemento vazio", "tipo do elemento ausente", "não suportada"}
	joined := strings.Join(problems, " | ")
	for _, w := range want {
		if !strings.Contains(joined, w) {
			t.Errorf("problems %q lack %q", joined, w)
		}
	}
}

func TestParseSelectorRejectsMalformedBracketsWithoutPanicking(t *testing.T) {
	for _, selector := range []string{"Grid][Text=x", "Grid[[Text=x]]", "Grid[Text=x"} {
		if _, problems := ParseSelector(selector); len(problems) == 0 {
			t.Errorf("accepted %q", selector)
		}
	}
}

func TestClassNames(t *testing.T) {
	if got := (Compound{Type: "Taskbar.TaskbarFrame"}).ClassNames(); len(got) != 1 || got[0] != "Taskbar.TaskbarFrame" {
		t.Fatalf("full name expanded: %v", got)
	}
	got := (Compound{Type: "Grid"}).ClassNames()
	if len(got) != 4 || got[0] != "Windows.UI.Xaml.Controls.Grid" {
		t.Fatalf("short name candidates = %v", got)
	}
}

func TestResolveValuePlain(t *testing.T) {
	cases := []struct {
		prop, value string
		ok          bool
	}{
		{"Margin", "0,0,0,4", true},
		{"Margin", "1,2", true},
		{"Margin", "1,2,3", false},
		{"CornerRadius", "10", true},
		{"CornerRadius", "1,2", false},
		{"Width", "Auto", true},
		{"Width", "wide", false},
		{"Visibility", "Collapsed", true},
		{"Visibility", "Hidden", false},
		{"Background", "#50808080", true},
		{"Background", "Transparent", true},
		{"Background", "#12", false},
		{"RenderTransform", "1", false},
		{"Bogus", "1", false},
	}
	for _, c := range cases {
		_, problem := resolveValue(c.prop, c.value)
		if (problem == "") != c.ok {
			t.Errorf("%s=%q: problem %q, want ok=%v", c.prop, c.value, problem, c.ok)
		}
	}
}

func TestResolveValueXAMLGetsNamespace(t *testing.T) {
	rs, problem := resolveValue("RenderTransform", `<TranslateTransform X="5" Y="2" />`)
	if problem != "" {
		t.Fatal(problem)
	}
	if rs.Kind != KindXAML || !strings.HasPrefix(rs.Value, `<TranslateTransform xmlns="`+xamlNamespace+`"`) {
		t.Fatalf("value = %q", rs.Value)
	}
	if _, problem := resolveValue("Background", `<SolidColorBrush Color="Red">`); problem == "" {
		t.Fatal("unclosed XAML accepted")
	}
	if _, problem := resolveValue("Margin", `<Thickness/>`); problem == "" {
		t.Fatal("XAML accepted on a plain-only property")
	}
}

func TestResolveValueBlur(t *testing.T) {
	rs, problem := resolveValue("Background", `<Blur Amount="20" TintColor="{ThemeResource SystemChromeDarkColor}" TintOpacity="0.15" />`)
	if problem != "" {
		t.Fatal(problem)
	}
	b := rs.Blur
	if rs.Kind != KindBlur || b.Amount != 20 || b.ThemeKey != "SystemChromeDarkColor" || b.Alpha != 38 || b.Fallback != 0x26202020 {
		t.Fatalf("blur = %+v", b)
	}

	rs, problem = resolveValue("Fill", `<Blur Amount="5" TintColor="#80112233" />`)
	if problem != "" || rs.Blur.ThemeKey != "" || rs.Blur.Fallback != 0x80112233 || rs.Blur.Alpha != 0x80 {
		t.Fatalf("literal tint: %+v, %q", rs.Blur, problem)
	}

	for _, bad := range []string{
		`<Blur Amount="20" />`,
		`<Blur Amount="x" TintColor="#000000" />`,
		`<Blur TintColor="#000000" TintOpacity="2" />`,
		`<Blur TintColor="#000000" Radius="3" />`,
	} {
		if _, problem := resolveValue("Background", bad); problem == "" {
			t.Errorf("%s accepted", bad)
		}
	}
	if _, problem := resolveValue("Margin", `<Blur TintColor="#000000" />`); problem == "" {
		t.Error("blur accepted on a non-brush property")
	}
}

func TestResolveExpandsVariablesAndAccumulatesProblems(t *testing.T) {
	theme := Theme{
		Name:      "default",
		Variables: []Variable{{Name: "Radius", Value: "10"}},
		Rules: []Rule{
			{Selector: "Grid#RootGrid", Line: 3, Styles: []Style{
				{Property: "CornerRadius", Value: "$Radius", Line: 4},
				{Property: "Margin", Value: "$Missing", Line: 5},
			}},
			{Selector: "Grid@State", Line: 6, Styles: []Style{{Property: "Width", Value: "abc", Line: 7}}},
		},
	}

	resolved, problems := theme.Resolve()

	if len(problems) != 3 {
		t.Fatalf("problems = %+v, want 3 (missing var, visual state, bad width)", problems)
	}
	if got := resolved.Rules[0].Styles[0]; got.Value != "10" || got.Kind != KindPlain {
		t.Fatalf("expanded style = %+v", got)
	}
	if problems[0].Line != 5 || problems[1].Line != 6 || problems[2].Line != 7 {
		t.Fatalf("lines = %d %d %d, want 5 6 7", problems[0].Line, problems[1].Line, problems[2].Line)
	}
}

func TestResolveRejectsEmptyTheme(t *testing.T) {
	_, problems := Theme{}.Resolve()
	if len(problems) != 2 {
		t.Fatalf("problems = %+v, want missing name and no rules", problems)
	}
}
