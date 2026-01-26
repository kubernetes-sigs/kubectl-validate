package validator

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/kubectl-validate/pkg/openapiclient"
)

func TestRBACValidation(t *testing.T) {
	tests := []struct {
		name      string
		file      string
		wantError bool
		errorMsg  string
	}{
		{
			name:      "ClusterRole with colon in name should be valid",
			file:      "./testdata/rbac-clusterrole-with-colon.yaml",
			wantError: false,
		},
		{
			name:      "ClusterRole with normal name should be valid",
			file:      "./testdata/rbac-clusterrole-normal.yaml",
			wantError: false,
		},
		{
			name:      "RoleBinding with colon in name should be valid",
			file:      "./testdata/rbac-rolebinding-with-colon.yaml",
			wantError: false,
		},
		{
			name:      "ClusterRole with slash in name should be invalid",
			file:      "./testdata/rbac-clusterrole-with-slash.yaml",
			wantError: true,
			errorMsg:  "may not contain '/'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create validator with hardcoded builtins
			client := openapiclient.NewHardcodedBuiltins("1.35")
			v, err := New(client)
			assert.NoError(t, err)
			assert.NotNil(t, v)

			// Read test file
			document, err := os.ReadFile(tt.file)
			assert.NoError(t, err)

			// Parse the document
			gvk, obj, err := v.Parse(document)
			assert.NoError(t, err)
			assert.NotNil(t, obj)
			assert.False(t, gvk.Empty())

			// Validate
			err = v.Validate(obj)

			if tt.wantError {
				assert.Error(t, err, "expected validation error but got none")
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err, "expected no validation error but got: %v", err)
			}
		})
	}
}

func TestRBACNameValidation_SystemNames(t *testing.T) {
	tests := []struct {
		name      string
		yamlName  string
		wantError bool
	}{
		{
			name:      "system:admin",
			yamlName:  "system:admin",
			wantError: false,
		},
		{
			name:      "system:kube-controller-manager",
			yamlName:  "system:kube-controller-manager",
			wantError: false,
		},
		{
			name:      "system:node",
			yamlName:  "system:node",
			wantError: false,
		},
		{
			name:      "cluster-admin",
			yamlName:  "cluster-admin",
			wantError: false,
		},
		{
			name:      "custom:with:multiple:colons",
			yamlName:  "custom:with:multiple:colons",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create validator
			client := openapiclient.NewHardcodedBuiltins("1.35")
			v, err := New(client)
			assert.NoError(t, err)

			// Create a ClusterRole with the test name
			yaml := `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: ` + tt.yamlName + `
rules:
  - apiGroups:
      - ""
    resources:
      - pods
    verbs:
      - get
`

			// Parse and validate
			gvk, obj, err := v.Parse([]byte(yaml))
			assert.NoError(t, err)
			assert.NotNil(t, obj)
			assert.False(t, gvk.Empty())

			err = v.Validate(obj)

			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err, "expected no error for RBAC name: %s", tt.yamlName)
			}
		})
	}
}
