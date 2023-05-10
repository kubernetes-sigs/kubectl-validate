package openapiclient

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	"k8s.io/apiextensions-apiserver/pkg/apiserver"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/client-go/openapi"
	"k8s.io/kube-openapi/pkg/validation/spec"
	"sigs.k8s.io/kubectl-validate/pkg/utils"
)

// client which provides openapi read from files on disk
type localFilesClient struct {
	dir string
}

type localGroupVersion struct {
	fs       fs.FS
	filepath string
}

func (g localGroupVersion) Schema(contentType string) ([]byte, error) {
	if strings.ToLower(contentType) != "application/json" {
		return nil, fmt.Errorf("only application/json content type is supported")
	}

	if g.fs == nil {
		return os.ReadFile(g.filepath)
	}
	return fs.ReadFile(g.fs, g.filepath)
}

// Dir should have openapi files following directory layout:
// /<apis>/<group>/<version>.json
// /api/<version>.json
func NewLocalFiles(dirPath string) openapi.Client {
	return &localFilesClient{dir: dirPath}
}

func (k *localFilesClient) Paths() (map[string]openapi.GroupVersion, error) {
	if len(k.dir) == 0 {
		return nil, nil
	}
	files, err := os.ReadDir(k.dir)
	if err != nil {
		return nil, fmt.Errorf("error listing %s: %w", k.dir, err)
	}

	codecs := serializer.NewCodecFactory(apiserver.Scheme).UniversalDecoder()
	crds := map[schema.GroupVersionResource]*spec.Schema{}
	for _, f := range files {
		path := filepath.Join(k.dir, f.Name())
		if info, err := os.Stat(path); err != nil {
			return nil, err
		} else if info.IsDir() {
			continue
		}

		if !utils.IsYamlOrJson(f.Name()) {
			continue
		}

		yamlFile, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", path, err)
		}

		crdObj, _, err := codecs.Decode(yamlFile, nil, &apiextensions.CustomResourceDefinition{})
		if err != nil {
			return nil, err
		}

		crd, ok := crdObj.(*apiextensions.CustomResourceDefinition)
		if !ok {
			return nil, fmt.Errorf("crd deserialized into incorrect type: %T", crdObj)
		}

		for _, v := range crd.Spec.Versions {
			// Convert schema to spec.Schema
			jsonSchema, err := json.Marshal(v.Schema.OpenAPIV3Schema)
			if err != nil {
				return nil, err
			}
			sch := spec.Schema{}
			if err := json.Unmarshal(jsonSchema, &sch); err != nil {
				return nil, err
			}
			gvk := schema.GroupVersionKind{
				Group:   crd.Spec.Group,
				Version: v.Name,
				Kind:    crd.Spec.Names.Kind,
			}
			gvkObj, err := runtime.DefaultUnstructuredConverter.ToUnstructured(gvk)
			if err != nil {
				return nil, err
			}

			gvr := gvk.GroupVersion().WithResource(crd.Spec.Names.Plural)
			sch.AddExtension("x-kubernetes-group-version-kind", gvkObj)
			crds[gvr] = &sch
		}
	}

	apiGroups, _ := os.ReadDir(filepath.Join(k.dir, "apis"))
	coregroup, _ := os.ReadDir(filepath.Join(k.dir, "api"))

	res := map[string]openapi.GroupVersion{}
	for _, f := range apiGroups {
		groupPath := filepath.Join(k.dir, "apis", f.Name())
		versions, err := os.ReadDir(groupPath)
		if err != nil {
			return nil, fmt.Errorf("failed reading local files dir %s: %w", groupPath, err)
		}

		for _, v := range versions {
			if utils.IsJson(v.Name()) {
				continue
			}
			name := strings.TrimSuffix(v.Name(), filepath.Ext(v.Name()))
			path := filepath.Join("apis", f.Name(), name)
			res[path] = localGroupVersion{filepath: filepath.Join(groupPath, v.Name())}
		}
	}

	for _, v := range coregroup {
		if utils.IsJson(v.Name()) {
			continue
		}
		name := strings.TrimSuffix(v.Name(), filepath.Ext(v.Name()))
		path := filepath.Join("api", name)
		res[path] = localGroupVersion{filepath: filepath.Join(k.dir, "api", v.Name())}
	}

	return res, nil
}
