package utils

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSplitDocuments(t *testing.T) {
	cases := []struct {
		name          string
		input         []byte
		expected      []Document
		expectedError string
	}{
		{
			name:  "single document",
			input: []byte(`{"key": "value","name":"doc1"}`),
			expected: []Document{
				Document([]byte(`{"key": "value","name":"doc1"}`)),
			},
		},
		{
			name:  "multiple documents",
			input: []byte(`{"key": "value","name":"doc1"}` + "\n---\n" + `{"key": "value","name":"doc2"}`),
			expected: []Document{
				Document([]byte(`{"key": "value","name":"doc1"}`)),
				Document([]byte(`{"key": "value","name":"doc2"}`)),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := SplitYamlDocuments(tc.input)
			if tc.expectedError != "" {
				require.ErrorContains(t, err, tc.expectedError)
			} else {
				require.NoError(t, err)
				for i := range actual {
					require.Equal(t, string(tc.expected[i]), string(actual[i]))
				}
				require.Len(t, actual, len(tc.expected))
			}
		})
	}
}
