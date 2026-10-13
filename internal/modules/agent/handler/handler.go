// Package handler é o transporte de linha de comando do Agent: lê os
// argumentos de cada comando, chama o service e escreve o resultado.
package handler

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/engenheiroaraujo/wintaskbar/internal/modules/agent/service"
	errs "github.com/engenheiroaraujo/wintaskbar/internal/platform/errors"
)

// Handler atende os comandos do Agent.
type Handler struct {
	svc        *service.Service
	out        io.Writer
	errOut     io.Writer
	defaultTAP string
}

// New monta o handler. defaultTAP é o caminho do TAP usado quando -dll não é
// informado.
func New(svc *service.Service, out, errOut io.Writer, defaultTAP string) *Handler {
	return &Handler{svc: svc, out: out, errOut: errOut, defaultTAP: defaultTAP}
}

// Apply anexa o TAP uma vez: wintaskbar apply [-dll caminho]
func (h *Handler) Apply(args []string) error {
	fs := flag.NewFlagSet("apply", flag.ExitOnError)
	dll := fs.String("dll", h.defaultTAP, "caminho da tap.dll")
	_ = fs.Parse(args)

	pid, err := h.svc.Apply(*dll)
	if err != nil {
		return err
	}
	fmt.Fprintf(h.out, "TAP anexado ao explorer (pid %d) com %s\n", pid, *dll)
	fmt.Fprintln(h.out, "log: %LOCALAPPDATA%\\wintaskbar\\tap.log   |   para reverter: reinicie o explorer")
	return nil
}

// Watch anexa o TAP e o reanexa depois que o explorer reinicia, até ser
// interrompido: wintaskbar watch [-dll caminho] [-every d]
func (h *Handler) Watch(args []string) error {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	dll := fs.String("dll", h.defaultTAP, "caminho da tap.dll")
	every := fs.Duration("every", 2*time.Second, "intervalo de verificação")
	_ = fs.Parse(args)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	h.svc.Watch(ctx, *dll, *every, func(e service.Event) {
		switch {
		case e.Err != nil && e.PID != 0:
			fmt.Fprintln(h.errOut, "wintaskbar: anexar:", e.Err)
		case e.Err != nil:
			fmt.Fprintln(h.errOut, "wintaskbar:", e.Err)
		default:
			fmt.Fprintf(h.out, "%s TAP anexado ao explorer (pid %d)\n", e.Time.Format("15:04:05"), e.PID)
		}
	})
	return nil
}

// AutoHide lê ou altera o "ocultar automaticamente" da Taskbar:
// wintaskbar autohide [status|on|off|toggle]
func (h *Handler) AutoHide(args []string) error {
	action := "status"
	if len(args) > 0 {
		action = args[0]
	}

	var on bool
	var err error
	switch action {
	case "status":
		on, err = h.svc.AutoHide()
	case "on", "off":
		on = action == "on"
		err = h.svc.SetAutoHide(on)
	case "toggle":
		on, err = h.svc.ToggleAutoHide()
	default:
		return errs.New(errs.InvalidInput, fmt.Sprintf("ação desconhecida %q (use status, on, off ou toggle)", action))
	}
	if err != nil {
		return err
	}
	state := "desligado"
	if on {
		state = "ligado"
	}
	fmt.Fprintf(h.out, "ocultar automaticamente: %s\n", state)
	return nil
}

// PID imprime o ID do processo do explorer que possui a Taskbar:
// wintaskbar pid
func (h *Handler) PID(_ []string) error {
	pid, err := h.svc.TaskbarPID()
	if err != nil {
		return err
	}
	fmt.Fprintln(h.out, pid)
	return nil
}
