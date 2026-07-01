package project

import "testing"

func TestParsePorcelainZ(t *testing.T) {
	t.Parallel()
	entries, err := ParsePorcelainZ([]byte(" M file\nname.go\x00?? new.go\x00"))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries=%d", len(entries))
	}
}

func TestGitModeSkipsSymlink(t *testing.T) {
	t.Parallel()
	_, skip := gitMode("120000", "blob")
	if !skip {
		t.Fatalf("git symlink was not skipped")
	}
}
