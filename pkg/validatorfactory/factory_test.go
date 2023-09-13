package validatorfactory

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/openapi"
	"sigs.k8s.io/kubectl-validate/pkg/openapiclient"
	"sigs.k8s.io/yaml"
)

func TestValidatorFactory_TestPatcher(t *testing.T) {
	tests := []struct {
		name   string
		file   string
		client openapi.Client
		want   *unstructured.Unstructured
	}{{
		name:   "configmap",
		file:   "../../testcases/manifests/configmap.yaml",
		client: openapiclient.NewHardcodedBuiltins("1.27"),
		want: &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":              "myapp",
					"finalizers":        []interface{}{"finalizers.compute.linkedin.com"},
					"creationTimestamp": nil,
				},
				"data": map[string]interface{}{
					"key": "value",
				},
			},
		},
	},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			document, err := os.ReadFile(tt.file)
			assert.NoError(t, err)
			var metadata metav1.TypeMeta
			assert.NoError(t, yaml.Unmarshal(document, &metadata))
			gvk := metadata.GetObjectKind().GroupVersionKind()
			assert.False(t, gvk.Empty())
			factory, err := New(tt.client)
			assert.NoError(t, err)
			assert.NotNil(t, factory)
			validator, err := factory.ValidatorsForGVK(gvk)
			assert.NoError(t, err)
			decoder, err := validator.Decoder(gvk)
			assert.NoError(t, err)
			info, ok := runtime.SerializerInfoForMediaType(decoder.SupportedMediaTypes(), runtime.ContentTypeYAML)
			assert.True(t, ok)
			var untyped unstructured.Unstructured
			_, _, err = decoder.DecoderToVersion(info.StrictSerializer, gvk.GroupVersion()).Decode(document, &gvk, &untyped)
			assert.NoError(t, err)
			res := validator.SchemaValidator().Validate(untyped.Object)
			assert.Empty(t, res.Errors)
		})
	}
}
