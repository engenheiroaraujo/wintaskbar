package repository

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	errs "github.com/engenheiroaraujo/wintaskbar/internal/platform/errors"
)

var update = flag.Bool("update", false, "rewrite testdata/default.compiled from the Default Theme")

// goldenPath é compartilhado com o teste de contrato do TAP (tap/theme_file_test.cpp).
var goldenPath = filepath.Join("testdata", "default.compiled")

func TestDefaultThemeIsValid(t *testing.T) {
	theme, err := Decode(DefaultYAML())
	if err != nil {
		t.Fatalf("decode: %v %v", err, errs.DetailsOf(err))
	}
	resolved, problems := theme.Resolve()
	if len(problems) > 0 {
		t.Fatalf("Default Theme has problems: %+v", problems)
	}
	if theme.Name != "default" || len(resolved.Rules) != 12 {
		t.Fatalf("name=%q rules=%d, want default with 12 rules", theme.Name, len(resolved.Rules))
	}
}

// TestCompiledGolden fixa a forma compilada do Default Theme: o
// contrato que o TAP lê. Rode `go test ./... -update` depois de uma mudança intencional.
func TestCompiledGolden(t *testing.T) {
	theme, err := Decode(DefaultYAML())
	if err != nil {
		t.Fatal(err)
	}
	resolved, problems := theme.Resolve()
	if len(problems) > 0 {
		t.Fatalf("problems: %+v", problems)
	}
	got := EncodeCompiled(resolved)

	if *update {
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden (run with -update to create it): %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatal("compiled Default Theme differs from testdata/default.compiled; if intended, run go test -update and rebuild the TAP contract test")
	}
}

func TestDecodeReportsLocatedProblems(t *testing.T) {
	src := strings.Join([]string{
		"name: x",
		"colors: {}",
		"rules:",
		"  - styles:",
		"      Margin: '1'",
		"  - selector: Grid",
		"    style: {}",
	}, "\n")

	_, err := Decode([]byte(src))

	if errs.CategoryOf(err) != errs.InvalidTheme {
		t.Fatalf("category = %q, want invalid_theme", errs.CategoryOf(err))
	}
	details := errs.DetailsOf(err)
	want := map[string]string{"linha 2": "campo desconhecido", "linha 4": "não tem selector", "linha 7": "campo desconhecido"}
	if len(details) != len(want) {
		t.Fatalf("details = %+v", details)
	}
	for _, d := range details {
		if !strings.Contains(d.Message, want[d.Where]) {
			t.Errorf("%s: %q, want it to mention %q", d.Where, d.Message, want[d.Where])
		}
	}
}

func TestDecodeRejectsInvalidYAML(t *testing.T) {
	_, err := Decode([]byte("name: [unclosed"))
	if errs.CategoryOf(err) != errs.InvalidTheme {
		t.Fatalf("category = %q, want invalid_theme", errs.CategoryOf(err))
	}
}

func TestEscape(t *testing.T) {
	if got := escape("a\tb\\c\nd"); got != `a\tb\\c\nd` {
		t.Fatalf("escape = %q", got)
	}
}

func TestInitUserNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	f := Files{UserTheme: filepath.Join(dir, "wintaskbar", "theme.yaml")}

	if _, created, err := f.InitUser(); err != nil || !created {
		t.Fatalf("first init: created=%v err=%v", created, err)
	}
	if err := os.WriteFile(f.UserTheme, []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, created, err := f.InitUser(); err != nil || created {
		t.Fatalf("second init: created=%v err=%v, want untouched", created, err)
	}
	if data, _ := os.ReadFile(f.UserTheme); string(data) != "edited" {
		t.Fatal("InitUser overwrote the user's file")
	}
}

func TestWriteCompiledIsReadable(t *testing.T) {
	dir := t.TempDir()
	f := Files{Compiled: filepath.Join(dir, "sub", "theme.compiled")}
	theme, _ := Decode(DefaultYAML())
	resolved, _ := theme.Resolve()

	if err := f.WriteCompiled(resolved); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(f.Compiled)
	if err != nil || !bytes.HasPrefix(data, []byte(CompiledMagic+"\t2\n")) {
		t.Fatalf("compiled file: err=%v head=%q", err, data[:min(len(data), 30)])
	}
	if _, err := os.Stat(f.Compiled + ".tmp"); err == nil {
		t.Fatal("temporary file left behind")
	}
}
