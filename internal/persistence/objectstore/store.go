package objectstore

import (
	"context"
	"io"
	"time"
)

type Object struct {
	Key         string
	Size        int64
	ContentType string
	ExpiresAt   time.Time
}

type Store interface {
	Put(context.Context, Object, io.Reader) error
	Get(context.Context, string) (io.ReadCloser, Object, error)
	Delete(context.Context, string) error
}
