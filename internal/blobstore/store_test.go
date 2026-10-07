package blobstore

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestImmutableBoundedFiles(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "private"), 200_000_000, 100_000_000)
	if err != nil {
		t.Fatal(err)
	}
	key := "IMMUTABLEOBJECT1234"
	data := []byte("private artifact")
	n, sum, err := store.Put(key, bytes.NewReader(data), int64(len(data)), "")
	if err != nil || n != int64(len(data)) || sum == "" {
		t.Fatalf("put failed: %d %s %v", n, sum, err)
	}
	if _, _, err = store.Put(key, bytes.NewReader(data), int64(len(data)), sum); err == nil {
		t.Fatal("immutable identity was overwritten")
	}
	file, err := store.Open(key)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(file)
	file.Close()
	if !bytes.Equal(got, data) {
		t.Fatal("stored bytes differ")
	}
	if err = store.Delete(key); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Open(key); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("deleted object remains: %v", err)
	}
}

func TestRejectsUnexpectedEntriesAndLimits(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/etc/passwd", filepath.Join(root, "UNEXPECTEDOBJECT123")); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(root, 200_000_000, 100_000_000); err == nil {
		t.Fatal("private storage accepted a symbolic link")
	}
	if err := os.Remove(filepath.Join(root, "UNEXPECTEDOBJECT123")); err != nil {
		t.Fatal(err)
	}
	store, err := Open(root, 200_000_000, 100_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.Put("BOUNDEDOBJECT123456", bytes.NewReader([]byte("too long")), 3, ""); err == nil {
		t.Fatal("byte limit was ignored")
	}
	if _, _, err = store.Put("../escape", bytes.NewReader(nil), 0, ""); err == nil {
		t.Fatal("path traversal key accepted")
	}
}
