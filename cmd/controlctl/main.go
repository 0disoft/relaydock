package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/your-org/ai-runtime-gateway/internal/persistence/atomicfile"
)

const maximumControlResponseBytes int64 = 32 << 20

type client struct {
	baseURL string
	token   string
	http    *http.Client
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	api, err := newClient(
		envOr("CONTROL_API_BASE_URL", "http://127.0.0.1:8081"),
		strings.TrimSpace(os.Getenv("CONTROL_BEARER_TOKEN")),
	)
	if err != nil {
		fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	switch os.Args[1] {
	case "signing-key":
		set := flag.NewFlagSet("signing-key", flag.ExitOnError)
		raw := set.Bool("raw", false, "print only the base64url public key")
		trustEntry := set.Bool("trust-entry", false, "print keyId=base64url-public-key for a trusted-key environment variable")
		_ = set.Parse(os.Args[2:])
		body, err := api.get(ctx, "/v1/signing-key")
		if err != nil {
			fail(err)
		}
		if *raw || *trustEntry {
			var payload struct {
				KeyID     string `json:"keyId"`
				PublicKey string `json:"publicKey"`
			}
			if err := decodeStrict(body, &payload); err != nil || strings.TrimSpace(payload.PublicKey) == "" {
				fail(fmt.Errorf("decode signing key: %w", err))
			}
			if *trustEntry {
				if strings.TrimSpace(payload.KeyID) == "" {
					fail(errors.New("control did not return a signing key ID"))
				}
				fmt.Printf("%s=%s\n", payload.KeyID, payload.PublicKey)
			} else {
				fmt.Println(payload.PublicKey)
			}
			return
		}
		writeOutput(body, "")
	case "snapshot":
		set := flag.NewFlagSet("snapshot", flag.ExitOnError)
		output := set.String("output", "", "write the signed snapshot atomically to this file")
		_ = set.Parse(os.Args[2:])
		body, err := api.get(ctx, "/v1/snapshot")
		if err != nil {
			fail(err)
		}
		writeOutput(body, *output)
	case "models":
		body, err := api.get(ctx, "/v1/models")
		if err != nil {
			fail(err)
		}
		writeOutput(body, "")
	case "publish":
		set := flag.NewFlagSet("publish", flag.ExitOnError)
		file := set.String("file", "", "JSON file containing a controlhttp.PublishRequest")
		_ = set.Parse(os.Args[2:])
		if strings.TrimSpace(*file) == "" {
			fail(errors.New("publish requires --file"))
		}
		body, err := atomicfile.Read(*file, maximumControlResponseBytes)
		if err != nil {
			fail(err)
		}
		if len(body) == 0 || !json.Valid(body) {
			fail(errors.New("publish file must contain one valid JSON object"))
		}
		response, err := api.post(ctx, "/v1/snapshot", body)
		if err != nil {
			fail(err)
		}
		writeOutput(response, "")
	default:
		usage()
		os.Exit(2)
	}
}

func newClient(rawURL, token string) (*client, error) {
	rawURL = strings.TrimRight(strings.TrimSpace(rawURL), "/")
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("invalid CONTROL_API_BASE_URL")
	}
	if parsed.User != nil || parsed.Fragment != "" || parsed.RawQuery != "" {
		return nil, fmt.Errorf("CONTROL_API_BASE_URL must not contain credentials, query, or fragment")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.ResponseHeaderTimeout = 20 * time.Second
	return &client{
		baseURL: rawURL,
		token:   token,
		http: &http.Client{
			Transport:     transport,
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}, nil
}

func (c *client) get(ctx context.Context, path string) ([]byte, error) {
	return c.do(ctx, http.MethodGet, path, nil)
}

func (c *client) post(ctx context.Context, path string, body []byte) ([]byte, error) {
	return c.do(ctx, http.MethodPost, path, body)
}

func (c *client) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, fmt.Errorf("control request: %w", err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, maximumControlResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read control response: %w", err)
	}
	if int64(len(payload)) > maximumControlResponseBytes {
		return nil, fmt.Errorf("control response exceeds %d bytes", maximumControlResponseBytes)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := strings.TrimSpace(string(payload))
		if len(message) > 2048 {
			message = message[:2048] + "…"
		}
		return nil, fmt.Errorf("control returned HTTP %d: %s", response.StatusCode, message)
	}
	if !json.Valid(payload) {
		return nil, fmt.Errorf("control returned malformed JSON")
	}
	return payload, nil
}

func decodeStrict(raw []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("multiple JSON values")
	}
	return nil
}

func writeOutput(body []byte, path string) {
	var formatted bytes.Buffer
	if err := json.Indent(&formatted, body, "", "  "); err != nil {
		fail(err)
	}
	formatted.WriteByte('\n')
	if strings.TrimSpace(path) == "" {
		_, _ = os.Stdout.Write(formatted.Bytes())
		return
	}
	if err := atomicfile.Write(path, formatted.Bytes(), 0o600); err != nil {
		fail(err)
	}
	fmt.Fprintln(os.Stderr, path)
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func usage() {
	fmt.Fprintln(os.Stderr, `Usage:
  controlctl signing-key [--raw | --trust-entry]
  controlctl snapshot [--output path]
  controlctl models
  controlctl publish --file publish-request.json

Environment:
  CONTROL_API_BASE_URL  default http://127.0.0.1:8081
  CONTROL_BEARER_TOKEN  required when controld authentication is enabled`)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "controlctl:", err)
	os.Exit(1)
}
