package groupversion

import (
	"fmt"
	"io"
	"net/http"

	"k8s.io/client-go/openapi"
)

type httpGroupVersion struct {
	uri string
}

func (gv *httpGroupVersion) Schema(contentType string) ([]byte, error) {
	//TODO: responses use and respect ETAG. use a disk cache
	req, err := http.NewRequest("GET", gv.uri, nil)
	if err != nil {
		return nil, fmt.Errorf("error creating request: %w", err)
	}
	req.Header.Set("Accept", contentType)
	// Make HTTP request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func (gv *httpGroupVersion) ServerRelativeURL() string {
	return ""
}

func NewForHttp(uri string) openapi.GroupVersion {
	return &httpGroupVersion{uri}
}
