package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
)

type FindingPhase string

const (
	FindingPhasePreflight  FindingPhase = "preflight"
	FindingPhasePostflight FindingPhase = "postflight"
	FindingPhasePatch      FindingPhase = "patch"
)

const (
	SeverityCritical = "critical"
	SeverityHigh     = "high"
	SeverityMedium   = "medium"
	SeverityLow      = "low"
	SeverityInfo     = "info"
)

type Finding struct {
	RunID          string
	Phase          FindingPhase
	Engine         string
	FilePath       string
	Line           int
	ColumnNumber   int
	RuleID         string
	Severity       string
	Fingerprint    string
	SecretSHA256   string
	RedactedSecret string
	Description    string
	CreatedAt      time.Time
}

type ScanResult struct {
	Findings   []Finding
	RawSecrets []string `json:"-"`
	Status     string
	Blocked    bool
}

type ScanRequest struct {
	RunID          string
	Phase          FindingPhase
	TargetPath     string
	RunDir         string
	TimeoutSeconds int
}

func RedactSecret(secret string) string {
	if len(secret) <= 16 {
		return "****"
	}
	return "..." + secret[len(secret)-4:]
}

func SecretSHA256(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func IsBlockingSeverity(severity string, blocklist []string) bool {
	normalized := strings.ToLower(strings.TrimSpace(severity))
	for _, candidate := range blocklist {
		if normalized == strings.ToLower(strings.TrimSpace(candidate)) {
			return true
		}
	}
	return false
}

func FindingsBlocked(findings []Finding, blocklist []string) bool {
	for _, finding := range findings {
		if IsBlockingSeverity(finding.Severity, blocklist) {
			return true
		}
	}
	return false
}
