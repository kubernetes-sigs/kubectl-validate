package cmd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLintMarshal(t *testing.T) {
	cases := []struct {
		name     string
		input    map[string][]metav1.Status
		expected string
	}{
		{
			name:     "empty",
			input:    map[string][]metav1.Status{},
			expected: ``,
		},
		{
			name: "success",
			input: map[string][]metav1.Status{
				"file.yaml": {
					{Status: metav1.StatusSuccess, Reason: "valid"},
				},
			},
			expected: ``,
		},
		{
			name: "single error, single cause",
			input: map[string][]metav1.Status{
				"../../testcases/manifests/configmap.yaml": {
					{Status: metav1.StatusFailure, Reason: "invalid", Details: &metav1.StatusDetails{
						Causes: []metav1.StatusCause{
							{
								Type:    "FailureType",
								Field:   "metadata.name",
								Message: "name is required or invalid somehow",
							},
						},
					}},
				},
			},
			expected: `../../testcases/manifests/configmap.yaml:8:9: field "metadata.name": (reason: "FailureType"; name is required or invalid somehow)`,
		},
		{
			name: "single error with ignored success",
			input: map[string][]metav1.Status{
				"../../testcases/manifests/configmap.yaml": {
					{Status: metav1.StatusSuccess, Reason: "valid"},
				},
				"../../testcases/manifests/apiservice.yaml": {
					{Status: metav1.StatusFailure, Reason: "invalid", Details: &metav1.StatusDetails{
						Causes: []metav1.StatusCause{
							{
								Type:    "FailureType",
								Field:   "metadata.name",
								Message: "name is required or invalid somehow but specific to apiservices",
							},
						},
					}},
				},
			},
			expected: `../../testcases/manifests/apiservice.yaml:14:9: field "metadata.name": (reason: "FailureType"; name is required or invalid somehow but specific to apiservices)`,
		},
		{
			name: "multiple errors, multiple causes",
			input: map[string][]metav1.Status{
				"../../testcases/manifests/configmap.yaml": {
					{Status: metav1.StatusFailure, Reason: "invalid", Details: &metav1.StatusDetails{
						Causes: []metav1.StatusCause{
							{
								Type:    "FailureType",
								Field:   "metadata.name",
								Message: "name is required or invalid somehow 1x1",
							},
							{
								Type:    "FailureType",
								Field:   "metadata.finalizers",
								Message: "something wrong with finalizers 1x2",
							},
						},
					}},
				},
				"../../testcases/manifests/apiservice.yaml": {
					{Status: metav1.StatusFailure, Reason: "invalid", Details: &metav1.StatusDetails{
						Causes: []metav1.StatusCause{
							{
								Type:    "FailureType",
								Field:   "metadata.name",
								Message: "name is required or invalid somehow 2x1",
							},
							{
								Type:    "FailureType",
								Field:   "metadata.name",
								Message: "name is required or invalid somehow 2x2",
							},
						},
					}},
				},
			},
			expected: strings.Join([]string{
				`../../testcases/manifests/apiservice.yaml:14:9: field "metadata.name": (reason: "FailureType"; name is required or invalid somehow 2x1), (reason: "FailureType"; name is required or invalid somehow 2x2)`,
				`../../testcases/manifests/configmap.yaml:8:9: field "metadata.name": (reason: "FailureType"; name is required or invalid somehow 1x1)`,
				`../../testcases/manifests/configmap.yaml:10:3: field "metadata.finalizers": (reason: "FailureType"; something wrong with finalizers 1x2)`,
			}, "\n"),
		},
		{
			name: "single error of complex field",
			input: map[string][]metav1.Status{
				"../../testcases/manifests/error_x_list_map_duplicate_key.yaml": {
					{Status: metav1.StatusFailure, Reason: "invalid", Details: &metav1.StatusDetails{
						Causes: []metav1.StatusCause{
							{
								Type:    "FieldValueDuplicate",
								Field:   "spec.containers[0].ports[2]",
								Message: `Duplicate value: map[string]interface{}{"key":"value"}`,
							},
						},
					}},
				},
			},
			expected: `../../testcases/manifests/error_x_list_map_duplicate_key.yaml:51:19: field "spec.containers[0].ports[2]": (reason: "FieldValueDuplicate"; Duplicate value: map[string]interface{}{"key":"value"})`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := lintMarshal(tc.input)
			require.NoError(t, err)
			require.Equal(t, tc.expected, string(actual))
		})
	}
}
