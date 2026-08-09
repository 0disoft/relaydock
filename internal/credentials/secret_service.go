package credentials

import (
	"context"
	"errors"
	"fmt"

	"github.com/0disoft/relaydock/internal/core"
	"github.com/godbus/dbus/v5"
)

const (
	secretServiceBus              = "org.freedesktop.secrets"
	secretServicePath             = dbus.ObjectPath("/org/freedesktop/secrets")
	secretServiceInterface        = "org.freedesktop.Secret.Service"
	secretCollectionInterface     = "org.freedesktop.Secret.Collection"
	secretItemInterface           = "org.freedesktop.Secret.Item"
	secretPromptInterface         = "org.freedesktop.Secret.Prompt"
	secretPromptCompleted         = secretPromptInterface + ".Completed"
	secretServiceMissingPath      = dbus.ObjectPath("/")
	secretServiceAttributeApp     = "relaydock-application"
	secretServiceAttributeTarget  = "relaydock-target"
	secretServiceApplicationValue = "RelayDock"
)

type secretServiceSecret struct {
	Session     dbus.ObjectPath
	Parameters  []byte
	Value       []byte
	ContentType string
}

type secretServiceTransport interface {
	Call(ctx context.Context, path dbus.ObjectPath, method string, args ...any) ([]any, error)
	GetSecret(ctx context.Context, item, session dbus.ObjectPath) (secretServiceSecret, error)
	Prompt(ctx context.Context, path dbus.ObjectPath) (bool, dbus.Variant, error)
	Close() error
}

type dbusSecretServiceTransport struct {
	conn *dbus.Conn
}

type linuxCredentialBackend struct {
	connect func(context.Context) (secretServiceTransport, error)
}

func connectSecretService(ctx context.Context) (secretServiceTransport, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("%w: D-Bus session bus is unavailable", ErrSystemStoreUnavailable)
	}
	return &dbusSecretServiceTransport{conn: conn}, nil
}

func (t *dbusSecretServiceTransport) Call(ctx context.Context, path dbus.ObjectPath, method string, args ...any) ([]any, error) {
	call := t.conn.Object(secretServiceBus, path).CallWithContext(ctx, method, 0, args...)
	if call.Err != nil {
		return nil, secretServiceDBusError(method, call.Err)
	}
	return call.Body, nil
}

func (t *dbusSecretServiceTransport) GetSecret(ctx context.Context, item, session dbus.ObjectPath) (secretServiceSecret, error) {
	var secret secretServiceSecret
	call := t.conn.Object(secretServiceBus, item).CallWithContext(ctx, secretItemInterface+".GetSecret", 0, session)
	if err := call.Store(&secret); err != nil {
		return secretServiceSecret{}, secretServiceDBusError("get secret", err)
	}
	return secret, nil
}

func (t *dbusSecretServiceTransport) Prompt(ctx context.Context, path dbus.ObjectPath) (bool, dbus.Variant, error) {
	options := []dbus.MatchOption{
		dbus.WithMatchObjectPath(path),
		dbus.WithMatchInterface(secretPromptInterface),
		dbus.WithMatchMember("Completed"),
	}
	signals := make(chan *dbus.Signal, 1)
	t.conn.Signal(signals)
	defer t.conn.RemoveSignal(signals)
	if err := t.conn.AddMatchSignalContext(ctx, options...); err != nil {
		return false, dbus.Variant{}, secretServiceDBusError("subscribe prompt", err)
	}
	defer func() { _ = t.conn.RemoveMatchSignal(options...) }()
	if _, err := t.Call(ctx, path, secretPromptInterface+".Prompt", ""); err != nil {
		return false, dbus.Variant{}, err
	}
	for {
		select {
		case <-ctx.Done():
			return false, dbus.Variant{}, ctx.Err()
		case signal, ok := <-signals:
			if !ok {
				return false, dbus.Variant{}, fmt.Errorf("%w: D-Bus connection closed during prompt", ErrSystemStoreUnavailable)
			}
			if signal == nil || signal.Path != path || signal.Name != secretPromptCompleted || len(signal.Body) != 2 {
				continue
			}
			dismissed, ok := signal.Body[0].(bool)
			if !ok {
				return false, dbus.Variant{}, fmt.Errorf("%w: malformed Secret Service prompt result", core.ErrInvalidConfiguration)
			}
			result, ok := signal.Body[1].(dbus.Variant)
			if !ok {
				return false, dbus.Variant{}, fmt.Errorf("%w: malformed Secret Service prompt value", core.ErrInvalidConfiguration)
			}
			return dismissed, result, nil
		}
	}
}

