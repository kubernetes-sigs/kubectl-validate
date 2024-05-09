package validator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"

	"golang.org/x/exp/maps"
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	"k8s.io/apiextensions-apiserver/pkg/registry/customresource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/openapi"
	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"
	"sigs.k8s.io/kubectl-validate/pkg/utils"
	"sigs.k8s.io/yaml"
)

type Validator struct {
	gvs            map[string]openapi.GroupVersion
	validatorCache map[schema.GroupVersionKind]*validatorEntry
}

func New(client openapi.Client) (*Validator, error) {
	gvs, err := client.Paths()
	if err != nil {
		return nil, err
	}

	return &Validator{
		gvs:            gvs,
		validatorCache: map[schema.GroupVersionKind]*validatorEntry{},
	}, nil
}

// Parse parses JSON or YAML text and parses it into unstructured.Unstructured.
// Unset fields with defaults in their schema will have the defaults populated.
//
// It will return errors when there is an issue parsing the object, or if
// it contains fields unknown to the schema, or if the schema was recursive.
func (s *Validator) Parse(document []byte) (schema.GroupVersionKind, *unstructured.Unstructured, error) {
	// Initialize an empty unstructured object
	obj := &unstructured.Unstructured{}

	// Parse the entire document into the unstructured object
	if err := yaml.Unmarshal(document, obj); err != nil {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("failed to parse yaml: %w", err)
	}

	// get the object name
	objName := obj.GetName()
	if len(objName) == 0 {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("object name cannot be empty")
	}

	// Get the GroupVersionKind of the object
	gvk := obj.GroupVersionKind()

	// gvk := metadata.GetObjectKind().GroupVersionKind()
	if gvk.Empty() {
		return schema.GroupVersionKind{}, nil, fmt.Errorf("GVK cannot be empty")
	}

	validators, err := s.infoForGVK(gvk)
	if err != nil {
		return gvk, nil, fmt.Errorf("failed to retrieve validator: %w", err)
	}

	// Fetch a decoder to decode this object from its structural schema
	decoder, err := validators.Decoder(gvk)
	if err != nil {
		return gvk, nil, err
	}

	const mediaType = runtime.ContentTypeYAML
	info, ok := runtime.SerializerInfoForMediaType(decoder.SupportedMediaTypes(), mediaType)
	if !ok {
		return gvk, nil, fmt.Errorf("unsupported media type %q", mediaType)
	}

	dec := decoder.DecoderToVersion(info.StrictSerializer, gvk.GroupVersion())
	runtimeObj, _, err := dec.Decode(document, &gvk, &unstructured.Unstructured{})
	if err != nil {
		return gvk, nil, fmt.Errorf("failed to parse YAML for object - Name: %s version: %s, kind: %s\n%w", objName, gvk.Version, gvk.Kind, err)
	}

	return gvk, runtimeObj.(*unstructured.Unstructured), nil
}

// Validate takes a parsed resource as input and validates it against
// its schema.
func (s *Validator) Validate(obj *unstructured.Unstructured) error {
	if obj == nil || obj.Object == nil {
		return errors.New("passed object cannot be nil")
	}
	// shallow copy input object, this method can modify apiVersion, kind, or metadata
	obj = &unstructured.Unstructured{Object: maps.Clone(obj.UnstructuredContent())}
	// deep copy metadata object
	obj.Object["metadata"] = runtime.DeepCopyJSONValue(obj.Object["metadata"])
	gvk := obj.GroupVersionKind()
	validators, err := s.infoForGVK(gvk)
	if err != nil {
		return fmt.Errorf("failed to retrieve validator: %w", err)
	}

	isNamespaced := validators.IsNamespaceScoped()
	if isNamespaced && obj.GetNamespace() == "" {
		obj.SetNamespace("default")
	}

	if obj.GetAPIVersion() == "v1" {
		// CRD validator expects unconditoinal slashes and nonempty group,
		// since it is not originally intended for built-in
		gvk.Group = "core"
		obj.SetAPIVersion("core/v1")
	}

	ss, err := validators.StructuralSchema()
	if err != nil || ss == nil {
		return err
	}

	strat := customresource.NewStrategy(validators.ObjectTyper(gvk), isNamespaced, gvk, validators.SchemaValidator(), nil,
		ss,
		nil, nil)

	rest.FillObjectMetaSystemFields(obj)
	return rest.BeforeCreate(strat, request.WithNamespace(context.TODO(), obj.GetNamespace()), obj)
}

