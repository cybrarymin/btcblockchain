package helpers

import (
	"bytes"
	"encoding/json"
)

func JsonMarshaller(data interface{}) ([]byte, error) {
	var b bytes.Buffer
	err := json.NewEncoder(&b).Encode(data)
	if err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func JsonUnMarshaller[T any](jsonData []byte) (T, error) {
	var output T
	b := bytes.NewReader(jsonData)
	err := json.NewDecoder(b).Decode(&output)
	return output, err
}
