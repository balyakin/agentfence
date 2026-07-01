package policy

import "testing"

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
