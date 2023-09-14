package utils

import (
	"k8s.io/kube-openapi/pkg/validation/spec"
)

// There is no "left" or "right" on this tree, so no in-order is necessary
type PreorderVisitor func(ctx VisitingContext, s *spec.Schema) (*spec.Schema, bool)
type PostorderVisitor func(ctx VisitingContext, s *spec.Schema) *spec.Schema

func (f PreorderVisitor) VisitBefore(ctx VisitingContext, s **spec.Schema) bool {
	var exploreChildren bool
	*s, exploreChildren = f(ctx, *s)
	return exploreChildren
}

func (f PreorderVisitor) VisitAfter(ctx VisitingContext, s **spec.Schema) {
}

func (f PostorderVisitor) VisitBefore(ctx VisitingContext, s **spec.Schema) bool {
	return true
}

func (f PostorderVisitor) VisitAfter(ctx VisitingContext, s **spec.Schema) {
	*s = f(ctx, *s)
}

type VisitingContext struct {
	// What field of the parent context was traversed to find the current
	// schema
	SchemaField string

	// If ShemaField is a collection, what key is this schema contained within
	// SchemaField
	Key string

	// If ShemaField is a collection, what index is this schema contained within
	// SchemaField
	//
	// Part of a Union with `key` If one is set, the other must be unset
	Index int

	Parent *VisitingContext
}

func (v *VisitingContext) WithSubField(field, key string) VisitingContext {
	return VisitingContext{
		Parent:      v,
		SchemaField: field,
		Key:         key,
	}
}

func (v *VisitingContext) WithSubIndex(field string, idx int) VisitingContext {
	return VisitingContext{
		Parent:      v,
		SchemaField: field,
		Index:       idx,
	}
}

type SchemaVisitor interface {
	// Called on a node before its children.
	// Return false to stop exploring this subtree, otherwise return true.
	VisitBefore(ctx VisitingContext, s **spec.Schema) bool

	// Called on a node after its children.
	VisitAfter(ctx VisitingContext, s **spec.Schema)
}

func VisitSchema(name string, s *spec.Schema, visitor SchemaVisitor) *spec.Schema {
	visitSchema(&s, visitor, VisitingContext{
		SchemaField: "schemas",
		Key:         name,
	})
	return s
}

func visitSchema(s **spec.Schema, visitor SchemaVisitor, context VisitingContext) {
	if s == nil {
		return
	}

	if !visitor.VisitBefore(context, s) {
		return
	}

	if s := *s; s != nil {
		for k, v := range s.Properties {
			pV := &v
			visitSchema(&pV, visitor, context.WithSubField("properties", k))

			if pV == nil {
				delete(s.Properties, k)
			} else {
				s.Properties[k] = *pV
			}
		}

		for k, v := range s.PatternProperties {
			pV := &v
			visitSchema(&pV, visitor, context.WithSubField("patternProperties", k))

			if pV == nil {
				delete(s.PatternProperties, k)
			} else {
				s.PatternProperties[k] = *pV
			}
		}

		for k, v := range s.AllOf {
			pV := &v
			visitSchema(&pV, visitor, context.WithSubIndex("allOf", k))
			s.AllOf[k] = *pV
		}

		for k, v := range s.AnyOf {
			pV := &v
			visitSchema(&pV, visitor, context.WithSubIndex("anyOf", k))
			s.AnyOf[k] = *pV
		}

		for k, v := range s.OneOf {
			pV := &v
			visitSchema(&pV, visitor, context.WithSubIndex("oneOf", k))
			s.OneOf[k] = *pV
		}

		if s.Not != nil {
			visitSchema(&s.Not, visitor, context.WithSubField("not", ""))
		}

		if soa := s.Items; soa != nil {
			if soa.Schema != nil {
				visitSchema(&soa.Schema, visitor, context.WithSubIndex("items", 0))
			}

			for k, v := range soa.Schemas {
				pV := &v
				visitSchema(&pV, visitor, context.WithSubIndex("items", k))
				soa.Schemas[k] = *pV
			}

		}

		if a := s.AdditionalProperties; a != nil {
			if a.Schema != nil {
				visitSchema(&a.Schema, visitor, context.WithSubField("additionalProperties", ""))
			}
		}

		if a := s.AdditionalItems; a != nil {
			if a.Schema != nil {
				visitSchema(&a.Schema, visitor, context.WithSubField("additionalItems", ""))
			}
		}
	}
	visitor.VisitAfter(context, s)
}
