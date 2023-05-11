package openapiclient

import (
	"io/fs"
	"os"
)

func stat(f fs.FS, filepath string) (fs.FileInfo, error) {
	if f == nil {
		return os.Stat(filepath)
	}
	return fs.Stat(f, filepath)
}

func readFile(f fs.FS, filepath string) ([]byte, error) {
	if f == nil {
		return os.ReadFile(filepath)
	}
	return fs.ReadFile(f, filepath)
}

func readDir(f fs.FS, filepath string) ([]fs.DirEntry, error) {
	if f == nil {
		return os.ReadDir(filepath)
	}
	return fs.ReadDir(f, filepath)
}
