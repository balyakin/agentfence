package execx

import (
	"bytes"
	"math"
	"regexp"
	"strings"
	"sync"
)

const redaction = "[REDACTED]"

type Redactor struct {
	mu      sync.RWMutex
	secrets []string
	regexes []*regexp.Regexp
}

func NewRedactor() *Redactor {
	return &Redactor{
		regexes: []*regexp.Regexp{
			regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
			regexp.MustCompile(`xox[baprs]-[0-9A-Za-z-]{10,}`),
			regexp.MustCompile(`[A-Za-z0-9+/=_-]{32,}`),
			regexp.MustCompile(`[a-fA-F0-9]{32,}`),
		},
	}
}

func (r *Redactor) RegisterSecret(secret string) {
	if secret == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, existing := range r.secrets {
		if existing == secret {
			return
		}
	}
	r.secrets = append(r.secrets, secret)
}

func (r *Redactor) RedactString(input string) string {
	r.mu.RLock()
	secrets := append([]string{}, r.secrets...)
	regexes := append([]*regexp.Regexp{}, r.regexes...)
	r.mu.RUnlock()

	output := input
	for _, secret := range secrets {
		output = strings.ReplaceAll(output, secret, redaction)
	}
	for _, pattern := range regexes {
		output = pattern.ReplaceAllStringFunc(output, func(candidate string) string {
			if looksSensitive(candidate) {
				return redaction
			}
			return candidate
		})
	}
	return output
}

func (r *Redactor) RedactBytes(input []byte) []byte {
	return []byte(r.RedactString(string(input)))
}

func (r *Redactor) TailWindow() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	maxLen := 128
	for _, secret := range r.secrets {
		if len(secret) > maxLen {
			maxLen = len(secret)
		}
	}
	return maxLen
}

func (r *Redactor) ProtectFlushBoundary(buffer []byte, boundary int) int {
	r.mu.RLock()
	secrets := append([]string{}, r.secrets...)
	r.mu.RUnlock()
	for _, secret := range secrets {
		secretBytes := []byte(secret)
		searchStart := 0
		for searchStart < len(buffer) {
			index := bytes.Index(buffer[searchStart:], secretBytes)
			if index < 0 {
				break
			}
			index += searchStart
			if index < boundary && index+len(secretBytes) > boundary {
				boundary = index
			}
			searchStart = index + 1
		}
	}
	return boundary
}

func looksSensitive(candidate string) bool {
	if strings.HasPrefix(candidate, "AKIA") || strings.HasPrefix(candidate, "xox") {
		return true
	}
	if len(candidate) < 32 {
		return false
	}
	if isHex(candidate) {
		return entropy(candidate) >= 3.5
	}
	return entropy(candidate) >= 4.5
}

func isHex(value string) bool {
	for _, char := range value {
		if !strings.ContainsRune("0123456789abcdefABCDEF", char) {
			return false
		}
	}
	return true
}

func entropy(value string) float64 {
	if value == "" {
		return 0
	}
	counts := map[rune]int{}
	for _, r := range value {
		counts[r]++
	}
	total := float64(len([]rune(value)))
	var result float64
	for _, count := range counts {
		p := float64(count) / total
		result -= p * math.Log2(p)
	}
	return result
}
