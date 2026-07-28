package project

import (
	"bytes"
	"fmt"

	"github.com/agentfence/agentfence/internal/domain"
)

func ParsePorcelainZ(data []byte) ([]domain.StatusEntry, error) {
	records := bytes.Split(data, []byte{0})
	var entries []domain.StatusEntry
	for i := 0; i < len(records); i++ {
		record := records[i]
		if len(record) == 0 {
			continue
		}
		if len(record) < 4 {
			return nil, fmt.Errorf("invalid porcelain record")
		}
		entry := domain.StatusEntry{
			Index:    record[0],
			Worktree: record[1],
			Path:     string(record[3:]),
		}
		if entry.Index == 'R' || entry.Index == 'C' {
			if i+1 >= len(records) || len(records[i+1]) == 0 {
				return nil, fmt.Errorf("missing rename source")
			}
			i++
			entry.OrigPath = string(records[i])
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

func IsDirty(entry domain.StatusEntry) bool {
	return entry.Index != '?' || entry.Worktree != '?'
}

func IsUntracked(entry domain.StatusEntry) bool {
	return entry.Index == '?' && entry.Worktree == '?'
}
