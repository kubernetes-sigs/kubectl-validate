package validator

import (
	"fmt"
	"path"
	"reflect"
	"strings"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	structuralschema "k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/kube-openapi/pkg/validation/spec"
)

// DEPRECATED
// This was a dirty hack to try out using StructuralSchemas against the recursive
// CRD schema. It actually works pretty well. But k8s doesnt expect recursive
// schemas and loops forever, so this was dropped. I'm sure it can be simplified
// a lot, but its complexity does afford some cool properties.
//
// A pointer-based structural schema to workaround the value-typed properties field
type DeferredStructural struct {
	*structuralschema.Structural
	Properties map[string]*structuralschema.Structural

	refSpec            *spec.Schema
	referredStructural *DeferredStructural
}

func (d *DeferredStructural) resolve() {
	if d.refSpec == nil {
		return
	}

	addts := d.AdditionalProperties
	*d.Structural = *d.referredStructural.Structural
	d.AdditionalProperties = addts

	if d.ValueValidation != nil {
		copy := *d.ValueValidation
		d.ValueValidation = &copy
	} else {
		d.ValueValidation = &structuralschema.ValueValidation{}
	}

	specToGeneric(d.refSpec, &d.Generic)
	specToExtensions(d.refSpec, &d.Extensions)
	specToValidationExtensions(d.refSpec, &d.ValidationExtensions)
	specToValidations(d.refSpec, d.ValueValidation)

	if reflect.DeepEqual(d.ValueValidation, &structuralschema.ValueValidation{}) {
		d.ValueValidation = nil
	}

	d.Properties = d.referredStructural.Properties
}

type structuralSchemaFactory struct {
	cache      map[string]*structuralschema.Structural
	components map[string]*spec.Schema
}

func NewStructuralSchemaFactory(components map[string]*spec.Schema) structuralSchemaFactory {
	return structuralSchemaFactory{
		cache:      map[string]*structuralschema.Structural{},
		components: components,
	}
}

