package groupversion

import (
	"fmt"
	"io/fs"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/openapi"
)

type fileGroupVersion struct {
	fs       fs.FS
	filepath string
}

func (gv *fileGroupVersion) Schema(contentType string) ([]byte, error) {
	if strings.ToLower(contentType) != runtime.ContentTypeJSON {
		return nil, fmt.Errorf("only application/json content type is supported")
	}
	return fs.ReadFile(gv.fs, gv.filepath)
}

func (gv *fileGroupVersion) ServerRelativeURL() string {
	return ""
}

func NewForFile(fs fs.FS, filepath string) openapi.GroupVersion {
	return &fileGroupVersion{fs, filepath}
}