func (t *dbusSecretServiceTransport) Close() error {
	return t.conn.Close()
}

func (b linuxCredentialBackend) Write(ctx context.Context, target string, value []byte) error {
	transport, err := b.open(ctx)
	if err != nil {
		return err
	}
	defer transport.Close()
	session, err := openSecretServiceSession(ctx, transport)
	if err != nil {
		return err
	}
	collection, err := readSecretServiceDefaultCollection(ctx, transport)
	if err != nil {
		return err
	}
	if err := unlockSecretServiceObjects(ctx, transport, []dbus.ObjectPath{collection}); err != nil {
		return err
	}
	secret := secretServiceSecret{
		Session: session, Value: append([]byte(nil), value...), ContentType: "application/octet-stream",
	}
	defer zeroCredential(secret.Value)
	properties := map[string]dbus.Variant{
		secretItemInterface + ".Label":      dbus.MakeVariant("RelayDock credential"),
		secretItemInterface + ".Attributes": dbus.MakeVariant(secretServiceAttributes(target)),
	}
	body, err := transport.Call(ctx, collection, secretCollectionInterface+".CreateItem", properties, secret, true)
	if err != nil {
		return err
	}
	item, prompt, err := secretServiceObjectAndPrompt(body, "create item")
	if err != nil {
		return err
	}
	if prompt != secretServiceMissingPath {
		dismissed, result, err := transport.Prompt(ctx, prompt)
		if err != nil {
			return err
		}
		if dismissed {
			return fmt.Errorf("Secret Service create prompt was dismissed")
		}
		item, err = secretServiceObjectPath(result.Value(), "create prompt")
		if err != nil {
			return err
		}
	}
	if item == secretServiceMissingPath {
		return fmt.Errorf("%w: Secret Service returned no created item", core.ErrInvalidConfiguration)
	}
	return nil
}

func (b linuxCredentialBackend) Read(ctx context.Context, target string) ([]byte, error) {
	transport, err := b.open(ctx)
	if err != nil {
		return nil, err
	}
	defer transport.Close()
	session, err := openSecretServiceSession(ctx, transport)
	if err != nil {
		return nil, err
	}
	item, locked, err := findSecretServiceItem(ctx, transport, target)
	if err != nil {
		return nil, err
	}
	if locked {
		if err := unlockSecretServiceObjects(ctx, transport, []dbus.ObjectPath{item}); err != nil {
			return nil, err
		}
	}
	secret, err := transport.GetSecret(ctx, item, session)
	if err != nil {
		return nil, err
	}
	if secret.Session != session || len(secret.Parameters) != 0 {
		return nil, fmt.Errorf("%w: invalid Secret Service plain-session secret", core.ErrInvalidConfiguration)
	}
	if len(secret.Value) == 0 || len(secret.Value) > MaximumSystemCredentialBytes {
		zeroCredential(secret.Value)
		return nil, fmt.Errorf("%w: Secret Service returned an invalid credential size", core.ErrInvalidConfiguration)
	}
	result := append([]byte(nil), secret.Value...)
	zeroCredential(secret.Value)
	return result, nil
}

