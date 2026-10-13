//go:build windows

// Comando wintaskbar é o Agent: aplica o Theme à Taskbar anexando o TAP ao
// explorer que a possui.
//
//	wintaskbar apply [-dll caminho]                anexa uma vez
//	wintaskbar watch [-dll caminho] [-every d]     anexa e reanexa quando o explorer reinicia
//	wintaskbar theme check [arquivo]            valida o Theme ativo, ou o arquivo
//	wintaskbar theme init                          cria a cópia editável do Default Theme
//	wintaskbar theme apply [-dll caminho]          compila, reinicia o explorer e anexa
//	wintaskbar autohide [status|on|off|toggle]     "Ocultar automaticamente a barra de tarefas" do Windows
//	wintaskbar pid                                 imprime o PID do explorer da Taskbar
//
// Este arquivo é a raiz de composição: monta os adaptadores da plataforma,
// os services e handlers dos módulos e despacha o comando.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	agenthandler "github.com/engenheiroaraujo/wintaskbar/internal/modules/agent/handler"
	agentservice "github.com/engenheiroaraujo/wintaskbar/internal/modules/agent/service"
	themehandler "github.com/engenheiroaraujo/wintaskbar/internal/modules/theme/handler"
	themerepository "github.com/engenheiroaraujo/wintaskbar/internal/modules/theme/repository"
	themeservice "github.com/engenheiroaraujo/wintaskbar/internal/modules/theme/service"
	errs "github.com/engenheiroaraujo/wintaskbar/internal/platform/errors"
	"github.com/engenheiroaraujo/wintaskbar/internal/platform/windows/appbar"
	"github.com/engenheiroaraujo/wintaskbar/internal/platform/windows/desktop"
	"github.com/engenheiroaraujo/wintaskbar/internal/platform/windows/explorer"
	"github.com/engenheiroaraujo/wintaskbar/internal/platform/windows/tray"
	"github.com/engenheiroaraujo/wintaskbar/internal/platform/windows/xamldiag"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "stop" {
		if err := tray.Stop(); err != nil {
			report(err)
			os.Exit(1)
		}
		return
	}
	files, err := themerepository.DefaultFiles()
	if err != nil {
		report(err)
		os.Exit(1)
	}

	// O agent compila o Theme ativo antes de cada anexação, e o service de
	// theme anexa pelo agent depois de reiniciar o explorer; por isso cada
	// service recebe o outro por uma interface pequena.
	var themes *themeservice.Service
	agentSvc := agentservice.New(explorer.Locator{}, xamldiag.Injector{}, appbar.Settings{}, func() error {
		_, _, err := themes.Compile()
		return err
	})
	themes = themeservice.New(files, explorer.Shell{}, agentSvc)

	tap := defaultTAP()
	agent := agenthandler.New(agentSvc, os.Stdout, os.Stderr, tap)
	theme := themehandler.New(themes, os.Stdout, tap)

	commands := map[string]func([]string) error{
		"apply":    agent.Apply,
		"watch":    agent.Watch,
		"theme":    theme.Run,
		"autohide": agent.AutoHide,
		"pid":      agent.PID,
	}

	if len(os.Args) < 2 {
		session := agentservice.NewDesktop(agentSvc, tap, func(ctx context.Context) (uint32, error) {
			_, _, pid, err := themes.Apply(ctx, tap)
			return pid, err
		}, func() error { _, err := explorer.Restart(); return err })

		err := agenthandler.RunDesktop(session, agenthandler.DesktopActions{
			Edit: func() error {
				path, _, err := themes.Init()
				if err != nil {
					return err
				}
				return desktop.Edit(path)
			},
			AutoHide:       agentSvc.AutoHide,
			ToggleAutoHide: agentSvc.ToggleAutoHide,
			Startup:        desktop.StartupEnabled,
			SetStartup:     desktop.SetStartup,
			Logs: func() error {
				dir := filepath.Dir(files.Compiled)
				if err := os.MkdirAll(dir, 0755); err != nil {
					return err
				}
				return desktop.Open(dir)
			},
		})
		if err != nil {
			tray.Message(err.Error(), true)
			os.Exit(1)
		}
		return
	}
	run, ok := commands[os.Args[1]]
	if !ok {
		usage()
		os.Exit(2)
	}
	if err := run(os.Args[2:]); err != nil {
		report(err)
		os.Exit(errs.ExitCode(err))
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "uso: wintaskbar <apply|watch|theme|autohide|pid> [opções]")
}

// report escreve err e seus detalhes localizados (por exemplo, cada erro de
// um arquivo de Theme) em stderr.
func report(err error) {
	if len(os.Args) < 2 {
		tray.Message(err.Error(), true)
		return
	}
	fmt.Fprintln(os.Stderr, "wintaskbar:", err)
	for _, d := range errs.DetailsOf(err) {
		fmt.Fprintf(os.Stderr, "  %s: %s\n", d.Where, d.Message)
	}
}

// defaultTAP é a tap.dll ao lado do executável, ou a saída de
// scripts\build-tap.ps1 quando roda a partir do código-fonte.
func defaultTAP() string {
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Dir(exe)
		for _, p := range []string{
			filepath.Join(dir, "tap.dll"),
			filepath.Join(dir, "release", "tap.dll"),
			filepath.Join(dir, "tap", "build", "tap.dll"),
			filepath.Join(dir, "..", "tap", "build", "tap.dll"),
			filepath.Join(dir, "..", "..", "tap", "build", "tap.dll"),
		} {
			if _, err := os.Stat(p); err == nil {
				return p
			}
		}
	}
	return filepath.Join("tap", "build", "tap.dll")
}
