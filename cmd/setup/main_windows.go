//go:build windows

// Setup é um instalador autocontido, por usuário. Não são necessários runtime do Go nem
// privilégios de administrador na máquina de destino.
package main

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/engenheiroaraujo/wintaskbar/internal/platform/windows/desktop"
	"github.com/engenheiroaraujo/wintaskbar/internal/platform/windows/tray"
)

//go:embed payload/*
var payload embed.FS

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--check" {
		if err := checkPayload(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	remove := len(os.Args) == 2 && os.Args[1] == "--uninstall"
	question := "Instalar o wintaskbar para este usuário?\n\nO aplicativo será adicionado ao menu Iniciar e à área de trabalho. Go não é necessário.\n\nA inicialização com o Windows pode ser ativada depois, pelo ícone da bandeja."
	if remove {
		question = "Desinstalar o wintaskbar?\n\nA barra será restaurada. Seus temas e logs serão preservados."
	}
	if !tray.Confirm(question) {
		return
	}
	var err error
	if remove {
		err = uninstall()
	} else {
		err = install()
	}
	if err != nil {
		tray.Message(err.Error(), true)
		os.Exit(1)
	}
	if remove {
		tray.Message("wintaskbar desinstalado. Seus temas e logs foram preservados.", false)
	}
}

func installDir() (string, error) {
	local := os.Getenv("LOCALAPPDATA")
	if !filepath.IsAbs(local) {
		return "", fmt.Errorf("LOCALAPPDATA is not an absolute path")
	}
	return filepath.Join(local, "Programs", "wintaskbar"), nil
}

func checkPayload() error {
	for _, name := range []string{"wintaskbar.exe", "tap.dll"} {
		data, err := payload.ReadFile("payload/" + name)
		if err != nil {
			return fmt.Errorf("instalador incompleto: execute scripts/build-release.ps1: %w", err)
		}
		if len(data) < 1024 || string(data[:2]) != "MZ" {
			return fmt.Errorf("invalid embedded executable: %s", name)
		}
	}
	return nil
}

func install() error {
	if err := checkPayload(); err != nil {
		return err
	}
	dir, err := installDir()
	if err != nil {
		return err
	}
	if err := tray.Stop(); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	// Grava o marcador primeiro, para que uma instalação parcial ainda possa ser removida.
	if err := os.WriteFile(filepath.Join(dir, "installation.txt"), []byte("wintaskbar-user-install-v1\n"), 0644); err != nil {
		return err
	}
	for _, name := range []string{"tap.dll", "wintaskbar.exe"} {
		data, _ := payload.ReadFile("payload/" + name)
		if err := replaceFile(filepath.Join(dir, name), data); err != nil {
			return fmt.Errorf("não foi possível atualizar %s. Encerre o wintaskbar e tente novamente: %w", name, err)
		}
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	setup := filepath.Join(dir, "uninstall.exe")
	if !strings.EqualFold(self, setup) {
		data, err := os.ReadFile(self)
		if err != nil {
			return err
		}
		if err := replaceFile(setup, data); err != nil {
			return err
		}
	}
	exe := filepath.Join(dir, "wintaskbar.exe")
	shortcuts, err := shortcutPaths()
	if err != nil {
		return err
	}
	for _, path := range shortcuts {
		if err := desktop.Shortcut(path, exe, ""); err != nil {
			return err
		}
	}
	for name, value := range map[string]string{
		"DisplayName": "wintaskbar", "DisplayVersion": "0.1.0", "Publisher": "wintaskbar",
		"InstallLocation": dir, "DisplayIcon": exe, "UninstallString": `"` + setup + `" --uninstall`,
	} {
		if err := desktop.SetString(desktop.UninstallKey, name, value); err != nil {
			return err
		}
	}
	// Preserva a opção já escolhida ao atualizar, mas aponta para esta instalação.
	if on, err := desktop.StartupEnabled(); err != nil {
		return err
	} else if on {
		if err := desktop.SetStartupFor(exe); err != nil {
			return err
		}
	}
	cmd := exec.Command(exe)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func replaceFile(path string, data []byte) error {
	temp := path + ".new"
	if err := os.WriteFile(temp, data, 0755); err != nil {
		return err
	}
	defer os.Remove(temp)
	return os.Rename(temp, path)
}

func shortcutPaths() ([]string, error) {
	// User Shell Folders considera uma Área de Trabalho redirecionada para o OneDrive.
	desk, err := desktop.ReadString(`Software\Microsoft\Windows\CurrentVersion\Explorer\Shell Folders`, "Desktop")
	if err != nil {
		return nil, err
	}
	if desk == "" {
		desk = filepath.Join(os.Getenv("USERPROFILE"), "Desktop")
	}
	programs, err := desktop.ReadString(`Software\Microsoft\Windows\CurrentVersion\Explorer\Shell Folders`, "Programs")
	if err != nil {
		return nil, err
	}
	if programs == "" {
		programs = filepath.Join(os.Getenv("APPDATA"), "Microsoft", "Windows", "Start Menu", "Programs")
	}
	if !filepath.IsAbs(desk) || !filepath.IsAbs(programs) {
		return nil, fmt.Errorf("invalid shortcut directory")
	}
	return []string{filepath.Join(desk, "wintaskbar.lnk"), filepath.Join(programs, "wintaskbar.lnk")}, nil
}

func uninstall() error {
	dir, err := installDir()
	if err != nil {
		return err
	}
	marker, err := os.ReadFile(filepath.Join(dir, "installation.txt"))
	if err != nil || string(marker) != "wintaskbar-user-install-v1\n" {
		return fmt.Errorf("instalação do wintaskbar não encontrada em %s", dir)
	}
	if err := tray.Stop(); err != nil {
		return err
	}
	if err := desktop.SetStartup(false); err != nil {
		return err
	}
	paths, err := shortcutPaths()
	if err != nil {
		return err
	}
	paths = append(paths, filepath.Join(dir, "wintaskbar.exe"), filepath.Join(dir, "tap.dll"))
	for _, path := range paths {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := desktop.DeleteKey(desktop.UninstallKey); err != nil {
		return err
	}
	if err := desktop.DeleteKey(desktop.SettingsKey); err != nil {
		return err
	}
	// Um executável em execução não pode apagar a si mesmo. O auxiliar oculto espera
	// este processo e então apaga SOMENTE os nossos arquivos nomeados e o diretório vazio.
	// Deliberadamente não usa exclusão recursiva; os arquivos do usuário são preservados.
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }
	script := fmt.Sprintf("Wait-Process -Id %d -ErrorAction SilentlyContinue; Remove-Item -LiteralPath %s,%s -Force -ErrorAction SilentlyContinue; if ((Get-ChildItem -LiteralPath %s -Force | Measure-Object).Count -eq 0) { Remove-Item -LiteralPath %s -ErrorAction SilentlyContinue }", os.Getpid(), quote(filepath.Join(dir, "uninstall.exe")), quote(filepath.Join(dir, "installation.txt")), quote(dir), quote(dir))
	cmd := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"), "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