func (s structuralSchemaFactory) ForDefinition(def string) (*structuralschema.Structural, error) {
	if existing, exists := s.cache[def]; exists {
		return existing, nil
	}

	res, err := s.doDefToStructural(def)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func (s structuralSchemaFactory) doDefToStructural(def string) (*structuralschema.Structural, error) {
	all := []*DeferredStructural{}
	working := map[string]*DeferredStructural{}

	res, err := s.defToStructural(def, working, &all)
	if err != nil {
		return nil, err
	}

	for i := len(all) - 1; i >= 0; i-- {
		all[i].resolve()
	}

	for i := len(all) - 1; i >= 0; i-- {
		newProps := map[string]structuralschema.Structural{}
		for k, v := range all[i].Properties {
			newProps[k] = *v
		}
		all[i].Structural.Properties = newProps
	}

	for k, v := range working {
		s.cache[k] = v.Structural
	}

	return res.Structural, nil
}

// Converts a "structural" spec to a structural schema
// Performs no validation
// Refs are left behind
// You should populate the refs with their referred schemas after
func (s structuralSchemaFactory) defToStructural(def string, working map[string]*DeferredStructural, all *[]*DeferredStructural) (*DeferredStructural, error) {
	if existing, exists := s.cache[def]; exists {
		return &DeferredStructural{Structural: existing}, nil
	}

	// Removes #!/components/schemas or #!/definitions component from path if its a ref
	def = strings.TrimPrefix(def, "#!/components/schemas/")
	def = strings.TrimPrefix(def, "#!/definitions/")

	if str, exists := working[def]; exists {
		return str, nil
	}

	sp, exists := s.components[def]
	if !exists {
		return nil, fmt.Errorf("unresolved reference: %v", def)
	}

	res := &DeferredStructural{
		Structural: &structuralschema.Structural{},
	}
	(*all) = append(*all, res)
	working[def] = res

	res2, err := s.specToStructural(sp, working, all)

	if err != nil {
		return nil, err
	}
	*res.Structural = *res2.Structural
	res.Properties = res2.Properties
	res.refSpec = res2.refSpec
	res.referredStructural = res2.referredStructural

	// Add value validations
	return res, nil
}

func (s structuralSchemaFactory) specToStructural(sp *spec.Schema, working map[string]*DeferredStructural, all *[]*DeferredStructural) (*DeferredStructural, error) {
	res := &DeferredStructural{
		Structural: &structuralschema.Structural{},
	}
	*all = append(*all, res)

	// A has property x which is a ref to B with override
	// B has a propety y which is a ref to A with override

	// 0. change all to an ordered array
	// 1. convert generics/extneions to "into" based + conditionals
	// 2. add fields for ref
	// 3. add a step before setting properties to resolve + override refs in backwards order. resolve must set ref's deffered.properties to the resolved things properties
	// 4. add step to set properties

	refString := sp.Ref.Ref.String()
	if len(refString) == 0 && len(sp.AllOf) == 1 && len(sp.AllOf[0].Ref.String()) > 0 && len(sp.Properties) == 0 && sp.Items == nil {
		//!Special case:
		// Treat a schema with just a lone allOf with ref similarly to a ref
		// this is because kube-openapi has this quirk. Otherwise, ref inside
		// allOf is ignored.
		//
		refString = sp.AllOf[0].Ref.String()
	}

	if len(refString) != 0 {
		refString = path.Base(refString)
		if existing, exists := working[refString]; exists {
			res.refSpec = sp
			res.referredStructural = existing
			return res, nil
		} else {
			def, err := s.defToStructural(refString, working, all)
			if err != nil {
				return nil, err
			}

			res.refSpec = sp
			res.referredStructural = def
			return res, nil
		}
	}

	// Extensions
	specToExtensions(sp, &res.Extensions)
	specToValidationExtensions(sp, &res.ValidationExtensions)
	specToGeneric(sp, &res.Generic)

	// Value Validations
	validations := structuralschema.ValueValidation{}
	specToValidations(sp, &validations)
	if !reflect.DeepEqual(validations, structuralschema.ValueValidation{}) {
		res.ValueValidation = &validations
	}

	if sp.Items != nil {
		if sp.Items.Schema != nil {
			v, err := s.specToStructural(sp.Items.Schema, working, all)
			if err != nil {
				return nil, err
			}
			res.Items = v.Structural
		} else if len(sp.Items.Schemas) > 0 {
			v, err := s.specToStructural(&sp.Items.Schemas[0], working, all)
			if err != nil {
				return nil, err
			}
			res.Items = v.Structural
		}
	}

	if sp.Properties != nil {
		res.Properties = map[string]*structuralschema.Structural{}
		for k, v := range sp.Properties {
			v := v
			converted, err := s.specToStructural(&v, working, all)
			if err != nil {
				return nil, err
			}

			res.Properties[k] = converted.Structural
		}
	}

	if sp.AdditionalProperties != nil {
		if sp.AdditionalProperties.Schema != nil {
			converted, err := s.specToStructural(sp.AdditionalProperties.Schema, working, all)
			if err != nil {
				return nil, err
			}
			res.AdditionalProperties = &structuralschema.StructuralOrBool{Structural: converted.Structural}
		} else {
			res.AdditionalProperties = &structuralschema.StructuralOrBool{Bool: sp.AdditionalProperties.Allows}
		}
	}

	return res, nil
}

func specToGeneric(sp *spec.Schema, res *structuralschema.Generic) {
	if len(sp.Description) > 0 {
		res.Description = sp.Description
	}

	if len(sp.Type) > 0 {
		res.Type = sp.Type[0]
	}

	if len(sp.Title) > 0 {
		res.Title = sp.Title
	}

	if sp.Default != nil {
		res.Default.Object = sp.Default
	}

	if !res.Nullable {
		res.Nullable = sp.Nullable
	}
}

func specToExtensions(sp *spec.Schema, res *structuralschema.Extensions) {
	if sp.Extensions == nil {
		return
	}

	if value, _ := sp.Extensions.GetBool("x-kubernetes-preserve-unknown-fields"); value {
		res.XPreserveUnknownFields = value
	}

	if value, _ := sp.Extensions.GetBool("x-kubernetes-embedded-resources"); value {
		res.XEmbeddedResource = value
	}

	if value, _ := sp.Extensions.GetBool("x-kubernetes-int-or-string"); value {
		res.XIntOrString = value
	}

	if value, exists := sp.Extensions.GetStringSlice("x-kubernetes-list-map-keys"); exists {
		res.XListMapKeys = value
	}

	if value, exists := sp.Extensions.GetString("x-kubernetes-list-type"); exists {
		res.XListType = &value
	}

	if value, exists := sp.Extensions.GetString("x-kubernetes-map-type"); exists {
		res.XMapType = &value
	}
}

func specToValidationExtensions(sp *spec.Schema, res *structuralschema.ValidationExtensions) {
	if sp.Extensions == nil {
		return
	}

	rules := apiextensionsv1.ValidationRules{}
	if err := sp.Extensions.GetObject("x-kubernetes-validations", &rules); err == nil {
		res.XValidations = rules
	}
}

func specToValidations(sp *spec.Schema, vals *structuralschema.ValueValidation) {
	// Cant tell difference between unset and empty
	if len(sp.Format) > 0 {
		vals.Format = sp.Format
	}

	if sp.Maximum != nil {
		vals.Maximum = sp.Maximum
	}

	// unfortunately there is no way to test if the field is actually present here
	// so overriding this value to false will not work
	// This can be fixed by changing spec.Schema to use a pointer, or by
	// doing this conversion from a type that does.
	if !vals.ExclusiveMaximum {
		vals.ExclusiveMaximum = sp.ExclusiveMaximum
	}

	if sp.Minimum != nil {
		vals.Minimum = sp.Minimum
	}

	// unfortunately there is no way to test if the field is actually present here
	// so overriding this value to false will not work
	if !vals.ExclusiveMaximum {
		vals.ExclusiveMinimum = sp.ExclusiveMinimum
	}

	if sp.MaxLength != nil {
		vals.MaxLength = sp.MaxLength
	}

	if sp.MinLength != nil {
		vals.MinLength = sp.MinLength
	}

	// Cant tell difference between unset and empty
	if len(sp.Pattern) > 0 {
		vals.Pattern = sp.Pattern
	}

	if sp.MaxItems != nil {
		vals.MaxItems = sp.MaxItems
	}

	if sp.MinItems != nil {
		vals.MinItems = sp.MinItems
	}

	if !vals.UniqueItems {
		vals.UniqueItems = sp.UniqueItems
	}

	if sp.MultipleOf != nil {
		vals.MultipleOf = sp.MultipleOf
	}

	if sp.Enum != nil {
		for _, v := range sp.Enum {
			vals.Enum = append(vals.Enum, structuralschema.JSON{
				Object: v,
			})
		}
	}

	if sp.MaxProperties != nil {
		vals.MaxProperties = sp.MaxProperties
	}

	if sp.MinProperties != nil {
		vals.MinProperties = sp.MinProperties
	}

	if sp.Required != nil {
		vals.Required = sp.Required
	}

	if sp.AllOf != nil {
		vals.AllOf = nil
		for _, v := range sp.AllOf {
			c := structuralschema.NestedValueValidation{}
			specToNestedValidations(&v, &c)
			vals.AllOf = append(vals.AllOf, c)
		}
	}

	if sp.OneOf != nil {
		vals.OneOf = nil
		for _, v := range sp.OneOf {
			c := structuralschema.NestedValueValidation{}
			specToNestedValidations(&v, &c)
			vals.OneOf = append(vals.OneOf, c)
		}
	}

	if sp.AnyOf != nil {
		vals.AnyOf = nil
		for _, v := range sp.AnyOf {
			c := structuralschema.NestedValueValidation{}
			specToNestedValidations(&v, &c)
			vals.AnyOf = append(vals.AnyOf, c)
		}
	}

	if sp.Not != nil {
		v := structuralschema.NestedValueValidation{}
		specToNestedValidations(sp.Not, &v)
		vals.Not = &v
	}
}

func specToNestedValidations(sp *spec.Schema, res *structuralschema.NestedValueValidation) {
	specToValidations(sp, &res.ValueValidation)

	if sp.Items != nil {
		if sp.Items.Schema != nil {
			v := structuralschema.NestedValueValidation{}
			specToNestedValidations(sp.Items.Schema, &v)
			res.Items = &v
		} else if len(sp.Items.Schemas) > 0 {
			v := structuralschema.NestedValueValidation{}
			specToNestedValidations(&sp.Items.Schemas[0], &v)
			res.Items = &v
		}
	}

	if sp.Properties != nil {
		res.Properties = map[string]structuralschema.NestedValueValidation{}
		for k, v := range sp.Properties {
			n := structuralschema.NestedValueValidation{}
			specToNestedValidations(&v, &n)
			res.Properties[k] = n
		}
	}
}
