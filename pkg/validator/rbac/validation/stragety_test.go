package validation

import "k8s.io/apiserver/pkg/registry/rest"

var _ rest.RESTCreateStrategy = strategy[any]{}
