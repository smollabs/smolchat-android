package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRotatingLogWriterCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "smolchat.log")

	w, err := newRotatingLogWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	defer w.file.Close()

	if _, err := w.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello" {
		t.Fatalf("expected hello, got %s", content)
	}
}

func TestRotatingLogWriterRotatesWhenFull(t *testing.T) {
	path := filepath.Join(t.TempDir(), "smolchat.log")

	w, err := newRotatingLogWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if w.file != nil {
			w.file.Close()
		}
	}()

	content := strings.Repeat("a", int(maxLogSize))
	if _, err := w.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte("b")); err != nil {
		t.Fatal(err)
	}

	backup, err := os.ReadFile(path + ".1")
	if err != nil {
		t.Fatal(err)
	}
	if string(backup) != content {
		t.Fatal("backup content does not match original log")
	}

	current, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(current) != "b" {
		t.Fatalf("expected new log to contain b, got %s", current)
	}
}

func TestRotatingLogWriterHandlesLargeWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "smolchat.log")

	w, err := newRotatingLogWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if w.file != nil {
			w.file.Close()
		}
	}()

	content := strings.Repeat("x", int(maxLogSize+1))
	n, err := w.Write([]byte(content))
	if err != nil {
		t.Fatal(err)
	}
	if n != len(content) {
		t.Fatalf("expected to write %d bytes, wrote %d", len(content), n)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 1 {
		t.Fatalf("expected current log size 1, got %d", info.Size())
	}

	backupInfo, err := os.Stat(path + ".1")
	if err != nil {
		t.Fatal(err)
	}
	if backupInfo.Size() != maxLogSize {
		t.Fatalf("expected backup size %d, got %d", maxLogSize, backupInfo.Size())
	}
}
