package policy

import (
	"testing"

	"github.com/agentfence/agentfence/internal/config"
)

func TestMatcher(t *testing.T) {
	t.Parallel()
	matcher, err := NewMatcher([]string{"**"}, []string{".env", ".env.*", "**/*.pem", "node_modules/**", "vendor/**"})
	if err != nil {
		t.Fatalf("matcher: %v", err)
	}
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{path: ".env", want: false},
		{path: ".env.production", want: false},
		{path: "certs/server.pem", want: false},
		{path: "certs/server.pem.bak", want: true},
		{path: "node_modules/lodash/index.js", want: false},
		{path: "vendor/pkg/main.go", want: false},
	}
	for _, testCase := range tests {
		allowed, err := matcher.Allowed(testCase.path)
		if err != nil {
			t.Fatalf("allowed: %v", err)
		}
		if allowed != testCase.want {
			t.Fatalf("%s allowed=%v want %v", testCase.path, allowed, testCase.want)
		}
	}
}

func TestMatcherValidationAndDefaultInclude(t *testing.T) {
	t.Parallel()
	if _, err := NewMatcher([]string{""}, nil); err == nil {
		t.Fatal("empty pattern was accepted")
	}
	if _, err := NewMatcher([]string{"["}, nil); err == nil {
		t.Fatal("invalid pattern was accepted")
	}
	matcher, err := NewMatcher(nil, nil)
	if err != nil {
		t.Fatalf("default matcher: %v", err)
	}
	allowed, err := matcher.Allowed("src/main.go")
	if err != nil {
		t.Fatalf("match default include: %v", err)
	}
	if !allowed {
		t.Fatal("default matcher rejected a normal path")
	}
}

func TestMandatoryMatcherExcludesNestedCaseVariants(t *testing.T) {
	t.Parallel()
	matcher, err := NewMatcher([]string{"**"}, config.MandatoryExcludes())
	if err != nil {
		t.Fatalf("matcher: %v", err)
	}
	for _, path := range []string{
		"services/api/.ENV",
		"certs/server.PEM",
		"keys/ID_RSA",
		"nested/.GITMODULES",
		"web/NODE_MODULES/pkg/index.js",
		"src/VENDOR/pkg/main.go",
	} {
		allowed, err := matcher.Allowed(path)
		if err != nil {
			t.Fatalf("allowed %s: %v", path, err)
		}
		if allowed {
			t.Fatalf("mandatory secret path was allowed: %s", path)
		}
	}
}

func FuzzDoublestarMatcher(f *testing.F) {
	f.Add("**", "src/main.go")
	f.Fuzz(func(t *testing.T, pattern string, path string) {
		matcher, err := NewMatcher([]string{pattern}, nil)
		if err != nil {
			return
		}
		_, _ = matcher.Allowed(path)
	})
}
