package app

import (
	"testing"

	"github.com/agentfence/agentfence/internal/errorsx"
)

func TestScanServiceConstructor(t *testing.T) {
	t.Parallel()
	if _, err := NewScanService(ScanServiceDeps{}); err == nil {
		t.Fatalf("expected missing dependency error")
	} else if public, ok := errorsx.IsPublic(err); !ok || public.Code != errorsx.CodeInternal {
		t.Fatalf("unexpected error: %v", err)
	}
}
