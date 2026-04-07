package smurfv1

// Register a JSON codec so our hand-written types can be used with gRPC
// without protobuf serialization. Both client and server import this package,
// so the init() runs on both sides automatically.

import (
	"encoding/json"
	"fmt"

	"google.golang.org/grpc/encoding"
)

func init() {
	encoding.RegisterCodec(JSONCodec{})
}

// JSONCodec is a gRPC codec that uses encoding/json.
type JSONCodec struct{}

func (JSONCodec) Name() string { return "proto" } // override the default proto codec

func (JSONCodec) Marshal(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("json marshal: %w", err)
	}
	return b, nil
}

func (JSONCodec) Unmarshal(data []byte, v any) error {
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("json unmarshal: %w", err)
	}
	return nil
}
