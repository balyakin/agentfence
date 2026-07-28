package scanner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/execx"
	"github.com/agentfence/agentfence/internal/ports"
)

func TestParseGitleaksReport(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "scanner", "gitleaks-valid.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	findings, rawSecrets, err := ParseGitleaksReport(data, "run", domain.FindingPhasePatch, "gitleaks", time.Now())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(findings) != 1 || findings[0].Phase != domain.FindingPhasePatch {
		t.Fatalf("unexpected findings: %#v", findings)
	}
	if len(rawSecrets) != 1 || rawSecrets[0] == "" {
		t.Fatalf("raw secret missing: %#v", rawSecrets)
	}
	if strings.Contains(findings[0].RedactedSecret, "fixture") {
		t.Fatalf("redacted secret leaked prefix")
	}
}

func TestParseGitleaksInvalidDoesNotLeak(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "scanner", "gitleaks-invalid.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	_, _, err = ParseGitleaksReport(data, "run", domain.FindingPhasePatch, "gitleaks", time.Now())
	if err == nil {
		t.Fatalf("expected error")
	}
	if strings.Contains(err.Error(), "fixture-secret-placeholder") {
		t.Fatalf("raw secret leaked in error")
	}
}

func TestGitleaksScanRejectsUnexpectedExitCodeAndRemovesReport(t *testing.T) {
	binDir := t.TempDir()
	executable := filepath.Join(binDir, "gitleaks")
	script := `#!/bin/sh
report=""
while [ "$#" -gt 0 ]; do
	if [ "$1" = "--report-path" ]; then
		shift
		report="$1"
	fi
	shift
done
printf '[]\n' > "$report"
exit 2
`
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatalf("write gitleaks stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	runDir := t.TempDir()
	scanner := NewGitleaks(execx.NewProcessRunner(nil), ports.SystemClock{})
	_, err := scanner.Scan(context.Background(), domain.ScanRequest{
		RunID: "run", Phase: domain.FindingPhasePatch, TargetPath: t.TempDir(), RunDir: runDir,
	})
	if err == nil {
		t.Fatalf("expected scanner exit error")
	}
	entries, readErr := os.ReadDir(filepath.Join(runDir, "scanner"))
	if readErr != nil {
		t.Fatalf("read scanner dir: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("raw reports left on disk: %#v", entries)
	}
}

func TestGitleaksScanUsesTrustedConfigAndDedicatedFindingsCode(t *testing.T) {
	binDir := t.TempDir()
	executable := filepath.Join(binDir, "gitleaks")
	script := `#!/bin/sh
report=""
config=""
exit_code=""
while [ "$#" -gt 0 ]; do
	case "$1" in
		--report-path)
			shift
			report="$1"
			;;
		--config)
			shift
			config="$1"
			;;
		--exit-code)
			shift
			exit_code="$1"
			;;
	esac
	shift
done
test -s "$config"
test "$exit_code" = "42"
printf '[]\n' > "$report"
`
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatalf("write gitleaks stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	_, err := NewGitleaks(execx.NewProcessRunner(nil), ports.SystemClock{}).Scan(
		context.Background(),
		domain.ScanRequest{
			RunID: "run", Phase: domain.FindingPhasePatch, TargetPath: t.TempDir(), RunDir: t.TempDir(),
		},
	)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
}

func TestGitleaksScanRequiresReport(t *testing.T) {
	binDir := t.TempDir()
	executable := filepath.Join(binDir, "gitleaks")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("write gitleaks stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	_, err := NewGitleaks(nil, nil).Scan(
		context.Background(),
		domain.ScanRequest{
			RunID: "run", Phase: domain.FindingPhasePatch, TargetPath: t.TempDir(), RunDir: t.TempDir(),
		},
	)
	if err == nil {
		t.Fatal("missing scanner report was accepted")
	}
}

func TestGitleaksScanAcceptsFindingsExitCode(t *testing.T) {
	binDir := t.TempDir()
	executable := filepath.Join(binDir, "gitleaks")
	script := `#!/bin/sh
report=""
while [ "$#" -gt 0 ]; do
	if [ "$1" = "--report-path" ]; then
		shift
		report="$1"
	fi
	shift
done
printf '%s\n' '[{"RuleID":"generic-api-key","File":"file.txt","Secret":"value","StartLine":1}]' > "$report"
exit 42
`
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatalf("write gitleaks stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	result, err := NewGitleaks(nil, nil).Scan(
		context.Background(),
		domain.ScanRequest{
			RunID: "run", Phase: domain.FindingPhasePatch, TargetPath: t.TempDir(), RunDir: t.TempDir(),
		},
	)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if result.Status != "findings" || len(result.Findings) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestGitleaksFatalExitOneIsNotClean(t *testing.T) {
	binDir := t.TempDir()
	executable := filepath.Join(binDir, "gitleaks")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatalf("write gitleaks stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	runDir := t.TempDir()
	_, err := NewGitleaks(execx.NewProcessRunner(nil), ports.SystemClock{}).Scan(
		context.Background(),
		domain.ScanRequest{
			RunID: "run", Phase: domain.FindingPhasePatch, TargetPath: t.TempDir(), RunDir: runDir,
		},
	)
	if err == nil {
		t.Fatalf("fatal exit 1 was accepted")
	}
	entries, readErr := os.ReadDir(filepath.Join(runDir, "scanner"))
	if readErr != nil {
		t.Fatalf("read scanner dir: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("scanner artifacts remained after fatal exit: %#v", entries)
	}
}

func TestParseGitleaksUnknownRuleUsesBlockingSeverity(t *testing.T) {
	t.Parallel()
	data := []byte(`[{"RuleID":"new-rule","File":"file.txt","Secret":"secret","StartLine":1}]`)
	findings, _, err := ParseGitleaksReport(
		data,
		"run",
		domain.FindingPhasePatch,
		"gitleaks",
		time.Now(),
	)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(findings) != 1 || findings[0].Severity != domain.SeverityHigh {
		t.Fatalf("unexpected fallback finding: %#v", findings)
	}
}

func TestSanitizeGitleaksReport(t *testing.T) {
	t.Parallel()
	data, err := SanitizeGitleaksReport(nil)
	if err != nil {
		t.Fatalf("sanitize empty report: %v", err)
	}
	if string(data) != "[]\n" {
		t.Fatalf("sanitized empty report = %q", data)
	}
	if _, err := SanitizeGitleaksReport([]byte("{")); err == nil {
		t.Fatal("invalid report was accepted")
	}
}

func TestParseTruffleHogReportReturnsRawSecrets(t *testing.T) {
	t.Parallel()
	data := []byte(`{"SourceName":"file.txt","DetectorName":"generic","Raw":"raw-secret-value","Redacted":"raw-...-value","Verified":true}` + "\n")
	findings, rawSecrets, sanitized, err := parseTruffleHogReport(data, "run", domain.FindingPhasePatch, time.Now())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("unexpected findings: %#v", findings)
	}
	if len(rawSecrets) != 1 || rawSecrets[0] != "raw-secret-value" {
		t.Fatalf("raw secret missing: %#v", rawSecrets)
	}
	if strings.Contains(string(sanitized), "raw-secret-value") {
		t.Fatalf("sanitized report leaked raw secret")
	}
}

func TestScannerNames(t *testing.T) {
	t.Parallel()
	if NewGitleaks(nil, nil).Name() != "gitleaks" {
		t.Fatal("unexpected Gitleaks name")
	}
	if NewTruffleHog().Name() != "trufflehog" {
		t.Fatal("unexpected TruffleHog name")
	}
}

func TestParseTruffleHogReportRejectsInvalidJSON(t *testing.T) {
	t.Parallel()
	_, _, _, err := parseTruffleHogReport(
		[]byte("{"),
		"run",
		domain.FindingPhasePatch,
		time.Now(),
	)
	if err == nil {
		t.Fatal("invalid TruffleHog report was accepted")
	}
}

func TestTruffleHogScanRejectsUnexpectedExitCodeAndRemovesReport(t *testing.T) {
	binDir := t.TempDir()
	executable := filepath.Join(binDir, "trufflehog")
	script := "#!/bin/sh\nprintf '{}\\n'\nexit 2\n"
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatalf("write trufflehog stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	runDir := t.TempDir()
	_, err := NewTruffleHog().Scan(context.Background(), domain.ScanRequest{
		RunID: "run", Phase: domain.FindingPhasePatch, TargetPath: t.TempDir(), RunDir: runDir,
	})
	if err == nil {
		t.Fatalf("expected scanner exit error")
	}
	entries, readErr := os.ReadDir(filepath.Join(runDir, "scanner"))
	if readErr != nil {
		t.Fatalf("read scanner dir: %v", readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("raw reports left on disk: %#v", entries)
	}
}

func TestTruffleHogFatalExitOneIsNotClean(t *testing.T) {
	binDir := t.TempDir()
	executable := filepath.Join(binDir, "trufflehog")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatalf("write trufflehog stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	_, err := NewTruffleHog().Scan(context.Background(), domain.ScanRequest{
		RunID: "run", Phase: domain.FindingPhasePatch, TargetPath: t.TempDir(), RunDir: t.TempDir(),
	})
	if err == nil {
		t.Fatalf("fatal exit 1 was accepted")
	}
}

func TestTruffleHogScanParsesFindingsFromSuccessfulOutput(t *testing.T) {
	binDir := t.TempDir()
	executable := filepath.Join(binDir, "trufflehog")
	script := `#!/bin/sh
printf '%s\n' '{"SourceName":"file.txt","DetectorName":"generic","Raw":"value","Redacted":"v...","Verified":false}'
`
	if err := os.WriteFile(executable, []byte(script), 0o700); err != nil {
		t.Fatalf("write trufflehog stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	result, err := NewTruffleHog().Scan(context.Background(), domain.ScanRequest{
		RunID: "run", Phase: domain.FindingPhasePatch, TargetPath: t.TempDir(), RunDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if result.Status != "findings" || len(result.Findings) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestTruffleHogScanAcceptsEmptySuccessfulOutput(t *testing.T) {
	binDir := t.TempDir()
	executable := filepath.Join(binDir, "trufflehog")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatalf("write trufflehog stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	result, err := NewTruffleHog().Scan(context.Background(), domain.ScanRequest{
		RunID: "run", Phase: domain.FindingPhasePatch, TargetPath: t.TempDir(), RunDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if result.Status != "clean" || len(result.Findings) != 0 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func FuzzGitleaksParse(f *testing.F) {
	f.Add("[]")
	f.Fuzz(func(t *testing.T, input string) {
		_, _, _ = ParseGitleaksReport([]byte(input), "run", domain.FindingPhasePatch, "gitleaks", time.Now())
	})
}
