//go:build windows

package handler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/engenheiroaraujo/wintaskbar/internal/modules/agent/service"
	errs "github.com/engenheiroaraujo/wintaskbar/internal/platform/errors"
	"github.com/engenheiroaraujo/wintaskbar/internal/platform/windows/tray"
)

const (
	menuApply uint32 = iota + 1
	menuDisable
	menuEdit
	menuAutoHide
	menuStartup
	menuLogs
	menuError
)

// DesktopActions adapta as configurações da plataforma e os casos de uso do tema na composição.
type DesktopActions struct {
	Edit           func() error
	AutoHide       func() (bool, error)
	ToggleAutoHide func() (bool, error)
	Startup        func() (bool, error)
	SetStartup     func(bool) error
	Logs           func() error
	// OnQuit roda ao sair, depois de restaurar a barra. Pode ser nulo.
	OnQuit func()
}

// RunDesktop mantém as operações lentas do Explorer fora da thread de UI, enquanto um worker
// serializa todas as ações com reanexação automática.
func RunDesktop(session *service.Desktop, actions DesktopActions) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	commands := make(chan uint32, 1)
	return tray.Run(func(icon *tray.Icon) {
		go desktopLoop(ctx, icon, session, actions, commands)
	}, func(id uint32) {
		select {
		case commands <- id:
		default:
		}
	})
}

func desktopLoop(ctx context.Context, icon *tray.Icon, session *service.Desktop, actions DesktopActions, commands <-chan uint32) {
	// Logo depois de iniciar (por exemplo, ao fazer logon) a barra ainda pode
	// não existir: nesse período a verificação é mais frequente e a falta da
	// barra não é tratada como erro.
	bootWindow := time.Now().Add(time.Minute)
	booting := func() bool { return time.Now().Before(bootWindow) }
	nextPoll := func() time.Duration {
		if booting() && session.Enabled() && !session.Attached() {
			return 400 * time.Millisecond
		}
		return 2 * time.Second
	}
	timer := time.NewTimer(nextPoll())
	defer timer.Stop()
	lastError := ""
	publish := func(err error, notice string) {
		status := "tema ativo"
		if !session.Enabled() {
			status = "tema desativado"
			if session.Attached() {
				status = "monitoramento pausado"
			}
		}
		waiting := err != nil && booting() && errs.CategoryOf(err) == errs.Unavailable
		if waiting {
			err, status = nil, "aguardando a barra de tarefas"
		}
		if err != nil {
			status = "precisa de atenção"
			message := err.Error()
			for _, d := range errs.DetailsOf(err) {
				message += "\n" + d.Where + ": " + d.Message
			}
			if message != lastError {
				notice = message
				writeDesktopLog(message)
			}
			lastError = message
		}
		auto, autoErr := actions.AutoHide()
		startup, startupErr := actions.Startup()
		items := []tray.Item{
			{Label: "wintaskbar — " + status, Disabled: true},
			{},
			{ID: menuApply, Label: "Aplicar / recarregar tema", Icon: "\uE72C"},
			{ID: menuDisable, Label: "Desativar tema (restaurar barra)", Icon: "\uE71A"},
			{ID: menuEdit, Label: "Editar tema…", Icon: "\uE70F"},
			{},
			{ID: menuAutoHide, Label: "Ocultar automaticamente", Icon: "\uE76C", Toggle: true, Checked: auto, Disabled: autoErr != nil},
			{ID: menuStartup, Label: "Iniciar com o Windows", Icon: "\uE768", Toggle: true, Checked: startup, Disabled: startupErr != nil},
			{},
			{ID: menuLogs, Label: "Abrir pasta de logs", Icon: "\uE8B7"},
			{ID: menuError, Label: "Ver último erro…", Icon: "\uE946", Disabled: lastError == ""},
			{ID: 100, Label: "Sair e restaurar a barra", Icon: "\uE7E8"},
		}
		icon.Post(tray.Update{Status: status, Notice: notice, Error: err != nil, Items: items})
	}
	publish(session.Poll(), "O wintaskbar está na bandeja. Clique com o botão direito no ícone para acessar as opções.")
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			publish(session.Poll(), "")
			timer.Reset(nextPoll())
		case id := <-commands:
			if id == menuError {
				if lastError != "" {
					tray.Message(lastError, true)
				}
				continue
			}
			icon.Post(tray.Update{Status: "aguarde…", Items: []tray.Item{{Label: "wintaskbar — aguarde…", Disabled: true}}})
			var err error
			notice := ""
			switch id {
			case menuApply:
				err = session.Reapply()
				notice = "Tema aplicado."
			case menuDisable:
				err = session.Disable()
				notice = "Barra restaurada. Use Aplicar para ativar o tema novamente."
			case menuEdit:
				err = actions.Edit()
				notice = "Após salvar o arquivo, use Aplicar / recarregar tema."
			case menuAutoHide:
				_, err = actions.ToggleAutoHide()
			case menuStartup:
				var on bool
				on, err = actions.Startup()
				if err == nil {
					err = actions.SetStartup(!on)
				}
			case menuLogs:
				err = actions.Logs()
			case 0, 100:
				err = session.Disable()
				if err == nil {
					if actions.OnQuit != nil {
						actions.OnQuit()
					}
					icon.Post(tray.Update{Quit: true})
					return
				}
			}
			if err != nil {
				notice = ""
			}
			publish(err, notice)
		}
	}
}

func writeDesktopLog(message string) {
	dir := filepath.Join(os.Getenv("LOCALAPPDATA"), "wintaskbar")
	if os.MkdirAll(dir, 0755) != nil {
		return
	}
	f, err := os.OpenFile(filepath.Join(dir, "agent.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s %s\n", time.Now().Format(time.RFC3339), strings.ReplaceAll(message, "\r", ""))
}
