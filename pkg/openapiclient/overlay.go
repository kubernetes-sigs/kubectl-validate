package openapiclient

import (
	"embed"
	"errors"
	"io/fs"
	"path/filepath"

	jsonpatch "github.com/evanphx/json-patch"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/openapi"
	"sigs.k8s.io/kubectl-validate/pkg/utils"
)

//go:embed patches
var patchesFS embed.FS

type PatchLoaderFn = func(string) []byte

func HardcodedPatchLoader(version string) PatchLoaderFn {
	return PatchLoaderFromDirectory(patchesFS, filepath.Join("patches", version))
}

func PatchLoaderFromDirectory(filesystem fs.FS, dir string) PatchLoaderFn {
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
	patchLoader PatchLoaderFn
}

type overlayGroupVersion struct {
	delegate    openapi.GroupVersion
	patchLoader PatchLoaderFn
	path        string
}

func NewOverlay(patchLoader PatchLoaderFn, delegate openapi.Client) openapi.Client {
	return overlayClient{
		patchLoader: patchLoader,
		delegate:    delegate,
	}
}

func (g overlayGroupVersion) Schema(contentType string) ([]byte, error) {
	patch := g.patchLoader(g.path)
	if patch == nil {
		return g.delegate.Schema(contentType)
	}

	if contentType != runtime.ContentTypeJSON {
		return nil, errors.New("unsupported content type")
	}
	delegateRes, err := g.delegate.Schema(contentType)
	if err != nil {
		return nil, err
	}

	patchedJS, err := jsonpatch.MergePatch(delegateRes, patch)
	if err == jsonpatch.ErrBadJSONPatch {
		return nil, k8serrors.NewBadRequest(err.Error())
	}
	return patchedJS, err
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
		res[k] = overlayGroupVersion{
			delegate:    v,
			patchLoader: o.patchLoader,
			path:        k,
		}
	}
	return res, nil
}
