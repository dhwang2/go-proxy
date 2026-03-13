package builders

import "encoding/json"

// setRaw marshals value and stores it in the raw map under the given key.
func setRaw(m map[string]json.RawMessage, key string, value any) {
	b, _ := json.Marshal(value)
	m[key] = b
}
