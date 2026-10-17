//go:build unix

package release

import (
	"os"
	"syscall"
)

func fileLock(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}
