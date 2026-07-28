package scanner

import (
	"context"
	"errors"
	"testing"

	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/ports"
)

func TestScanAllCombinesAndBlocksFindings(t *testing.T) {
	t.Parallel()
	engine := &scannerStub{
		name: "stub",
		result: domain.ScanResult{
			Findings:   []domain.Finding{{Severity: domain.SeverityHigh}},
			RawSecrets: []string{"registered-value"},
		},
	}
	result, err := ScanAll(
		context.Background(),
		[]ports.Scanner{engine},
		domain.ScanRequest{},
		[]string{domain.SeverityHigh},
		false,
	)
	if err != nil {
		t.Fatalf("scan all: %v", err)
	}
	if !result.Blocked || result.Status != "blocked" || len(result.RawSecrets) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestScanAllFailOnAnyFinding(t *testing.T) {
	t.Parallel()
	result, err := ScanAll(
		context.Background(),
		[]ports.Scanner{&scannerStub{
			name: "stub",
			result: domain.ScanResult{
				Findings: []domain.Finding{{Severity: domain.SeverityInfo}},
			},
		}},
		domain.ScanRequest{},
		[]string{domain.SeverityHigh},
		true,
	)
	if err != nil {
		t.Fatalf("scan all: %v", err)
	}
	if !result.Blocked {
		t.Fatalf("fail_on_findings did not block")
	}
}

func TestScanAllWrapsEngineError(t *testing.T) {
	t.Parallel()
	_, err := ScanAll(
		context.Background(),
		[]ports.Scanner{&scannerStub{name: "stub", err: errors.New("failed")}},
		domain.ScanRequest{},
		[]string{domain.SeverityHigh},
		false,
	)
	if err == nil {
		t.Fatalf("scanner error was ignored")
	}
}

type scannerStub struct {
	name   string
	result domain.ScanResult
	err    error
}

func (s *scannerStub) Name() string {
	return s.name
}

func (s *scannerStub) Scan(ctx context.Context, req domain.ScanRequest) (domain.ScanResult, error) {
	return s.result, s.err
}
