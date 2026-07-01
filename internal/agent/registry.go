package agent

import (
	"context"
	"fmt"

	"github.com/agentfence/agentfence/internal/config"
	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/ports"
)

type Registry struct {
	cfg     config.AgentConfig
	adapter *GenericAdapter
}

var _ ports.AgentRegistry = (*Registry)(nil)

func NewRegistry(cfg config.AgentConfig) *Registry {
	if cfg.Adapters == nil {
		cfg.Adapters = Presets()
	}
	return &Registry{cfg: cfg, adapter: NewGenericAdapter(cfg.EnvAllowlist)}
}

func (r *Registry) WithConfig(cfg config.AgentConfig) ports.AgentRegistry {
	return NewRegistry(cfg)
}

func (r *Registry) BuildInvocation(ctx context.Context, agentName string, task string, agentArgs []string, override *config.AgentAdapterConfig) (domain.Invocation, error) {
	if agentName == "" {
		agentName = r.cfg.Default
	}
	var adapterConfig config.AgentAdapterConfig
	if override != nil {
		adapterConfig = *override
	} else {
		var ok bool
		adapterConfig, ok = r.cfg.Adapters[agentName]
		if !ok {
			return domain.Invocation{}, fmt.Errorf("agent adapter %q not configured", agentName)
		}
	}
	return r.adapter.Build(ctx, adapterConfig, task, agentArgs)
}
