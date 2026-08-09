package localipc

import "encoding/json"

type Request struct {
	ID      string          `json:"id"`
	Method  string          `json:"method"`
	Payload json.RawMessage `json:"payload"`
}

type Response struct {
	ID     string          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
