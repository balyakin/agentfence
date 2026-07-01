package ports

import (
	"context"

	"github.com/agentfence/agentfence/internal/config"
	"github.com/agentfence/agentfence/internal/domain"
)

type AgentRegistry interface {
	BuildInvocation(ctx context.Context, agentName string, task string, agentArgs []string, override *config.AgentAdapterConfig) (domain.Invocation, error)
}
