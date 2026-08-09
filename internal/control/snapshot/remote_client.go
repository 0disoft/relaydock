package snapshot

import (
	"bufio"
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/core"
)

const defaultMaximumSnapshotBytes int64 = 32 << 20

type RemoteClient struct {
	BaseURL             string
	BearerToken         string
	Verifier            Verifier
	HTTPClient          *http.Client
	MaximumSnapshotSize int64
	MaximumFutureSkew   time.Duration
	Now                 func() time.Time
}

func NewRemoteClient(baseURL, bearerToken string, publicKey ed25519.PublicKey, client *http.Client) (*RemoteClient, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%w: control-plane Ed25519 public key", core.ErrInvalidConfiguration)
	}
	return NewRemoteClientWithVerifier(baseURL, bearerToken, NewEd25519Signer(nil, publicKey), client)
}

func NewRemoteClientWithVerifier(baseURL, bearerToken string, verifier Verifier, client *http.Client) (*RemoteClient, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Fragment != "" {
		return nil, fmt.Errorf("%w: invalid control-plane URL", core.ErrInvalidConfiguration)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("%w: control-plane URL must use http or https", core.ErrInvalidConfiguration)
	}
	if verifier == nil {
		return nil, fmt.Errorf("%w: control-plane snapshot verifier", core.ErrInvalidConfiguration)
	}
	if client == nil {
		client = &http.Client{Transport: http.DefaultTransport}
	}
	return &RemoteClient{
		BaseURL: baseURL, BearerToken: strings.TrimSpace(bearerToken), Verifier: verifier, HTTPClient: client,
		MaximumSnapshotSize: defaultMaximumSnapshotBytes, MaximumFutureSkew: 5 * time.Minute,
		Now: func() time.Time { return time.Now().UTC() },
	}, nil
}

func (c *RemoteClient) FetchCurrent(ctx context.Context, etag string) (Snapshot, string, error) {
	if err := c.validate(); err != nil {
		return Snapshot{}, "", err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/snapshot", nil)
	if err != nil {
		return Snapshot{}, "", err
	}
	request.Header.Set("Accept", "application/json")
	if etag = strings.TrimSpace(etag); etag != "" {
		request.Header.Set("If-None-Match", etag)
	}
	c.authorize(request)
	response, err := c.HTTPClient.Do(request)
	if err != nil {
		return Snapshot{}, "", fmt.Errorf("fetch runtime snapshot: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotModified {
		return Snapshot{}, strings.TrimSpace(response.Header.Get("ETag")), core.ErrNotModified
	}
	if response.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		return Snapshot{}, "", fmt.Errorf("fetch runtime snapshot: HTTP %d: %s", response.StatusCode, boundedRemoteMessage(message))
	}
	raw, err := readSnapshotBody(response.Body, c.maximumBytes())
	if err != nil {
		return Snapshot{}, "", err
	}
	var value Snapshot
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return Snapshot{}, "", fmt.Errorf("decode runtime snapshot: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Snapshot{}, "", err
	}
	if err := c.verify(value); err != nil {
		return Snapshot{}, "", err
	}
	return value, strings.TrimSpace(response.Header.Get("ETag")), nil
}

// Watch blocks until the server closes the event stream, the context is
// cancelled, or a snapshot fails validation. The caller owns reconnection and
// backoff so it can combine watch failures with last-known-good policy.
func (c *RemoteClient) Watch(ctx context.Context, after int64, consume func(context.Context, Snapshot) error) error {
	if err := c.validate(); err != nil {
		return err
	}
	if consume == nil {
		return fmt.Errorf("%w: snapshot watch consumer", core.ErrInvalidArgument)
	}
	endpoint := c.BaseURL + "/v1/snapshots/watch?after=" + strconv.FormatInt(after, 10)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "text/event-stream")
	request.Header.Set("Cache-Control", "no-cache")
	c.authorize(request)
	response, err := c.HTTPClient.Do(request)
	if err != nil {
		return fmt.Errorf("watch runtime snapshots: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
		return fmt.Errorf("watch runtime snapshots: HTTP %d: %s", response.StatusCode, boundedRemoteMessage(message))
	}
	contentType := strings.ToLower(response.Header.Get("Content-Type"))
	if !strings.Contains(contentType, "text/event-stream") {
		return fmt.Errorf("%w: control watch returned %q", core.ErrInvalidConfiguration, response.Header.Get("Content-Type"))
	}

	scanner := bufio.NewScanner(io.LimitReader(response.Body, c.maximumBytes()*4))
	scanner.Buffer(make([]byte, 64<<10), int(c.maximumBytes()))
	var eventName string
	var data strings.Builder
	flush := func() error {
		payload := strings.TrimSpace(data.String())
		data.Reset()
		name := strings.TrimSpace(eventName)
		eventName = ""
		if payload == "" || (name != "" && name != "snapshot") {
			return nil
		}
		var value Snapshot
		decoder := json.NewDecoder(strings.NewReader(payload))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&value); err != nil {
			return fmt.Errorf("decode watched runtime snapshot: %w", err)
		}
		if err := ensureJSONEOF(decoder); err != nil {
			return err
		}
		if err := c.verify(value); err != nil {
			return err
		}
		if value.Revision <= after {
			return nil
		}
		if err := consume(ctx, value); err != nil {
			return err
		}
		after = value.Revision
		return nil
	}

	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, found := strings.Cut(line, ":")
		if !found {
			field, value = line, ""
		} else {
			value = strings.TrimPrefix(value, " ")
		}
		switch field {
		case "event":
			eventName = value
		case "data":
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			if int64(data.Len()+len(value)) > c.maximumBytes() {
				return core.ErrFrameTooLarge
			}
			data.WriteString(value)
		case "id", "retry":
			// Transport metadata. Revision is taken from the signed payload.
		default:
			// Unknown SSE fields must be ignored.
		}
	}
	if err := scanner.Err(); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return fmt.Errorf("read runtime snapshot stream: %w", err)
	}
	if data.Len() > 0 {
		if err := flush(); err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return io.ErrUnexpectedEOF
}

