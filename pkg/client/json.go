package client

import (
	"encoding/json"
	"fmt"
	"os"
)

func loadJSONData(filePath string) (*LoadedData, error) {
	rawBytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("baton-file: failed to read json file: %w", err)
	}

	cleanBytes := stripJSONComments(rawBytes)

	if err := ValidateLoadedData(cleanBytes, "json"); err != nil {
		return nil, err
	}

	var data LoadedData
	if err := json.Unmarshal(cleanBytes, &data); err != nil {
		return nil, fmt.Errorf("baton-file: failed to unmarshal json: %w", err)
	}

	return &data, nil
}

// stripJSONComments removes // line comments, /* block comments */, and
// trailing commas from JSONC input, producing valid JSON. String literals
// are preserved — comments inside quoted strings are not touched.
func stripJSONComments(data []byte) []byte {
	out := make([]byte, 0, len(data))
	i := 0

	for i < len(data) {
		if data[i] == '"' {
			out = append(out, data[i])
			i++
			for i < len(data) {
				out = append(out, data[i])
				if data[i] == '"' && !isEscaped(data, i) {
					i++
					break
				}
				i++
			}
			continue
		}

		if i+1 < len(data) && data[i] == '/' && data[i+1] == '/' {
			for i < len(data) && data[i] != '\n' {
				i++
			}
			continue
		}

		if i+1 < len(data) && data[i] == '/' && data[i+1] == '*' {
			i += 2
			closed := false
			for i+1 < len(data) {
				if data[i] == '*' && data[i+1] == '/' {
					i += 2
					closed = true
					break
				}
				i++
			}
			if !closed {
				i = len(data) // unterminated block comment — consume rest; unmarshal will catch the error
			}
			continue
		}

		out = append(out, data[i])
		i++
	}

	return stripTrailingCommas(out)
}

func isEscaped(data []byte, i int) bool {
	n := 0
	for j := i - 1; j >= 0 && data[j] == '\\'; j-- {
		n++
	}
	return n%2 == 1
}

// stripTrailingCommas removes commas followed only by whitespace then ] or }.
// It is string-aware to avoid corrupting string content.
func stripTrailingCommas(data []byte) []byte {
	out := make([]byte, 0, len(data))
	i := 0

	for i < len(data) {
		if data[i] == '"' {
			out = append(out, data[i])
			i++
			for i < len(data) {
				out = append(out, data[i])
				if data[i] == '"' && !isEscaped(data, i) {
					i++
					break
				}
				i++
			}
			continue
		}

		if data[i] == ',' {
			j := i + 1
			for j < len(data) && (data[j] == ' ' || data[j] == '\t' || data[j] == '\n' || data[j] == '\r') {
				j++
			}
			if j < len(data) && (data[j] == ']' || data[j] == '}') {
				i = j
				continue
			}
		}

		out = append(out, data[i])
		i++
	}

	return out
}
