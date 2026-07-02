package testutil

import (
	"context"

	"github.com/agentfence/agentfence/internal/config"
	"github.com/agentfence/agentfence/internal/domain"
)

type FakeAgentRegistry struct {
	Called bool
	Env    []string
}

func (a *FakeAgentRegistry) BuildInvocation(ctx context.Context, agentName string, task string, agentArgs []string, override *config.AgentAdapterConfig) (domain.Invocation, error) {
	a.Called = true
	return domain.Invocation{Executable: "/bin/true", Env: append([]string{}, a.Env...)}, nil
}
