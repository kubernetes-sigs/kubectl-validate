package utils

import (
	"os"
	"path"
	"strings"
)

func IsYaml(file string) bool {
	ext := strings.ToLower(path.Ext(file))
	return ext == ".yaml" || ext == ".yml"
}

func IsJson(file string) bool {
	ext := strings.ToLower(path.Ext(file))
	return ext == ".json"
}

func IsYamlOrJson(file string) bool {
	return IsYaml(file) || IsJson(file)
}

func FindFiles(args ...string) ([]string, error) {
	var files []string
	for _, fileOrDir := range args {
		info, err := os.Stat(fileOrDir)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			sub, err := findFilesInDir(fileOrDir)
			if err != nil {
				return nil, err
			}
			files = append(files, sub...)
		} else {
			files = append(files, fileOrDir)
		}
	}
	return files, nil
}

func findFilesInDir(dir string) ([]string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	if info.IsDir() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			fileOrDir := path.Join(dir, entry.Name())
			if entry.IsDir() {
				sub, err := findFilesInDir(fileOrDir)
				if err != nil {
					return nil, err
				}
				files = append(files, sub...)
			} else {
				if IsYamlOrJson(fileOrDir) {
					files = append(files, fileOrDir)
				}
			}
		}
	}
	return files, nil
}
