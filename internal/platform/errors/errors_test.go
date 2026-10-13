package errs

import (
	"errors"
	"fmt"
	"testing"
)

func TestExitCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil", nil, 0},
		{"uncategorized", errors.New("boom"), 1},
		{"invalid input", New(InvalidInput, "x"), 2},
		{"invalid theme", WithDetails(InvalidTheme, "x", nil), 3},
		{"unavailable", ErrNoTaskbar, 4},
		{"wrapped keeps category", fmt.Errorf("context: %w", ErrNoDiagConnection), 4},
	}
	for _, c := range cases {
		if got := ExitCode(c.err); got != c.want {
			t.Errorf("%s: ExitCode = %d, want %d", c.name, got, c.want)
		}
	}
}

func TestPreserveOrKeepsExistingCategory(t *testing.T) {
	err := PreserveOr(InvalidInput, "outer", ErrNoTaskbar)
	if CategoryOf(err) != Unavailable {
		t.Fatalf("category = %q, want %q", CategoryOf(err), Unavailable)
	}
	if !errors.Is(err, ErrNoTaskbar) {
		t.Fatal("wrapped error lost the sentinel")
	}
}

func TestPreserveOrCategorizesPlainError(t *testing.T) {
	err := PreserveOr(Unavailable, "attach", errors.New("hresult"))
	if CategoryOf(err) != Unavailable {
		t.Fatalf("category = %q, want %q", CategoryOf(err), Unavailable)
	}
}
