package openapiclient

import (
	"embed"
	"io/fs"
	"path"

	"k8s.io/client-go/openapi"
	"sigs.k8s.io/kubectl-validate/pkg/openapiclient/groupversion"
)

//go:embed patches
var patchesFS embed.FS

func HardcodedPatchLoader(version string) groupversion.PatchLoaderFn {
	sub, err := fs.Sub(patchesFS, path.Join("patches", version))
	if err != nil {
		return nil
	}
	return PatchLoaderFromDirectory(sub)
}

func PatchLoaderFromDirectory(filesystem fs.FS) groupversion.PatchLoaderFn {
	if filesystem == nil {
		return nil
	}
	return func(s string) []byte {
		if res, err := fs.ReadFile(filesystem, path.Join(s+".json")); err == nil {
			return res
		} else if res, err := fs.ReadFile(filesystem, path.Join(s+".yaml")); err == nil {
			return res
		} else if res, err := fs.ReadFile(filesystem, path.Join(s+".yml")); err == nil {
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
