package utils

import (
	"fmt"
	"os"
)

// MakeDir creates a directory with all parent directories
func MakeDir(path string) error {
	return os.MkdirAll(path, 0755)
}

// DeleteDir removes a directory and all its contents
func DeleteDir(path string) error {
	return os.RemoveAll(path)
}

// DeleteFile removes a file
func DeleteFile(path string) error {
	return os.Remove(path)
}

// MoveFile moves or renames a file
func MoveFile(src, dst string) error {
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("failed to move file from %s to %s: %w", src, dst, err)
	}
	return nil
}

// MoveDir moves or renames a directory
func MoveDir(src, dst string) error {
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("failed to move directory from %s to %s: %w", src, dst, err)
	}
	return nil
}
