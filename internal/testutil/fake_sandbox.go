package testutil

import (
	"context"

	"github.com/agentfence/agentfence/internal/domain"
)

type FakeSandbox struct {
	Result  domain.SandboxRunResult
	Called  bool
	Request domain.SandboxRunRequest
}

func (s *FakeSandbox) Run(ctx context.Context, req domain.SandboxRunRequest) (domain.SandboxRunResult, error) {
	s.Called = true
	s.Request = req
	return s.Result, nil
}
