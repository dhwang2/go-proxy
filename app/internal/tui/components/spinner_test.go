package components

import (
	"strings"
	"testing"
)

func TestTimedSpinnerShowsElapsedSeconds(t *testing.T) {
	model := NewTimedSpinner("等待证书签发...", 120)

	if got := model.View(); !strings.Contains(got, "0s/120s") {
		t.Fatalf("initial timed spinner view = %q, want 0s/120s", got)
	}

	updated, _ := model.Update(spinnerElapsedMsg{})
	next := updated.(SpinnerModel)
	if got := next.View(); !strings.Contains(got, "1s/120s") {
		t.Fatalf("updated timed spinner view = %q, want 1s/120s", got)
	}
}