func (b linuxCredentialBackend) Delete(ctx context.Context, target string) error {
	transport, err := b.open(ctx)
	if err != nil {
		return err
	}
	defer transport.Close()
	item, locked, err := findSecretServiceItem(ctx, transport, target)
	if err != nil {
		return err
	}
	if locked {
		if err := unlockSecretServiceObjects(ctx, transport, []dbus.ObjectPath{item}); err != nil {
			return err
		}
	}
	body, err := transport.Call(ctx, item, secretItemInterface+".Delete")
	if err != nil {
		return err
	}
	if len(body) != 1 {
		return fmt.Errorf("%w: malformed Secret Service delete response", core.ErrInvalidConfiguration)
	}
	prompt, err := secretServiceObjectPath(body[0], "delete")
	if err != nil {
		return err
	}
	if prompt == secretServiceMissingPath {
		return nil
	}
	dismissed, _, err := transport.Prompt(ctx, prompt)
	if err != nil {
		return err
	}
	if dismissed {
		return fmt.Errorf("Secret Service delete prompt was dismissed")
	}
	return nil
}

func (b linuxCredentialBackend) open(ctx context.Context) (secretServiceTransport, error) {
	if b.connect == nil {
		return nil, fmt.Errorf("%w: Secret Service connector", core.ErrInvalidConfiguration)
	}
	transport, err := b.connect(ctx)
	if err != nil {
		return nil, err
	}
	if transport == nil {
		return nil, fmt.Errorf("%w: Secret Service transport", core.ErrInvalidConfiguration)
	}
	return transport, nil
}

func openSecretServiceSession(ctx context.Context, transport secretServiceTransport) (dbus.ObjectPath, error) {
	body, err := transport.Call(ctx, secretServicePath, secretServiceInterface+".OpenSession", "plain", dbus.MakeVariant(""))
	if err != nil {
		return "", err
	}
	if len(body) != 2 {
		return "", fmt.Errorf("%w: malformed Secret Service session response", core.ErrInvalidConfiguration)
	}
	output, ok := body[0].(dbus.Variant)
	if !ok || output.Value() != "" {
		return "", fmt.Errorf("%w: Secret Service returned an invalid plain-session negotiation", core.ErrInvalidConfiguration)
	}
	session, err := secretServiceObjectPath(body[1], "session")
	if err != nil || session == secretServiceMissingPath {
		return "", fmt.Errorf("%w: Secret Service returned no session", core.ErrInvalidConfiguration)
	}
	return session, nil
}

func readSecretServiceDefaultCollection(ctx context.Context, transport secretServiceTransport) (dbus.ObjectPath, error) {
	body, err := transport.Call(ctx, secretServicePath, secretServiceInterface+".ReadAlias", "default")
	if err != nil {
		return "", err
	}
	if len(body) != 1 {
		return "", fmt.Errorf("%w: malformed Secret Service alias response", core.ErrInvalidConfiguration)
	}
	collection, err := secretServiceObjectPath(body[0], "default collection")
	if err != nil {
		return "", err
	}
	if collection == secretServiceMissingPath {
		return "", fmt.Errorf("%w: Secret Service has no default collection", ErrSystemStoreUnavailable)
	}
	return collection, nil
}

func findSecretServiceItem(ctx context.Context, transport secretServiceTransport, target string) (dbus.ObjectPath, bool, error) {
	body, err := transport.Call(ctx, secretServicePath, secretServiceInterface+".SearchItems", secretServiceAttributes(target))
	if err != nil {
		return "", false, err
	}
	if len(body) != 2 {
		return "", false, fmt.Errorf("%w: malformed Secret Service search response", core.ErrInvalidConfiguration)
	}
	unlocked, okUnlocked := body[0].([]dbus.ObjectPath)
	locked, okLocked := body[1].([]dbus.ObjectPath)
	if !okUnlocked || !okLocked || !secretServicePathsValid(unlocked) || !secretServicePathsValid(locked) {
		return "", false, fmt.Errorf("%w: invalid Secret Service search paths", core.ErrInvalidConfiguration)
	}
	if len(unlocked)+len(locked) == 0 {
		return "", false, core.ErrNotFound
	}
	if len(unlocked)+len(locked) != 1 {
		return "", false, fmt.Errorf("%w: duplicate Secret Service credential items", core.ErrInvalidConfiguration)
	}
	if len(unlocked) == 1 {
		return unlocked[0], false, nil
	}
	return locked[0], true, nil
}

