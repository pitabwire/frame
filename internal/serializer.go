package internal

import (
	"encoding/json"

	"google.golang.org/protobuf/proto"
)

func Marshal(payload any) ([]byte, error) {
	switch v := payload.(type) {
	case []byte:
		return v, nil
	case json.RawMessage:
		return v.MarshalJSON()
	case string:
		return []byte(v), nil
	default:
		protoMsg, ok := payload.(proto.Message)
		if ok {
			return proto.Marshal(protoMsg)
		}

		return json.Marshal(payload)
	}
}

//nolint:ineffassign,wastedassign,staticcheck //holder is accessed by reference
func Unmarshal(data []byte, holder any) error {
	switch v := holder.(type) {
	case []byte:
		// Direct byte slice - no allocation needed
		holder = data

	case json.RawMessage:
		// Direct raw message - no allocation needed
		holder = json.RawMessage(data)

	case string:
		// Convert to string and return pointer to match expected type
		holder = string(data)

	default:
		// Handle protobuf messages efficiently
		if protoMsg, ok0 := v.(proto.Message); ok0 {
			// Clone the prototype to avoid modifying the template
			clonedMsg := proto.Clone(protoMsg)
			err := proto.Unmarshal(data, clonedMsg)
			if err != nil {
				return err
			}
			holder = clonedMsg
		} else {
			// Handle JSON unmarshaling with proper error context
			err := json.Unmarshal(data, &v)
			if err != nil {
				return err
			}
			holder = v
		}
	}
	return nil
}
