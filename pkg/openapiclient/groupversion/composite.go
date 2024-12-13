package groupversion

import (
	"encoding/json"
	"fmt"

	"k8s.io/client-go/openapi"
	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

type compositeGroupVersion struct {
	gvFetchers []openapi.GroupVersion
}

func (gv *compositeGroupVersion) Schema(contentType string) ([]byte, error) {
	if len(gv.gvFetchers) == 0 {
		return nil, fmt.Errorf("no fetches for groupversion")
	} else if len(gv.gvFetchers) == 1 {
		return gv.gvFetchers[0].Schema(contentType)
	}

	combined := spec3.OpenAPI{
		Components: &spec3.Components{
			Schemas: map[string]*spec.Schema{},
		},
	}

	for _, fetcher := range gv.gvFetchers {
		fetched, err := fetcher.Schema(contentType)
		if err != nil {
			return nil, err
		}

		var parsed spec3.OpenAPI
		if err := json.Unmarshal(fetched, &parsed); err != nil {
			return nil, err
		} else if parsed.Components == nil {
			continue
		}

		for k, d := range parsed.Components.Schemas {
			if _, existing := combined.Components.Schemas[k]; !existing {
				combined.Components.Schemas[k] = d
			}
		}
	}

	return json.Marshal(&combined)
}

func (gv *compositeGroupVersion) ServerRelativeURL() string {
	return ""
}
func NewForComposite(gvFetchers ...openapi.GroupVersion) openapi.GroupVersion {
	return &compositeGroupVersion{gvFetchers}
}
