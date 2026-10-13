//go:build windows

package desktop

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
)

// O subprocesso só consulta o próprio console; não toca na barra ou no registro.
func TestBackgroundProcessWithoutConsole(t *testing.T) {
	if os.Getenv("WINTASKBAR_CONSOLE_TEST_CHILD") == "1" {
		window, _, _ := syscall.NewLazyDLL(system32("kernel32.dll")).NewProc("GetConsoleWindow").Call()
		fmt.Printf("console=%d", window)
		os.Exit(0)
	}
	if os.Getenv("WINTASKBAR_TEST_BACKGROUND") != "1" {
		t.Skip("set WINTASKBAR_TEST_BACKGROUND=1 for background process test")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := hidden(exec.Command(exe, "-test.run=^TestBackgroundProcessWithoutConsole$"))
	cmd.Env = append(os.Environ(), "WINTASKBAR_CONSOLE_TEST_CHILD=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("background child: %v: %s", err, out)
	}
	if strings.TrimSpace(string(out)) != "console=0" {
		t.Fatalf("background child has a console: %s", out)
	}
}
