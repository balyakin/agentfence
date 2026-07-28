package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/agentfence/agentfence/internal/errorsx"
)

func TestWritePublicErrorAndReturnPreservesFailure(t *testing.T) {
	t.Parallel()
	runErr := errorsx.Wrap(errorsx.CodeRepoDirty, "repository is dirty", errorsx.ExitUsage, errors.New("dirty"))
	var output bytes.Buffer
	err := writePublicErrorAndReturn(&output, runErr)
	if !errors.Is(err, runErr) {
		t.Fatalf("original error was lost: %v", err)
	}
	if !strings.Contains(output.String(), errorsx.CodeRepoDirty) {
		t.Fatalf("json error missing: %s", output.String())
	}
}
