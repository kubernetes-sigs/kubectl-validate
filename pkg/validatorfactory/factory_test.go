package validatorfactory

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/kubectl-validate/pkg/openapiclient"
	"sigs.k8s.io/yaml"
)

func TestValidatorFactory_TestPatcher(t *testing.T) {
	tests := []struct {
		name  string
		file  string
		typed interface{}
		want  interface{}
	}{{
		name:  "configmap",
		file:  "../../testcases/manifests/configmap.yaml",
		typed: &corev1.ConfigMap{},
		want: &corev1.ConfigMap{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "v1",
				Kind:       "ConfigMap",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:       "myapp",
				Finalizers: []string{"finalizers.compute.linkedin.com"},
			},
			Data: map[string]string{
				"key": "value",
			},
		},
	}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			document, err := os.ReadFile(tt.file)
			assert.NoError(t, err)
			var metadata metav1.TypeMeta
			assert.NoError(t, yaml.Unmarshal(document, &metadata))
			gvk := metadata.GetObjectKind().GroupVersionKind()
			assert.False(t, gvk.Empty())
			client := openapiclient.NewHardcodedBuiltins("1.27")
			factory, err := New(client)
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
			assert.NoError(t, runtime.DefaultUnstructuredConverter.FromUnstructuredWithValidation(untyped.UnstructuredContent(), tt.typed, true))
			assert.Equal(t, tt.want, tt.typed)
		})
	}
}
