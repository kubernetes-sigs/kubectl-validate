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
	"k8s.io/kube-openapi/pkg/validation/spec"
	"sigs.k8s.io/kubectl-validate/pkg/openapiclient"
	"sigs.k8s.io/kubectl-validate/pkg/utils"
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
					applySchemaPatches(i, gv, k, d)
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

type SchemaPatch struct {
	Slug        string
	Description string

	// (Inclusive) version range for which this patch applies
	MinMinorVersion int
	MaxMinorVersion int

	// Nil is wildcard
	AppliesToGV func(schema.GroupVersion) bool
	Transformer utils.SchemaVisitor
}

var zero int64 = int64(0)
var schemaPatches []SchemaPatch = []SchemaPatch{
	{
		Slug:            "AllowEmptyByteFormat",
		Description:     "Work around discrepency between treatment of native vs CRD `byte` strings. Native types allow empty, CRDs do not",
		MinMinorVersion: 0,
		MaxMinorVersion: 0,
		AppliesToGV:     nil,
		Transformer: utils.PostorderVisitor(func(ctx utils.VisitingContext, s *spec.Schema) bool {
			if s.Format != "byte" || len(s.Type) != 1 || s.Type[0] != "string" {
				return true
			}

			// Change format to "", and add new `$and: {$or: [{format: "byte"}, {maxLength: 0}]}
			s.AllOf = append(s.AllOf, spec.Schema{
				SchemaProps: spec.SchemaProps{
					AnyOf: []spec.Schema{
						{
							SchemaProps: spec.SchemaProps{
								Format: s.Format,
							},
						},
						{
							SchemaProps: spec.SchemaProps{
								MaxLength: &zero,
							},
						},
					},
				},
			})
			s.Format = ""
			return true
		}),
	},
	{
		Slug:        "AllowEmptyDateTime",
		Description: "metav1.Time published OpenAPI definitions do not allow empty/null, but Kubernetes in practice does.",
		Transformer: utils.PostorderVisitor(func(ctx utils.VisitingContext, s *spec.Schema) bool {
			if s.Format != "date-time" || len(s.Type) != 1 || s.Type[0] != "string" {
				return true
			}

			// Change format to "", and add new `$and: {$or: [{format: "date-time"}, {maxLength: 0}]}
			s.AllOf = append(s.AllOf, spec.Schema{
				SchemaProps: spec.SchemaProps{
					AnyOf: []spec.Schema{
						{
							SchemaProps: spec.SchemaProps{
								Format: s.Format,
							},
						},
						{
							SchemaProps: spec.SchemaProps{
								MaxLength: &zero,
							},
						},
					},
				},
			})
			s.Format = ""
			return true
		}),
	},
	{
		Slug:        "RemoveInvalidObjectDefaults",
		Description: "`default: {}` is supplied for many schemas which are not objects. This would cause errors for CRDs but not native types",
		Transformer: utils.PostorderVisitor(func(ctx utils.VisitingContext, s *spec.Schema) bool {
			if s.Default == nil {
				return true
			} else if len(s.Type) > 1 {
				return true
			} else if objDefault, ok := s.Default.(map[string]interface{}); !ok || len(objDefault) != 0 {
				// Skip defaults that are not 0-length maps
				return true
			}

			if len(s.Type) == 1 {
				switch s.Type[0] {
				case "object":
					// do nothing
				case "array":
					// replace with array-typed zero value
					s.Default = []interface{}{}
				case "string":
					s.Default = ""
				case "number":
					s.Default = float32(0)
				case "integer":
					s.Default = int32(0)
				case "boolean":
					s.Default = false
				default:
					// Unknown type, wipe default
					s.Default = nil
				}
			} else if len(s.Ref.String()) > 0 {
				//!TODO: this does not evaulate wheter a ref might be an object
				s.Default = nil
			} else if len(s.AllOf) == 1 && len(s.AllOf[0].Ref.String()) > 0 && !s.AllOf[0].Type.Contains("object") {
				//!TODO: this does not evaulate wheter a ref might be an object
				s.Default = nil
			}

			return true
		}),
	},
}

func applySchemaPatches(k8sVersion int, gv schema.GroupVersion, defName string, schema *spec.Schema) {
	for _, p := range schemaPatches {
		if p.MinMinorVersion != 0 && p.MinMinorVersion > k8sVersion {
			continue
		} else if p.MaxMinorVersion != 0 && p.MaxMinorVersion < k8sVersion {
			continue
		} else if p.AppliesToGV != nil && !p.AppliesToGV(gv) {
			continue
		}

		utils.VisitSchema(defName, schema, p.Transformer)
	}
}
