package proto

import (
	"buf.build/go/protovalidate"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var (
	MarshalOptions = protojson.MarshalOptions{
		EmitUnpopulated: true,
		UseProtoNames:   true,
	}
	UnmarshalOptions = protojson.UnmarshalOptions{
		DiscardUnknown: true,
	}
)

var validator protovalidate.Validator

func init() {
	var err error
	validator, err = protovalidate.New()
	if err != nil {
		panic(err)
	}
}

func Marshal(m proto.Message) ([]byte, error) {
	return MarshalOptions.Marshal(m)
}

func Unmarshal(data []byte, m proto.Message) error {
	return UnmarshalOptions.Unmarshal(data, m)
}

func Validate(m proto.Message) error {
	return validator.Validate(m)
}
