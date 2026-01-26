package validator

import (
	"context"

	"k8s.io/apimachinery/pkg/api/validation/path"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// CustomValidator defines custom validation logic for specific resource types
type CustomValidator interface {
	// Matches returns true if this validator should be used for the given GVK
	Matches(gvk schema.GroupVersionKind) bool

	// ValidateName validates the resource name according to resource-specific rules
	// Returns nil if standard DNS subdomain validation should be used
	ValidateName(name string, prefix bool) []string

	// ValidateResource performs additional resource-specific validation
	// This is called after name validation and can add more field validations
	ValidateResource(ctx context.Context, obj *unstructured.Unstructured, namespaceScoped bool) field.ErrorList
}

// rbacValidator implements custom validation for RBAC resources
type rbacValidator struct{}

func (v *rbacValidator) Matches(gvk schema.GroupVersionKind) bool {
	return gvk.Group == "rbac.authorization.k8s.io" &&
		(gvk.Kind == "ClusterRole" ||
			gvk.Kind == "Role" ||
			gvk.Kind == "ClusterRoleBinding" ||
			gvk.Kind == "RoleBinding")
}

func (v *rbacValidator) ValidateName(name string, prefix bool) []string {
	// RBAC resources use path segment validation (allows colons)
	return path.ValidatePathSegmentName(name, prefix)
}

func (v *rbacValidator) ValidateResource(ctx context.Context, obj *unstructured.Unstructured, namespaceScoped bool) field.ErrorList {
	// Could add additional RBAC-specific validation here
	// For example: validate PolicyRules, subjects, roleRef, etc.
	return nil
}

// customValidatorRegistry holds all registered custom validators
var customValidatorRegistry = []CustomValidator{
	&rbacValidator{},
	// Add more validators here as needed:
}

// findCustomValidator returns the custom validator for a GVK, or nil if none exists
func findCustomValidator(gvk schema.GroupVersionKind) CustomValidator {
	for _, validator := range customValidatorRegistry {
		if validator.Matches(gvk) {
			return validator
		}
	}
	return nil
}
