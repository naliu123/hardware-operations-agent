// Package blobstore owns immutable private files. Object keys are generated
// identities, never display names or paths from a browser/model.
package blobstore

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sync"

	"golang.org/x/sys/unix"
)

var keyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,80}$`)
var ErrCapacity = errors.New("private file storage quota or free space exhausted")

type Store struct {
	dir                  string
	mu                   sync.Mutex
	quota, reserve, used int64
}

func Open(dir string, quota, reserve int64) (*Store, error) {
	if quota < 100_000_000 || reserve < 100_000_000 {
		return nil, errors.New("explicit storage quota and reserve must each be at least 100 MB")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("private storage must be a real directory with mode 0700")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	s := &Store{dir: dir, quota: quota, reserve: reserve}
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil, errors.New("unexpected private storage entry")
		}
		if !keyPattern.MatchString(entry.Name()) {
			// Only this module's own incomplete writes are disposable.
			if len(entry.Name()) > 8 && entry.Name()[:8] == ".upload-" {
				if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
					return nil, err
				}
				continue
			}
			return nil, errors.New("invalid private storage key")
		}
		s.used += info.Size()
	}
	if s.used > quota {
		return nil, ErrCapacity
	}
	return s, nil
}

func (s *Store) Put(key string, input io.Reader, limit int64, expectedHash string) (int64, string, error) {
	if !keyPattern.MatchString(key) || limit < 0 || limit > 100_000_000 {
		return 0, "", errors.New("invalid file key or limit")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.used+limit > s.quota {
		return 0, "", ErrCapacity
	}
	var disk unix.Statfs_t
	if unix.Statfs(s.dir, &disk) != nil || int64(disk.Bavail)*int64(disk.Bsize)-limit < s.reserve {
		return 0, "", ErrCapacity
	}
	if _, err := os.Lstat(filepath.Join(s.dir, key)); !errors.Is(err, os.ErrNotExist) {
		return 0, "", errors.New("immutable file identity already exists")
	}
	f, err := os.CreateTemp(s.dir, ".upload-")
	if err != nil {
		return 0, "", err
	}
	defer os.Remove(f.Name())
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, hash), io.LimitReader(input, limit+1))
	sum := fmt.Sprintf("%x", hash.Sum(nil))
	if err == nil && n > limit {
		err = errors.New("file byte limit exceeded")
	}
	if err == nil && expectedHash != "" && sum != expectedHash {
		err = errors.New("file content hash mismatch")
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(f.Name(), filepath.Join(s.dir, key))
	}
	if err == nil {
		err = syncDirectory(s.dir)
	}
	if err != nil {
		return 0, "", err
	}
	s.used += n
	return n, sum, nil
}

func (s *Store) Open(key string) (*os.File, error) {
	if !keyPattern.MatchString(key) {
		return nil, os.ErrNotExist
	}
	path := filepath.Join(s.dir, key)
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, os.ErrNotExist
	}
	return os.Open(path)
}

func (s *Store) Delete(key string) error {
	if !keyPattern.MatchString(key) {
		return errors.New("invalid file key")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	info, err := os.Lstat(filepath.Join(s.dir, key))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("unexpected file type")
	}
	if err = os.Remove(filepath.Join(s.dir, key)); err == nil {
		s.used -= info.Size()
		err = syncDirectory(s.dir)
	}
	return err
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
