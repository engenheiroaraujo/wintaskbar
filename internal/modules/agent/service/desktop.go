package service

import (
	"context"
	"errors"
	"time"

	errs "github.com/engenheiroaraujo/wintaskbar/internal/platform/errors"
)

// DesktopAgent é o subconjunto de operações do agent usado pela sessão de desktop.
type DesktopAgent interface {
	TaskbarPID() (uint32, error)
	Apply(string) (uint32, error)
}

// Desktop serializa anexação, trocas de tema e restauração. Todos os métodos
// rodam em um único worker, então a verificação periódica não reanexa durante Disable ou Reapply.
type Desktop struct {
	agent    DesktopAgent
	reapply  func(context.Context) (uint32, error)
	restore  func() error
	tap      string
	enabled  bool
	attached uint32
}

func NewDesktop(agent DesktopAgent, tap string, reapply func(context.Context) (uint32, error), restore func() error) *Desktop {
	return &Desktop{agent: agent, tap: tap, reapply: reapply, restore: restore, enabled: true}
}

func (d *Desktop) Enabled() bool { return d.enabled }

func (d *Desktop) Attached() bool { return d.attached != 0 }

// Poll attaches once per Explorer process, retrying failed attachments.
func (d *Desktop) Poll() error {
	if !d.enabled {
		return nil
	}
	pid, err := d.agent.TaskbarPID()
	if err != nil {
		return err
	}
	if pid == d.attached {
		return nil
	}
	pid, err = d.agent.Apply(d.tap)
	if err == nil {
		d.attached = pid
	}
	return err
}

// Reapply valida o tema antes de reiniciar o Explorer. Em caso de falha,
// o monitoramento fica pausado até o usuário corrigir o tema e tentar de novo explicitamente.
func (d *Desktop) Reapply() error {
	d.enabled = false
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pid, err := d.reapply(ctx)
	if err != nil {
		return err
	}
	d.attached, d.enabled = pid, true
	return nil
}

// Disable interrompe o monitoramento antes de restaurar a barra original. Lembra
// da anexação em caso de falha, para que uma tentativa posterior ainda possa restaurá-la.
func (d *Desktop) Disable() error {
	d.enabled = false
	if d.attached == 0 {
		return nil
	}
	pid, err := d.agent.TaskbarPID()
	if errors.Is(err, errs.ErrNoTaskbar) || (err == nil && pid != d.attached) {
		d.attached = 0 // aquele Explorer já encerrou; não resta nada a restaurar
		return nil
	}
	if err != nil {
		return err
	}
	if err := d.restore(); err != nil {
		return err
	}
	d.attached = 0
	return nil
}
