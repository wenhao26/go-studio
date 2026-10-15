package task

import (
	"errors"
	"testing"
)

// TestCanTransition 覆盖 requirements §6.2 的完整合法转换表。
// 未列出的组合（含一切涉及 paused 的转换）一律非法。
func TestCanTransition(t *testing.T) {
	all := []Status{
		StatusPending, StatusRunning, StatusCompleted, StatusFailed, StatusCanceled, StatusPaused,
	}
	legal := map[[2]Status]bool{
		{StatusPending, StatusRunning}:   true,
		{StatusPending, StatusCanceled}:  true,
		{StatusRunning, StatusCompleted}: true,
		{StatusRunning, StatusFailed}:    true,
		{StatusRunning, StatusCanceled}:  true,
	}

	for _, from := range all {
		for _, to := range all {
			want := legal[[2]Status{from, to}]
			if got := canTransition(from, to); got != want {
				t.Errorf("canTransition(%s → %s) = %v, want %v", from, to, got, want)
			}
		}
	}
}

func TestIsTerminal(t *testing.T) {
	tests := []struct {
		status Status
		want   bool
	}{
		{StatusPending, false},
		{StatusRunning, false},
		{StatusPaused, false},
		{StatusCompleted, true},
		{StatusFailed, true},
		{StatusCanceled, true},
	}
	for _, tt := range tests {
		if got := IsTerminal(tt.status); got != tt.want {
			t.Errorf("IsTerminal(%s) = %v, want %v", tt.status, got, tt.want)
		}
	}
}

// TestTransitionError 保留错误链：调用方必须能用 errors.Is 判别非法转换。
func TestTransitionError(t *testing.T) {
	err := transitionError(7, StatusRunning, StatusRunning)
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("want ErrInvalidTransition, got %v", err)
	}
	if got := err.Error(); got != "task 7: running → running: invalid task status transition" {
		t.Fatalf("message = %q", got)
	}
}
