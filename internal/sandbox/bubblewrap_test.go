package sandbox

import (
	"testing"

	"github.com/agentfence/agentfence/internal/domain"
)

func TestBuildBubblewrapArgv(t *testing.T) {
	t.Parallel()
	argv, err := BuildBubblewrapArgv(domain.SandboxRunRequest{
		ShadowPath:  "/tmp/shadow",
		NetworkMode: "deny",
		Invocation:  domain.Invocation{Executable: "/usr/bin/true"},
	})
	if err != nil {
		t.Fatalf("argv: %v", err)
	}
	clearIndex := indexOf(argv, "--clearenv")
	setenvIndex := indexOf(argv, "--setenv")
	if clearIndex < 0 || setenvIndex < 0 || clearIndex > setenvIndex {
		t.Fatalf("--clearenv must precede --setenv: %#v", argv)
	}
	if indexOf(argv, "--unshare-net") < 0 {
		t.Fatalf("network deny must add --unshare-net")
	}
}

func TestBuildBubblewrapArgvUsesConfiguredIsolation(t *testing.T) {
	t.Parallel()
	argv, err := BuildBubblewrapArgv(domain.SandboxRunRequest{
		ShadowPath:    "/tmp/shadow",
		NetworkMode:   "allow",
		TmpfsPaths:    []string{"/tmp", "/agent-home"},
		WritablePaths: []string{"/workspace", "/cache"},
		Invocation:    domain.Invocation{Executable: "/usr/bin/true"},
	})
	if err != nil {
		t.Fatalf("argv: %v", err)
	}
	if indexOf(argv, "--unshare-all") >= 0 || indexOf(argv, "--unshare-net") >= 0 {
		t.Fatalf("network allow must not unshare network: %#v", argv)
	}
	if !hasArgPair(argv, "--tmpfs", "/cache") {
		t.Fatalf("configured writable path missing: %#v", argv)
	}
}

func indexOf(values []string, want string) int {
	for i, value := range values {
		if value == want {
			return i
		}
	}
	return -1
}

func hasArgPair(values []string, first string, second string) bool {
	for index := 0; index+1 < len(values); index++ {
		if values[index] == first && values[index+1] == second {
			return true
		}
	}
	return false
}
