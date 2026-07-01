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

func FuzzGitleaksParse(f *testing.F) {
	f.Add("[]")
	f.Fuzz(func(t *testing.T, input string) {
		_, _, _ = ParseGitleaksReport([]byte(input), "run", domain.FindingPhasePatch, "gitleaks", time.Now())
	})
}
