// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pool

import (
	"context"
	"io"
	"testing"
	"time"
)

func TestEmptyPool(t *testing.T) {
	p := New(context.Background(), []Worker{})
	if err := p.Run(); err != nil {
		t.Fatalf("got error from empty pool: %v", err)
	}
}

type countCloser struct {
	c int
}

func (c *countCloser) Close() error {
	c.c++
	return nil
}

type boolWorker struct {
	countCloser
	exec bool
}

func (b *boolWorker) Run(_ context.Context) error {
	b.exec = true
	return Done
}

func TestPool(t *testing.T) {
	workers := []Worker{
		&boolWorker{},
		&boolWorker{},
		&boolWorker{},
		&boolWorker{},
	}
	p := New(context.Background(), workers)
	if err := p.Run(); err != nil {
		t.Fatalf("got error from good pool: %v", err)
	}
	for _, w := range workers {
		if !w.(*boolWorker).exec {
			t.Fatal("found worker that was never executed")
		}
	}
}

type badWorker struct {
	countCloser
}

func (b *badWorker) Run(_ context.Context) error {
	return io.EOF
}

func TestPoolError(t *testing.T) {
	workers := []Worker{
		&boolWorker{},
		&boolWorker{},
		&boolWorker{},
		&badWorker{},
	}
	p := New(context.Background(), workers)
	if err := p.Run(); err == nil {
		t.Fatal("expected error from bad worker")
	} else if err != io.EOF {
		t.Fatalf("unexpected error from pool: %v", err)
	}
}

type foreverWorker struct {
	countCloser
}

func (f *foreverWorker) Run(ctx context.Context) error {
	return nil
}

func TestPoolCancel(t *testing.T) {
	workers := []Worker{
		&foreverWorker{},
		&foreverWorker{},
		&foreverWorker{},
		&foreverWorker{},
	}
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	p := New(ctx, workers)
	sig := make(chan struct{})
	go func() {
		if err := p.Run(); err != nil {
			t.Fatalf("got error from good pool: %v", err)
		}
		sig <- struct{}{}
	}()
	cancel()
	select {
	case <-sig:
	case <-time.After(3 * time.Second):
		t.Fatal("test timed out after 3 seconds")
	}
}
