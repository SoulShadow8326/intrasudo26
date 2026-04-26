package handlers

import "encoding/json"

func jsonMarshal(v any) ([]byte, error) {
	return json.Marshal(v)
}

func jsonUnmarshal(data []byte, dest any) error {
	return json.Unmarshal(data, dest)
}
