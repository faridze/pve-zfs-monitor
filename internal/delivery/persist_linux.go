//go:build linux

package delivery

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	stateFileName = "state.json"
	lockFileName  = "state.lock"
	maxDocSize    = 256 << 10
)

// Store provides atomic monitor state + outbox persistence in one directory.
type Store struct {
	Directory string
}

func (s Store) withLock(fn func(dir int) error) error {
	dir, err := openPrivateDirectory(s.Directory)
	if err != nil {
		return err
	}
	defer syscall.Close(dir)
	lock, err := syscall.Openat(dir, lockFileName,
		syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0o600)
	if err != nil {
		return fmt.Errorf("open state lock: %w", err)
	}
	defer syscall.Close(lock)
	if err := verifyPrivateRegular(lock); err != nil {
		return fmt.Errorf("invalid state lock: %w", err)
	}
	if err := syscall.Flock(lock, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("state lock busy: %w", err)
	}
	defer syscall.Flock(lock, syscall.LOCK_UN)
	return fn(dir)
}

func (s Store) Load() (Document, error) {
	var doc Document
	err := s.withLock(func(dir int) error {
		data, err := readFileAt(dir, stateFileName)
		if err != nil {
			return err
		}
		doc, err = decodeDocument(data)
		return err
	})
	return doc, err
}

func (s Store) Save(doc Document) error {
	return s.withLock(func(dir int) error {
		data, err := encodeDocument(doc)
		if err != nil {
			return err
		}
		if len(data) > maxDocSize {
			return errors.New("document exceeds size limit")
		}
		return atomicWriteAt(dir, stateFileName, data)
	})
}

func readFileAt(dir int, name string) ([]byte, error) {
	fd, err := syscall.Openat(dir, name, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if errors.Is(err, syscall.ENOENT) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", name, err)
	}
	f := os.NewFile(uintptr(fd), name)
	defer f.Close()
	if err := verifyPrivateRegular(fd); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, maxDocSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxDocSize {
		return nil, errors.New("oversized file")
	}
	if len(data) == 0 {
		return nil, errors.New("empty state file")
	}
	return data, nil
}

func atomicWriteAt(dir int, name string, data []byte) error {
	var entropy [12]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return err
	}
	tmp := ".doc-" + hex.EncodeToString(entropy[:]) + ".tmp"
	fd, err := syscall.Openat(dir, tmp,
		syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0o600)
	if err != nil {
		return err
	}
	created := true
	defer func() {
		if created {
			syscall.Unlinkat(dir, tmp)
		}
	}()
	f := os.NewFile(uintptr(fd), tmp)
	if err := verifyPrivateRegular(fd); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := syscall.Renameat(dir, tmp, dir, name); err != nil {
		return err
	}
	created = false
	return syscall.Fsync(dir)
}

func openPrivateDirectory(path string) (int, error) {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path || path == "/" {
		return -1, errors.New("state directory must be a clean absolute path")
	}
	fd, err := syscall.Open("/", syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_CLOEXEC, 0)
	if err != nil {
		return -1, err
	}
	for _, part := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if part == "" || part == "." || part == ".." {
			syscall.Close(fd)
			return -1, errors.New("invalid state directory component")
		}
		next, err := syscall.Openat(fd, part,
			syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
		syscall.Close(fd)
		if err != nil {
			return -1, err
		}
		fd = next
	}
	var st syscall.Stat_t
	if err := syscall.Fstat(fd, &st); err != nil {
		syscall.Close(fd)
		return -1, err
	}
	if st.Mode&syscall.S_IFMT != syscall.S_IFDIR ||
		st.Mode&0o077 != 0 || st.Uid != uint32(os.Geteuid()) {
		syscall.Close(fd)
		return -1, errors.New("state directory must be privately owned (0700)")
	}
	return fd, nil
}

func verifyPrivateRegular(fd int) error {
	var st syscall.Stat_t
	if err := syscall.Fstat(fd, &st); err != nil {
		return err
	}
	if st.Mode&syscall.S_IFMT != syscall.S_IFREG ||
		st.Mode&0o077 != 0 || st.Uid != uint32(os.Geteuid()) || st.Nlink != 1 {
		return errors.New("file must be private regular file")
	}
	return nil
}
