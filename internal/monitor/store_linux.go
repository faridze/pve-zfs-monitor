//go:build linux

package monitor

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const (
	stateFileName = "state.json"
	lockFileName  = "state.lock"
	maxStateSize  = 128 << 10
)

// FileStore is an *opt-in*, single-process-at-a-time, durable state store.
// No daemon or command invokes it by default. The caller supplies an existing
// private directory; FileStore never changes system configuration or disks.
type FileStore struct {
	Directory string
}

// Advance holds an exclusive non-blocking lock across read -> pure decision ->
// atomic durable write. It returns event decisions ONLY after persistence.
// It never sends notifications. A future delivery implementation must add a
// durable outbox/ack protocol before considering crash-safe delivery.
func (s FileStore) Advance(readings []Signal, now time.Time, opts Options) ([]Event, error) {
	dir, err := openPrivateDirectory(s.Directory)
	if err != nil {
		return nil, fmt.Errorf("state directory: %w", err)
	}
	defer syscall.Close(dir)

	lock, err := syscall.Openat(dir, lockFileName,
		syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open state lock: %w", err)
	}
	defer syscall.Close(lock)
	if err := verifyPrivateRegular(lock); err != nil {
		return nil, fmt.Errorf("invalid state lock: %w", err)
	}
	if err := syscall.Flock(lock, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return nil, fmt.Errorf("state lock busy: %w", err)
	}
	defer syscall.Flock(lock, syscall.LOCK_UN)

	prev, err := readStateAt(dir)
	if err != nil {
		return nil, err
	}
	next, events, err := Decide(prev, readings, now, opts)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(next)
	if err != nil {
		return nil, fmt.Errorf("encode state: %w", err)
	}
	if len(data) > maxStateSize {
		return nil, errors.New("state exceeds size limit")
	}
	if err := atomicWriteStateAt(dir, data); err != nil {
		return nil, err
	}
	return events, nil
}

// openPrivateDirectory traverses every component with O_NOFOLLOW, not just
// the final path. Only the target directory must be private and owned by the
// calling user. No parent directories or symlinks are created by this code.
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
		return errors.New("state/lock must be a private, single-linked regular file")
	}
	return nil
}

func readStateAt(dir int) (State, error) {
	fd, err := syscall.Openat(dir, stateFileName,
		syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if errors.Is(err, syscall.ENOENT) {
		return State{}, nil // first run only; corrupt existing state is NOT reset
	}
	if err != nil {
		return State{}, fmt.Errorf("open prior state: %w", err)
	}
	f := os.NewFile(uintptr(fd), stateFileName)
	defer f.Close()
	if err := verifyPrivateRegular(fd); err != nil {
		return State{}, fmt.Errorf("invalid state file: %w", err)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxStateSize+1))
	if err != nil {
		return State{}, fmt.Errorf("read state: %w", err)
	}
	if len(data) == 0 || len(data) > maxStateSize {
		return State{}, errors.New("missing or oversized state contents")
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	var previous State
	if err := dec.Decode(&previous); err != nil {
		return State{}, fmt.Errorf("corrupt state: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return State{}, errors.New("trailing or malformed state content")
	}
	if err := validatePersistedState(previous); err != nil {
		return State{}, fmt.Errorf("corrupt state: %w", err)
	}
	return previous, nil
}

func atomicWriteStateAt(dir int, data []byte) error {
	var entropy [12]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return fmt.Errorf("random temporary filename: %w", err)
	}
	tmp := ".state-" + hex.EncodeToString(entropy[:]) + ".tmp"
	fd, err := syscall.Openat(dir, tmp,
		syscall.O_WRONLY|syscall.O_CREAT|syscall.O_EXCL|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0o600)
	if err != nil {
		return fmt.Errorf("create temporary state: %w", err)
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
		return fmt.Errorf("temporary state permissions: %w", err)
	}
	_, writeErr := f.Write(data)
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil {
		return fmt.Errorf("write state: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close state: %w", closeErr)
	}
	if err := syscall.Renameat(dir, tmp, dir, stateFileName); err != nil {
		return fmt.Errorf("commit state: %w", err)
	}
	created = false
	if err := syscall.Fsync(dir); err != nil {
		return fmt.Errorf("sync state directory: %w", err)
	}
	return nil
}
