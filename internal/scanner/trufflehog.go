package scanner

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/errorsx"
	"github.com/agentfence/agentfence/internal/execx"
	"github.com/agentfence/agentfence/internal/ports"
)

const (
	truffleHogExitClean    = 0
	truffleHogExitFindings = 1
)

type TruffleHog struct {
	runner *execx.ProcessRunner
	clock  ports.Clock
}

var _ ports.Scanner = (*TruffleHog)(nil)

func NewTruffleHog() *TruffleHog {
	return &TruffleHog{runner: execx.NewProcessRunner(nil), clock: ports.SystemClock{}}
}

func (t *TruffleHog) Name() string {
	return "trufflehog"
}

func (t *TruffleHog) Scan(ctx context.Context, req domain.ScanRequest) (domain.ScanResult, error) {
	if _, err := exec.LookPath("trufflehog"); err != nil {
		return domain.ScanResult{}, errorsx.Wrap(errorsx.CodeScannerNotFound, "trufflehog not found", errorsx.ExitDependencyMissing, err)
	}
	scannerDir := filepath.Join(req.RunDir, "scanner")
	if err := os.MkdirAll(scannerDir, 0o700); err != nil {
		return domain.ScanResult{}, fmt.Errorf("create scanner directory: %w", err)
	}
	report, err := os.CreateTemp(scannerDir, "trufflehog-*.json")
	if err != nil {
		return domain.ScanResult{}, fmt.Errorf("create trufflehog report: %w", err)
	}
	reportPath := report.Name()
	cleanupRaw := true
	defer func() {
		if cleanupRaw {
			_ = os.Remove(reportPath)
		}
	}()
	args := []string{"filesystem", "--json", "--no-update", req.TargetPath}
	scanCtx := ctx
	cancel := func() {}
	if req.TimeoutSeconds > 0 {
		scanCtx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutSeconds)*time.Second)
	}
	defer cancel()
	result, runErr := t.runner.Run(scanCtx, execx.ProcessRequest{Executable: "trufflehog", Args: args, Stdout: report})
	closeErr := report.Close()
	if runErr != nil {
		return domain.ScanResult{}, fmt.Errorf("run trufflehog: %w", runErr)
	}
	if result.ExitCode != truffleHogExitClean && result.ExitCode != truffleHogExitFindings {
		return domain.ScanResult{}, fmt.Errorf("trufflehog exited with code %d", result.ExitCode)
	}
	if closeErr != nil {
		return domain.ScanResult{}, fmt.Errorf("close trufflehog report: %w", closeErr)
	}
	data, err := os.ReadFile(reportPath)
	if err != nil {
		return domain.ScanResult{}, fmt.Errorf("read trufflehog report: %w", err)
	}
	findings, rawSecrets, sanitized, err := parseTruffleHogReport(data, req.RunID, req.Phase, t.clock.Now())
	if err != nil {
		return domain.ScanResult{}, err
	}
	if err := os.WriteFile(reportPath, sanitized, 0o600); err != nil {
		return domain.ScanResult{}, fmt.Errorf("write redacted trufflehog report: %w", err)
	}
	cleanupRaw = false
	status := "clean"
	if len(findings) > 0 {
		status = "findings"
	}
	return domain.ScanResult{Findings: findings, RawSecrets: rawSecrets, Status: status}, nil
}

type truffleHogFinding struct {
	SourceName   string `json:"SourceName"`
	DetectorName string `json:"DetectorName"`
	DetectorType string `json:"DetectorType"`
	Raw          string `json:"Raw"`
	RawV2        string `json:"RawV2"`
	Redacted     string `json:"Redacted"`
	Verified     bool   `json:"Verified"`
}

func parseTruffleHogReport(data []byte, runID string, phase domain.FindingPhase, now time.Time) ([]domain.Finding, []string, []byte, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil, []byte{}, nil
	}
	scan := bufio.NewScanner(bytes.NewReader(data))
	scan.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	var findings []domain.Finding
	var rawSecrets []string
	var sanitized bytes.Buffer
	for scan.Scan() {
		line := bytes.TrimSpace(scan.Bytes())
		if len(line) == 0 {
			continue
		}
		var item truffleHogFinding
		if err := json.Unmarshal(line, &item); err != nil {
			return nil, nil, nil, fmt.Errorf("parse trufflehog json line: %w", err)
		}
		secret := item.Raw
		if secret == "" {
			secret = item.RawV2
		}
		if secret != "" {
			rawSecrets = append(rawSecrets, secret)
		}
		severity := domain.SeverityHigh
		if item.Verified {
			severity = domain.SeverityCritical
		}
		findings = append(findings, domain.Finding{
			RunID:          runID,
			Phase:          phase,
			Engine:         "trufflehog",
			FilePath:       item.SourceName,
			RuleID:         firstNonEmpty(item.DetectorName, item.DetectorType, "trufflehog"),
			Severity:       severity,
			Fingerprint:    domain.SecretSHA256(item.SourceName + "\x00" + item.DetectorName + "\x00" + item.Redacted),
			SecretSHA256:   domain.SecretSHA256(secret),
			RedactedSecret: domain.RedactSecret(secret),
			Description:    firstNonEmpty(item.DetectorName, item.DetectorType, "TruffleHog finding"),
			CreatedAt:      now,
		})
		item.Raw = "[REDACTED]"
		item.RawV2 = ""
		encoded, err := json.Marshal(item)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("marshal redacted trufflehog line: %w", err)
		}
		sanitized.Write(encoded)
		sanitized.WriteByte('\n')
	}
	if err := scan.Err(); err != nil {
		return nil, nil, nil, fmt.Errorf("read trufflehog report: %w", err)
	}
	return findings, rawSecrets, sanitized.Bytes(), nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
