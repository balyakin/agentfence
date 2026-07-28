package scanner

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/errorsx"
	"github.com/agentfence/agentfence/internal/execx"
	"github.com/agentfence/agentfence/internal/ports"
)

//go:embed gitleaks-severity.json trusted-gitleaks.toml
var severityFS embed.FS

const (
	gitleaksExitClean    = 0
	gitleaksExitFindings = 42
	maxScannerReportSize = 50 << 20
)

type Gitleaks struct {
	runner *execx.ProcessRunner
	clock  ports.Clock
}

var _ ports.Scanner = (*Gitleaks)(nil)

func NewGitleaks(runner *execx.ProcessRunner, clock ports.Clock) *Gitleaks {
	if runner == nil {
		runner = execx.NewProcessRunner(nil)
	}
	if clock == nil {
		clock = ports.SystemClock{}
	}
	return &Gitleaks{runner: runner, clock: clock}
}

func (g *Gitleaks) Name() string {
	return "gitleaks"
}

func (g *Gitleaks) Scan(ctx context.Context, req domain.ScanRequest) (domain.ScanResult, error) {
	if _, err := exec.LookPath("gitleaks"); err != nil {
		return domain.ScanResult{}, errorsx.Wrap(errorsx.CodeScannerNotFound, "gitleaks not found", errorsx.ExitDependencyMissing, err)
	}
	scannerDir := filepath.Join(req.RunDir, "scanner")
	if err := os.MkdirAll(scannerDir, 0o700); err != nil {
		return domain.ScanResult{}, fmt.Errorf("create scanner directory: %w", err)
	}
	report, err := os.CreateTemp(scannerDir, "gitleaks-*.json")
	if err != nil {
		return domain.ScanResult{}, fmt.Errorf("create gitleaks report: %w", err)
	}
	reportPath := report.Name()
	cleanupRaw := true
	defer func() {
		if cleanupRaw {
			_ = os.Remove(reportPath)
		}
	}()
	if err := report.Close(); err != nil {
		return domain.ScanResult{}, fmt.Errorf("close gitleaks report: %w", err)
	}
	if err := os.Remove(reportPath); err != nil {
		return domain.ScanResult{}, fmt.Errorf("remove empty gitleaks report: %w", err)
	}
	configData, err := severityFS.ReadFile("trusted-gitleaks.toml")
	if err != nil {
		return domain.ScanResult{}, fmt.Errorf("read trusted gitleaks config: %w", err)
	}
	configPath := filepath.Join(scannerDir, "trusted-gitleaks.toml")
	if err := os.WriteFile(configPath, configData, 0o600); err != nil {
		return domain.ScanResult{}, fmt.Errorf("write trusted gitleaks config: %w", err)
	}
	defer func() {
		_ = os.Remove(configPath)
	}()
	args := []string{
		"dir",
		"--config", configPath,
		"--exit-code", fmt.Sprint(gitleaksExitFindings),
		"--report-format", "json",
		"--report-path", reportPath,
		req.TargetPath,
	}
	scanCtx := ctx
	cancel := func() {}
	if req.TimeoutSeconds > 0 {
		scanCtx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutSeconds)*time.Second)
	}
	defer cancel()
	result, runErr := g.runner.Run(scanCtx, execx.ProcessRequest{
		Executable: "gitleaks",
		Args:       args,
		Env:        trustedScannerEnv(),
	})
	if runErr != nil {
		return domain.ScanResult{}, fmt.Errorf("run gitleaks: %w", runErr)
	}
	if result.ExitCode != gitleaksExitClean && result.ExitCode != gitleaksExitFindings {
		return domain.ScanResult{}, fmt.Errorf("gitleaks exited with code %d", result.ExitCode)
	}
	info, statErr := os.Lstat(reportPath)
	if statErr != nil {
		return domain.ScanResult{}, fmt.Errorf("stat gitleaks report: %w", statErr)
	}
	if !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > maxScannerReportSize {
		return domain.ScanResult{}, fmt.Errorf("gitleaks report is missing, invalid, or too large")
	}
	data, readErr := os.ReadFile(reportPath)
	if readErr != nil {
		return domain.ScanResult{}, fmt.Errorf("read gitleaks report: %w", readErr)
	}
	findings, rawSecrets, err := ParseGitleaksReport(data, req.RunID, req.Phase, g.Name(), g.clock.Now())
	if err != nil {
		return domain.ScanResult{}, err
	}
	hasFindings := len(findings) > 0
	if hasFindings != (result.ExitCode == gitleaksExitFindings) {
		return domain.ScanResult{}, fmt.Errorf("gitleaks report does not match exit code %d", result.ExitCode)
	}
	sanitized, err := SanitizeGitleaksReport(data)
	if err != nil {
		return domain.ScanResult{}, err
	}
	if err := os.WriteFile(reportPath, sanitized, 0o600); err != nil {
		return domain.ScanResult{}, fmt.Errorf("write redacted gitleaks report: %w", err)
	}
	cleanupRaw = false
	status := "clean"
	if len(findings) > 0 {
		status = "findings"
	}
	return domain.ScanResult{Findings: findings, RawSecrets: rawSecrets, Status: status}, nil
}

