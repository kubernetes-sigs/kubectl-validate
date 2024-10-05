package cmd

import (
	"cmp"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/parser"
	"golang.org/x/exp/maps"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// convert the given map of filenames to validation errors into a lint output format: '%f:%l:%c: %m'
// %f - file, %l - line, %c - column, %m - message
func lintMarshal(details map[string][]metav1.Status) ([]byte, error) {
	const (
		nilValue = "<nil>"
	)
	files := maps.Keys(details)
	slices.Sort(files)

	results := []string{}
DETAILS:
	for _, file := range files {
		status := details[file]
		causes := make(map[string][]metav1.StatusCause)
		for _, s := range status {
			if s.Status == metav1.StatusSuccess {
				continue DETAILS // only lint errors
			}
			for _, c := range s.Details.Causes {
				if c.Field == nilValue {
					continue // no field to lookup/annotate
				}
				key := string(c.Type)
				causes[key] = append(causes[key], c)
			}
		}
		if len(causes) == 0 {
			continue // nothing to do, no causes deemed problematic
		}
		b, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		// group causes by position, so that we can group them together in the same output line
		errors := make(map[position][]metav1.StatusCause)
		for _, items := range causes {
			for _, c := range items {
				position, err := getPosition(c.Field, b)
				if err != nil {
					return nil, err
				}
				errors[position] = append(errors[position], c)
			}
		}
		keys := maps.Keys(errors)
		slices.SortFunc(keys, func(i, j position) int {
			return cmp.Or(
				cmp.Compare(i.Line, j.Line),
				cmp.Compare(i.Column, j.Column),
			)
		})
		for _, position := range keys {
			causes := errors[position]
			messages := make(map[string][]string)
			for _, c := range causes {
				messages[c.Field] = append(messages[c.Field], fmt.Sprintf("(reason: %q; %s)", c.Type, c.Message))
			}
			fieldMessages := []string{}
			for field, msgs := range messages {
				fieldMessages = append(fieldMessages, fmt.Sprintf("field %q: %s", field, strings.Join(msgs, ", ")))
			}
			le := lintError{
				File:    file,
				Line:    position.Line,
				Column:  position.Column,
				Message: strings.Join(fieldMessages, ", "),
			}
			results = append(results, le.String())
		}
	}
	return []byte(strings.Join(results, "\n")), nil
}

type position struct {
	Line   int
	Column int
}

type lintError struct {
	File    string
	Line    int
	Column  int
	Message string
}

func (e lintError) String() string {
	return fmt.Sprintf("%s:%d:%d: %s", e.File, e.Line, e.Column, e.Message)
}

func getPosition(field string, source []byte) (position, error) {
	path, err := yaml.PathString(fmt.Sprintf("$.%s", field))
	if err != nil {
		return position{}, err
	}
	file, err := parser.ParseBytes([]byte(source), 0)
	if err != nil {
		return position{}, err
	}
	node, err := path.FilterFile(file)
	if err != nil {
		return position{}, err
	}
	return position{
		Line:   node.GetToken().Position.Line,
		Column: node.GetToken().Position.Column,
	}, nil
}
