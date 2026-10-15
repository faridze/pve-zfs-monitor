//go:build !unix

package release

import (
	"fmt"
	"os"
)

func fileLock(f *os.File) error {
	return fmt.Errorf("file locking unsupported on this platform")
}
