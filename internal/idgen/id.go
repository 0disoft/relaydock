package idgen

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"strings"
	"time"
)

var encoding = base32.NewEncoding("0123456789abcdefghjkmnpqrstvwxyz").WithPadding(base32.NoPadding)

// New returns a sortable, URL-safe identifier. The timestamp prefix is useful
// for operations, while the random suffix prevents collisions across processes.
func New(prefix string) string {
	var entropy [10]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		// crypto/rand failures indicate a broken host. Keep the identifier unique
		// enough for error paths without panicking the entire process.
		now := time.Now().UnixNano()
		for i := range entropy {
			entropy[i] = byte(now >> (i * 6))
		}
	}
	stamp := strings.ToLower(encoding.EncodeToString([]byte(fmt.Sprintf("%013d", time.Now().UTC().UnixMilli()))))
	random := strings.ToLower(encoding.EncodeToString(entropy[:]))
	if prefix == "" {
		return stamp + random
	}
	return prefix + "_" + stamp + random
}
