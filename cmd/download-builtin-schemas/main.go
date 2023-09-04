package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/kube-openapi/pkg/spec3"
	"sigs.k8s.io/kubectl-validate/pkg/openapiclient"
	"sigs.k8s.io/kubectl-validate/pkg/validatorfactory"
)

// Downloads builtin schemas from GitHub and saves them to disk for embedding
func main() {
	if len(os.Args) != 2 && len(os.Args) != 3 {
		fmt.Printf("Usage: download-builtin-schemas outputDirectory [patchesOutputDirectory]")
		return
	}

	outputDir := os.Args[1]
	err := os.MkdirAll(outputDir, 0o755)
	if err != nil {
		panic(err)
	}

	patchesDir := ""
	if len(os.Args) == 3 {
		patchesDir = os.Args[2]

		if err := os.MkdirAll(patchesDir, 0o755); err != nil {
			panic(err)
		}
	}

	// Versions 1.0-1.22 did not have OpenAPIV3 schemas.
	for i := 23; ; i++ {
		version := fmt.Sprintf("1.%d", i)
		fetcher := openapiclient.NewGitHubBuiltins(version)
		// fetcher := openapiclient.NewHardcodedBuiltins(version)
		schemas, err := fetcher.Paths()
		if err != nil {
			break
		}

		for k, v := range schemas {
			data, err := v.Schema("application/json")
			if err != nil {
				panic(err)
			}

			var gv schema.GroupVersion
			if strings.HasPrefix(k, "apis/") {
				gv, err = schema.ParseGroupVersion(k[5:])
				if err != nil {
					panic(err)
				}
			} else if strings.HasPrefix(k, "api/") {
				gv, err = schema.ParseGroupVersion(k[4:])
				if err != nil {
					panic(err)
				}
			} else {
				panic(fmt.Errorf("unknown path %s", k))
			}

			path := filepath.Join(outputDir, version, k+".json")
			dir, _ := filepath.Split(path)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				panic(err)
			}

			if err := os.WriteFile(path, data, 0o755); err != nil {
				panic(err)
			}

			// Postprocess schema and save off the diff
			if len(patchesDir) > 0 {
				patchPath := filepath.Join(patchesDir, version, k+".json")
				dir, _ := filepath.Split(patchPath)

				parsed := &spec3.OpenAPI{}
				if err := json.Unmarshal(data, parsed); err != nil {
					panic(err)
				}

				for k, d := range parsed.Components.Schemas {
					validatorfactory.ApplySchemaPatches(i, gv, k, d)
				}

				newJSON, err := json.Marshal(parsed)
				if err != nil {
					panic(err)
				}

				patch, err := jsonpatch.CreateMergePatch(data, newJSON)
				if err != nil {
					panic(err)
				}

				if len(patch) > 2 {
					buf := bytes.NewBuffer(nil)
					if err := json.Indent(buf, patch, "", "    "); err != nil {
						panic(err)
					}

					if err := os.MkdirAll(dir, 0o755); err != nil {
						panic(err)
					}

					if err := os.WriteFile(patchPath, buf.Bytes(), 0o755); err != nil {
						panic(err)
					}
				}
			}
		}
	}

	//TODO: Download OpenAPIV3 schemas and convert to V3?
	// might be error prone since some (very few like IntOrString) types are
	// handled differently
}
