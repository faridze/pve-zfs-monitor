package device

import (
	"os"
	"path/filepath"
)

func loadFixture(name string) []byte {
	candidates := []string{
		filepath.Join("testdata", name),
		filepath.Join("internal", "device", "testdata", name),
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err == nil {
			return data
		}
	}
	panic("fixture not found: " + name)
}

func loadZFSFixture(name string) []byte {
	candidates := []string{
		filepath.Join("..", "zfs", "testdata", name),
		filepath.Join("internal", "zfs", "testdata", name),
	}
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err == nil {
			return data
		}
	}
	panic("zfs fixture not found: " + name)
}
