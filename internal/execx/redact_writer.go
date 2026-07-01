package execx

import (
	"fmt"
	"io"
)

type RedactingWriter struct {
	dst      io.Writer
	redactor *Redactor
	buffer   []byte
}

func NewRedactingWriter(dst io.Writer, redactor *Redactor) *RedactingWriter {
	if redactor == nil {
		redactor = NewRedactor()
	}
	return &RedactingWriter{dst: dst, redactor: redactor}
}

func (w *RedactingWriter) Write(data []byte) (int, error) {
	w.buffer = append(w.buffer, data...)
	tail := w.redactor.TailWindow()
	if len(w.buffer) <= tail {
		return len(data), nil
	}
	flushLen := safeFlushLen(w.buffer, tail)
	if flushLen == 0 {
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
