// Package service contém os casos de uso do Theme: validar um arquivo de Theme, compilar o
// Theme ativo para o TAP, criar a cópia editável do usuário e aplicar um
// Theme reiniciando o explorer.
package service

import (
	"context"
	"fmt"

	"github.com/engenheiroaraujo/wintaskbar/internal/modules/theme/model"
	errs "github.com/engenheiroaraujo/wintaskbar/internal/platform/errors"
)

// Store carrega Themes e grava a forma compilada.
type Store interface {
	LoadActive() (model.Theme, string, error)
	Load(path string) (model.Theme, error)
	WriteCompiled(model.Resolved) error
	InitUser() (path string, created bool, err error)
}

// Shell reinicia o explorer e devolve o PID do que encerrou.
type Shell interface {
	RestartExplorer() (oldPID uint32, err error)
}

// Attacher anexa o TAP ao explorer que substituiu oldPID.
type Attacher interface {
	ApplyAfterRestart(ctx context.Context, oldPID uint32, tapPath string) (uint32, error)
}

// Service implementa os casos de uso do Theme.
type Service struct {
	store    Store
	shell    Shell
	attacher Attacher
}

// New monta o service.
func New(store Store, shell Shell, attacher Attacher) *Service {
	return &Service{store: store, shell: shell, attacher: attacher}
}

// Check valida o arquivo de Theme em path, ou o Theme ativo quando path está
// vazio. Devolve o Theme resolvido e de onde veio; cada problema
// encontrado está nos detalhes do erro.
func (s *Service) Check(path string) (model.Resolved, string, error) {
	var theme model.Theme
	var source string
	var err error
	if path == "" {
		theme, source, err = s.store.LoadActive()
	} else {
		theme, err = s.store.Load(path)
		source = path
	}
	if err != nil {
		return model.Resolved{}, source, err
	}

	resolved, problems := theme.Resolve()
	if len(problems) > 0 {
		return model.Resolved{}, source, errs.WithDetails(errs.InvalidTheme,
			fmt.Sprintf("o tema tem %d erro(s)", len(problems)), details(problems))
	}
	return resolved, source, nil
}

// Compile valida o Theme ativo e grava sua forma compilada para o TAP.
func (s *Service) Compile() (model.Resolved, string, error) {
	resolved, source, err := s.Check("")
	if err != nil {
		return resolved, source, err
	}
	if err := s.store.WriteCompiled(resolved); err != nil {
		return resolved, source, fmt.Errorf("write compiled theme: %w", err)
	}
	return resolved, source, nil
}

// Init cria o arquivo de Theme editável do usuário a partir do Default Theme.
func (s *Service) Init() (string, bool, error) {
	return s.store.InitUser()
}

// Apply compila o Theme ativo, reinicia o explorer para a Taskbar começar
// do visual original, e anexa o TAP ao novo explorer.
func (s *Service) Apply(ctx context.Context, tapPath string) (model.Resolved, string, uint32, error) {
	resolved, source, err := s.Compile()
	if err != nil {
		return resolved, source, 0, err
	}
	oldPID, err := s.shell.RestartExplorer()
	if err != nil {
		return resolved, source, 0, err
	}
	pid, err := s.attacher.ApplyAfterRestart(ctx, oldPID, tapPath)
	return resolved, source, pid, err
}

func details(problems []model.Problem) []errs.Detail {
	out := make([]errs.Detail, 0, len(problems))
	for _, p := range problems {
		where := p.Where
		if p.Line > 0 {
			where = fmt.Sprintf("linha %d, %s", p.Line, p.Where)
		}
		out = append(out, errs.Detail{Where: where, Message: p.Message})
	}
	return out
}
