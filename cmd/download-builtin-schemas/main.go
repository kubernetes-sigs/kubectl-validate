package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	jsonpatch "github.com/evanphx/json-patch"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
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

func isTimeSchema(s string) bool {
	return s == "io.k8s.apimachinery.pkg.apis.meta.v1.Time" || s == "io.k8s.apimachinery.pkg.apis.meta.v1.MicroTime"
}

func isBuiltInType(gv schema.GroupVersion) bool {
	// filter out non built-in types
	if gv.Group == "" {
		return true
	}
	if strings.HasSuffix(gv.Group, ".k8s.io") {
		return true
	}
	if gv.Group == "apps" || gv.Group == "autoscaling" || gv.Group == "batch" || gv.Group == "policy" {
		return true
	}
	return false
}

type SchemaPatch struct {
	Slug        string
	Description string

	// (Inclusive) version range for which this patch applies
	MinMinorVersion int
	MaxMinorVersion int

	// Nil is wildcard
	AppliesToGV         func(schema.GroupVersion) bool
	AppliesToDefinition func(string) bool
	Transformer         utils.SchemaVisitor
}

var zero int64 = int64(0)
var schemaPatches []SchemaPatch = []SchemaPatch{
	{
		Slug:            "AllowEmptyByteFormat",
		Description:     "Work around discrepency between treatment of native vs CRD `byte` strings. Native types allow empty, CRDs do not",
		MinMinorVersion: 0,
		MaxMinorVersion: 0,
		AppliesToGV:     isBuiltInType,
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
		Slug:                "FixTime",
		AppliesToDefinition: isTimeSchema,
		Description:         "metav1.Time published OpenAPI definitions do not allow empty/null, but Kubernetes in practice does.",
		Transformer: utils.PostorderVisitor(func(ctx utils.VisitingContext, s *spec.Schema) bool {
			if s.Format != "date-time" || len(s.Type) != 1 || s.Type[0] != "string" || s.Nullable {
				return true
			}

			s.Nullable = true
			return true
		}),
	},
	{
		Slug:        "RemoveInvalidDefaults",
		Description: "Kubernetes publishes a {} default for any struct type. This doesn't make sense if the type is special with custom marshalling",
		Transformer: utils.PostorderVisitor(func(ctx utils.VisitingContext, s *spec.Schema) bool {
			if s.Default == nil || !(reflect.DeepEqual(s.Default, map[string]any{}) || reflect.DeepEqual(s.Default, map[any]any{})) {
				return true
			}

			// k8s forces default of {} for struct types
			// A bug in the code-generator makes it not realize these "struct" types
			//	have custom marshalling and OpenAPI types for which {} does not
			//	make sense
			// These are all struct types in upstream k8s which implement
			//	OpenAPISchemaType to something other than `struct`
			toWipe := sets.New(
				"io.k8s.apimachinery.pkg.api.resource.Quantity",
				"io.k8s.apimachinery.pkg.runtime.RawExtension",
				"io.k8s.apimachinery.pkg.util.intstr.IntOrString",
				"io.k8s.apimachinery.pkg.apis.meta.v1.Time",
				"io.k8s.apimachinery.pkg.apis.meta.v1.MicroTime",
				"io.k8s.apimachinery.pkg.apis.meta.v1.Duration",
				"io.k8s.apiextensions-apiserver.pkg.apis.apiextensions.v1.JSON",
				"io.k8s.apiextensions-apiserver.pkg.apis.apiextensions.v1.JSONSchemaPropsOrBool",
				"io.k8s.apiextensions-apiserver.pkg.apis.apiextensions.v1.JSONSchemaPropsOrStringArray",
			)
			shouldPatch := toWipe.Has(filepath.Base(s.Ref.String()))
			for _, subschema := range s.AllOf {
				if toWipe.Has(filepath.Base(subschema.Ref.String())) {
					shouldPatch = true
					break
				}
			}

			if !shouldPatch {
				return true
			}

			s.Default = nil
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
		} else if p.AppliesToDefinition != nil && !p.AppliesToDefinition(defName) {
			continue
		}

		utils.VisitSchema(defName, schema, p.Transformer)
	}
}
