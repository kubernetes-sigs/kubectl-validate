package utils

import (
	"io/fs"
	"os"
)

func Stat(f fs.FS, filepath string) (fs.FileInfo, error) {
	if f == nil {
		return os.Stat(filepath)
	}
	return fs.Stat(f, filepath)
}

func ReadFile(f fs.FS, filepath string) ([]byte, error) {
	if f == nil {
		return os.ReadFile(filepath)
	}
	return fs.ReadFile(f, filepath)
}

func ReadDir(f fs.FS, filepath string) ([]fs.DirEntry, error) {
	if f == nil {
		return os.ReadDir(filepath)
	}
	return fs.ReadDir(f, filepath)
}
