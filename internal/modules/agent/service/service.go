// Package service contém os casos de uso do Agent: anexar o TAP ao explorer
// que possui a Taskbar, mantê-lo anexado quando o explorer reinicia, e ler ou
// alterar o "ocultar automaticamente" da Taskbar.
package service

import (
	"context"
	"errors"
	"time"

	errs "github.com/engenheiroaraujo/wintaskbar/internal/platform/errors"
)

// TaskbarLocator encontra o processo do explorer que possui a Taskbar.
type TaskbarLocator interface {
	TaskbarPID() (uint32, error)
}

// Injector anexa o TAP em tapPath ao processo pid.
type Injector interface {
	Inject(pid uint32, tapPath string) error
}

// TaskbarSettings lê e altera o "ocultar automaticamente" da Taskbar.
type TaskbarSettings interface {
	AutoHide() (bool, error)
	SetAutoHide(on bool) error
	ToggleAutoHide() (bool, error)
}

// Event relata uma ação do Watch: uma anexação (PID preenchido, Err nulo) ou
// uma falha (Err preenchido).
type Event struct {
	Time time.Time
	PID  uint32
	Err  error
}

// Service implementa os casos de uso do Agent.
type Service struct {
	locator  TaskbarLocator
	injector Injector
	settings TaskbarSettings
	// prepare roda antes de cada anexação e grava o Theme compilado que o TAP
	// lê. Um erro impede a anexação. Nil significa que não há o que preparar.
	prepare func() error
	now     func() time.Time
	// retryEvery dá o ritmo do ApplyAfterRestart enquanto o novo explorer sobe.
	retryEvery time.Duration
}

// New monta o service. prepare roda antes de cada anexação (nil para nenhum).
func New(locator TaskbarLocator, injector Injector, settings TaskbarSettings, prepare func() error) *Service {
	return &Service{
		locator:    locator,
		injector:   injector,
		settings:   settings,
		prepare:    prepare,
		now:        time.Now,
		retryEvery: 400 * time.Millisecond,
	}
}

func (s *Service) runPrepare() error {
	if s.prepare == nil {
		return nil
	}
	return s.prepare()
}

// TaskbarPID devolve o ID do processo do explorer que possui a Taskbar.
func (s *Service) TaskbarPID() (uint32, error) {
	return s.locator.TaskbarPID()
}

// Apply anexa o TAP uma vez e devolve o PID do explorer ao qual anexou.
func (s *Service) Apply(tapPath string) (uint32, error) {
	pid, err := s.locator.TaskbarPID()
	if err != nil {
		return 0, err
	}
	if err := s.runPrepare(); err != nil {
		return pid, err
	}
	if err := s.injector.Inject(pid, tapPath); err != nil {
		return pid, err
	}
	return pid, nil
}

// ApplyAfterRestart espera um explorer diferente de oldPID possuir a Taskbar
// e anexa o TAP a ele, tentando de novo enquanto o novo explorer ainda sobe,
// até ctx terminar.
func (s *Service) ApplyAfterRestart(ctx context.Context, oldPID uint32, tapPath string) (uint32, error) {
	var lastErr error = errs.ErrNoTaskbar
	for {
		pid, err := s.locator.TaskbarPID()
		switch {
		case errors.Is(err, errs.ErrNoTaskbar) || (err == nil && pid == oldPID):
			// o novo explorer ainda não subiu
		case err != nil:
			return 0, err
		default:
			err = s.injector.Inject(pid, tapPath)
			if err == nil {
				return pid, nil
			}
			if errs.CategoryOf(err) != errs.Unavailable {
				return pid, err
			}
			lastErr = err // o explorer subiu, mas o XAML ainda não está pronto
		}

		select {
		case <-ctx.Done():
			return 0, errs.PreserveOr(errs.Unavailable, "o novo explorer não aceitou o TAP a tempo", lastErr)
		case <-time.After(s.retryEvery):
		}
	}
}

// Watch anexa o TAP e o reanexa sempre que o explorer reinicia, até ctx
// terminar. Cada anexação ou falha é passada a report; a mesma falha repetida
// a cada verificação é relatada uma única vez.
func (s *Service) Watch(ctx context.Context, tapPath string, every time.Duration, report func(Event)) {
	tick := time.NewTicker(every)
	defer tick.Stop()

	lastErr := ""
	deduped := func(e Event) {
		if e.Err != nil {
			if e.Err.Error() == lastErr {
				return
			}
			lastErr = e.Err.Error()
		} else {
			lastErr = ""
		}
		report(e)
	}

	var attached uint32
	for {
		attached = s.sync(attached, tapPath, deduped)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// sync é um passo do Watch: anexa quando o explorer da Taskbar difere do já
// anexado e devolve o PID agora anexado (0 quando nenhum).
func (s *Service) sync(attached uint32, tapPath string, report func(Event)) uint32 {
	pid, err := s.locator.TaskbarPID()
	switch {
	case errors.Is(err, errs.ErrNoTaskbar):
		return 0 // o explorer caiu; reanexa quando ele voltar
	case err != nil:
		report(Event{Time: s.now(), Err: err})
		return attached
	case pid == attached:
		return attached
	}

	if err := s.runPrepare(); err != nil {
		report(Event{Time: s.now(), Err: err}) // ex.: o Theme tem erros: não anexa
		return attached
	}
	if err := s.injector.Inject(pid, tapPath); err != nil {
		report(Event{Time: s.now(), PID: pid, Err: err})
		return attached
	}
	report(Event{Time: s.now(), PID: pid})
	return pid
}

// AutoHide informa se a Taskbar oculta automaticamente.
func (s *Service) AutoHide() (bool, error) { return s.settings.AutoHide() }

// SetAutoHide liga ou desliga o "ocultar automaticamente" da Taskbar.
func (s *Service) SetAutoHide(on bool) error { return s.settings.SetAutoHide(on) }

// ToggleAutoHide inverte o "ocultar automaticamente" e devolve o novo estado.
func (s *Service) ToggleAutoHide() (bool, error) { return s.settings.ToggleAutoHide() }
