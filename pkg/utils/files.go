package utils

import (
	"fmt"
	"os"
)

func MakeDir(path string) error {
	return os.MkdirAll(path, 0755)
}

func DeleteDir(path string) error {
	return os.RemoveAll(path)
}

func DeleteFile(path string) error {
	return os.Remove(path)
}

func MoveFile(src, dst string) error {
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("failed to move file from %s to %s: %w", src, dst, err)
	}
	return nil
}

func MoveDir(src, dst string) error {
	if err := os.Rename(src, dst); err != nil {
		return fmt.Errorf("failed to move directory from %s to %s: %w", src, dst, err)
	}
	return nil
}
