package openapiclient

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"io/fs"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	"k8s.io/apiextensions-apiserver/pkg/apiserver"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/openapi"
	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"
	"sigs.k8s.io/kubectl-validate/pkg/openapiclient/groupversion"
	"sigs.k8s.io/kubectl-validate/pkg/utils"
)

//go:embed local_crds_metadata.json
var metadataSchemasJSON string

var metadataSchemas map[string]*spec.Schema = func() map[string]*spec.Schema {
	res := map[string]*spec.Schema{}
	if err := json.Unmarshal([]byte(metadataSchemasJSON), &res); err != nil {
		panic(err)
	}
	return res
}()

// client which provides openapi read from files on disk
type localCRDsClient struct {
	fileSystems []fs.FS
}

// Dir should have openapi files following directory layout:
// myCRD.yaml (groupversions read from file)
func NewLocalCRDFiles(fs ...fs.FS) openapi.Client {
	return &localCRDsClient{
		fileSystems: fs,
	}
}

func (k *localCRDsClient) Paths() (map[string]openapi.GroupVersion, error) {
	if len(k.fileSystems) == 0 {
		return nil, nil
	}
	var documents []utils.Document

	for _, current := range k.fileSystems {
		files, err := fs.ReadDir(current, ".")
		if err != nil {
			if crossPlatformCheckDirExists(current, ".") {
				return nil, fmt.Errorf("error listing: %w", err)
			}
		}
		for _, f := range files {
			path := f.Name()
			if f.IsDir() {
				continue
			}
			if utils.IsJson(f.Name()) {
				fileBytes, err := fs.ReadFile(current, path)
				if err != nil {
					return nil, fmt.Errorf("failed to read %s: %w", path, err)
				}
				documents = append(documents, fileBytes)
			} else if utils.IsYaml(f.Name()) {
				fileBytes, err := fs.ReadFile(current, path)
				if err != nil {
					return nil, fmt.Errorf("failed to read %s: %w", path, err)
				}
				yamlDocs, err := utils.SplitYamlDocuments(fileBytes)
				if err != nil {
					return nil, fmt.Errorf("failed to read %s: %w", path, err)
				}
				for _, document := range yamlDocs {
					if !utils.IsEmptyYamlDocument(document) {
						documents = append(documents, document)
					}
				}
			}
		}
	}

	codecs := serializer.NewCodecFactory(apiserver.Scheme).UniversalDecoder()
	crds := map[schema.GroupVersion]*spec3.OpenAPI{}
	crdGVK := schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Version: runtime.APIVersionInternal,
		Kind:    "CustomResourceDefinition",
	}
	for _, document := range documents {
		crdObj, parsedGVK, err := codecs.Decode(
			document,
			&crdGVK, nil)

		// If the error is that the GVK is not registered, or
		// this objects's GK is not what we were looking for,
		// then just skip it
		if runtime.IsNotRegisteredError(err) {
			continue
		} else if parsedGVK.GroupKind() != crdGVK.GroupKind() {
			continue
		}

		if err != nil {
			return nil, err
		}
		crd, ok := crdObj.(*apiextensions.CustomResourceDefinition)
		if !ok {
			return nil, fmt.Errorf("crd deserialized into incorrect type: %T", crdObj)
		}
		for _, v := range crd.Spec.Versions {
			// Convert schema to spec.Schema
			jsProps, err := apiextensions.GetSchemaForVersion(crd, v.Name)
			if err != nil {
				return nil, err
			}
			ss, err := structuralschema.NewStructural(jsProps.OpenAPIV3Schema)
			if err != nil {
				return nil, err
			}
			sch := ss.ToKubeOpenAPI()
			gvk := schema.GroupVersionKind{
				Group:   crd.Spec.Group,
				Version: v.Name,
				Kind:    crd.Spec.Names.Kind,
			}
			gvkObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&gvk)
			if err != nil {
				return nil, err
			}
			sch.AddExtension("x-kubernetes-group-version-kind", []interface{}{gvkObj})
			// Add schema extension to propagate the scope
			sch.AddExtension("x-kubectl-validate-scope", string(crd.Spec.Scope))
			key := fmt.Sprintf("%s/%s.%s", gvk.Group, gvk.Version, gvk.Kind)

			if sch.Properties == nil {
				sch.Properties = map[string]spec.Schema{}
			}

			// Emulate APIServer behavior by injecting ObjectMeta & its Dependencies into CRD
			sch.Properties["metadata"] = spec.Schema{
				SchemaProps: spec.SchemaProps{
					AllOf: []spec.Schema{
						{
							SchemaProps: spec.SchemaProps{
								Ref: spec.MustCreateRef("#/components/schemas/io.k8s.apimachinery.pkg.apis.meta.v1.ObjectMeta"),
							},
						},
					},
					Default:     map[string]interface{}{},
					Description: "Standard object metadata; More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata.",
				},
			}
			sch.Properties["apiVersion"] = spec.Schema{
				SchemaProps: spec.SchemaProps{
					Default:     "",
					Description: "API version of the referent.",
					Type:        spec.StringOrArray{"string"},
				},
			}
			sch.Properties["kind"] = spec.Schema{
				SchemaProps: spec.SchemaProps{
					Default:     "",
					Description: "Kind of the referent. More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds",
					Type:        spec.StringOrArray{"string"},
				},
			}

			if existing, exists := crds[gvk.GroupVersion()]; exists {
				existing.Components.Schemas[key] = sch
			} else {
				crds[gvk.GroupVersion()] = &spec3.OpenAPI{
					Components: &spec3.Components{
						Schemas: map[string]*spec.Schema{
							key: sch,
						},
					},
				}
			}
		}
	}

	res := map[string]openapi.GroupVersion{}
	for k, v := range crds {
		// Inject metadata definitions into each group-version document
		for defName, def := range metadataSchemas {
			v.Components.Schemas[defName] = def
		}
		res[fmt.Sprintf("apis/%s/%s", k.Group, k.Version)] = groupversion.NewForOpenAPI(v)
	}
	return res, nil
}
