package service

import (
	"context"
	"errors"
	"testing"

	"github.com/engenheiroaraujo/wintaskbar/internal/modules/theme/model"
	errs "github.com/engenheiroaraujo/wintaskbar/internal/platform/errors"
)

type fakeStore struct {
	theme    model.Theme
	written  *model.Resolved
	writeErr error
}

func (f *fakeStore) LoadActive() (model.Theme, string, error) { return f.theme, "active", nil }
func (f *fakeStore) Load(string) (model.Theme, error)         { return f.theme, nil }
func (f *fakeStore) WriteCompiled(r model.Resolved) error {
	f.written = &r
	return f.writeErr
}
func (f *fakeStore) InitUser() (string, bool, error) { return "theme.yaml", true, nil }

type fakeShell struct {
	restarted bool
	err       error
}

func (f *fakeShell) RestartExplorer() (uint32, error) {
	f.restarted = true
	return 10, f.err
}

type fakeAttacher struct{ oldPID uint32 }

func (f *fakeAttacher) ApplyAfterRestart(_ context.Context, oldPID uint32, _ string) (uint32, error) {
	f.oldPID = oldPID
	return 11, nil
}

var validTheme = model.Theme{
	Name:  "t",
	Rules: []model.Rule{{Selector: "Grid#RootGrid", Styles: []model.Style{{Property: "Margin", Value: "0,0,0,4"}}}},
}

var invalidTheme = model.Theme{
	Name:  "t",
	Rules: []model.Rule{{Selector: "Grid#RootGrid", Line: 3, Styles: []model.Style{{Property: "Margin", Value: "x", Line: 4}}}},
}

func TestCheckReportsProblemsWithLines(t *testing.T) {
	svc := New(&fakeStore{theme: invalidTheme}, nil, nil)

	_, _, err := svc.Check("")

	if errs.CategoryOf(err) != errs.InvalidTheme {
		t.Fatalf("category = %q, want invalid_theme", errs.CategoryOf(err))
	}
	d := errs.DetailsOf(err)
	if len(d) != 1 || d[0].Where != `linha 4, regra "Grid#RootGrid", Margin` {
		t.Fatalf("details = %+v", d)
	}
}

func TestCompileWritesOnlyValidThemes(t *testing.T) {
	bad := &fakeStore{theme: invalidTheme}
	if _, _, err := New(bad, nil, nil).Compile(); err == nil || bad.written != nil {
		t.Fatalf("invalid theme compiled: err=%v written=%v", err, bad.written)
	}

	good := &fakeStore{theme: validTheme}
	if _, _, err := New(good, nil, nil).Compile(); err != nil || good.written == nil {
		t.Fatalf("valid theme not written: err=%v", err)
	}
}

func TestApplyDoesNotRestartExplorerForInvalidTheme(t *testing.T) {
	shell := &fakeShell{}
	_, _, _, err := New(&fakeStore{theme: invalidTheme}, shell, &fakeAttacher{}).Apply(context.Background(), "tap.dll")
	if err == nil || shell.restarted {
		t.Fatalf("err=%v restarted=%v, want an error and explorer untouched", err, shell.restarted)
	}
}

func TestApplyCompilesRestartsAndAttaches(t *testing.T) {
	store, shell, attacher := &fakeStore{theme: validTheme}, &fakeShell{}, &fakeAttacher{}

	_, _, pid, err := New(store, shell, attacher).Apply(context.Background(), "tap.dll")

	if err != nil || store.written == nil || !shell.restarted || attacher.oldPID != 10 || pid != 11 {
		t.Fatalf("err=%v written=%v restarted=%v oldPID=%d pid=%d", err, store.written != nil, shell.restarted, attacher.oldPID, pid)
	}
}

func TestApplyStopsWhenRestartFails(t *testing.T) {
	attacher := &fakeAttacher{}
	_, _, _, err := New(&fakeStore{theme: validTheme}, &fakeShell{err: errors.New("denied")}, attacher).
		Apply(context.Background(), "tap.dll")
	if err == nil || attacher.oldPID != 0 {
		t.Fatalf("err=%v attached=%v, want error and no attach", err, attacher.oldPID != 0)
	}
}
