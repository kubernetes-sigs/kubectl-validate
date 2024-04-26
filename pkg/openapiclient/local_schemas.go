package openapiclient

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"strings"

	"k8s.io/client-go/openapi"
	"sigs.k8s.io/kubectl-validate/pkg/openapiclient/groupversion"
	"sigs.k8s.io/kubectl-validate/pkg/utils"
)

// client which provides openapi read from files on disk
type localSchemasClient struct {
	fs fs.FS
}

// Dir should have openapi files following directory layout:
// /<apis>/<group>/<version>.json
// /api/<version>.json
func NewLocalSchemaFiles(fs fs.FS) openapi.Client {
	return &localSchemasClient{
		fs: fs,
	}
}

func (k *localSchemasClient) Paths() (map[string]openapi.GroupVersion, error) {
	if k.fs == nil {
		return nil, nil
	}
	// check if '.' can be listed
	if _, err := fs.ReadDir(k.fs, "."); err != nil {
		if crossPlatformCheckDirExists(k.fs, ".") {
			return nil, fmt.Errorf("error listing %s: %w", ".", err)
		}
	}
	res := map[string]openapi.GroupVersion{}
	apiGroups, err := fs.ReadDir(k.fs, "apis")
	if err != nil {
		if crossPlatformCheckDirExists(k.fs, "apis") {
			return nil, fmt.Errorf("error listing %s: %w", "apis", err)
		}
	}
	for _, f := range apiGroups {
		groupPath := path.Join("apis", f.Name())
		versions, err := fs.ReadDir(k.fs, groupPath)
		if err != nil {
			return nil, fmt.Errorf("error listing %s: %w", groupPath, err)
		}
		for _, v := range versions {
			if !utils.IsJson(v.Name()) {
				continue
			}
			name := strings.TrimSuffix(v.Name(), path.Ext(v.Name()))
			apisPath := path.Join("apis", f.Name(), name)
			res[apisPath] = groupversion.NewForFile(k.fs, path.Join(groupPath, v.Name()))
		}
	}
	coregroup, err := fs.ReadDir(k.fs, "api")
	if err != nil {
		if crossPlatformCheckDirExists(k.fs, "api") {
			return nil, fmt.Errorf("error listing %s: %w", "api", err)
		}
	}
	for _, v := range coregroup {
		if !utils.IsJson(v.Name()) {
			continue
		}
		name := strings.TrimSuffix(v.Name(), path.Ext(v.Name()))
		apiPath := path.Join("api", name)
		res[apiPath] = groupversion.NewForFile(k.fs, path.Join("api", v.Name()))
	}
	return res, nil
}

func crossPlatformCheckDirExists(f fs.FS, path string) bool {
	_, err := fs.Stat(f, path)
	if err != nil {
		return !errors.Is(err, fs.ErrNotExist)
	}
	return true
}
