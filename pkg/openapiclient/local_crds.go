package openapiclient

import (
	"encoding/json"
	"fmt"
	"io/fs"
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
	fs  fs.FS
	dir string
}

type inmemoryGroupVersion struct {
	*spec3.OpenAPI
}

func (g inmemoryGroupVersion) Schema(contentType string) ([]byte, error) {
	if strings.ToLower(contentType) != runtime.ContentTypeJSON {
		return nil, fmt.Errorf("only application/json content type is supported")
	}
	return json.Marshal(g.OpenAPI)
}

// Dir should have openapi files following directory layout:
// myCRD.yaml (groupversions read from file)
func NewLocalCRDFiles(fs fs.FS, dirPath string) openapi.Client {
	return &localCRDsClient{
		fs:  fs,
		dir: dirPath,
	}
}

func (k *localCRDsClient) Paths() (map[string]openapi.GroupVersion, error) {
	if len(k.dir) == 0 {
		return nil, nil
	}
	files, err := utils.ReadDir(k.fs, k.dir)
	if err != nil {
		return nil, fmt.Errorf("error listing %s: %w", k.dir, err)
	}
	var documents []utils.Document
	for _, f := range files {
		path := filepath.Join(k.dir, f.Name())
		if f.IsDir() {
			continue
		}
		if utils.IsJson(f.Name()) {
			fileBytes, err := utils.ReadFile(k.fs, path)
			if err != nil {
				return nil, fmt.Errorf("failed to read %s: %w", path, err)
			}
			documents = append(documents, fileBytes)
		} else if utils.IsYaml(f.Name()) {
			fileBytes, err := utils.ReadFile(k.fs, path)
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
	codecs := serializer.NewCodecFactory(apiserver.Scheme).UniversalDecoder()
	crds := map[schema.GroupVersion]*spec3.OpenAPI{}
	for _, document := range documents {
		crdObj, _, err := codecs.Decode(
			document,
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
			sch.AddExtension("x-kubernetes-group-version-kind", []interface{}{gvkObj})
			key := fmt.Sprintf("%s/%s.%s", gvk.Group, gvk.Version, gvk.Kind)
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
		res[fmt.Sprintf("apis/%s/%s", k.Group, k.Version)] = inmemoryGroupVersion{v}
	}
	return res, nil
}
