package openapiclient

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/openapi"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
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
			return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
		}

		clientset, err := kubernetes.NewForConfig(config)
		if err != nil {
			return nil, fmt.Errorf("failed to create clientset for kubeconfig")
		}

		k.client = clientset.Discovery().OpenAPIV3()
	}

	res, err := k.client.Paths()
	if err != nil {
		return nil, fmt.Errorf("failed to download schemas from kubeconfig cluster: %w", err)
	}

	return res, nil
}
