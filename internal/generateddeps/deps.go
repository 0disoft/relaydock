//go:build tools

// Package generateddeps anchors dependencies required by generated API
// contracts even when generated output is absent from a clean checkout.
package generateddeps

import (
	_ "connectrpc.com/connect"
	_ "google.golang.org/protobuf/reflect/protoreflect"
)