func unlockSecretServiceObjects(ctx context.Context, transport secretServiceTransport, objects []dbus.ObjectPath) error {
	body, err := transport.Call(ctx, secretServicePath, secretServiceInterface+".Unlock", objects)
	if err != nil {
		return err
	}
	if len(body) != 2 {
		return fmt.Errorf("%w: malformed Secret Service unlock response", core.ErrInvalidConfiguration)
	}
	unlocked, ok := body[0].([]dbus.ObjectPath)
	if !ok || !secretServicePathsValid(unlocked) {
		return fmt.Errorf("%w: invalid Secret Service unlock paths", core.ErrInvalidConfiguration)
	}
	prompt, err := secretServiceObjectPath(body[1], "unlock")
	if err != nil {
		return err
	}
	if prompt != secretServiceMissingPath {
		dismissed, result, err := transport.Prompt(ctx, prompt)
		if err != nil {
			return err
		}
		if dismissed {
			return fmt.Errorf("Secret Service unlock prompt was dismissed")
		}
		promptUnlocked, ok := result.Value().([]dbus.ObjectPath)
		if !ok || !secretServicePathsValid(promptUnlocked) {
			return fmt.Errorf("%w: malformed Secret Service unlock prompt result", core.ErrInvalidConfiguration)
		}
		unlocked = append(unlocked, promptUnlocked...)
	}
	for _, wanted := range objects {
		found := false
		for _, path := range unlocked {
			if path == wanted {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("Secret Service did not unlock the requested object")
		}
	}
	return nil
}

func secretServiceAttributes(target string) map[string]string {
	return map[string]string{
		secretServiceAttributeApp:    secretServiceApplicationValue,
		secretServiceAttributeTarget: target,
	}
}

func secretServiceObjectAndPrompt(body []any, operation string) (dbus.ObjectPath, dbus.ObjectPath, error) {
	if len(body) != 2 {
		return "", "", fmt.Errorf("%w: malformed Secret Service %s response", core.ErrInvalidConfiguration, operation)
	}
	object, err := secretServiceObjectPath(body[0], operation)
	if err != nil {
		return "", "", err
	}
	prompt, err := secretServiceObjectPath(body[1], operation+" prompt")
	return object, prompt, err
}

func secretServiceObjectPath(value any, field string) (dbus.ObjectPath, error) {
	path, ok := value.(dbus.ObjectPath)
	if !ok || !path.IsValid() {
		return "", fmt.Errorf("%w: invalid Secret Service %s path", core.ErrInvalidConfiguration, field)
	}
	return path, nil
}

func secretServicePathsValid(paths []dbus.ObjectPath) bool {
	for _, path := range paths {
		if path == secretServiceMissingPath || !path.IsValid() {
			return false
		}
	}
	return true
}

func secretServiceDBusError(operation string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var dbusErr *dbus.Error
	if errors.As(err, &dbusErr) {
		switch dbusErr.Name {
		case "org.freedesktop.DBus.Error.ServiceUnknown", "org.freedesktop.DBus.Error.NameHasNoOwner":
			return fmt.Errorf("%w: Secret Service is unavailable", ErrSystemStoreUnavailable)
		default:
			return fmt.Errorf("Secret Service %s failed with D-Bus error %s", operation, dbusErr.Name)
		}
	}
	return fmt.Errorf("Secret Service %s failed", operation)
}
