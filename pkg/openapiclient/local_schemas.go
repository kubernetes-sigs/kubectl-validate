package openapiclient

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"k8s.io/client-go/openapi"
	"sigs.k8s.io/kubectl-validate/pkg/openapiclient/groupversion"
	"sigs.k8s.io/kubectl-validate/pkg/utils"
)

// client which provides openapi read from files on disk
type localSchemasClient struct {
	fs  fs.FS
	dir string
}

// Dir should have openapi files following directory layout:
// /<apis>/<group>/<version>.json
// /api/<version>.json
func NewLocalSchemaFiles(fs fs.FS, dirPath string) openapi.Client {
	return &localSchemasClient{
		fs:  fs,
		dir: dirPath,
	}
}

func (k *localSchemasClient) Paths() (map[string]openapi.GroupVersion, error) {
	if len(k.dir) == 0 {
		return nil, nil
	}
	res := map[string]openapi.GroupVersion{}
	apiGroups, err := utils.ReadDir(k.fs, filepath.Join(k.dir, "apis"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("failed reading local files dir %s: %w", filepath.Join(k.dir, "apis"), err)
	}
	for _, f := range apiGroups {
		groupPath := filepath.Join(k.dir, "apis", f.Name())
		versions, err := utils.ReadDir(k.fs, groupPath)
		if err != nil {
			return nil, fmt.Errorf("failed reading local files dir %s: %w", groupPath, err)
		}
		for _, v := range versions {
			if !utils.IsJson(v.Name()) {
				continue
			}
			name := strings.TrimSuffix(v.Name(), filepath.Ext(v.Name()))
			path := filepath.Join("apis", f.Name(), name)
			res[path] = groupversion.NewForFile(k.fs, filepath.Join(groupPath, v.Name()))
		}
	}
	coregroup, err := utils.ReadDir(k.fs, filepath.Join(k.dir, "api"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("failed reading local files dir %s: %w", filepath.Join(k.dir, "api"), err)
	}
	for _, v := range coregroup {
		if !utils.IsJson(v.Name()) {
			continue
		}
		name := strings.TrimSuffix(v.Name(), filepath.Ext(v.Name()))
		path := filepath.Join("api", name)
		res[path] = groupversion.NewForFile(k.fs, filepath.Join(k.dir, "api", v.Name()))
	}
	return res, nil
}
