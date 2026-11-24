package app

import (
	"fmt"
	"io"
	"os"
	"sync"
)

const maxLogSize int64 = 1 << 20 // 1 MiB

// rotatingLogWriter writes logs and rotates the file when it reaches its size limit.
type rotatingLogWriter struct {
	mu       sync.Mutex
	path     string
	file     *os.File
	size     int64
	fallback io.Writer
}

func newRotatingLogWriter(path string) (*rotatingLogWriter, error) {
	w := &rotatingLogWriter{
		path:     path,
		fallback: os.Stderr,
	}

	info, err := os.Stat(path)
	if err == nil && info.Size() >= maxLogSize {
		if err := w.rotate(); err != nil {
			return nil, err
		}

		return w, nil
	} else if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("stat log file: %w", err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open log file: %w", err)
	}

	info, err = file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("stat open log file: %w", err)
	}

	w.file = file
	w.size = info.Size()

	return w, nil
}

func (w *rotatingLogWriter) rotate() error {
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			w.file = nil
			return fmt.Errorf("close log file: %w", err)
		}
		w.file = nil
	}

	backupPath := w.path + ".1"

	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove old log backup: %w", err)
	}

	if err := os.Rename(w.path, backupPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("rotate log file: %w", err)
	}

	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open new log file: %w", err)
	}

	w.file = file
	w.size = 0

	return nil
}

func (w *rotatingLogWriter) useFallback() {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
}

func (w *rotatingLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	written := 0

	for len(p) > 0 {
		if w.file == nil {
			n, err := w.fallback.Write(p)
			return written + n, err
		}

		if w.size >= maxLogSize {
			if err := w.rotate(); err != nil {
				w.useFallback()
				n, fallbackErr := w.fallback.Write(p)
				if fallbackErr != nil {
					return written + n, fallbackErr
				}
				return written + n, nil
			}
		}

		remaining := maxLogSize - w.size
		chunkSize := int64(len(p))
		if chunkSize > remaining {
			chunkSize = remaining
		}

		n, err := w.file.Write(p[:int(chunkSize)])
		w.size += int64(n)
		written += n
		p = p[n:]

		if err != nil {
			return written, err
		}
		if n == 0 {
			return written, fmt.Errorf("log write made no progress")
		}
	}

	return written, nil
}
