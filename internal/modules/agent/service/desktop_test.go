package service

import (
	"context"
	"errors"
	"testing"
)

type desktopAgentFake struct {
	pid   uint32
	calls int
	err   error
}

func (a *desktopAgentFake) TaskbarPID() (uint32, error)  { return a.pid, a.err }
func (a *desktopAgentFake) Apply(string) (uint32, error) { a.calls++; return a.pid, a.err }

func TestDesktopRestartDisableAndReapply(t *testing.T) {
	a := &desktopAgentFake{pid: 10}
	restored := 0
	d := NewDesktop(a, "tap.dll", func(context.Context) (uint32, error) { a.pid = 30; return 30, nil }, func() error { restored++; return nil })
	for i := 0; i < 3; i++ {
		if err := d.Poll(); err != nil {
			t.Fatal(err)
		}
	}
	if a.calls != 1 {
		t.Fatalf("attached %d times to same process", a.calls)
	}
	a.pid = 20
	if err := d.Poll(); err != nil {
		t.Fatal(err)
	}
	if a.calls != 2 {
		t.Fatal("did not reattach after restart")
	}
	if err := d.Disable(); err != nil {
		t.Fatal(err)
	}
	a.pid = 21
	_ = d.Poll()
	if a.calls != 2 || restored != 1 || d.Enabled() {
		t.Fatal("disable did not stop attachment")
	}
	if err := d.Reapply(); err != nil {
		t.Fatal(err)
	}
	_ = d.Poll()
	if a.calls != 2 || !d.Enabled() {
		t.Fatal("reapply caused duplicate attachment")
	}
}

func TestDesktopRestoreFailureCanRetry(t *testing.T) {
	a := &desktopAgentFake{pid: 1}
	fail := true
	d := NewDesktop(a, "tap.dll", nil, func() error {
		if fail {
			return errors.New("restart failed")
		}
		return nil
	})
	_ = d.Poll()
	if d.Disable() == nil {
		t.Fatal("missing failure")
	}
	_ = d.Poll()
	if a.calls != 1 {
		t.Fatal("reattached while disabled")
	}
	fail = false
	if err := d.Disable(); err != nil {
		t.Fatal(err)
	}
	if d.attached != 0 {
		t.Fatal("attachment not cleared")
	}
}

func TestDesktopInvalidThemePausesMonitoring(t *testing.T) {
	a := &desktopAgentFake{pid: 1}
	d := NewDesktop(a, "tap.dll", func(context.Context) (uint32, error) { return 0, errors.New("invalid theme") }, func() error { return nil })
	_ = d.Poll()
	if d.Reapply() == nil {
		t.Fatal("missing validation error")
	}
	a.pid = 2
	_ = d.Poll()
	if d.Enabled() || a.calls != 1 {
		t.Fatal("invalid theme did not pause monitoring")
	}
}
