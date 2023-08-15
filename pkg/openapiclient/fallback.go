package openapiclient

import (
	"errors"
	"sync/atomic"

	"k8s.io/client-go/openapi"
)

type fallbackClient struct {
	clients      []openapi.Client
	chosenClient atomic.Pointer[openapi.Client]
}

func (f *fallbackClient) Paths() (map[string]openapi.GroupVersion, error) {
	if chosen := f.chosenClient.Load(); chosen != nil {
		return (*chosen).Paths()
	}

	var errs []error
	for _, c := range f.clients {
		res, err := c.Paths()
		if err == nil {
			f.chosenClient.Store(&c)
			return res, err
		} else {
			errs = append(errs, err)
		}
	}

	return nil, errors.Join(errs...)
}

// Creates an OpenAPI client which forwards calls to the first
// argument which does not return an error
func NewFallback(clients ...openapi.Client) openapi.Client {
	return &fallbackClient{clients: clients}
}
