package sandbox

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/agentfence/agentfence/internal/config"
	"github.com/agentfence/agentfence/internal/domain"
	"github.com/agentfence/agentfence/internal/errorsx"
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
	if !hasArgPair(argv, "--size", strconv.FormatInt(sandboxTmpfsBytes, 10)) {
		t.Fatalf("tmpfs size limit missing: %#v", argv)
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

func TestBuildBubblewrapArgvRejectsUnknownNetwork(t *testing.T) {
	t.Parallel()
	_, err := BuildBubblewrapArgv(domain.SandboxRunRequest{
		ShadowPath:  "/tmp/shadow",
		NetworkMode: "denny",
		Invocation:  domain.Invocation{Executable: "/usr/bin/true"},
	})
	if err == nil {
		t.Fatalf("unknown network mode was accepted")
	}
}

func TestBuildBubblewrapArgvAuthMountIsReadonlyByDefault(t *testing.T) {
	hostPath := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(hostPath, []byte("token"), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}
	argv, err := BuildBubblewrapArgv(domain.SandboxRunRequest{
		ShadowPath: "/tmp/shadow",
		Invocation: domain.Invocation{Executable: "/usr/bin/true"},
		AuthMounts: []config.AuthMount{
			{HostPath: hostPath, SandboxPath: "/agent-home/token"},
		},
	})
	if err != nil {
		t.Fatalf("argv: %v", err)
	}
	if !hasArgPair(argv, "--ro-bind", hostPath) {
		t.Fatalf("auth mount was not readonly: %#v", argv)
	}
}

func TestBuildBubblewrapArgvRejectsBroadAuthMount(t *testing.T) {
	t.Parallel()
	_, err := BuildBubblewrapArgv(domain.SandboxRunRequest{
		ShadowPath: "/tmp/shadow",
		Invocation: domain.Invocation{Executable: "/usr/bin/true"},
		AuthMounts: []config.AuthMount{
			{HostPath: "/", SandboxPath: "/agent-home/host"},
		},
	})
	if err == nil {
		t.Fatalf("broad auth mount was accepted")
	}
}

func TestBuildBubblewrapArgvValidatesRequestAndMounts(t *testing.T) {
	t.Parallel()
	if _, err := BuildBubblewrapArgv(domain.SandboxRunRequest{}); err == nil {
		t.Fatal("incomplete request was accepted")
	}
	hostPath := filepath.Join(t.TempDir(), "auth")
	if err := os.WriteFile(hostPath, []byte("auth"), 0o600); err != nil {
		t.Fatalf("write auth file: %v", err)
	}
	argv, err := BuildBubblewrapArgv(domain.SandboxRunRequest{
		ShadowPath: "/tmp/shadow",
		Invocation: domain.Invocation{
			Executable: "/usr/bin/true",
			Args:       []string{"argument"},
			Env:        []string{"VALID=value", "INVALID"},
		},
		AuthMounts: []config.AuthMount{{
			HostPath: hostPath, SandboxPath: "/agent-home/auth", Writable: true,
		}},
	})
	if err != nil {
		t.Fatalf("build argv: %v", err)
	}
	if !hasArgPair(argv, "--bind", hostPath) || !hasArgPair(argv, "--setenv", "VALID") {
		t.Fatalf("configured mount or env missing: %#v", argv)
	}
	for _, mount := range []config.AuthMount{
		{HostPath: "relative", SandboxPath: "/agent-home/auth"},
		{HostPath: filepath.Join(t.TempDir(), "missing"), SandboxPath: "/agent-home/auth"},
		{HostPath: hostPath, SandboxPath: "/etc/auth"},
	} {
		request := domain.SandboxRunRequest{
			ShadowPath: "/tmp/shadow",
			Invocation: domain.Invocation{Executable: "/usr/bin/true"},
			AuthMounts: []config.AuthMount{mount},
		}
		if _, err := BuildBubblewrapArgv(request); err == nil {
			t.Fatalf("unsafe mount accepted: %#v", mount)
		}
	}
}

func TestSandboxFactoryAndSoftRun(t *testing.T) {
	t.Parallel()
	if _, err := New(config.SandboxConfig{Mode: "unknown"}, nil); err == nil {
		t.Fatal("unknown sandbox mode was accepted")
	}
	sandbox, err := New(config.SandboxConfig{Mode: "soft"}, nil)
	if err != nil {
		t.Fatalf("new soft sandbox: %v", err)
	}
	request := domain.SandboxRunRequest{
		ShadowPath: t.TempDir(),
		Invocation: domain.Invocation{
			Executable: "/bin/sh",
			Args:       []string{"-c", "printf output"},
		},
	}
	if _, err := sandbox.Run(context.Background(), request); err == nil {
		t.Fatal("soft mode ran without explicit opt-in")
	}
	var output bytes.Buffer
	request.AllowSoftMode = true
	request.Stdout = &output
	result, err := sandbox.Run(context.Background(), request)
	if err != nil {
		t.Fatalf("soft run: %v", err)
	}
	if result.ExitCode != 0 || result.IsolationLevel != "soft" || output.String() != "output" {
		t.Fatalf("unexpected soft result: %#v output=%q", result, output.String())
	}
}

func TestBubblewrapReportsUnsupportedPlatform(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "linux" {
		t.Skip("non-Linux behavior")
	}
	_, err := NewBubblewrap(nil).Run(context.Background(), domain.SandboxRunRequest{})
	public, ok := errorsx.IsPublic(err)
	if !ok || public.Code != errorsx.CodeUnsupportedSandbox {
		t.Fatalf("unexpected bubblewrap error: %v", err)
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
