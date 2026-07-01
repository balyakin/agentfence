package logging

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/agentfence/agentfence/internal/execx"
)

func TestRedactingHandlerRedactsAnyAttr(t *testing.T) {
	t.Parallel()
	secret := "abcdefghijklmnopqrstuvwxyz123456"
	redactor := execx.NewRedactor()
	redactor.RegisterSecret(secret)
	var buf bytes.Buffer
	logger := slog.New(NewRedactingHandler(slog.NewTextHandler(&buf, nil), redactor))
	logger.Error("failed", slog.Any("error", errors.New("token="+secret)))
	if strings.Contains(buf.String(), secret) {
		t.Fatalf("secret leaked in any attr: %q", buf.String())
	}
}
