package service

import (
	"context"
	"errors"
	"testing"
	"time"

	errs "github.com/engenheiroaraujo/wintaskbar/internal/platform/errors"
)

var errNoTaskbar = errs.ErrNoTaskbar

type fakeLocator struct {
	pid uint32
	err error
}

func (f *fakeLocator) TaskbarPID() (uint32, error) { return f.pid, f.err }

type fakeInjector struct {
	calls []uint32
	err   error
}

func (f *fakeInjector) Inject(pid uint32, _ string) error {
	f.calls = append(f.calls, pid)
	return f.err
}

func newTestService(loc *fakeLocator, inj *fakeInjector) *Service {
	return New(loc, inj, nil, nil)
}

func collect() (*[]Event, func(Event)) {
	var events []Event
	return &events, func(e Event) { events = append(events, e) }
}

func TestSyncAttachesWhenExplorerChanges(t *testing.T) {
	loc, inj := &fakeLocator{pid: 42}, &fakeInjector{}
	events, report := collect()

	got := newTestService(loc, inj).sync(0, "tap.dll", report)

	if got != 42 {
		t.Fatalf("attached = %d, want 42", got)
	}
	if len(inj.calls) != 1 || inj.calls[0] != 42 {
		t.Fatalf("inject calls = %v, want [42]", inj.calls)
	}
	if len(*events) != 1 || (*events)[0].PID != 42 || (*events)[0].Err != nil {
		t.Fatalf("events = %+v, want one attach to 42", *events)
	}
}

func TestSyncDoesNotReattachSameExplorer(t *testing.T) {
	loc, inj := &fakeLocator{pid: 42}, &fakeInjector{}
	events, report := collect()

	got := newTestService(loc, inj).sync(42, "tap.dll", report)

	if got != 42 || len(inj.calls) != 0 || len(*events) != 0 {
		t.Fatalf("attached=%d calls=%v events=%v, want 42 with no inject and no event", got, inj.calls, *events)
	}
}

func TestSyncForgetsExplorerWhileTaskbarIsMissing(t *testing.T) {
	loc, inj := &fakeLocator{err: errNoTaskbar}, &fakeInjector{}
	events, report := collect()

	got := newTestService(loc, inj).sync(42, "tap.dll", report)

	if got != 0 || len(inj.calls) != 0 || len(*events) != 0 {
		t.Fatalf("attached=%d calls=%v events=%v, want 0 silently", got, inj.calls, *events)
	}
}

func TestSyncReportsInjectFailureAndKeepsPreviousState(t *testing.T) {
	loc, inj := &fakeLocator{pid: 7}, &fakeInjector{err: errors.New("busy")}
	events, report := collect()

	got := newTestService(loc, inj).sync(0, "tap.dll", report)

	if got != 0 {
		t.Fatalf("attached = %d, want 0 so the next tick retries", got)
	}
	if len(*events) != 1 || (*events)[0].Err == nil || (*events)[0].PID != 7 {
		t.Fatalf("events = %+v, want one failure for pid 7", *events)
	}
}

func TestSyncReportsLocatorFailure(t *testing.T) {
	loc, inj := &fakeLocator{err: errors.New("access denied")}, &fakeInjector{}
	events, report := collect()

	got := newTestService(loc, inj).sync(42, "tap.dll", report)

	if got != 42 || len(inj.calls) != 0 {
		t.Fatalf("attached=%d calls=%v, want 42 and no inject", got, inj.calls)
	}
	if len(*events) != 1 || (*events)[0].Err == nil {
		t.Fatalf("events = %+v, want one failure", *events)
	}
}

func TestSyncDoesNotAttachWhenPrepareFails(t *testing.T) {
	loc, inj := &fakeLocator{pid: 5}, &fakeInjector{}
	events, report := collect()
	svc := New(loc, inj, nil, func() error { return errs.New(errs.InvalidTheme, "bad theme") })

	got := svc.sync(0, "tap.dll", report)

	if got != 0 || len(inj.calls) != 0 {
		t.Fatalf("attached=%d calls=%v, want no attach with an invalid Theme", got, inj.calls)
	}
	if len(*events) != 1 || errs.CategoryOf((*events)[0].Err) != errs.InvalidTheme {
		t.Fatalf("events = %+v, want the Theme error reported", *events)
	}
}

// seqLocator devolve suas respostas em ordem, repetindo a última.
type seqLocator struct {
	pids []uint32
	errs []error
	i    int
}

func (s *seqLocator) TaskbarPID() (uint32, error) {
	i := min(s.i, len(s.pids)-1)
	s.i++
	return s.pids[i], s.errs[i]
}

// failingInjector falha com err nas n primeiras chamadas.
type failingInjector struct {
	n     int
	err   error
	calls int
}

func (f *failingInjector) Inject(uint32, string) error {
	f.calls++
	if f.calls <= f.n {
		return f.err
	}
	return nil
}

func TestApplyAfterRestartWaitsForNewExplorerAndRetries(t *testing.T) {
	loc := &seqLocator{
		pids: []uint32{10, 0, 20},
		errs: []error{nil, errs.ErrNoTaskbar, nil},
	}
	inj := &failingInjector{n: 2, err: errs.ErrNoDiagConnection}
	svc := New(loc, inj, nil, nil)
	svc.retryEvery = time.Millisecond

	pid, err := svc.ApplyAfterRestart(context.Background(), 10, "tap.dll")

	if err != nil || pid != 20 || inj.calls != 3 {
		t.Fatalf("pid=%d err=%v calls=%d, want 20 after two retries", pid, err, inj.calls)
	}
}

func TestApplyAfterRestartGivesUpWhenContextEnds(t *testing.T) {
	loc := &seqLocator{pids: []uint32{10}, errs: []error{nil}} // never replaced
	svc := New(loc, &fakeInjector{}, nil, nil)
	svc.retryEvery = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	_, err := svc.ApplyAfterRestart(ctx, 10, "tap.dll")

	if errs.CategoryOf(err) != errs.Unavailable {
		t.Fatalf("err = %v, want unavailable", err)
	}
}

func TestApplyAttachesOnce(t *testing.T) {
	loc, inj := &fakeLocator{pid: 9}, &fakeInjector{}

	pid, err := newTestService(loc, inj).Apply("tap.dll")

	if err != nil || pid != 9 || len(inj.calls) != 1 {
		t.Fatalf("pid=%d err=%v calls=%v, want 9, nil, one inject", pid, err, inj.calls)
	}
}
