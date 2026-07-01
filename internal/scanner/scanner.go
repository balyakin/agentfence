package scanner

import (
	"context"
	"fmt"

	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/ports"
)

func ScanAll(ctx context.Context, scanners []ports.Scanner, req domain.ScanRequest, blocklist []string) (domain.ScanResult, error) {
	var combined domain.ScanResult
	combined.Status = "clean"
	for _, engine := range scanners {
		result, err := engine.Scan(ctx, req)
		if err != nil {
			return domain.ScanResult{}, fmt.Errorf("scan with %s: %w", engine.Name(), err)
		}
		combined.Findings = append(combined.Findings, result.Findings...)
		combined.RawSecrets = append(combined.RawSecrets, result.RawSecrets...)
	}
	if len(combined.Findings) > 0 {
		combined.Status = "findings"
	}
	if domain.FindingsBlocked(combined.Findings, blocklist) {
		combined.Status = "blocked"
		combined.Blocked = true
	}
	return combined, nil
}
