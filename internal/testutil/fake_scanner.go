package testutil

import (
	"context"

	"github.com/agentfence/agentfence/internal/domain"
)

type FakeScanner struct {
	Result domain.ScanResult
	Calls  int
}

func (s *FakeScanner) Name() string { return "fake" }
func (s *FakeScanner) Scan(ctx context.Context, req domain.ScanRequest) (domain.ScanResult, error) {
	s.Calls++
	result := s.Result
	for i := range result.Findings {
		result.Findings[i].RunID = req.RunID
		result.Findings[i].Phase = req.Phase
	}
	return result, nil
}
