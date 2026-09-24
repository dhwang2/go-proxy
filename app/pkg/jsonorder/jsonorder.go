// Package jsonorder decodes JSON without losing the order of object keys.
// encoding/json decodes an object into a map, and encoding a map sorts its
// keys; a configuration that went through that round trip reads alphabetically
// rather than in the order it was written.
package jsonorder

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type Kind int

const (
	Scalar Kind = iota // string, number, true, false or null
	Object
	Array
)

// Value is one decoded JSON value. An object keeps its keys in Keys and the
// matching values in Fields; a scalar keeps its encoded form in Raw.
type Value struct {
	Kind   Kind
	Keys   []string
	Fields []*Value
	Items  []*Value
	Raw    json.RawMessage
}

// Parse decodes exactly one JSON value.
func Parse(data []byte) (*Value, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	value, err := parse(dec)
	if err != nil {
		return nil, err
	}
	if dec.More() {
		return nil, fmt.Errorf("unexpected data after JSON value")
	}
	return value, nil
}

func parse(dec *json.Decoder) (*Value, error) {
	token, err := dec.Token()
	if err != nil {
		return nil, err
	}
	switch token {
	case json.Delim('{'):
		value := &Value{Kind: Object}
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return nil, err
			}
			name, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("object key is not a string")
			}
			field, err := parse(dec)
			if err != nil {
				return nil, err
			}
			value.Keys = append(value.Keys, name)
			value.Fields = append(value.Fields, field)
		}
		_, err = dec.Token()
		return value, err
	case json.Delim('['):
		value := &Value{Kind: Array, Items: []*Value{}}
		for dec.More() {
			item, err := parse(dec)
			if err != nil {
				return nil, err
			}
			value.Items = append(value.Items, item)
		}
		_, err = dec.Token()
		return value, err
	}
	raw, err := encode(token)
	if err != nil {
		return nil, err
	}
	return &Value{Kind: Scalar, Raw: raw}, nil
}

// String returns a scalar string value, encoded without HTML escaping so
// "<redacted>" stays readable.
func String(text string) *Value {
	raw, _ := encode(text)
	return &Value{Kind: Scalar, Raw: raw}
}

func encode(value any) (json.RawMessage, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// Get returns the value of an object's key, or nil.
func (v *Value) Get(key string) *Value {
	if v == nil || v.Kind != Object {
		return nil
	}
	for index, name := range v.Keys {
		if name == key {
			return v.Fields[index]
		}
	}
	return nil
}

// Set replaces the value of an existing key or appends a new one.
func (v *Value) Set(key string, field *Value) {
	for index, name := range v.Keys {
		if name == key {
			v.Fields[index] = field
			return
		}
	}
	v.Keys = append(v.Keys, key)
	v.Fields = append(v.Fields, field)
}

// Delete removes a key, if present.
func (v *Value) Delete(key string) {
	for index, name := range v.Keys {
		if name == key {
			v.Keys = append(v.Keys[:index], v.Keys[index+1:]...)
			v.Fields = append(v.Fields[:index], v.Fields[index+1:]...)
			return
		}
	}
}

// Reorder moves the named keys to the front, in the order given. Keys not
// named keep their relative order after them.
func (v *Value) Reorder(order ...string) {
	if v == nil || v.Kind != Object {
		return
	}
	keys := make([]string, 0, len(v.Keys))
	fields := make([]*Value, 0, len(v.Fields))
	taken := make([]bool, len(v.Keys))
	for _, want := range order {
		for index, name := range v.Keys {
			if name == want && !taken[index] {
				keys = append(keys, name)
				fields = append(fields, v.Fields[index])
				taken[index] = true
			}
		}
	}
	for index, name := range v.Keys {
		if !taken[index] {
			keys = append(keys, name)
			fields = append(fields, v.Fields[index])
		}
	}
	v.Keys, v.Fields = keys, fields
}

// MarshalJSON encodes the value compactly, keys in their kept order.
func (v *Value) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	v.write(&buf)
	return buf.Bytes(), nil
}

func (v *Value) write(buf *bytes.Buffer) {
	switch v.Kind {
	case Object:
		buf.WriteByte('{')
		for index, name := range v.Keys {
			if index > 0 {
				buf.WriteByte(',')
			}
			key, _ := encode(name)
			buf.Write(key)
			buf.WriteByte(':')
			v.Fields[index].write(buf)
		}
		buf.WriteByte('}')
	case Array:
		buf.WriteByte('[')
		for index, item := range v.Items {
			if index > 0 {
				buf.WriteByte(',')
			}
			item.write(buf)
		}
		buf.WriteByte(']')
	default:
		buf.Write(v.Raw)
	}
}
