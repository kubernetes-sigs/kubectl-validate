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
	}{
		{
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
		{
			name:   "cronjob_crd",
			file:   "./testdata/cronjob.yaml",
			client: openapiclient.NewLocalCRDFiles(nil, "./testdata/crds/"),
			want: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "batch.tutorial.kubebuilder.io/v1",
					"kind":       "CronJob",
					"metadata": map[string]interface{}{
						"name": "cronjob-sample",
						"labels": map[string]interface{}{
							"app.kubernetes.io/name":       "cronjob",
							"app.kubernetes.io/instance":   "cronjob-sample",
							"app.kubernetes.io/part-of":    "project",
							"app.kubernetes.io/managed-by": "kustomize",
							"app.kubernetes.io/created-by": "project",
						},
						"creationTimestamp": nil,
					},
					"spec": map[string]interface{}{
						"schedule":                "*/1 * * * *",
						"startingDeadlineSeconds": int64(60),
						"concurrencyPolicy":       "Allow",
						"jobTemplate": map[string]interface{}{
							"spec": map[string]interface{}{
								"template": map[string]interface{}{
									"spec": map[string]interface{}{
										"restartPolicy": "OnFailure",
										"containers": []interface{}{
											map[string]interface{}{
												"name":  "hello",
												"image": "busybox",
												"args": []interface{}{
													"/bin/sh",
													"-c",
													"date; echo Hello from the Kubernetes cluster",
												},
											},
										},
									},
								},
							},
						},
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
			assert.Equal(t, tt.want, &untyped)
		})
	}
}
