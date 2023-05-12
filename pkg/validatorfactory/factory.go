package validatorfactory

import (
	"encoding/json"
	"fmt"
	"path"
	"reflect"
	"strings"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/conversion"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/openapi"
	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"
	"k8s.io/kube-openapi/pkg/validation/strfmt"
	"k8s.io/kube-openapi/pkg/validation/validate"
)

type ValidatorFactory struct {
	gvs            map[string]openapi.GroupVersion
	validatorCache map[schema.GroupVersionKind]*ValidatorEntry
}

type ValidatorEntry struct {
	*spec.Schema
	name                    string
	namespaceScoped         bool
	structuralSchemaFactory structuralSchemaFactory
	schemaValidator         *validate.SchemaValidator
	ss                      *structuralschema.Structural
}

func newValidatorEntry(name string, namespaceScoped bool, openapiSchema *spec.Schema, ssf structuralSchemaFactory) *ValidatorEntry {
	return &ValidatorEntry{Schema: openapiSchema, name: name, namespaceScoped: namespaceScoped, structuralSchemaFactory: ssf}
}

func (v *ValidatorEntry) IsNamespaceScoped() bool {
	return v.namespaceScoped
}

func (v *ValidatorEntry) SchemaValidator() *validate.SchemaValidator {
	if v.schemaValidator != nil {
		return v.schemaValidator
	}

	v.schemaValidator = validate.NewSchemaValidator(v.Schema, nil, "", strfmt.Default)
	return v.schemaValidator
}

func (v *ValidatorEntry) ObjectTyper(gvk schema.GroupVersionKind) runtime.ObjectTyper {
	parameterScheme := runtime.NewScheme()
	parameterScheme.AddUnversionedTypes(schema.GroupVersion{Group: gvk.Group, Version: gvk.Version},
		&metav1.ListOptions{},
		&metav1.GetOptions{},
		&metav1.DeleteOptions{},
	)
	return newUnstructuredObjectTyper(parameterScheme)
}

func (v *ValidatorEntry) Decoder(gvk schema.GroupVersionKind) (runtime.NegotiatedSerializer, error) {
	ssMap := map[string]*structuralschema.Structural{}
	ss, err := v.StructuralSchema()
	if err != nil {
		return nil, err
	}

	ssMap[gvk.Version] = ss

	safeConverter, _, err := conversion.NewDelegatingConverter(&apiextensionsv1.CustomResourceDefinition{
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: gvk.Group,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind: gvk.Kind,
			},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name: gvk.Version,
				},
			},
		},
	}, conversion.NewNOPConverter())
	if err != nil {
		return nil, err
	}

	preserve, _ := v.Extensions.GetBool("x-kubernetes-preserve-unknown-fields")
	return unstructuredNegotiatedSerializer{
		typer:                 v.ObjectTyper(gvk),
		creator:               unstructuredCreator{},
		converter:             safeConverter,
		structuralSchemas:     ssMap,
		structuralSchemaGK:    gvk.GroupKind(),
		preserveUnknownFields: preserve,
	}, nil
}

func (v *ValidatorEntry) StructuralSchema() (*structuralschema.Structural, error) {
	if v.ss == nil {
		//!TODO: dont try to marshal a potentially recursive schema. should validate
		// that schema (except CRD) is not recursive before moving foreward
		jsonText, err := json.Marshal(v.Schema)
		if err != nil {
			return nil, err
		}

		propsdv1 := apiextensionsv1.JSONSchemaProps{}
		if err := json.Unmarshal(jsonText, &propsdv1); err != nil {
			return nil, err
		}

		propsd := apiextensions.JSONSchemaProps{}
		if err := apiextensionsv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(&propsdv1, &propsd, nil); err != nil {
			return nil, err
		}

		ss, err := structuralschema.NewStructural(&propsd)
		if err != nil {
			return nil, err
		}

		v.ss = ss
	}

	return v.ss, nil
	// return v.structuralSchemaFactory.ForDefinition(v.name)
}

