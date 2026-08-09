package localipc

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/idgen"
)

type Client interface {
	Call(context.Context, string, any, any) error
}
type SocketClient struct {
	endpoint          string
	MaximumFrameBytes uint32
}

func NewClient(endpoint string) *SocketClient {
	return &SocketClient{endpoint: endpoint, MaximumFrameBytes: 4 << 20}
}
func (c *SocketClient) Call(ctx context.Context, method string, input any, output any) error {
	if method == "" {
		return fmt.Errorf("%w: IPC method", core.ErrInvalidArgument)
	}
	payload, err := EncodePayload(input)
	if err != nil {
		return err
	}
	conn, err := dialEndpoint(ctx, c.endpoint)
	if err != nil {
		return err
	}
	defer conn.Close()
	request := Request{ID: idgen.New("ipc"), Method: method, Payload: payload}
	raw, err := json.Marshal(request)
	if err != nil {
		return err
	}
	if uint32(len(raw)) > c.MaximumFrameBytes {
		return core.ErrFrameTooLarge
	}
	if err := writeFrame(conn, raw); err != nil {
		return err
	}
	responseRaw, err := readFrame(bufio.NewReader(conn), c.MaximumFrameBytes)
	if err != nil {
		return err
	}
	var response Response
	if err := json.Unmarshal(responseRaw, &response); err != nil {
		return err
	}
	if response.ID != request.ID {
		return fmt.Errorf("%w: mismatched response ID", core.ErrConflict)
	}
	if response.Error != nil {
		return &RemoteError{Code: response.Error.Code, Message: response.Error.Message}
	}
	if output != nil && len(response.Result) > 0 {
		if err := json.Unmarshal(response.Result, output); err != nil {
			return err
		}
	}
	return nil
}
func EncodePayload(value any) (json.RawMessage, error) {
	if value == nil {
		return json.RawMessage("null"), nil
	}
	return json.Marshal(value)
}

type RemoteError struct{ Code, Message string }

func (e *RemoteError) Error() string { return e.Code + ": " + e.Message }
func writeFrame(w io.Writer, payload []byte) error {
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)))
	if _, err := w.Write(header[:]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}
func readFrame(r io.Reader, max uint32) ([]byte, error) {
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return nil, err
	}
	size := binary.BigEndian.Uint32(header[:])
	if size == 0 {
		return nil, fmt.Errorf("%w: empty IPC frame", core.ErrInvalidArgument)
	}
	if size > max {
		return nil, core.ErrFrameTooLarge
	}
	payload := make([]byte, size)
	_, err := io.ReadFull(r, payload)
	return payload, err
}
