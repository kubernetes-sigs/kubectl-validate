package validator

import (
	"encoding/json"

	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/conversion"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/kube-openapi/pkg/validation/spec"
	"k8s.io/kube-openapi/pkg/validation/strfmt"
	"k8s.io/kube-openapi/pkg/validation/validate"
)

type validatorEntry struct {
	*spec.Schema
	name            string
	namespaceScoped bool
	schemaValidator validation.SchemaValidator
	ss              *structuralschema.Structural
}

func newValidatorEntry(name string, namespaceScoped bool, openapiSchema *spec.Schema) *validatorEntry {
	return &validatorEntry{Schema: openapiSchema, name: name, namespaceScoped: namespaceScoped}
}

func (v *validatorEntry) IsNamespaceScoped() bool {
	return v.namespaceScoped
}

func (v *validatorEntry) SchemaValidator() validation.SchemaValidator {
	if v.schemaValidator != nil {
		return v.schemaValidator
	}

	v.schemaValidator = &basicValidatorAdapter{SchemaValidator: validate.NewSchemaValidator(v.Schema, nil, "", strfmt.Default)}
	return v.schemaValidator
}

func (v *validatorEntry) ObjectTyper(gvk schema.GroupVersionKind) runtime.ObjectTyper {
	parameterScheme := runtime.NewScheme()
	parameterScheme.AddUnversionedTypes(schema.GroupVersion{Group: gvk.Group, Version: gvk.Version},
		&metav1.ListOptions{},
		&metav1.GetOptions{},
		&metav1.DeleteOptions{},
	)
	return newUnstructuredObjectTyper(parameterScheme)
}

func (v *validatorEntry) Decoder(gvk schema.GroupVersionKind) (runtime.NegotiatedSerializer, error) {
	ssMap := map[string]*structuralschema.Structural{}
	ss, err := v.StructuralSchema()
	if err != nil {
		return nil, err
	}

	ssMap[gvk.Version] = ss
	cf, err := conversion.NewCRConverterFactory(nil, nil)
	if err != nil {
		return nil, err
	}

	safeConverter, _, err := cf.NewConverter(&apiextensionsv1.CustomResourceDefinition{
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: gvk.Group,
			Names: apiextensionsv1.CustomResourceDefinitionNames{
				Kind: gvk.Kind,
			},
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
				{
					Name: gvk.Version,
				},
			},
			Conversion: &apiextensionsv1.CustomResourceConversion{
				Strategy: apiextensionsv1.NoneConverter,
			},
		},
	})
	if err != nil {
		return nil, err
	}

	preserve, _ := v.Extensions.GetBool("x-kubernetes-preserve-unknown-fields")
	return unstructuredNegotiatedSerializer{
		typer:                 v.ObjectTyper(gvk),
		creator:               unstructuredCreator{},
		converter:             safeConverter,
		structuralSchemas:     ssMap,
		structuralSchemaGK:    gvk.GroupKind(),
		preserveUnknownFields: preserve,
	}, nil
}

func (v *validatorEntry) StructuralSchema() (*structuralschema.Structural, error) {
	if v.ss == nil {
		//!TODO: dont try to marshal a potentially recursive schema. should validate
		// that schema (except CRD) is not recursive before moving foreward
		jsonText, err := json.Marshal(v.Schema)
		if err != nil {
			return nil, err
		}

		propsdv1 := apiextensionsv1.JSONSchemaProps{}
		if err := json.Unmarshal(jsonText, &propsdv1); err != nil {
			return nil, err
		}

		propsd := apiextensions.JSONSchemaProps{}
		if err := apiextensionsv1.Convert_v1_JSONSchemaProps_To_apiextensions_JSONSchemaProps(&propsdv1, &propsd, nil); err != nil {
			return nil, err
		}

		ss, err := structuralschema.NewStructural(&propsd)
		if err != nil {
			return nil, err
		}

		v.ss = ss
	}

	return v.ss, nil
}

type basicValidatorAdapter struct {
	*validate.SchemaValidator
}

func (s *basicValidatorAdapter) Validate(new interface{}, options ...validation.ValidationOption) *validate.Result {
	return s.SchemaValidator.Validate(new)
}

func (s *basicValidatorAdapter) ValidateUpdate(new, _ interface{}, options ...validation.ValidationOption) *validate.Result {
	return s.Validate(new)
}