// func (v *ValidatorEntry) CELValidator() (*cel.Validator, error) {
// 	if v.celValidator == nil {
// 		ss, err := v.StructuralSchema()
// 		if err != nil {
// 			return nil, err
// 		}

// 		// if ss != nil {
// 		//!TODO: switch CEL to use OpenAPI directly once that constructor
// 		// becomes avaialble
// 		// v.celValidator = cel.NewValidator(ss, false, cel.PerCallLimit)
// 		// }
// 	}
// 	return v.celValidator, nil
// }

func New(client openapi.Client) (*ValidatorFactory, error) {
	gvs, err := client.Paths()
	if err != nil {
		return nil, err
	}

	return &ValidatorFactory{
		gvs:            gvs,
		validatorCache: map[schema.GroupVersionKind]*ValidatorEntry{},
	}, nil
}

// Replaces subschemas that contain refs with copy of the thing they refer to
// No need for stack/queue approach since we mutate same dictionary/slice instances
// destructively.
// !TODO validate that no cyces are created by this process. If so, do not
// allow structural schema creation via JSON
// !TODO: track unresolved references?
func removeRefs(defs map[string]*spec.Schema, sch spec.Schema) spec.Schema {
	if r := sch.Ref.String(); len(r) > 0 {
		defName := path.Base(r)
		if resolved, ok := defs[defName]; ok {

			if defName == "io.k8s.apimachinery.pkg.util.intstr.IntOrString" {
				return spec.Schema{
					VendorExtensible: spec.VendorExtensible{
						Extensions: spec.Extensions{
							"x-kubernetes-int-or-string": true,
						},
					},
				}
			}
			return *resolved
		}
	}

	// SPECIAL CASE
	// OpenAPIV3 does not support having Refs in schemas with fields like
	// Description, Default filled in. So k8s stuffs the Ref into a standalone
	// AllOf in these cases.
	// But structural schema doesn't like schemas that specify fields inside AllOf
	// SO in the case of
	// Properties
	//	-> AllOf
	//		-> Ref
	// Where the schema containing AllOf only has `Description` or `Default` set,
	// we squash it so that the Ref is direct without AllOf

	if len(sch.AllOf) == 1 && len(sch.AllOf[0].Ref.String()) > 0 {
		vCopy := sch
		vCopy.AllOf = nil
		vCopy.Default = nil
		vCopy.Example = nil
		vCopy.Description = ""

		if reflect.DeepEqual(vCopy, spec.Schema{}) {
			return removeRefs(defs, sch.AllOf[0])
		}
	}

	for k, v := range sch.Properties {
		sch.Properties[k] = removeRefs(defs, v)
	}

	for k, v := range sch.AllOf {
		sch.AllOf[k] = removeRefs(defs, v)
	}

	if soa := sch.Items; soa != nil {
		if soa.Schema != nil {
			r := removeRefs(defs, *soa.Schema)
			soa.Schema = &r
		}

		for k, v := range soa.Schemas {
			soa.Schemas[k] = removeRefs(defs, v)
		}
	}

	if a := sch.AdditionalProperties; a != nil {
		if a.Schema != nil {
			r := removeRefs(defs, *a.Schema)
			a.Schema = &r
		}
	}

	if a := sch.AdditionalItems; a != nil {
		if a.Schema != nil {
			r := removeRefs(defs, *a.Schema)
			a.Schema = &r
		}
	}

	return sch
}

