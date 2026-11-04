package blobstore

import (
	"context"
	"fmt"
)

// Prefixed is a store inside another, every key under prefix: what shares a
// bucket with other files reaches none of them. A key is checked before the
// prefix goes on, so nothing a caller passes resolves outside it.
func Prefixed(store Store, prefix string) (Store, error) {
	if err := checkKey(prefix); err != nil {
		return nil, fmt.Errorf("blobstore: prefix: %w", err)
	}
	return prefixed{store, prefix + "/"}, nil
}

type prefixed struct {
	store  Store
	prefix string
}

func (p prefixed) Put(ctx context.Context, key string, data []byte) error {
	if err := checkKey(key); err != nil {
		return err
	}
	return p.store.Put(ctx, p.prefix+key, data)
}

func (p prefixed) Get(ctx context.Context, key string) ([]byte, error) {
	if err := checkKey(key); err != nil {
		return nil, err
	}
	return p.store.Get(ctx, p.prefix+key)
}

func (p prefixed) Delete(ctx context.Context, key string) error {
	if err := checkKey(key); err != nil {
		return err
	}
	return p.store.Delete(ctx, p.prefix+key)
}
