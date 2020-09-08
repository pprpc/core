package pprpc

import "github.com/golang/protobuf/proto"

// Marshal 编码接口
func Marshal(v interface{}) ([]byte, error) {
	return proto.Marshal(v.(proto.Message))
}

// MarshalJSON 编码为Json数据
func MarshalJSON(v interface{}) ([]byte, error) {
	return proto.MarshalMessageSetJSON(v)
}

// Unmarshal 解码接口
func Unmarshal(data []byte, v interface{}) error {
	return proto.Unmarshal(data, v.(proto.Message))
}

// UnmarshalJSON 解码接收到的Json数据
func UnmarshalJSON(data []byte, v interface{}) error {
	return proto.UnmarshalMessageSetJSON(data, v.(proto.Message))
}
