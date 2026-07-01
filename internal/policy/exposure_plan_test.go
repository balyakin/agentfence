package policy

import (
	"context"
	"testing"

	"github.com/agentfence/agentfence/internal/config"
	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/testutil"
)

func TestExposurePlanExcludesEnv(t *testing.T) {
	t.Parallel()
	git := &testutil.FakeGit{RepoRoot: "/repo"}
	planner := NewPlanner(git)
	cfg := config.DefaultConfig()
	plan, err := planner.BuildExposurePlan(context.Background(), domain.ExposurePlanRequest{RepoPath: "/repo", Config: cfg})
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if len(plan.Files) != 1 || plan.Files[0].RelativePath != "main.go" {
		t.Fatalf("unexpected plan: %#v", plan.Files)
	}
}
