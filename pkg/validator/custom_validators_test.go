package validator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestRBACValidator_Matches(t *testing.T) {
	validator := &rbacValidator{}

	tests := []struct {
		name    string
		gvk     schema.GroupVersionKind
		matches bool
	}{
		{
			name: "ClusterRole matches",
			gvk: schema.GroupVersionKind{
				Group:   "rbac.authorization.k8s.io",
				Version: "v1",
				Kind:    "ClusterRole",
			},
			matches: true,
		},
		{
			name: "Role matches",
			gvk: schema.GroupVersionKind{
				Group:   "rbac.authorization.k8s.io",
				Version: "v1",
				Kind:    "Role",
			},
			matches: true,
		},
		{
			name: "ClusterRoleBinding matches",
			gvk: schema.GroupVersionKind{
				Group:   "rbac.authorization.k8s.io",
				Version: "v1",
				Kind:    "ClusterRoleBinding",
			},
			matches: true,
		},
		{
			name: "RoleBinding matches",
			gvk: schema.GroupVersionKind{
				Group:   "rbac.authorization.k8s.io",
				Version: "v1",
				Kind:    "RoleBinding",
			},
			matches: true,
		},
		{
			name: "Pod does not match",
			gvk: schema.GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "Pod",
			},
			matches: false,
		},
		{
			name: "Deployment does not match",
			gvk: schema.GroupVersionKind{
				Group:   "apps",
				Version: "v1",
				Kind:    "Deployment",
			},
			matches: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := validator.Matches(tt.gvk)
			assert.Equal(t, tt.matches, result)
		})
	}
}

func TestRBACValidator_ValidateName(t *testing.T) {
	validator := &rbacValidator{}

	tests := []struct {
		name      string
		inputName string
		prefix    bool
		wantErrs  bool
	}{
		{
			name:      "name with colon is valid",
			inputName: "system:admin",
			prefix:    false,
			wantErrs:  false,
		},
		{
			name:      "name with multiple colons is valid",
			inputName: "role:subrole:subsubrole",
			prefix:    false,
			wantErrs:  false,
		},
		{
			name:      "name with slash is invalid",
			inputName: "system/admin",
			prefix:    false,
			wantErrs:  true,
		},
		{
			name:      "name with percent is invalid",
			inputName: "system%admin",
			prefix:    false,
			wantErrs:  true,
		},
		{
			name:      "name with dots is valid",
			inputName: "system.admin.reader",
			prefix:    false,
			wantErrs:  false,
		},
		{
			name:      "normal DNS name is valid",
			inputName: "my-cluster-role",
			prefix:    false,
			wantErrs:  false,
		},
		{
			name:      "dot as name is invalid",
			inputName: ".",
			prefix:    false,
			wantErrs:  true,
		},
		{
			name:      "double dot as name is invalid",
			inputName: "..",
			prefix:    false,
			wantErrs:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validator.ValidateName(tt.inputName, tt.prefix)
			if tt.wantErrs {
				assert.NotEmpty(t, errs, "expected validation errors but got none")
			} else {
				assert.Empty(t, errs, "expected no validation errors but got: %v", errs)
			}
		})
	}
}

func TestRBACValidator_ValidateResource(t *testing.T) {
	validator := &rbacValidator{}

	tests := []struct {
		name     string
		obj      *unstructured.Unstructured
		wantErrs bool
	}{
		{
			name: "valid ClusterRole",
			obj: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": "rbac.authorization.k8s.io/v1",
					"kind":       "ClusterRole",
					"metadata": map[string]interface{}{
						"name": "test-role",
					},
					"rules": []interface{}{
						map[string]interface{}{
							"apiGroups": []interface{}{""},
							"resources": []interface{}{"pods"},
							"verbs":     []interface{}{"get", "list"},
						},
					},
				},
			},
			wantErrs: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validator.ValidateResource(context.Background(), tt.obj, false)
			if tt.wantErrs {
				assert.NotEmpty(t, errs)
			} else {
				assert.Empty(t, errs)
			}
		})
	}
}

func TestFindCustomValidator(t *testing.T) {
	tests := []struct {
		name      string
		gvk       schema.GroupVersionKind
		wantFound bool
	}{
		{
			name: "finds RBAC validator for ClusterRole",
			gvk: schema.GroupVersionKind{
				Group:   "rbac.authorization.k8s.io",
				Version: "v1",
				Kind:    "ClusterRole",
			},
			wantFound: true,
		},
		{
			name: "finds RBAC validator for Role",
			gvk: schema.GroupVersionKind{
				Group:   "rbac.authorization.k8s.io",
				Version: "v1",
				Kind:    "Role",
			},
			wantFound: true,
		},
		{
			name: "returns nil for Pod",
			gvk: schema.GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "Pod",
			},
			wantFound: false,
		},
		{
			name: "returns nil for Deployment",
			gvk: schema.GroupVersionKind{
				Group:   "apps",
				Version: "v1",
				Kind:    "Deployment",
			},
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findCustomValidator(tt.gvk)
			if tt.wantFound {
				assert.NotNil(t, result, "expected to find a custom validator")
			} else {
				assert.Nil(t, result, "expected no custom validator")
			}
		})
	}
}
