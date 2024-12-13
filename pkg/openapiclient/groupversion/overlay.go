package groupversion

import (
	"errors"

	jsonpatch "github.com/evanphx/json-patch"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/openapi"
)

type PatchLoaderFn = func(string) []byte

type overlayGroupVersion struct {
	delegate    openapi.GroupVersion
	patchLoader PatchLoaderFn
	path        string
}

func (gv *overlayGroupVersion) Schema(contentType string) ([]byte, error) {
	patch := gv.patchLoader(gv.path)
	if patch == nil {
		return gv.delegate.Schema(contentType)
	}

	if contentType != runtime.ContentTypeJSON {
		return nil, errors.New("unsupported content type")
	}
	delegateRes, err := gv.delegate.Schema(contentType)
	if err != nil {
		return nil, err
	}

	patchedJS, err := jsonpatch.MergePatch(delegateRes, patch)
	if err == jsonpatch.ErrBadJSONPatch {
		return nil, k8serrors.NewBadRequest(err.Error())
	}
	return patchedJS, err
}

func (gv *overlayGroupVersion) ServerRelativeURL() string {
	return ""
}

func NewForOverlay(delegate openapi.GroupVersion, patchLoader PatchLoaderFn, path string) openapi.GroupVersion {
	return &overlayGroupVersion{delegate, patchLoader, path}
}
