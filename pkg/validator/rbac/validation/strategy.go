package validation

import (
	"context"
	rbac "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/apiserver/pkg/storage/names"
)

type Validator[T any] func(T) field.ErrorList

type strategy[T any] struct {
	runtime.ObjectTyper
	names.NameGenerator
	NamespacedScoped bool
	validate         func(x *T) field.ErrorList
}

func ClusterRoleStrategy(typer runtime.ObjectTyper, namespaceScoped bool) *strategy[rbac.ClusterRole] {
	return &strategy[rbac.ClusterRole]{
		ObjectTyper:      typer,
		NameGenerator:    names.SimpleNameGenerator,
		NamespacedScoped: namespaceScoped,
		validate:         clusterRoleValidator,
	}
}

func ClusterRoleBindingStrategy(typer runtime.ObjectTyper, namespaceScoped bool) *strategy[rbac.ClusterRoleBinding] {
	return &strategy[rbac.ClusterRoleBinding]{
		ObjectTyper:      typer,
		NameGenerator:    names.SimpleNameGenerator,
		NamespacedScoped: namespaceScoped,
		validate:         ValidateClusterRoleBinding,
	}
}

func RoleStrategy(typer runtime.ObjectTyper, namespaceScoped bool) *strategy[rbac.Role] {
	return &strategy[rbac.Role]{
		ObjectTyper:      typer,
		NameGenerator:    names.SimpleNameGenerator,
		NamespacedScoped: namespaceScoped,
		validate:         ValidateRole,
	}
}

func RoleBindingStrategy(typer runtime.ObjectTyper, namespaceScoped bool) *strategy[rbac.RoleBinding] {
	return &strategy[rbac.RoleBinding]{
		ObjectTyper:      typer,
		NameGenerator:    names.SimpleNameGenerator,
		NamespacedScoped: namespaceScoped,
		validate:         ValidateRoleBinding,
	}
}

func clusterRoleValidator(role *rbac.ClusterRole) field.ErrorList {
	return ValidateClusterRole(role, ClusterRoleValidationOptions{
		AllowInvalidLabelValueInSelector: false,
	})
}

func (s strategy[T]) NamespaceScoped() bool {
	return s.NamespacedScoped
}

func (s strategy[T]) WarningsOnCreate(ctx context.Context, obj runtime.Object) []string {
	return nil
}

func (s strategy[T]) Canonicalize(obj runtime.Object) {}

func (s strategy[T]) PrepareForCreate(ctx context.Context, obj runtime.Object) {}

func (s strategy[T]) Validate(ctx context.Context, obj runtime.Object) field.ErrorList {
	u := obj.(*unstructured.Unstructured).Object
	var v T
	_ = runtime.DefaultUnstructuredConverter.FromUnstructuredWithValidation(u, &v, false)
	return s.validate(&v)
}
