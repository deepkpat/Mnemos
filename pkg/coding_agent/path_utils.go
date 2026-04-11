package coding_agent

import (
	"os"
	"path/filepath"
)

// resolveToCwd resolves a relative path to an absolute path based on the current working directory
func resolveToCwd(path string, cwd string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(cwd, path)
}

// resolveReadPath is specifically for read tool, handling special cases
func resolveReadPath(path string, cwd string) string {
	// Handle path like "foo.txt" or "./foo.txt" or "../foo.txt"
	// Similar to resolveToCwd but can be extended for read-specific logic
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(cwd, path)
}

// GetWorkingDir returns the current working directory
func GetWorkingDir() (string, error) {
	return os.Getwd()
}

// NormalizePath normalizes a path (resolves . and ..)
func NormalizePath(path string) string {
	return filepath.Clean(path)
}

// JoinPath joins path elements
func JoinPath(elem ...string) string {
	return filepath.Join(elem...)
}

// GetBaseName returns the base name of a path
func GetBaseName(path string) string {
	return filepath.Base(path)
}

// GetDirName returns the directory part of a path
func GetDirName(path string) string {
	return filepath.Dir(path)
}

// GetExt returns the extension of a path
func GetExt(path string) string {
	return filepath.Ext(path)
}

// ToPosixPath converts a path to POSIX format (forward slashes)
func ToPosixPath(path string) string {
	result := filepath.ToSlash(path)
	return result
}

// FromPosixPath converts a POSIX path to the OS-specific format
func FromPosixPath(path string) string {
	return filepath.FromSlash(path)
}

// Rel returns the relative path from base to target
func Rel(base, target string) (string, error) {
	return filepath.Rel(base, target)
}

// Abs returns the absolute representation of path
func Abs(path string) (string, error) {
	return filepath.Abs(path)
}

// IsAbs returns whether the path is absolute
func IsAbs(path string) bool {
	return filepath.IsAbs(path)
}
