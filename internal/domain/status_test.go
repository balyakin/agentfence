package domain

import "testing"

func TestRunStatusTerminal(t *testing.T) {
	t.Parallel()
	if RunStatusRunning.IsTerminal() {
		t.Fatalf("running must not be terminal")
	}
	if !RunStatusSucceeded.IsTerminal() {
		t.Fatalf("succeeded must be terminal")
	}
}
