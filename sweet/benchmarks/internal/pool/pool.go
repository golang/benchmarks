// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
	This package provides a facility to run a heterogeneous pool of workers.

	Each worker is defined by an interface, and the pool execute's each worker's
	Run method repeatedly concurrently. The worker may exit early by returning
	the Done error. Each worker's Run method accepts a context.Context which is
	passed to it through the pool. If this context is cancelled, it may cancel
	workers and will always cancel the pool.

	Each worker is guaranteed to start immediately when the pool's Run method is
	called and not any sooner.
*/
package pool

import (
	"context"
	"errors"
	"sync"

	"golang.org/x/sync/errgroup"
)

// Done is used to signal to the pool that the worker has no more useful work
// to do.
var Done error = errors.New("pool worker is done")

// Worker represents a stateful task which is executed repeatedly by calling
// its Run method. Any resources associated with the Worker may be freed with
// Close.
type Worker interface {
	// Run executes a task once, returning an error on failure.
	Run(context.Context) error

	// Close releases any resources associated with the Worker.
	Close() error
}

// P implements a heterogeneous pool of Workers.
type P struct {
	workers []Worker
	gun     chan struct{}
	g       *errgroup.Group
}

// New creates a new pool of the given workers.
//
// The provided context will be passed to all workers' run methods.
func New(ctx context.Context, workers []Worker) *P {
	g, ctx := errgroup.WithContext(ctx)
	gun := make(chan struct{})

	var ready sync.WaitGroup
	ready.Add(len(workers))

	// Spin up workers.
	for _, w := range workers {
		w := w
		g.Go(func() error {
			ready.Done()
			<-gun // wait for starting gun to close
			for {
				err := w.Run(ctx)
				if err == Done || ctx.Err() != nil {
					return nil
				} else if err != nil {
					return err
				}
			}
		})
	}

	// Wait for all workers to be ready.
	ready.Wait()

	return &P{
		workers: workers,
		gun:     gun,
		g:       g,
	}
}

// Run signals all the workers to begin and waits for all of them to complete.
//
// Each Worker's Run method is called in a loop until the worker returns an
// error or the context passed to New is cancelled. If the error is Done, then
// it does not propagate to Run and instead the worker stops looping.
//
// If the context is cancelled for any reason no error is returned. Check the
// context for any errors in that case.
//
// Always cleans up the pool's workers by calling Close before returning.
//
// Returns the first error encountered from any worker that failed and cancels
// the rest immediately.
func (p *P) Run() error {
	close(p.gun) // fire starting gun
	defer func() {
		// Clean up on exit.
		for _, w := range p.workers {
			w.Close()
		}
	}()
	return p.g.Wait()
}
