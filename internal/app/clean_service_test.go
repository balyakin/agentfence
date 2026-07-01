package app

import (
	"testing"

	"github.com/agentfence/agentfence/internal/config"
	"github.com/agentfence/agentfence/internal/errorsx"
	"github.com/agentfence/agentfence/internal/ports"
	"github.com/agentfence/agentfence/internal/state"
)

func TestCleanServiceConstructor(t *testing.T) {
	t.Parallel()
	if _, err := NewCleanService(nil, state.Paths{}, config.DefaultConfig().Retention, ports.SystemClock{}); err == nil {
		t.Fatalf("expected missing dependency error")
	} else if public, ok := errorsx.IsPublic(err); !ok || public.Code != errorsx.CodeInternal {
		t.Fatalf("unexpected error: %v", err)
	}
}
