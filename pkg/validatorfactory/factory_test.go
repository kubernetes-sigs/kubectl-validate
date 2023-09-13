package validatorfactory

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/openapi"
	"sigs.k8s.io/kubectl-validate/pkg/openapiclient"
	"sigs.k8s.io/yaml"
)

func TestValidatorFactory_TestPatcher(t *testing.T) {
	type ConcurrencyPolicy string

	const (
		AllowConcurrent   ConcurrencyPolicy = "Allow"
		ForbidConcurrent  ConcurrencyPolicy = "Forbid"
		ReplaceConcurrent ConcurrencyPolicy = "Replace"
	)

	type CronJobSpec struct {
		Schedule                   string                  `json:"schedule"`
		StartingDeadlineSeconds    *int64                  `json:"startingDeadlineSeconds,omitempty"`
		ConcurrencyPolicy          ConcurrencyPolicy       `json:"concurrencyPolicy,omitempty"`
		Suspend                    *bool                   `json:"suspend,omitempty"`
		JobTemplate                batchv1.JobTemplateSpec `json:"jobTemplate"`
		SuccessfulJobsHistoryLimit *int32                  `json:"successfulJobsHistoryLimit,omitempty"`
		FailedJobsHistoryLimit     *int32                  `json:"failedJobsHistoryLimit,omitempty"`
	}

	type CronJobStatus struct {
		Active           []corev1.ObjectReference `json:"active,omitempty"`
		LastScheduleTime *metav1.Time             `json:"lastScheduleTime,omitempty"`
	}

	type CronJob struct {
		metav1.TypeMeta   `json:",inline"`
		metav1.ObjectMeta `json:"metadata,omitempty"`
		Spec              CronJobSpec   `json:"spec,omitempty"`
		Status            CronJobStatus `json:"status,omitempty"`
	}
	tests := []struct {
		name   string
		file   string
		client openapi.Client
		typed  interface{}
		want   interface{}
	}{{
		name:   "configmap",
		file:   "../../testcases/manifests/configmap.yaml",
		client: openapiclient.NewHardcodedBuiltins("1.27"),
		typed:  &corev1.ConfigMap{},
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
	}, {
		name:   "cronjob",
		file:   "./testdata/cronjob.yaml",
		client: openapiclient.NewLocalCRDFiles(nil, "./testdata/crds/"),
		typed:  &CronJob{},
		want: &CronJob{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "batch.tutorial.kubebuilder.io/v1",
				Kind:       "CronJob",
			},
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"app.kubernetes.io/name":       "cronjob",
					"app.kubernetes.io/instance":   "cronjob-sample",
					"app.kubernetes.io/part-of":    "project",
					"app.kubernetes.io/managed-by": "kustomize",
					"app.kubernetes.io/created-by": "project",
				},
				Name: "cronjob-sample",
			},
			Spec: CronJobSpec{
				Schedule: "*/1 * * * *",
				StartingDeadlineSeconds: func() *int64 {
					var out int64 = 60
					return &out
				}(),
				ConcurrencyPolicy: AllowConcurrent,
				JobTemplate: batchv1.JobTemplateSpec{
					Spec: batchv1.JobSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								RestartPolicy: corev1.RestartPolicyOnFailure,
								Containers: []corev1.Container{{
									Name:  "hello",
									Image: "busybox",
									Args: []string{
										"/bin/sh",
										"-c",
										"date; echo Hello from the Kubernetes cluster",
									},
								}},
							},
						},
					},
				},
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
			assert.NoError(t, runtime.DefaultUnstructuredConverter.FromUnstructuredWithValidation(untyped.UnstructuredContent(), tt.typed, true))
			assert.Equal(t, tt.want, tt.typed)
		})
	}
}
