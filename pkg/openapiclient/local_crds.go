package openapiclient

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	"k8s.io/apiextensions-apiserver/pkg/apiserver"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/openapi"
	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"
	"sigs.k8s.io/kubectl-validate/pkg/utils"
)

// client which provides openapi read from files on disk
type localCRDsClient struct {
	dir string
}

type inmemoryGroupVersion struct {
	*spec3.OpenAPI
}

func (g inmemoryGroupVersion) Schema(contentType string) ([]byte, error) {
	if strings.ToLower(contentType) != "application/json" {
		return nil, fmt.Errorf("only application/json content type is supported")
	}
	return json.Marshal(g.OpenAPI)
}

// Dir should have openapi files following directory layout:
// myCRD.yaml (groupversions read from file)
func NewLocalCRDFiles(dirPath string) openapi.Client {
	return &localCRDsClient{dir: dirPath}
}

func (k *localCRDsClient) Paths() (map[string]openapi.GroupVersion, error) {
	if len(k.dir) == 0 {
		return nil, nil
	}
	files, err := os.ReadDir(k.dir)
	if err != nil {
		return nil, fmt.Errorf("error listing %s: %w", k.dir, err)
	}
	codecs := serializer.NewCodecFactory(apiserver.Scheme).UniversalDecoder()
	crds := map[schema.GroupVersion]*spec3.OpenAPI{}
	for _, f := range files {
		path := filepath.Join(k.dir, f.Name())
		if f.IsDir() {
			continue
		}

		if !utils.IsYamlOrJson(f.Name()) {
			continue
		}

		yamlFile, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", path, err)
		}

		crdObj, _, err := codecs.Decode(
			yamlFile,
			&schema.GroupVersionKind{
				Group:   "apiextensions.k8s.io",
				Version: runtime.APIVersionInternal,
				Kind:    "CustomResourceDefinition",
			}, nil)
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

			gvr := gvk.GroupVersion().WithResource(crd.Spec.Names.Plural)
			sch.AddExtension("x-kubernetes-group-version-kind", []interface{}{gvkObj})

			key := fmt.Sprintf("%s/%s.%s", gvk.Group, gvk.Version, gvk.Kind)
			if existing, exists := crds[gvr.GroupVersion()]; exists {
				existing.Components.Schemas[key] = sch
			} else {
				crds[gvr.GroupVersion()] = &spec3.OpenAPI{
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
		res[fmt.Sprintf("apis/%s/%s", k.Group, k.Version)] = inmemoryGroupVersion{v}
	}
	return res, nil
}
