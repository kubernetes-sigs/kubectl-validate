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
		onlyComments := true
		for _, line := range strings.Split(string(document), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			} else if !strings.HasPrefix(line, "#") {
				onlyComments = false
				break
			}
		}
		if !onlyComments {
			documents = append(documents, []byte(document))
		}
	}
	return documents, nil
}
