package utils

import (
	"reflect"
	"testing"
)

func TestNormalizeKeyValuePairs(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "single_json_object",
			input:    []string{`{"foo":"bar","hello":"world"}`},
			expected: []string{"foo=bar", "hello=world"},
		},
		{
			name:     "comma_split_json_object_reconstructed",
			input:    []string{`{"foo":"bar"`, `"hello":"world"}`},
			expected: []string{"foo=bar", "hello=world"},
		},
		{
			name:     "preserve_plain_key_value_pairs",
			input:    []string{"foo=bar", "hello=world"},
			expected: []string{"foo=bar", "hello=world"},
		},
		{
			name:     "mixed_plain_and_json",
			input:    []string{"foo=bar", `{"hello":"world"}`},
			expected: []string{"foo=bar", "hello=world"},
		},
		{
			name:     "drop_empty_values",
			input:    []string{"", "  ", "foo=bar"},
			expected: []string{"foo=bar"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual := NormalizeKeyValuePairs(test.input)
			if !reflect.DeepEqual(actual, test.expected) {
				t.Fatalf("unexpected normalized values: got %v want %v", actual, test.expected)
			}
		})
	}
}
