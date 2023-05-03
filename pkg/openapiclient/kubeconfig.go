package openapiclient

import (
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/openapi"
	"k8s.io/client-go/tools/clientcmd"
)

// Creates an openapi client that connects directly to cluster
type kubeConfig struct {
	client    openapi.Client
	overrides clientcmd.ConfigOverrides
}

func NewKubeConfig(overrides clientcmd.ConfigOverrides) openapi.Client {
	return &kubeConfig{}
}

func (k *kubeConfig) Paths() (map[string]openapi.GroupVersion, error) {
	if k.client == nil {
		loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
		kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &k.overrides)

		config, err := kubeConfig.ClientConfig()
		if err != nil {
			return nil, err
		}

		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			return nil, err
		}

		k.client = clientset.Discovery().OpenAPIV3()
	}

	return k.client.Paths()
}
