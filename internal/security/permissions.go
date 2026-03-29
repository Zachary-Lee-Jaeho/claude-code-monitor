package security

import (
	"fmt"
	"os"
)

// EnsureDir creates a directory with 0700 permissions if it doesn't exist.
func EnsureDir(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return os.MkdirAll(path, 0o700)
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s exists but is not a directory", path)
	}
	// Fix permissions if wrong
	if info.Mode().Perm() != 0o700 {
		return os.Chmod(path, 0o700)
	}
	return nil
}

// EnsureFilePermissions sets the given permissions on a file.
func EnsureFilePermissions(path string, perm os.FileMode) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm() != perm {
		return os.Chmod(path, perm)
	}
	return nil
}

// CheckFilePermissions warns if a file has wrong permissions.
func CheckFilePermissions(path string, expected os.FileMode) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	actual := info.Mode().Perm()
	if actual != expected {
		return fmt.Errorf("%s has permissions %04o, expected %04o", path, actual, expected)
	}
	return nil
}
