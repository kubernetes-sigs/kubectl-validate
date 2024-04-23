package utils

import (
	"bufio"
	"bytes"
	"io"
	"strings"

	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
)

type Document = []byte

func SplitYamlDocuments(fileBytes Document) ([]Document, error) {
	var documents [][]byte
	reader := utilyaml.NewYAMLReader(bufio.NewReader(bytes.NewBuffer(fileBytes)))
	for {
		document, err := reader.Read()
		if err == io.EOF || len(document) == 0 {
			break
		} else if err != nil {
			return nil, err
		}
		documents = append(documents, []byte(document))
	}
	return documents, nil
}

// IsEmptyYamlDocument checks if a yaml document is empty (contains only comments)
//
// Returns true for comment-only single documents, and strings with multiple documents
// where all docs are comment-only.
func IsEmptyYamlDocument(document Document) bool {
	for _, line := range strings.Split(string(document), "\n") {
		line := strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") && line != "---" {
			return false
		}
	}
	return true
}
