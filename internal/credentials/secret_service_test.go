package credentials

/* llmnav/1 file
id=relaydock.credentials.secret-service.contract
role=Verify Linux Secret Service create, unlock, read, delete, cancellation, byte clearing, and remote error redaction behavior.
search=Secret Service tests|zero credential transport value|redact D-Bus error
stability=contract
*/

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/godbus/dbus/v5"
)

type fakeSecretServiceResponse struct {
	body []any
	err  error
}

type fakeSecretServicePrompt struct {
	dismissed bool
	result    dbus.Variant
	err       error
}

type fakeSecretServiceTransport struct {
	responses          map[string][]fakeSecretServiceResponse
	prompts            map[dbus.ObjectPath]fakeSecretServicePrompt
	secret             secretServiceSecret
	createdValue       []byte
	createdValueHandle []byte
	createdAttributes  map[string]string
	closed             bool
}

func (f *fakeSecretServiceTransport) Call(ctx context.Context, _ dbus.ObjectPath, method string, args ...any) ([]any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if method == secretCollectionInterface+".CreateItem" {
		properties, ok := args[0].(map[string]dbus.Variant)
		if !ok {
			return nil, errors.New("invalid test properties")
		}
		attributes, ok := properties[secretItemInterface+".Attributes"].Value().(map[string]string)
		if !ok {
			return nil, errors.New("invalid test attributes")
		}
		f.createdAttributes = attributes
		secret, ok := args[1].(secretServiceSecret)
		if !ok {
			return nil, errors.New("invalid test secret")
		}
		f.createdValue = append([]byte(nil), secret.Value...)
		f.createdValueHandle = secret.Value
	}
	queue := f.responses[method]
	if len(queue) == 0 {
		return nil, errors.New("unexpected test call: " + method)
	}
	response := queue[0]
	f.responses[method] = queue[1:]
	return response.body, response.err
}

func (f *fakeSecretServiceTransport) GetSecret(ctx context.Context, _, _ dbus.ObjectPath) (secretServiceSecret, error) {
	if err := ctx.Err(); err != nil {
		return secretServiceSecret{}, err
	}
	return f.secret, nil
}

func (f *fakeSecretServiceTransport) Prompt(ctx context.Context, path dbus.ObjectPath) (bool, dbus.Variant, error) {
	if err := ctx.Err(); err != nil {
		return false, dbus.Variant{}, err
	}
	prompt, ok := f.prompts[path]
	if !ok {
		return false, dbus.Variant{}, errors.New("unexpected test prompt")
	}
	return prompt.dismissed, prompt.result, prompt.err
}

func (f *fakeSecretServiceTransport) Close() error {
	f.closed = true
	return nil
}

func TestLinuxCredentialBackendCreatesItemAndZerosTransportValue(t *testing.T) {
	session := dbus.ObjectPath("/session/1")
	collection := dbus.ObjectPath("/collection/default")
	item := dbus.ObjectPath("/collection/default/item/1")
	fake := &fakeSecretServiceTransport{responses: map[string][]fakeSecretServiceResponse{
		secretServiceInterface + ".OpenSession":   {{body: []any{dbus.MakeVariant(""), session}}},
		secretServiceInterface + ".ReadAlias":     {{body: []any{collection}}},
		secretServiceInterface + ".Unlock":        {{body: []any{[]dbus.ObjectPath{collection}, secretServiceMissingPath}}},
		secretCollectionInterface + ".CreateItem": {{body: []any{item, secretServiceMissingPath}}},
	}}
	backend := linuxBackendWithFake(fake)
	value := []byte("provider-key")
	if err := backend.Write(context.Background(), "opaque-target", value); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(fake.createdValue, value) {
		t.Fatalf("created value=%q", fake.createdValue)
	}
	if !bytes.Equal(fake.createdValueHandle, make([]byte, len(value))) {
		t.Fatal("transport credential buffer was not zeroed")
	}
	if fake.createdAttributes[secretServiceAttributeTarget] != "opaque-target" || fake.createdAttributes[secretServiceAttributeApp] != secretServiceApplicationValue {
		t.Fatalf("created attributes=%#v", fake.createdAttributes)
	}
	if !fake.closed {
		t.Fatal("transport was not closed")
	}
}