func (c *RemoteClient) verify(value Snapshot) error {
	if err := c.Verifier.Verify(context.Background(), value); err != nil {
		return fmt.Errorf("verify runtime snapshot signature: %w", err)
	}
	if err := Validate(value, ValidationOptions{
		Now:               c.now(),
		MaximumFutureSkew: c.MaximumFutureSkew,
		RequireUnexpired:  true,
	}); err != nil {
		return err
	}
	return nil
}

func (c *RemoteClient) authorize(request *http.Request) {
	if token := strings.TrimSpace(c.BearerToken); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
}

func (c *RemoteClient) validate() error {
	if c == nil || strings.TrimSpace(c.BaseURL) == "" || c.HTTPClient == nil || c.Verifier == nil {
		return fmt.Errorf("%w: remote snapshot client", core.ErrInvalidConfiguration)
	}
	return nil
}

func (c *RemoteClient) maximumBytes() int64 {
	if c.MaximumSnapshotSize <= 0 {
		return defaultMaximumSnapshotBytes
	}
	return c.MaximumSnapshotSize
}

func (c *RemoteClient) now() time.Time {
	if c.Now == nil {
		return time.Now().UTC()
	}
	return c.Now().UTC()
}

func readSnapshotBody(reader io.Reader, maximum int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, maximum+1))
	if err != nil {
		return nil, fmt.Errorf("read runtime snapshot: %w", err)
	}
	if int64(len(raw)) > maximum {
		return nil, core.ErrFrameTooLarge
	}
	return raw, nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return fmt.Errorf("%w: multiple JSON values", core.ErrInvalidArgument)
		}
		return fmt.Errorf("%w: malformed trailing JSON: %v", core.ErrInvalidArgument, err)
	}
	return nil
}

func boundedRemoteMessage(raw []byte) string {
	value := strings.TrimSpace(string(raw))
	if value == "" {
		return "empty response"
	}
	if len(value) > 1024 {
		return value[:1024] + "…"
	}
	return value
}
