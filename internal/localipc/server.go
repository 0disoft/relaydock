package localipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/0disoft/relaydock/internal/core"
)

type Handler interface {
	Handle(context.Context, Request) Response
}

type HandlerFunc func(context.Context, Request) Response

func (f HandlerFunc) Handle(ctx context.Context, request Request) Response {
	return f(ctx, request)
}

type Server struct {
	endpoint           string
	handler            Handler
	MaximumFrameBytes  uint32
	MaximumConnections int

	mu       sync.Mutex
	listener net.Listener
}

func NewServer(endpoint string, handler Handler) *Server {
	return &Server{
		endpoint:           endpoint,
		handler:            handler,
		MaximumFrameBytes:  4 << 20,
		MaximumConnections: 32,
	}
}

func (s *Server) Run(ctx context.Context) error {
	if s.handler == nil {
		return fmt.Errorf("%w: IPC handler", core.ErrInvalidConfiguration)
	}
	listener, err := listenEndpoint(s.endpoint)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.listener = listener
	s.mu.Unlock()
	defer func() {
		_ = listener.Close()
		s.mu.Lock()
		s.listener = nil
		s.mu.Unlock()
		cleanupEndpoint(s.endpoint)
	}()

	go func() {
		<-ctx.Done()
		_ = listener.Close()
	}()

	maximumConnections := s.MaximumConnections
	if maximumConnections <= 0 {
		maximumConnections = 32
	}
	sem := make(chan struct{}, maximumConnections)
	var wg sync.WaitGroup
	defer wg.Wait()

	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		select {
		case sem <- struct{}{}:
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				s.serveConnection(ctx, conn)
			}()
		default:
			_ = conn.Close()
		}
	}
}

func (s *Server) Ready() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.listener != nil
}

func (s *Server) Endpoint() string { return s.endpoint }

func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.listener == nil {
		return nil
	}
	return s.listener.Close()
}

func (s *Server) serveConnection(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	for {
		raw, err := readFrame(reader, s.MaximumFrameBytes)
		if err != nil {
			if !errors.Is(err, io.EOF) {
				_ = s.writeError(conn, "", "invalid_frame", err.Error())
			}
			return
		}

		var request Request
		if err := json.Unmarshal(raw, &request); err != nil {
			_ = s.writeError(conn, "", "invalid_json", err.Error())
			return
		}
		if request.ID == "" || request.Method == "" {
			_ = s.writeError(conn, request.ID, "invalid_request", "id and method are required")
			return
		}

		response := s.handler.Handle(ctx, request)
		response.ID = request.ID
		encoded, err := json.Marshal(response)
		if err != nil {
			_ = s.writeError(conn, request.ID, "encoding_failed", err.Error())
			return
		}
		if uint32(len(encoded)) > s.MaximumFrameBytes {
			_ = s.writeError(conn, request.ID, "frame_too_large", core.ErrFrameTooLarge.Error())
			return
		}
		if err := writeFrame(conn, encoded); err != nil {
			return
		}
	}
}

func (s *Server) writeError(writer io.Writer, id, code, message string) error {
	raw, _ := json.Marshal(Response{ID: id, Error: &Error{Code: code, Message: message}})
	return writeFrame(writer, raw)
}

type Mux struct {
	mu      sync.RWMutex
	methods map[string]func(context.Context, json.RawMessage) (any, error)
}

func NewMux() *Mux {
	return &Mux{methods: make(map[string]func(context.Context, json.RawMessage) (any, error))}
}

func (m *Mux) Register(method string, handler func(context.Context, json.RawMessage) (any, error)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if method == "" || handler == nil {
		panic("localipc: invalid method registration")
	}
	if _, exists := m.methods[method]; exists {
		panic("localipc: duplicate method " + method)
	}
	m.methods[method] = handler
}

func (m *Mux) Handle(ctx context.Context, request Request) Response {
	m.mu.RLock()
	handler, ok := m.methods[request.Method]
	m.mu.RUnlock()
	if !ok {
		return Response{ID: request.ID, Error: &Error{Code: "method_not_found", Message: "unknown IPC method"}}
	}
	result, err := handler(ctx, request.Payload)
	if err != nil {
		return Response{ID: request.ID, Error: &Error{Code: errorCode(err), Message: err.Error()}}
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return Response{ID: request.ID, Error: &Error{Code: "encoding_failed", Message: err.Error()}}
	}
	return Response{ID: request.ID, Result: raw}
}

func errorCode(err error) string {
	switch {
	case errors.Is(err, core.ErrInvalidArgument):
		return "invalid_argument"
	case errors.Is(err, core.ErrUnauthorized):
		return "unauthorized"
	case errors.Is(err, core.ErrForbidden):
		return "forbidden"
	case errors.Is(err, core.ErrNotFound):
		return "not_found"
	case errors.Is(err, core.ErrConflict):
		return "conflict"
	case errors.Is(err, core.ErrBudgetExceeded):
		return "budget_exceeded"
	default:
		return "internal_error"
	}
}
