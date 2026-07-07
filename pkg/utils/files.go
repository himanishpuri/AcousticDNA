package utils

import (
	"fmt"
	"os"
)

func MakeDir(path string) error {
	return os.MkdirAll(path, 0755)
}

func MoveFile(src, dst string) error {
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("failed to move file from %s to %s: %w", src, dst, err)
	}
	return nil
}
