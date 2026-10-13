package desktop

import (
	"encoding/xml"
	"strings"
	"testing"
)

func TestStartupTaskXMLIsWellFormedAndEscaped(t *testing.T) {
	doc := StartupTaskXML(`C:\Users\A & B\<app>\wintaskbar.exe`, `PC\A & B`)

	var out struct {
		Command string `xml:"Actions>Exec>Command"`
		WorkDir string `xml:"Actions>Exec>WorkingDirectory"`
		User    string `xml:"Triggers>LogonTrigger>UserId"`
		Level   string `xml:"Principals>Principal>RunLevel"`
	}
	body := strings.Replace(doc, `encoding="UTF-16"`, `encoding="UTF-8"`, 1)
	if err := xml.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("XML inválido: %v\n%s", err, doc)
	}
	if out.Command != `C:\Users\A & B\<app>\wintaskbar.exe` || out.WorkDir != `C:\Users\A & B\<app>` {
		t.Fatalf("comando = %q, pasta = %q", out.Command, out.WorkDir)
	}
	if out.User != `PC\A & B` || out.Level != "LeastPrivilege" {
		t.Fatalf("usuário = %q, nível = %q", out.User, out.Level)
	}
}