func getGVKsFromExtensions(extensions spec.Extensions) []schema.GroupVersionKind {
	var result []schema.GroupVersionKind
	if extensions == nil {
		return nil
	}
	gvks, ok := extensions["x-kubernetes-group-version-kind"]
	if !ok {
		return nil
	}
	var gvksList []interface{}
	if list, ok := gvks.([]interface{}); ok {
		gvksList = list
	} else if obj, ok := gvks.(map[string]interface{}); ok {
		gvksList = append(gvksList, obj)
	} else {
		return nil
	}
	for _, specGVK := range gvksList {
		if stringMap, ok := specGVK.(map[string]string); ok {
			g, ok1 := stringMap["group"]
			v, ok2 := stringMap["version"]
			k, ok3 := stringMap["kind"]
			if !ok1 || !ok2 || !ok3 {
				continue
			}
			result = append(result, schema.GroupVersionKind{
				Group:   g,
				Version: v,
				Kind:    k,
			})
		} else if anyMap, ok := specGVK.(map[string]interface{}); ok {
			gAny, ok1 := anyMap["group"]
			vAny, ok2 := anyMap["version"]
			kAny, ok3 := anyMap["kind"]
			if !ok1 || !ok2 || !ok3 {
				continue
			}
			g, ok1 := gAny.(string)
			v, ok2 := vAny.(string)
			k, ok3 := kAny.(string)
			if !ok1 || !ok2 || !ok3 {
				continue
			}
			result = append(result, schema.GroupVersionKind{
				Group:   g,
				Version: v,
				Kind:    k,
			})
		}
	}
	return result
}

func getGVKsFromPath(path *spec3.Path) []schema.GroupVersionKind {
	var result []schema.GroupVersionKind
	if path.Get != nil {
		result = append(result, getGVKsFromExtensions(path.Get.Extensions)...)
	}
	if path.Put != nil {
		result = append(result, getGVKsFromExtensions(path.Put.Extensions)...)
	}
	if path.Post != nil {
		result = append(result, getGVKsFromExtensions(path.Post.Extensions)...)
	}
	if path.Delete != nil {
		result = append(result, getGVKsFromExtensions(path.Delete.Extensions)...)
	}
	return result
}

func (s *ValidatorFactory) ValidatorsForGVK(gvk schema.GroupVersionKind) (*ValidatorEntry, error) {
	if existing, ok := s.validatorCache[gvk]; ok {
		return existing, nil
	}

	// Otherwise, fetch the open API schema for this GV and do the above
	// Lookup gvk in client
	// Guess the rest mapping since we don't have a rest mapper for the target
	// cluster
	path := "apis/" + gvk.Group + "/" + gvk.Version
	if len(gvk.Group) == 0 {
		path = "api/" + gvk.Version
	}
	gvFetcher, exists := s.gvs[path]
	if !exists {
		return nil, fmt.Errorf("failed to locate OpenAPI spec for GV: %v", gvk.GroupVersion())
	}

	documentBytes, err := gvFetcher.Schema("application/json")
	if err != nil {
		return nil, fmt.Errorf("error fetching openapi at path %s: %w", path, err)
	}

	openapiSpec := spec3.OpenAPI{}
	if err := json.Unmarshal(documentBytes, &openapiSpec); err != nil {
		return nil, fmt.Errorf("error parsing openapi spec: %w", err)
	}

	openapiSpec2 := spec3.OpenAPI{}
	if err := json.Unmarshal(documentBytes, &openapiSpec2); err != nil {
		return nil, fmt.Errorf("error parsing openapi spec: %w", err)
	}

	ssf := NewStructuralSchemaFactory(openapiSpec2.Components.Schemas)

	namespaced := sets.New[schema.GroupVersionKind]()
	if openapiSpec.Paths != nil {
		for path, pathInfo := range openapiSpec.Paths.Paths {
			for _, gvk := range getGVKsFromPath(pathInfo) {
				if !namespaced.Has(gvk) {
					if strings.Contains(path, "namespaces/{namespace}") {
						namespaced.Insert(gvk)
					}
				}
			}
		}
	}

	for nam, def := range openapiSpec.Components.Schemas {
		removeRefs(openapiSpec.Components.Schemas, *def)

		gvks := getGVKsFromExtensions(def.Extensions)
		if len(gvks) == 0 {
			continue
		}

		val := newValidatorEntry(nam, namespaced.Has(gvk), def, ssf)

		for _, specGVK := range gvks {
			s.validatorCache[specGVK] = val
		}
	}

	// Check again to see if the desired GVK was added to the spec cache.
	// If so, create validator for it
	if existing, ok := s.validatorCache[gvk]; ok {
		return existing, nil
	}

	return nil, fmt.Errorf("kind %v not found in %v groupversion", gvk.Kind, gvk.GroupVersion())
}
