package execx

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestRedactorNoPrefix(t *testing.T) {
	t.Parallel()
	secret := "abcdefghijklmnopqrstuvwxyz123456"
	redactor := NewRedactor()
	redactor.RegisterSecret(secret)
	out := redactor.RedactString("token=" + secret)
	if strings.Contains(out, "abcd") {
		t.Fatalf("secret prefix leaked: %q", out)
	}
}

func TestRedactingWriterSplitSecret(t *testing.T) {
	t.Parallel()
	secret := "abcdefghijklmnopqrstuvwxyz123456"
	redactor := NewRedactor()
	redactor.RegisterSecret(secret)
	var buf bytes.Buffer
	writer := NewRedactingWriter(&buf, redactor)
	if _, err := writer.Write([]byte(secret[:10])); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := writer.Write([]byte(secret[10:])); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if strings.Contains(buf.String(), secret) {
		t.Fatalf("split secret leaked")
	}
}

func TestRedactorRedactsHexToken(t *testing.T) {
	t.Parallel()
	token := strings.Repeat("0123456789abcdef", 4)
	redacted := NewRedactor().RedactString("token=" + token)
	if strings.Contains(redacted, token) {
		t.Fatalf("hex token leaked: %q", redacted)
	}
}

func TestRedactingWriterDoesNotSplitLongRegexToken(t *testing.T) {
	t.Parallel()
	token := strings.Repeat("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/_-", 4)
	var buf bytes.Buffer
	writer := NewRedactingWriter(&buf, NewRedactor())
	for _, char := range []byte(token) {
		if _, err := writer.Write([]byte{char}); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if buf.Len() != 0 {
		t.Fatalf("token flushed before boundary: %q", buf.String())
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if strings.Contains(buf.String(), token) {
		t.Fatalf("long regex token leaked")
	}
}

func TestRedactingWriterRetainsBufferAfterWriteError(t *testing.T) {
	t.Parallel()
	dst := &flakyWriter{fail: true}
	writer := NewRedactingWriter(dst, NewRedactor())
	payload := []byte(strings.Repeat("x ", 100))
	n, err := writer.Write(payload)
	if err == nil {
		t.Fatalf("expected write error")
	}
	if n != len(payload) {
		t.Fatalf("write returned n=%d, want %d", n, len(payload))
	}
	dst.fail = false
	if err := writer.Flush(); err != nil {
		t.Fatalf("flush after recovered writer: %v", err)
	}
	if got := dst.buf.String(); got != string(payload) {
		t.Fatalf("buffer was not retained, got %q", got)
	}
}

type flakyWriter struct {
	fail bool
	buf  bytes.Buffer
}

func (w *flakyWriter) Write(p []byte) (int, error) {
	if w.fail {
		return 0, errors.New("write failed")
	}
	return w.buf.Write(p)
}

func FuzzRedactor(f *testing.F) {
	f.Add("AKIA1234567890ABCDEF")
	f.Fuzz(func(t *testing.T, input string) {
		_ = NewRedactor().RedactString(input)
	})
}
