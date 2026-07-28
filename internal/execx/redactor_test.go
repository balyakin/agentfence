package execx

import (
	"bytes"
	"errors"
	"io"
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

func TestRedactorIgnoresEmptyAndDuplicatePatterns(t *testing.T) {
	t.Parallel()
	redactor := NewRedactor()
	redactor.RegisterSecret("")
	redactor.RegisterSecret("registered-value")
	redactor.RegisterSecret("registered-value")
	if output := redactor.RedactString("registered-value"); output != redaction {
		t.Fatalf("output=%q", output)
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

func TestRedactingWriterSplitPunctuatedSecret(t *testing.T) {
	t.Parallel()
	pattern := "registered:" + strings.Repeat("value.", 8) + "$end"
	for split := 1; split < len(pattern); split++ {
		var buf bytes.Buffer
		redactor := NewRedactor()
		redactor.RegisterSecret(pattern)
		writer := NewRedactingWriter(&buf, redactor)
		if _, err := writer.Write([]byte("prefix:" + pattern[:split])); err != nil {
			t.Fatalf("write prefix at %d: %v", split, err)
		}
		if _, err := writer.Write([]byte(pattern[split:] + ":suffix")); err != nil {
			t.Fatalf("write suffix at %d: %v", split, err)
		}
		if err := writer.Flush(); err != nil {
			t.Fatalf("flush at %d: %v", split, err)
		}
		if strings.Contains(buf.String(), pattern) {
			t.Fatalf("punctuated secret leaked at split %d", split)
		}
	}
}

func TestRedactingWriterLimitsLongToken(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	writer := NewRedactingWriter(&buf, NewRedactor())
	payload := []byte(strings.Repeat("x", maxRedactingBufferBytes+1))
	_, err := writer.Write(payload)
	if !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("expected output limit, got %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("oversized token was written")
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

func TestRedactingWriterCloseFlushesWithDefaultRedactor(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	writer := NewRedactingWriter(&buf, nil)
	if _, err := writer.Write([]byte("plain output")); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if buf.String() != "plain output" {
		t.Fatalf("output=%q", buf.String())
	}
}

func TestRedactingWriterRejectsTotalInputLimit(t *testing.T) {
	t.Parallel()
	writer := NewRedactingWriter(&bytes.Buffer{}, NewRedactor())
	writer.inputBytes = maxRedactedInputBytes
	_, err := writer.Write([]byte("x"))
	if !errors.Is(err, ErrOutputLimit) {
		t.Fatalf("error = %v, want output limit", err)
	}
}

func TestRedactingWriterReportsShortWrites(t *testing.T) {
	t.Parallel()
	writer := NewRedactingWriter(shortWriter{}, NewRedactor())
	if _, err := writer.Write([]byte(strings.Repeat("word ", 40))); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("write error = %v, want short write", err)
	}
	writer = NewRedactingWriter(shortWriter{}, NewRedactor())
	if _, err := writer.Write([]byte("buffered")); err != nil {
		t.Fatalf("buffer input: %v", err)
	}
	if err := writer.Flush(); !errors.Is(err, io.ErrShortWrite) {
		t.Fatalf("flush error = %v, want short write", err)
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

type shortWriter struct{}

func (shortWriter) Write(data []byte) (int, error) {
	return len(data) - 1, nil
}

func FuzzRedactor(f *testing.F) {
	f.Add("AKIA1234567890ABCDEF")
	f.Fuzz(func(t *testing.T, input string) {
		_ = NewRedactor().RedactString(input)
	})
}