type gitleaksFinding struct {
	Description string `json:"Description"`
	StartLine   int    `json:"StartLine"`
	StartColumn int    `json:"StartColumn"`
	RuleID      string `json:"RuleID"`
	File        string `json:"File"`
	Secret      string `json:"Secret"`
	Match       string `json:"Match"`
	Fingerprint string `json:"Fingerprint"`
}

func ParseGitleaksReport(data []byte, runID string, phase domain.FindingPhase, engine string, now time.Time) ([]domain.Finding, []string, error) {
	severityMap, err := loadSeverityMap()
	if err != nil {
		return nil, nil, err
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return nil, nil, nil
	}
	var raw []gitleaksFinding
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return nil, nil, fmt.Errorf("parse gitleaks report json: %w", err)
	}
	findings := make([]domain.Finding, 0, len(raw))
	rawSecrets := make([]string, 0, len(raw))
	for _, item := range raw {
		secret := item.Secret
		if secret == "" {
			secret = item.Match
		}
		if secret != "" {
			rawSecrets = append(rawSecrets, secret)
		}
		severity := severityMap[item.RuleID]
		if severity == "" {
			severity = domain.SeverityHigh
		}
		findings = append(findings, domain.Finding{
			RunID:          runID,
			Phase:          phase,
			Engine:         engine,
			FilePath:       item.File,
			Line:           item.StartLine,
			ColumnNumber:   item.StartColumn,
			RuleID:         item.RuleID,
			Severity:       severity,
			Fingerprint:    item.Fingerprint,
			SecretSHA256:   domain.SecretSHA256(secret),
			RedactedSecret: domain.RedactSecret(secret),
			Description:    item.Description,
			CreatedAt:      now,
		})
	}
	return findings, rawSecrets, nil
}

func trustedScannerEnv() []string {
	return []string{
		"HOME=",
		"LC_ALL=C",
		"PATH=" + os.Getenv("PATH"),
	}
}

func SanitizeGitleaksReport(data []byte) ([]byte, error) {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return []byte("[]\n"), nil
	}
	var records []map[string]any
	if err := json.Unmarshal([]byte(trimmed), &records); err != nil {
		return nil, fmt.Errorf("parse gitleaks report for redaction: %w", err)
	}
	for _, record := range records {
		for _, key := range []string{"Secret", "Match"} {
			if value, ok := record[key].(string); ok && value != "" {
				record[key] = "[REDACTED]"
			}
		}
	}
	sanitized, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal redacted gitleaks report: %w", err)
	}
	return append(sanitized, '\n'), nil
}

func loadSeverityMap() (map[string]string, error) {
	data, err := severityFS.ReadFile("gitleaks-severity.json")
	if err != nil {
		return nil, fmt.Errorf("read gitleaks severity map: %w", err)
	}
	result := map[string]string{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("parse gitleaks severity map: %w", err)
	}
	return result, nil
}
