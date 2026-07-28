package execx

import (
	"errors"
	"fmt"
	"io"
)

const (
	maxRedactingBufferBytes = 1 << 20
	maxRedactedInputBytes   = 10 << 20
)

var ErrOutputLimit = errors.New("process output limit exceeded")

type RedactingWriter struct {
	dst        io.Writer
	redactor   *Redactor
	buffer     []byte
	inputBytes int64
}

func NewRedactingWriter(dst io.Writer, redactor *Redactor) *RedactingWriter {
	if redactor == nil {
		redactor = NewRedactor()
	}
	return &RedactingWriter{dst: dst, redactor: redactor}
}

func (w *RedactingWriter) Write(data []byte) (int, error) {
	if w.inputBytes+int64(len(data)) > maxRedactedInputBytes {
		return 0, ErrOutputLimit
	}
	w.inputBytes += int64(len(data))
	w.buffer = append(w.buffer, data...)
	tail := w.redactor.TailWindow()
	if len(w.buffer) <= tail {
		return len(data), nil
	}
	flushLen := safeFlushLen(w.buffer, tail)
	flushLen = w.redactor.ProtectFlushBoundary(w.buffer, flushLen)
	if flushLen == 0 {
		if len(w.buffer) > maxRedactingBufferBytes {
			return len(data), ErrOutputLimit
		}
		return len(data), nil
	}
	chunk := append([]byte{}, w.buffer[:flushLen]...)
	redacted := w.redactor.RedactBytes(chunk)
	n, err := w.dst.Write(redacted)
	if err != nil {
		return len(data), fmt.Errorf("write redacted data: %w", err)
	}
	if n != len(redacted) {
		return len(data), fmt.Errorf("write redacted data: %w", io.ErrShortWrite)
	}
	w.buffer = append(w.buffer[:0], w.buffer[flushLen:]...)
	return len(data), nil
}

func safeFlushLen(buffer []byte, tail int) int {
	flushLen := len(buffer) - tail
	for flushLen > 0 && flushLen < len(buffer) && isTokenByte(buffer[flushLen-1]) && isTokenByte(buffer[flushLen]) {
		flushLen--
	}
	return flushLen
}

func isTokenByte(value byte) bool {
	return value >= 'A' && value <= 'Z' ||
		value >= 'a' && value <= 'z' ||
		value >= '0' && value <= '9' ||
		value == '+' ||
		value == '/' ||
		value == '=' ||
		value == '_' ||
		value == '-'
}

func (w *RedactingWriter) Flush() error {
	if len(w.buffer) == 0 {
		return nil
	}
	redacted := w.redactor.RedactBytes(append([]byte{}, w.buffer...))
	n, err := w.dst.Write(redacted)
	if err != nil {
		return fmt.Errorf("flush redacted data: %w", err)
	}
	if n != len(redacted) {
		return fmt.Errorf("flush redacted data: %w", io.ErrShortWrite)
	}
	w.buffer = w.buffer[:0]
	return nil
}

func (w *RedactingWriter) Close() error {
	return w.Flush()
}
