package openapiclient

import (
	"encoding/json"
	"fmt"

	"k8s.io/client-go/openapi"
	"k8s.io/kube-openapi/pkg/spec3"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

type compositeClient struct {
	clients []openapi.Client
}

type compositeGroupVersion struct {
	gvFetchers []openapi.GroupVersion
}

func (g compositeGroupVersion) Schema(contentType string) ([]byte, error) {
	if len(g.gvFetchers) == 0 {
		return nil, fmt.Errorf("no fetches for groupversion")
	} else if len(g.gvFetchers) == 1 {
		return g.gvFetchers[0].Schema(contentType)
	}

	combined := spec3.OpenAPI{
		Components: &spec3.Components{
			Schemas: map[string]*spec.Schema{},
		},
	}

	for _, fetcher := range g.gvFetchers {
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

// client which tries multiple clients in a priority order for an openapi spec
func NewComposite(clients ...openapi.Client) openapi.Client {
	return compositeClient{clients: clients}
}

func (c compositeClient) Paths() (map[string]openapi.GroupVersion, error) {
	merged := map[string]openapi.GroupVersion{}
	for _, client := range c.clients {
		singleMap, err := client.Paths()
		if err != nil {
			continue
		}

		for k, v := range singleMap {
			if existing, exists := merged[k]; exists {
				existing.(*compositeGroupVersion).gvFetchers = append(existing.(*compositeGroupVersion).gvFetchers, v)
			} else {
				merged[k] = &compositeGroupVersion{gvFetchers: []openapi.GroupVersion{v}}
			}
		}
	}
	return merged, nil
}
