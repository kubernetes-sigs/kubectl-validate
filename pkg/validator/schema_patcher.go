package validator

import (
	"path"
	"reflect"
	"strings"

	"k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/kube-openapi/pkg/validation/spec"
	"sigs.k8s.io/kubectl-validate/pkg/utils"
)

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

// These are native types in k8s which have a custom `MarshalJSON` which handles
// `null`
var nullableSchemas sets.Set[string] = sets.New(
	"io.k8s.apimachinery.pkg.runtime.RawExtension",
	"io.k8s.apimachinery.pkg.apis.meta.v1.Time",
	"io.k8s.apimachinery.pkg.apis.meta.v1.MicroTime",
	"io.k8s.apimachinery.pkg.apis.meta.v1.Duration",
	"io.k8s.apiextensions-apiserver.pkg.apis.apiextensions.v1.JSON",
	"io.k8s.apiextensions-apiserver.pkg.apis.apiextensions.v1.JSONSchemaPropsOrBool",
	"io.k8s.apiextensions-apiserver.pkg.apis.apiextensions.v1.JSONSchemaPropsOrStringArray",
	"io.k8s.apimachinery.pkg.api.resource.Quantity",
)

var invalidDefaultSchemas sets.Set[string] = func() sets.Set[string] {
	res := nullableSchemas.Clone()
	res.Insert(
		"io.k8s.apimachinery.pkg.util.intstr.IntOrString",
	)

	return res
}()

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

var zero int64 = int64(0)
var schemaPatches []SchemaPatch = []SchemaPatch{
	{
		Slug:            "AllowEmptyByteFormat",
		Description:     "Work around discrepency between treatment of native vs CRD `byte` strings. Native types allow empty, CRDs do not",
		MinMinorVersion: 0,
		MaxMinorVersion: 0,
		AppliesToGV:     isBuiltInType,
		Transformer: utils.PostorderVisitor(func(ctx utils.VisitingContext, s *spec.Schema) *spec.Schema {
			if s.Format != "byte" || len(s.Type) != 1 || s.Type[0] != "string" {
				return s
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
			return s
		}),
	},
	{
		Slug:                "AnnotateNullable",
		AppliesToDefinition: nullableSchemas.Has,
		Description:         "Some published OpenAPI definitions do not allow empty/null, but Kubernetes in practice does.",
		Transformer: utils.PostorderVisitor(func(ctx utils.VisitingContext, s *spec.Schema) *spec.Schema {
			s.Nullable = true
			return s
		}),
	},
	{
		Slug:        "RemoveInvalidDefaults",
		Description: "Kubernetes publishes a {} default for any struct type. This doesn't make sense if the type is special with custom marshalling",
		Transformer: utils.PostorderVisitor(func(ctx utils.VisitingContext, s *spec.Schema) *spec.Schema {
			if s.Default == nil || !(reflect.DeepEqual(s.Default, map[string]any{}) || reflect.DeepEqual(s.Default, map[any]any{})) {
				return s
			}

			shouldPatch := invalidDefaultSchemas.Has(path.Base(s.Ref.String()))
			for _, subschema := range s.AllOf {
				if invalidDefaultSchemas.Has(path.Base(subschema.Ref.String())) {
					shouldPatch = true
					break
				}
			}

			if shouldPatch {
				s.Default = nil
			}

			return s
		}),
	},
	{
		Slug:        "StripUnsupportedFormats",
		Description: "Some formats are not supported by the Kubernetes API server",
		Transformer: utils.PostorderVisitor(func(ctx utils.VisitingContext, s *spec.Schema) *spec.Schema {
			validation.StripUnsupportedFormatsPostProcess(s)
			return s
		}),
	},
	{
		Slug:        "IntOrStringSupplement",
		Description: "Some native k8s schemas use oneOf for IntOrString, but CRDs use x-kubernetes-int-or-string and only recognizes anyOf patterns",
		Transformer: utils.PostorderVisitor(func(ctx utils.VisitingContext, s *spec.Schema) *spec.Schema {
			if v, ok := s.Extensions.GetBool("x-kubernetes-int-or-string"); ok && v {
				return s
			}

			if len(s.Type) == 0 && len(s.OneOf) == 2 && len(s.OneOf[0].Type) > 0 && len(s.OneOf[1].Type) > 0 {
				oneOfTypes := sets.New[string](s.OneOf[0].Type[0], s.OneOf[1].Type[0])
				if oneOfTypes.Has("string") && oneOfTypes.Has("integer") {
					extCopy := make(spec.Extensions, len(s.Extensions))
					for k, v := range s.Extensions {
						extCopy[k] = v
					}
					s.Extensions = extCopy
					s.AddExtension("x-kubernetes-int-or-string", true)

					// OneOf is not valid in structural schema, so better to avoid it
					// in favor of the x-kubernetes extension
					if len(oneOfTypes) == 2 {
						s.OneOf = nil
					}
				}
			}
			return s
		}),
	},
}

func ApplySchemaPatches(k8sVersion int, gv schema.GroupVersion, defName string, schema *spec.Schema) *spec.Schema {
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
		schema = utils.VisitSchema(defName, schema, p.Transformer)
	}
	return schema
}
