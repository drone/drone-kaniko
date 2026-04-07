package utils

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// CustomStringSliceFlag is like a regular StringSlice flag but with
// semicolon as a delimiter
type CustomStringSliceFlag struct {
	Value []string
}

// GetValue returns the slice of strings stored in the flag
func (f *CustomStringSliceFlag) GetValue() []string {
	if f.Value == nil {
		return make([]string, 0)
	}
	return f.Value
}

// String returns a string representation of the flag
func (f *CustomStringSliceFlag) String() string {
	if f.Value == nil {
		return ""
	}
	return strings.Join(f.Value, ";")
}

// Set sets the value of the flag from a string
func (f *CustomStringSliceFlag) Set(v string) error {
	for _, s := range strings.Split(v, ";") {
		s = strings.TrimSpace(s)
		f.Value = append(f.Value, s)
	}
	return nil
}

// NormalizeKeyValuePairs normalizes build args / labels inputs.
// It supports legacy key=value slices and JSON object map inputs.
func NormalizeKeyValuePairs(values []string) []string {
	joined := strings.TrimSpace(strings.Join(values, ","))
	if pairs, ok := parseJSONMap(joined); ok {
		return pairs
	}

	normalized := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}

		if pairs, ok := parseJSONMap(trimmed); ok {
			normalized = append(normalized, pairs...)
			continue
		}

		normalized = append(normalized, trimmed)
	}
	return normalized
}

func parseJSONMap(raw string) ([]string, bool) {
	if raw == "" || !strings.HasPrefix(raw, "{") || !strings.HasSuffix(raw, "}") {
		return nil, false
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil, false
	}

	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	normalized := make([]string, 0, len(payload))
	for _, key := range keys {
		normalized = append(normalized, fmt.Sprintf("%s=%v", key, payload[key]))
	}
	return normalized, true
}
