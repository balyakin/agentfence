package project

import (
	"testing"

	"github.com/agentfence/agentfence/internal/domain"
)

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

func TestParsePorcelainRenameAndInvalidRecords(t *testing.T) {
	t.Parallel()
	entries, err := ParsePorcelainZ([]byte("R  new.go\x00old.go\x00"))
	if err != nil {
		t.Fatalf("parse rename: %v", err)
	}
	if len(entries) != 1 || entries[0].OrigPath != "old.go" || entries[0].Path != "new.go" {
		t.Fatalf("unexpected rename: %#v", entries)
	}
	for _, data := range [][]byte{[]byte("x\x00"), []byte("R  new.go\x00")} {
		if _, err := ParsePorcelainZ(data); err == nil {
			t.Fatalf("invalid record accepted: %q", data)
		}
	}
	if !IsDirty(entries[0]) {
		t.Fatal("rename was not dirty")
	}
	if IsUntracked(entries[0]) {
		t.Fatal("rename was untracked")
	}
	untracked := domain.StatusEntry{Index: '?', Worktree: '?'}
	if IsDirty(untracked) || !IsUntracked(untracked) {
		t.Fatalf("unexpected untracked classification: %#v", untracked)
	}
}
