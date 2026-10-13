//go:build windows

package desktop

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// Verificação de integração opcional: registra, consulta e remove uma tarefa
// temporária de logon (com outro nome, apontando para o notepad.exe, que nunca
// é iniciado). Não toca na tarefa nem na chave Run do wintaskbar.
func TestStartupTaskIntegration(t *testing.T) {
	if os.Getenv("WINTASKBAR_TEST_STARTUP") != "1" {
		t.Skip("defina WINTASKBAR_TEST_STARTUP=1 para a verificação da tarefa de inicialização")
	}
	name := fmt.Sprintf("wintaskbar-test-%d", os.Getpid())
	defer deleteTask(name)

	exe := filepath.Join(os.Getenv("SystemRoot"), "System32", "notepad.exe")
	if taskExists(name) {
		t.Fatal("a tarefa de teste já existia")
	}
	if err := registerTask(name, exe); err != nil {
		t.Fatalf("registrar: %v", err)
	}
	if !taskExists(name) {
		t.Fatal("a tarefa não aparece depois de registrada")
	}
	if err := registerTask(name, exe); err != nil {
		t.Fatalf("registrar de novo (substituir): %v", err)
	}
	deleteTask(name)
	if taskExists(name) {
		t.Fatal("a tarefa continua depois de removida")
	}
}
