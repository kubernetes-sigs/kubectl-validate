package validator

import (
	"context"

	"k8s.io/apimachinery/pkg/api/validation"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/registry/rest"
)

// customValidationStrategy wraps a strategy and applies custom validation for specific resource types
type customValidationStrategy struct {
	base            interface{} // The actual customResourceStrategy, stored as interface{}
	gvk             schema.GroupVersionKind
	customValidator CustomValidator
}

// Validate overrides the standard validation to apply custom validation rules
func (s *customValidationStrategy) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	// Call the base strategy's Validate method using type assertion
	type validator interface {
		Validate(context.Context, runtime.Object) field.ErrorList
	}
	baseValidator := s.base.(validator)
	allErrs := baseValidator.Validate(ctx, obj)

	// If there's a custom validator for this resource type, apply it
	if s.customValidator != nil {
		// Remove DNS subdomain validation errors for metadata.name
		// We'll re-validate with custom rules
		filtered := field.ErrorList{}
		hasNameError := false
		for _, err := range allErrs {
			// Skip DNS subdomain errors on metadata.name
			if err.Field == "metadata.name" && err.Type == field.ErrorTypeInvalid {
				hasNameError = true
				continue
			}
			filtered = append(filtered, err)
		}
		allErrs = filtered

		// If there was a name error, re-validate with custom rules
		if hasNameError {
			u, ok := obj.(*unstructured.Unstructured)
			if ok {
				// Get namespace scoped status
				type scopeChecker interface {
					NamespaceScoped() bool
				}
				namespaceScoped := s.base.(scopeChecker).NamespaceScoped()

				// Validate with custom name validation
				allErrs = append(allErrs, validation.ValidateObjectMetaAccessor(u, namespaceScoped, s.customValidator.ValidateName, field.NewPath("metadata"))...)
			}
		}

		// Apply additional resource-specific validation
		if u, ok := obj.(*unstructured.Unstructured); ok {
			type scopeChecker interface {
				NamespaceScoped() bool
			}
			namespaceScoped := s.base.(scopeChecker).NamespaceScoped()
			allErrs = append(allErrs, s.customValidator.ValidateResource(ctx, u, namespaceScoped)...)
		}
	}

	return allErrs
}

// Forward all other methods to the base strategy using type assertions

func (s *customValidationStrategy) NamespaceScoped() bool {
	type scopeChecker interface {
		NamespaceScoped() bool
	}
	return s.base.(scopeChecker).NamespaceScoped()
}

func (s *customValidationStrategy) PrepareForCreate(ctx context.Context, obj runtime.Object) {
	type preparer interface {
		PrepareForCreate(context.Context, runtime.Object)
	}
	s.base.(preparer).PrepareForCreate(ctx, obj)
}

func (s *customValidationStrategy) PrepareForUpdate(ctx context.Context, obj, old runtime.Object) {
	type preparer interface {
		PrepareForUpdate(context.Context, runtime.Object, runtime.Object)
	}
	s.base.(preparer).PrepareForUpdate(ctx, obj, old)
}

func (s *customValidationStrategy) ValidateUpdate(ctx context.Context, obj, old runtime.Object) field.ErrorList {
	type validator interface {
		ValidateUpdate(context.Context, runtime.Object, runtime.Object) field.ErrorList
	}
	return s.base.(validator).ValidateUpdate(ctx, obj, old)
}

func (s *customValidationStrategy) WarningsOnCreate(ctx context.Context, obj runtime.Object) []string {
	type warner interface {
		WarningsOnCreate(context.Context, runtime.Object) []string
	}
	return s.base.(warner).WarningsOnCreate(ctx, obj)
}

func (s *customValidationStrategy) WarningsOnUpdate(ctx context.Context, obj, old runtime.Object) []string {
	type warner interface {
		WarningsOnUpdate(context.Context, runtime.Object, runtime.Object) []string
	}
	return s.base.(warner).WarningsOnUpdate(ctx, obj, old)
}

func (s *customValidationStrategy) Canonicalize(obj runtime.Object) {
	type canonicalizer interface {
		Canonicalize(runtime.Object)
	}
	s.base.(canonicalizer).Canonicalize(obj)
}

func (s *customValidationStrategy) AllowCreateOnUpdate() bool {
	type checker interface {
		AllowCreateOnUpdate() bool
	}
	return s.base.(checker).AllowCreateOnUpdate()
}

func (s *customValidationStrategy) AllowUnconditionalUpdate() bool {
	type checker interface {
		AllowUnconditionalUpdate() bool
	}
	return s.base.(checker).AllowUnconditionalUpdate()
}

func (s *customValidationStrategy) GetResetFields() map[interface{}]interface{} {
	type fieldGetter interface {
		GetResetFields() map[interface{}]interface{}
	}
	return s.base.(fieldGetter).GetResetFields()
}

func (s *customValidationStrategy) GenerateName(base string) string {
	type nameGenerator interface {
		GenerateName(string) string
	}
	return s.base.(nameGenerator).GenerateName(base)
}

func (s *customValidationStrategy) ObjectKinds(obj runtime.Object) ([]schema.GroupVersionKind, bool, error) {
	type objectTyper interface {
		ObjectKinds(runtime.Object) ([]schema.GroupVersionKind, bool, error)
	}
	return s.base.(objectTyper).ObjectKinds(obj)
}

func (s *customValidationStrategy) Recognizes(gvk schema.GroupVersionKind) bool {
	type objectTyper interface {
		Recognizes(schema.GroupVersionKind) bool
	}
	return s.base.(objectTyper).Recognizes(gvk)
}

// newCustomValidationStrategy wraps a strategy to apply custom validation
func newCustomValidationStrategy(base interface{}, gvk schema.GroupVersionKind) rest.RESTCreateStrategy {
	return &customValidationStrategy{
		base:            base,
		gvk:             gvk,
		customValidator: findCustomValidator(gvk),
	}
}