func (s *Validator) infoForGVK(gvk schema.GroupVersionKind) (*validatorEntry, error) {
	if existing, ok := s.validatorCache[gvk]; ok {
		return existing, nil
	}

	// Otherwise, fetch the open API schema for this GV and do the above
	// Lookup gvk in client
	// Guess the rest mapping since we don't have a rest mapper for the target
	// cluster
	gvPath := "apis/" + gvk.Group + "/" + gvk.Version
	if len(gvk.Group) == 0 {
		gvPath = "api/" + gvk.Version
	}
	gvFetcher, exists := s.gvs[gvPath]
	if !exists {
		return nil, fmt.Errorf("failed to locate OpenAPI spec for GV: %v", gvk.GroupVersion())
	}

	documentBytes, err := gvFetcher.Schema("application/json")
	if err != nil {
		return nil, fmt.Errorf("error fetching openapi at path %s: %w", gvPath, err)
	}

	openapiSpec := spec3.OpenAPI{}
	if err := json.Unmarshal(documentBytes, &openapiSpec); err != nil {
		return nil, fmt.Errorf("error parsing openapi spec: %w", err)
	}

	// Apply our transformations to workaround known k8s schema deficiencies
	for nam, def := range openapiSpec.Components.Schemas {
		//!TODO: would be useful to know which version of k8s each schema is believed
		// to come from.
		openapiSpec.Components.Schemas[nam] = ApplySchemaPatches(0, gvk.GroupVersion(), nam, def)
	}

	// Remove all references/indirection.
	// This is kinda hacky because we still do allow recursive schemas via
	// pointer trickery.
	// No need for stack/queue approach since we mutate same dictionary/slice instances
	// destructively.
	// Replaces subschemas that contain refs with copy of the thing they refer to
	// !TODO validate that no cyces are created by this process. If so, do not
	// allow structural schema creation via JSON
	// !TODO: track unresolved references?
	// !TODO: Once Declarative Validation for native types lands we will be
	//	able to validate against the spec.Schema directly rather than
	//	StructuralSchema, so this will be able to be removed
	var referenceErrors []error
	for nam, def := range openapiSpec.Components.Schemas {
		// This hack only works because top level schemas never have references
		// so we can reliably copy them knowing they wont change and pointer-share
		// their subfields. The only schemas being modified here should be sub-fields.
		openapiSpec.Components.Schemas[nam] = utils.VisitSchema(nam, def, utils.PreorderVisitor(func(ctx utils.VisitingContext, sch *spec.Schema) (*spec.Schema, bool) {
			defName := sch.Ref.String()

			if len(sch.AllOf) == 1 && len(sch.AllOf[0].Ref.String()) > 0 {
				// SPECIAL CASE
				// OpenAPIV3 does not support having Refs in schemas with fields like
				// Description, Default filled in. So k8s stuffs the Ref into a standalone
				// AllOf in these cases.
				// But structural schema doesn't like schemas that specify fields inside AllOf
				// SO in the case of
				// Properties
				//	-> AllOf
				//		-> Ref
				defName = sch.AllOf[0].Ref.String()
			}

			if len(defName) == 0 {
				// Nothing to do for no references
				return sch, true
			}

			defName = path.Base(defName)
			resolved, ok := openapiSpec.Components.Schemas[defName]
			if !ok {
				// Can't resolve schema. This is an error.
				var path []string
				for cursor := &ctx; cursor != nil; cursor = cursor.Parent {
					if len(cursor.Key) == 0 {
						path = append(path, fmt.Sprint(cursor.Index))
					} else {
						path = append(path, cursor.Key)
					}
				}
				sort.Stable(sort.Reverse(sort.StringSlice(path)))
				referenceErrors = append(referenceErrors, fmt.Errorf("cannot resolve reference %v in %v.%v", defName, nam, strings.Join(path, ".")))
				return sch, true
			}

			resolvedCopy := *resolved

			if sch.Default != nil {
				resolvedCopy.Default = sch.Default
			}

			// NOTE: No way to tell if field overrides nullable
			// or if it is unset. Right now if the referred schema is
			// nullable we will resolve to a nullable schema.
			// There are no upstream schemas where nullable is used as a field
			// level override, so we will assume `false` means `unset`.
			// But this should be fixed in kube-openapi.
			resolvedCopy.Nullable = resolvedCopy.Nullable || sch.Nullable

			if len(sch.Type) > 0 {
				resolvedCopy.Type = sch.Type
			}

			if len(sch.Description) > 0 {
				resolvedCopy.Description = sch.Description
			}

			newExtensions := spec.Extensions{}
			for k, v := range resolvedCopy.Extensions {
				newExtensions.Add(k, v)
			}
			for k, v := range sch.Extensions {
				newExtensions.Add(k, v)
			}
			if len(newExtensions) > 0 {
				resolvedCopy.Extensions = newExtensions
			}

			// Sanity check that schemas generated by k8s do not have a ref
			// AND override fields other than these
			// schCopy := *sch
			// if len(sch.AllOf) == 1 && len(sch.AllOf[0].Ref.String()) > 0 {
			// 	  schCopy.AllOf = nil
			// }
			// schCopy.Extensions = nil
			// schCopy.Type = nil
			// schCopy.Description = ""
			// schCopy.Nullable = false
			// schCopy.Default = nil

			// if !reflect.DeepEqual(schCopy, spec.Schema{}) {
			// 	referenceErrors = append(referenceErrors, fmt.Errorf("not empty %#v", sch))
			// }

			// Don't explore children. This was a reference node and shares
			// pointers with its schema which will be traversed in this loop.
			return &resolvedCopy, false
		}))
	}

	if len(referenceErrors) > 0 {
		return nil, errors.Join(referenceErrors...)
	}

	namespaced := sets.New[schema.GroupVersionKind]()
	if openapiSpec.Paths != nil {
		for path, pathInfo := range openapiSpec.Paths.Paths {
			for _, gvk := range utils.ExtractPathGVKs(pathInfo) {
				if !namespaced.Has(gvk) {
					if strings.Contains(path, "namespaces/{namespace}") {
						namespaced.Insert(gvk)
					}
				}
			}
		}
	}

	for nam, def := range openapiSpec.Components.Schemas {
		gvks := utils.ExtractExtensionGVKs(def.Extensions)
		if len(gvks) == 0 {
			continue
		}

		// Try to infer the scope from paths
		nsScoped := namespaced.Has(gvk)
		// Check schema extensions to see if the scope was manually added
		if scope, ok := def.Extensions.GetString("x-kubectl-validate-scope"); ok {
			nsScoped = strings.EqualFold(scope, string(apiextensions.NamespaceScoped))
		}

		val := newValidatorEntry(nam, nsScoped, def)

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
