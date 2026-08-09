package httpadapter

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/0disoft/relaydock/internal/protocol/canonical"
	"github.com/0disoft/relaydock/internal/protocol/stream"
)

type responseStream struct {
	mu       sync.Mutex
	resp     *http.Response
	provider string
	protocol canonical.Protocol
	scanner  *bufio.Scanner
	lineMode bool
	sseEvent string
	sseData  strings.Builder
	jsonBody []byte
	readErr  error
	queue    []stream.Event
	sequence int64
	done     bool
	closed   bool
	max      int64
}

func newResponseStream(resp *http.Response, providerName string, protocol canonical.Protocol, max int64) *responseStream {
	s := &responseStream{resp: resp, provider: providerName, protocol: protocol, max: max}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(ct, "text/event-stream") || strings.Contains(ct, "ndjson") || strings.Contains(ct, "json-seq") {
		s.scanner = bufio.NewScanner(resp.Body)
		s.lineMode = strings.Contains(ct, "ndjson") || strings.Contains(ct, "json-seq")
		buf := make([]byte, 64<<10)
		s.scanner.Buffer(buf, int(max))
	} else {
		s.jsonBody, s.readErr = readLimited(resp.Body, max)
		_ = resp.Body.Close()
	}
	return s
}
func (s *responseStream) Next(ctx context.Context) (stream.Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return stream.Event{}, err
	}
	if s.closed {
		return stream.Event{}, core.ErrClosed
	}
	if s.readErr != nil {
		err := s.readErr
		s.readErr = nil
		s.done = true
		return stream.Event{}, err
	}
	for {
		if len(s.queue) > 0 {
			event := s.queue[0]
			s.queue = s.queue[1:]
			s.sequence++
			event.Sequence = s.sequence
			if event.Kind == stream.EventCompleted || event.Kind == stream.EventFailed {
				s.done = true
			}
			return event, nil
		}
		if s.done {
			return stream.Event{}, io.EOF
		}
		if s.scanner == nil {
			events := DecodePayload(s.provider, s.protocol, s.jsonBody)
			s.jsonBody = nil
			if len(events) == 0 {
				s.done = true
				return stream.Event{}, io.ErrUnexpectedEOF
			}
			s.queue = append(s.queue, events...)
			continue
		}
		if !s.scanner.Scan() {
			if err := s.scanner.Err(); err != nil {
				s.done = true
				if strings.Contains(strings.ToLower(err.Error()), "token too long") {
					return stream.Event{}, core.ErrFrameTooLarge
				}
				return stream.Event{}, err
			}
			if !s.lineMode && s.sseData.Len() > 0 {
				s.flushSSEEvent()
				continue
			}
			s.done = true
			return stream.Event{}, io.ErrUnexpectedEOF
		}

		line := strings.TrimSuffix(s.scanner.Text(), "\r")
		if s.lineMode {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if line == "[DONE]" {
				s.queue = append(s.queue, stream.Event{Kind: stream.EventCompleted})
				continue
			}
			s.queue = append(s.queue, DecodePayload(s.provider, s.protocol, []byte(line))...)
			continue
		}

		if line == "" {
			s.flushSSEEvent()
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, found := strings.Cut(line, ":")
		if !found {
			field, value = line, ""
		} else if strings.HasPrefix(value, " ") {
			value = strings.TrimPrefix(value, " ")
		}
		switch field {
		case "event":
			s.sseEvent = value
		case "data":
			if s.sseData.Len() > 0 {
				s.sseData.WriteByte('\n')
			}
			s.sseData.WriteString(value)
		case "id", "retry":
			// Event IDs and retry hints are transport metadata. They are retained
			// by the upstream connection, not exposed as semantic model events.
		default:
			// Ignore unknown SSE fields as required by the event-stream format.
		}
	}
}

func (s *responseStream) flushSSEEvent() {
	if s.sseData.Len() == 0 {
		s.sseEvent = ""
		return
	}
	payload := s.sseData.String()
	eventName := s.sseEvent
	s.sseData.Reset()
	s.sseEvent = ""
	if strings.TrimSpace(payload) == "[DONE]" {
		s.queue = append(s.queue, stream.Event{Kind: stream.EventCompleted})
		return
	}
	s.queue = append(s.queue, decodeSSEPayload(s.provider, s.protocol, eventName, []byte(payload))...)
}

func decodeSSEPayload(providerName string, protocol canonical.Protocol, eventName string, raw []byte) []stream.Event {
	if strings.TrimSpace(eventName) == "" {
		return DecodePayload(providerName, protocol, raw)
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return DecodePayload(providerName, protocol, raw)
	}
	if _, exists := root["type"]; !exists {
		root["type"] = eventName
		if encoded, err := json.Marshal(root); err == nil {
			raw = encoded
		}
	}
	return DecodePayload(providerName, protocol, raw)
}

func (s *responseStream) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.resp != nil && s.resp.Body != nil {
		return s.resp.Body.Close()
	}
	return nil
}
