package utils

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/kube-openapi/pkg/spec3"
)

func ExtractExtensionGVKs(extensions map[string]interface{}) []schema.GroupVersionKind {
	var result []schema.GroupVersionKind
	if extensions == nil {
		return nil
	}
	gvks, ok := extensions["x-kubernetes-group-version-kind"]
	if !ok {
		return nil
	}
	var gvksList []interface{}
	if list, ok := gvks.([]interface{}); ok {
		gvksList = list
	} else if obj, ok := gvks.(map[string]interface{}); ok {
		gvksList = append(gvksList, obj)
	} else {
		return nil
	}
	for _, specGVK := range gvksList {
		if stringMap, ok := specGVK.(map[string]string); ok {
			g, ok1 := stringMap["group"]
			v, ok2 := stringMap["version"]
			k, ok3 := stringMap["kind"]
			if !ok1 || !ok2 || !ok3 {
				continue
			}
			result = append(result, schema.GroupVersionKind{
				Group:   g,
				Version: v,
				Kind:    k,
			})
		} else if anyMap, ok := specGVK.(map[string]interface{}); ok {
			gAny, ok1 := anyMap["group"]
			vAny, ok2 := anyMap["version"]
			kAny, ok3 := anyMap["kind"]
			if !ok1 || !ok2 || !ok3 {
				continue
			}
			g, ok1 := gAny.(string)
			v, ok2 := vAny.(string)
			k, ok3 := kAny.(string)
			if !ok1 || !ok2 || !ok3 {
				continue
			}
			result = append(result, schema.GroupVersionKind{
				Group:   g,
				Version: v,
				Kind:    k,
			})
		}
	}
	return result
}

func ExtractPathGVKs(path *spec3.Path) []schema.GroupVersionKind {
	var result []schema.GroupVersionKind
	if path.Get != nil {
		result = append(result, ExtractExtensionGVKs(path.Get.Extensions)...)
	}
	if path.Put != nil {
		result = append(result, ExtractExtensionGVKs(path.Put.Extensions)...)
	}
	if path.Post != nil {
		result = append(result, ExtractExtensionGVKs(path.Post.Extensions)...)
	}
	if path.Delete != nil {
		result = append(result, ExtractExtensionGVKs(path.Delete.Extensions)...)
	}
	return result
}