func TestLinuxCredentialBackendUnlocksPromptAndCopiesSecret(t *testing.T) {
	session := dbus.ObjectPath("/session/1")
	item := dbus.ObjectPath("/collection/default/item/1")
	prompt := dbus.ObjectPath("/prompt/1")
	original := []byte("stored-provider-key")
	fake := &fakeSecretServiceTransport{
		responses: map[string][]fakeSecretServiceResponse{
			secretServiceInterface + ".OpenSession": {{body: []any{dbus.MakeVariant(""), session}}},
			secretServiceInterface + ".SearchItems": {{body: []any{[]dbus.ObjectPath{}, []dbus.ObjectPath{item}}}},
			secretServiceInterface + ".Unlock":      {{body: []any{[]dbus.ObjectPath{}, prompt}}},
		},
		prompts: map[dbus.ObjectPath]fakeSecretServicePrompt{
			prompt: {result: dbus.MakeVariant([]dbus.ObjectPath{item})},
		},
		secret: secretServiceSecret{Session: session, Value: original, ContentType: "application/octet-stream"},
	}
	backend := linuxBackendWithFake(fake)
	value, err := backend.Read(context.Background(), "opaque-target")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(value, []byte("stored-provider-key")) {
		t.Fatalf("read value=%q", value)
	}
	if !bytes.Equal(original, make([]byte, len(original))) {
		t.Fatal("transport read buffer was not zeroed")
	}
}

func TestLinuxCredentialBackendDeletesUnlockedItem(t *testing.T) {
	item := dbus.ObjectPath("/collection/default/item/1")
	fake := fakeLinuxSearchTransport([]dbus.ObjectPath{item}, []dbus.ObjectPath{})
	fake.responses[secretItemInterface+".Delete"] = []fakeSecretServiceResponse{{body: []any{secretServiceMissingPath}}}
	if err := linuxBackendWithFake(fake).Delete(context.Background(), "opaque-target"); err != nil {
		t.Fatal(err)
	}
	if !fake.closed {
		t.Fatal("transport was not closed")
	}
}

func TestLinuxCredentialBackendPropagatesCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := linuxBackendWithFake(fakeLinuxSearchTransport(nil, nil)).Read(ctx, "opaque-target")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v, want context.Canceled", err)
	}
}

func TestLinuxCredentialBackendRejectsMissingDuplicateAndDismissedItems(t *testing.T) {
	tests := []struct {
		name string
		fake *fakeSecretServiceTransport
		want error
	}{
		{
			name: "missing",
			fake: fakeLinuxSearchTransport([]dbus.ObjectPath{}, []dbus.ObjectPath{}),
			want: core.ErrNotFound,
		},
		{
			name: "duplicate",
			fake: fakeLinuxSearchTransport([]dbus.ObjectPath{"/item/1", "/item/2"}, []dbus.ObjectPath{}),
			want: core.ErrInvalidConfiguration,
		},
		{
			name: "dismissed",
			fake: func() *fakeSecretServiceTransport {
				fake := fakeLinuxSearchTransport([]dbus.ObjectPath{}, []dbus.ObjectPath{"/item/1"})
				fake.responses[secretServiceInterface+".Unlock"] = []fakeSecretServiceResponse{{body: []any{[]dbus.ObjectPath{}, dbus.ObjectPath("/prompt/1")}}}
				fake.prompts = map[dbus.ObjectPath]fakeSecretServicePrompt{"/prompt/1": {dismissed: true}}
				return fake
			}(),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := linuxBackendWithFake(test.fake).Read(context.Background(), "opaque-target")
			if err == nil {
				t.Fatal("expected read failure")
			}
			if test.want != nil && !errors.Is(err, test.want) {
				t.Fatalf("error=%v, want %v", err, test.want)
			}
		})
	}
}

func TestSecretServiceDBusErrorOmitsRemoteBody(t *testing.T) {
	secret := "remote-error-secret"
	err := secretServiceDBusError("read", dbus.NewError("org.freedesktop.Secret.Error.IsLocked", []any{secret}))
	if err == nil || strings.Contains(err.Error(), secret) {
		t.Fatalf("unsafe error: %v", err)
	}
	unavailable := secretServiceDBusError("read", dbus.NewError("org.freedesktop.DBus.Error.ServiceUnknown", nil))
	if !errors.Is(unavailable, ErrSystemStoreUnavailable) {
		t.Fatalf("service error=%v", unavailable)
	}
}

func linuxBackendWithFake(fake *fakeSecretServiceTransport) linuxCredentialBackend {
	return linuxCredentialBackend{connect: func(context.Context) (secretServiceTransport, error) {
		return fake, nil
	}}
}

func fakeLinuxSearchTransport(unlocked, locked []dbus.ObjectPath) *fakeSecretServiceTransport {
	return &fakeSecretServiceTransport{responses: map[string][]fakeSecretServiceResponse{
		secretServiceInterface + ".OpenSession": {{body: []any{dbus.MakeVariant(""), dbus.ObjectPath("/session/1")}}},
		secretServiceInterface + ".SearchItems": {{body: []any{unlocked, locked}}},
	}}
}
