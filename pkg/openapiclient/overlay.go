package openapiclient

import (
	"embed"
	"io/fs"
	"path/filepath"

	"k8s.io/client-go/openapi"
	"sigs.k8s.io/kubectl-validate/pkg/openapiclient/groupversion"
	"sigs.k8s.io/kubectl-validate/pkg/utils"
)

//go:embed patches
var patchesFS embed.FS

//go:embed patches_generated
var patchesGeneratedFS embed.FS

func HardcodedPatchLoader(version string) groupversion.PatchLoaderFn {
	return PatchLoaderFromDirectory(patchesFS, filepath.Join("patches", version))
}

func HardcodedGeneratedPatchLoader(version string) groupversion.PatchLoaderFn {
	return PatchLoaderFromDirectory(patchesGeneratedFS, filepath.Join("patches_generated", version))
}

func PatchLoaderFromDirectory(filesystem fs.FS, dir string) groupversion.PatchLoaderFn {
	if len(dir) == 0 && filesystem == nil {
		return nil
	}
	return func(s string) []byte {
		if res, err := utils.ReadFile(filesystem, filepath.Join(dir, s+".json")); err == nil {
			return res
		} else if res, err := utils.ReadFile(filesystem, filepath.Join(dir, s+".yaml")); err == nil {
			return res
		} else if res, err := utils.ReadFile(filesystem, filepath.Join(dir, s+".yml")); err == nil {
			return res
		}
		return nil
	}
}

type overlayClient struct {
	delegate    openapi.Client
	patchLoader groupversion.PatchLoaderFn
}

func NewOverlay(patchLoader groupversion.PatchLoaderFn, delegate openapi.Client) openapi.Client {
	return overlayClient{
		patchLoader: patchLoader,
		delegate:    delegate,
	}
}

func (o overlayClient) Paths() (map[string]openapi.GroupVersion, error) {
	delegateRes, err := o.delegate.Paths()
	if err != nil {
		return nil, err
	}

	if o.patchLoader == nil {
		return delegateRes, err
	}

	res := map[string]openapi.GroupVersion{}
	for k, v := range delegateRes {
		res[k] = groupversion.NewForOverlay(v, o.patchLoader, k)
	}
	return res, nil
}
