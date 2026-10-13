// Package handler é o transporte de linha de comando do Theme:
//
//	wintaskbar theme check [arquivo]        valida o Theme ativo, ou o arquivo
//	wintaskbar theme init                   cria a cópia editável do Default Theme
//	wintaskbar theme apply [-dll caminho]  compila, reinicia o explorer e anexa
package handler

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/engenheiroaraujo/wintaskbar/internal/modules/theme/service"
	errs "github.com/engenheiroaraujo/wintaskbar/internal/platform/errors"
)

// Handler atende os comandos de theme.
type Handler struct {
	svc        *service.Service
	out        io.Writer
	defaultTAP string
}

// New monta o handler. defaultTAP é o caminho do TAP usado quando -dll não é
// informado.
func New(svc *service.Service, out io.Writer, defaultTAP string) *Handler {
	return &Handler{svc: svc, out: out, defaultTAP: defaultTAP}
}

// Run despacha "wintaskbar theme <subcomando>".
func (h *Handler) Run(args []string) error {
	if len(args) == 0 {
		return errs.New(errs.InvalidInput, "use: wintaskbar theme <check|init|apply>")
	}
	switch args[0] {
	case "check":
		return h.check(args[1:])
	case "init":
		return h.init()
	case "apply":
		return h.apply(args[1:])
	default:
		return errs.New(errs.InvalidInput, fmt.Sprintf("subcomando desconhecido %q (use check, init ou apply)", args[0]))
	}
}

func (h *Handler) check(args []string) error {
	path := ""
	if len(args) > 0 {
		path = args[0]
	}
	resolved, source, err := h.svc.Check(path)
	if err != nil {
		return errs.PreserveOr(errs.InvalidTheme, source, err)
	}
	fmt.Fprintf(h.out, "ok: tema %q (%s), %d regras, sem erros\n", resolved.Name, source, len(resolved.Rules))
	return nil
}

func (h *Handler) init() error {
	path, created, err := h.svc.Init()
	if err != nil {
		return err
	}
	if !created {
		fmt.Fprintf(h.out, "o tema editável já existe: %s\n", path)
		return nil
	}
	fmt.Fprintf(h.out, "tema editável criado: %s\n", path)
	fmt.Fprintln(h.out, "edite o arquivo e rode: wintaskbar theme apply")
	return nil
}

func (h *Handler) apply(args []string) error {
	fs := flag.NewFlagSet("theme apply", flag.ExitOnError)
	dll := fs.String("dll", h.defaultTAP, "caminho da tap.dll")
	timeout := fs.Duration("timeout", 30*time.Second, "tempo máximo para o novo explorer aceitar o TAP")
	_ = fs.Parse(args)

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	resolved, source, pid, err := h.svc.Apply(ctx, *dll)
	if err != nil {
		return err
	}
	fmt.Fprintf(h.out, "tema %q (%s) aplicado: %d regras, explorer pid %d\n", resolved.Name, source, len(resolved.Rules), pid)
	return nil
}
