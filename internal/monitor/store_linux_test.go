//go:build linux

package monitor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func privateTestStore(t *testing.T) FileStore {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "state")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return FileStore{Directory: dir}
}

func TestFileStoreDurabilityDedupeRecoveryAndPrivacy(t *testing.T) {
	s := privateTestStore(t)
	now := baseTime
	failed := signal("pool", "sensitive-zfs-pool", Critical, now)
	events, err := s.Advance([]Signal{failed}, now, Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertEvent(t, events, Alert, Critical)
	// A new FileStore instance is a new process equivalent; state survives.
	other := FileStore{Directory: s.Directory}
	failed.ObservedAt = now.Add(time.Minute)
	events, err = other.Advance([]Signal{failed}, failed.ObservedAt, Options{})
	if err != nil || len(events) != 0 {
		t.Fatalf("dedup across store reopen failed: events=%+v err=%v", events, err)
	}
	now = now.Add(24 * time.Hour)
	failed.ObservedAt = now
	events, err = other.Advance([]Signal{failed}, now, Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertEvent(t, events, Reminder, Critical)

	failed.Level = Healthy
	now = now.Add(time.Minute)
	failed.ObservedAt = now
	events, err = s.Advance([]Signal{failed}, now, Options{})
	if err != nil || len(events) != 0 {
		t.Fatalf("recovered before verification: %+v %v", events, err)
	}
	now = now.Add(time.Minute)
	failed.ObservedAt = now
	events, err = s.Advance([]Signal{failed}, now, Options{})
	if err != nil {
		t.Fatal(err)
	}
	assertEvent(t, events, Recovery, Healthy)

	raw, err := os.ReadFile(filepath.Join(s.Directory, stateFileName))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "sensitive-zfs-pool") ||
		strings.Contains(string(raw), "/dev/") {
		t.Fatal("private identity leaked in state")
	}
	var state State
	if err := json.Unmarshal(raw, &state); err != nil || state.Version != 1 {
		t.Fatalf("state not valid persisted JSON: %v", err)
	}
	for _, name := range []string{stateFileName, lockFileName} {
		fi, err := os.Lstat(filepath.Join(s.Directory, name))
		if err != nil || fi.Mode().Perm() != 0o600 || !fi.Mode().IsRegular() {
			t.Fatalf("%s not private regular file: mode=%v err=%v", name, fi.Mode(), err)
		}
	}
}

func TestFileStoreRejectsCorruptionWithoutOverwriting(t *testing.T) {
	for _, contents := range []string{
		"garbled",
		"",
		"{\"version\":99,\"records\":{}}",
		"{\"version\":1,\"records\":{},\"unknown_field\":\"untrusted\"}",
		strings.Repeat("x", maxStateSize+1),
	} {
		t.Run("corrupt-"+string(rune(len(contents)%200+65)), func(t *testing.T) {
			s := privateTestStore(t)
			file := filepath.Join(s.Directory, stateFileName)
			if err := os.WriteFile(file, []byte(contents), 0o600); err != nil {
				t.Fatal(err)
			}
			bad := signal("pool", "some-pool", Critical, baseTime)
			events, err := s.Advance([]Signal{bad}, baseTime, Options{})
			if err == nil || len(events) != 0 {
				t.Fatalf("corruption accepted: events=%v err=%v", events, err)
			}
			raw, err := os.ReadFile(file)
			if err != nil || string(raw) != contents {
				t.Fatal("corrupted state overwritten")
			}
		})
	}
}

func TestFileStoreRejectsSymlinksAndInsecurePaths(t *testing.T) {
	bad := signal("pool", "pool", Critical, baseTime)
	t.Run("symlink directory", func(t *testing.T) {
		s := privateTestStore(t)
		link := filepath.Join(t.TempDir(), "link")
		if err := os.Symlink(s.Directory, link); err != nil {
			t.Fatal(err)
		}
		if ev, err := (FileStore{Directory: link}).Advance([]Signal{bad}, baseTime, Options{}); err == nil || len(ev) > 0 {
			t.Fatalf("symlink directory accepted: %v %v", ev, err)
		}
	})
	t.Run("symlink ancestor", func(t *testing.T) {
		s := privateTestStore(t)
		link := filepath.Join(t.TempDir(), "link")
		if err := os.Symlink(filepath.Dir(s.Directory), link); err != nil {
			t.Fatal(err)
		}
		if ev, err := (FileStore{Directory: filepath.Join(link, "state")}).Advance([]Signal{bad}, baseTime, Options{}); err == nil || len(ev) > 0 {
			t.Fatalf("symlink ancestor accepted: %v %v", ev, err)
		}
	})
	t.Run("symlink state file", func(t *testing.T) {
		s := privateTestStore(t)
		other := filepath.Join(t.TempDir(), "outside")
		if err := os.WriteFile(other, []byte("sentinel"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(other, filepath.Join(s.Directory, stateFileName)); err != nil {
			t.Fatal(err)
		}
		if ev, err := s.Advance([]Signal{bad}, baseTime, Options{}); err == nil || len(ev) > 0 {
			t.Fatalf("symlink state accepted: %v %v", ev, err)
		}
		b, _ := os.ReadFile(other)
		if string(b) != "sentinel" {
			t.Fatal("outside file changed")
		}
	})
	t.Run("symlink lock", func(t *testing.T) {
		s := privateTestStore(t)
		if err := os.Symlink("/dev/null", filepath.Join(s.Directory, lockFileName)); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Advance([]Signal{bad}, baseTime, Options{}); err == nil {
			t.Fatal("symlink lock accepted")
		}
	})
	t.Run("world-readable dir", func(t *testing.T) {
		s := privateTestStore(t)
		if err := os.Chmod(s.Directory, 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Advance([]Signal{bad}, baseTime, Options{}); err == nil {
			t.Fatal("public state directory accepted")
		}
	})
	t.Run("world-readable state", func(t *testing.T) {
		s := privateTestStore(t)
		if err := os.WriteFile(filepath.Join(s.Directory, stateFileName), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Advance([]Signal{bad}, baseTime, Options{}); err == nil {
			t.Fatal("public state file accepted")
		}
	})
	t.Run("relative path", func(t *testing.T) {
		if _, err := (FileStore{Directory: "relative/path"}).Advance([]Signal{bad}, baseTime, Options{}); err == nil {
			t.Fatal("relative path accepted")
		}
	})
}

func TestFileStoreNonBlockingConcurrencyLock(t *testing.T) {
	s := privateTestStore(t)
	dir, err := openPrivateDirectory(s.Directory)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(dir)
	lock, err := syscall.Openat(dir, lockFileName,
		syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer syscall.Close(lock)
	if err := syscall.Flock(lock, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatal(err)
	}
	defer syscall.Flock(lock, syscall.LOCK_UN)
	sig := signal("pool", "pool", Critical, baseTime)
	if events, err := s.Advance([]Signal{sig}, baseTime, Options{}); err == nil || len(events) != 0 {
		t.Fatalf("concurrent writer acquired lock: %v %v", events, err)
	}
	if _, err := os.Stat(filepath.Join(s.Directory, stateFileName)); !os.IsNotExist(err) {
		t.Fatalf("locked writer created state: %v", err)
	}
}

func TestFileStoreRejectsInvalidInputWithoutChangingState(t *testing.T) {
	s := privateTestStore(t)
	ok := signal("pool", "pool", Healthy, baseTime)
	if _, err := s.Advance([]Signal{ok}, baseTime, Options{}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(s.Directory, stateFileName)
	before, _ := os.ReadFile(path)
	invalid := signal("pool", "pool", Critical, baseTime)
	invalid.Key = "/dev/nvme0n1"
	if events, err := s.Advance([]Signal{invalid}, baseTime.Add(time.Minute), Options{}); err == nil || len(events) != 0 {
		t.Fatalf("invalid decision produced events: %+v %v", events, err)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("invalid observation replaced durable state")
	}
}
