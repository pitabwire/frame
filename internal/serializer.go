package internal

import (
	"encoding/json"
	"errors"
	"reflect"

	"google.golang.org/protobuf/proto"
)

// Errors
var (
	errNilHolder       = errors.New("holder is nil")
	errNonPointerOrNil = errors.New("holder must be a non-nil pointer")
)

// Marshal marshals payload into bytes with zero-allocation fast paths.
func Marshal(payload any) ([]byte, error) {
	if payload == nil {
		return []byte("null"), nil
	}

	switch v := payload.(type) {
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	case json.RawMessage:
		return v, nil
	case proto.Message:
		// Fast path: direct proto marshal (no reflection)
		return proto.Marshal(v)
	default:
		// Fallback to JSON (still reasonably fast)
		return json.Marshal(payload)
	}
}

// Unmarshal unmarshals data into holder with zero-copy where possible.
// holder must be a pointer.
func Unmarshal(data []byte, holder any) error {
	if holder == nil {
		return errNilHolder
	}

	// Fast path: reflect to get real value and type
	rv := reflect.ValueOf(holder)
	if rv.Kind() != reflect.Ptr || rv.IsNil() {
		return errNonPointerOrNil
	}
	rv = rv.Elem()

	switch target := holder.(type) {
	case *[]byte:
		*target = append((*target)[:0], data...)
		return nil

	case *json.RawMessage:
		*target = append((*target)[:0], data...)
		return nil

	case *string:
		// Still needs allocation (strings are immutable)
		*target = string(data)
		return nil

	default:
		if rv.Kind() == reflect.Ptr && !rv.IsNil() {
			if pm, ok := rv.Interface().(proto.Message); ok {
				// Reset + UnmarshalMerge is faster than Clone + Unmarshal
				proto.Reset(pm)
				return proto.Unmarshal(data, pm)
			}
		}

		// If it's a pointer to struct that implements proto.Message via interface satisfaction
    if pm, ok := holder.(proto.Message); ok {
      proto.Reset(pm)
      return proto.Unmarshal(data, pm)
    }

		// Fallback to JSON
		return json.Unmarshal(data, holder)
	}
}
