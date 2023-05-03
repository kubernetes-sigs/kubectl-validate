package main

import (
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/kubectl-validate/pkg/openapiclient"
)

// Downloads builtin schemas from GitHub and saves them to disk for embedding
func main() {
	if len(os.Args) != 2 {
		fmt.Printf("Usage: download-builtin-schemas outputDirectory")
		return
	}

	outputDir := os.Args[1]
	err := os.Mkdir(outputDir, 0o755)
	if err != nil {
		panic(err)
	}

	// Versions 1.0-1.22 did not have OpenAPIV3 schemas.
	for i := 23; ; i++ {
		version := fmt.Sprintf("1.%d", i)
		fetcher := openapiclient.NewGitHubBuiltins(version)
		schemas, err := fetcher.Paths()
		if err != nil {
			break
		}

		for k, v := range schemas {
			data, err := v.Schema("application/json")
			if err != nil {
				panic(err)
			}

			path := filepath.Join(outputDir, version, k+".json")
			dir, _ := filepath.Split(path)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				panic(err)
			}

			if err := os.WriteFile(path, data, 0o755); err != nil {
				panic(err)
			}
		}
	}

	//TODO: Download OpenAPIV3 schemas and convert to V3?
	// might be error prone since some (very few like IntOrString) types are
	// handled differently
}
