// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"golang.org/x/benchmarks/sweet/benchmarks/internal/driver"
	"golang.org/x/benchmarks/sweet/common/diagnostics"
)

// FetchDiagnostic reads a profile or trace from the pprof endpoint at host. The
// returned stop function finalizes the diagnostic file on disk and returns the
// total size in bytes. Because of limitations of net/http/pprof, this cannot
// actually stop collection on the server side, so stop should only be called
// when the server is about to be shut down.
func FetchDiagnostic(host string, diag *driver.Diagnostics, typ diagnostics.Type, name string) (stop func()) {
	if typ.HTTPEndpoint() == "" {
		panic("diagnostic " + string(typ) + " has no endpoint")
	}

	if !driver.DiagnosticEnabled(typ) {
		return func() {}
	}

	// If this is a snapshot-type diagnostic, wait until the end to collect it.
	if typ.IsSnapshot() {
		return func() {
			err := collectTo(context.Background(), host, diag, typ, name)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to read diagnostic %s: %v", typ, err)
			}
		}
	}

	// Otherwise, start collecting it now. If it can be truncated, then we try
	// to collect it in one long run and cut if off when stop is called.
	// If it can be merged, we can collect several of them.
	var wg sync.WaitGroup
	wg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer wg.Done()

		for {
			err := collectTo(ctx, host, diag, typ, name)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					fmt.Fprintf(os.Stderr, "failed to read diagnostic %s: %v", typ, err)
				}
				break
			}
			if !typ.CanMerge() {
				break
			}
		}
	}()
	return func() {
		// Stop the loop.
		cancel()
		wg.Wait()
	}
}

func collectTo(ctx context.Context, host string, diag *driver.Diagnostics, typ diagnostics.Type, name string) error {
	// Construct the endpoint URL
	var endpoint string
	endpoint = fmt.Sprintf("http://%s/%s", host, typ.HTTPEndpoint())
	if typ.CanMerge() && !typ.CanTruncate() {
		// Collect in lots of small increments because we won't be able to just
		// stop it.
		endpoint += "?seconds=1"
	} else if typ.CanTruncate() {
		// Collect a long run that we can cut off.
		endpoint += "?seconds=999999"
	}

	// Start profile collection.
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Read into a diagnostic file
	f, err := diag.CreateNamed(typ, name)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	if err == nil || typ.CanTruncate() {
		// If we got a complete file, or it's fine to truncate it anyway, commit
		// the diagnostic file.
		f.Close()
		f.Commit()
	}
	return err
}

// TODO: Delete below here

func CollectDiagnostic(host, tmpDir, benchName string, typ diagnostics.Type) (int64, error) {
	// We attempt to use the benchmark name to create a temp file so replace all
	// path separators with "_".
	benchName = strings.Replace(benchName, "/", "_", -1)
	benchName = strings.Replace(benchName, string(os.PathSeparator), "_", -1)
	f, err := os.CreateTemp(tmpDir, benchName+"."+string(typ))
	if err != nil {
		return 0, err
	}
	defer f.Close()
	resp, err := http.Get(fmt.Sprintf("http://%s/debug/pprof/%s", host, endpoint(typ)))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	n, err := io.Copy(f, resp.Body)
	if err != nil {
		return 0, err
	}
	return n, driver.CopyDiagnosticData(f.Name(), typ, benchName)
}

func endpoint(typ diagnostics.Type) string {
	switch typ {
	case diagnostics.CPUProfile:
		return "profile?seconds=1"
	case diagnostics.MemProfile:
		return "heap"
	case diagnostics.Trace:
		return "trace?seconds=1"
	}
	panic("diagnostic " + string(typ) + " has no endpoint")
}

func PollDiagnostic(host, tmpDir, benchName string, typ diagnostics.Type) (stop func() uint64) {
	// TODO(mknyszek): This is kind of a hack. We really should find a way to just
	// enable diagnostic collection at a lower level for the entire server run.
	var stopc chan struct{}
	var wg sync.WaitGroup
	var size uint64
	wg.Add(1)
	stopc = make(chan struct{})
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stopc:
				return
			default:
			}
			n, err := CollectDiagnostic(host, tmpDir, benchName, typ)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to read diagnostic %s: %v", typ, err)
				return
			}
			size += uint64(n)
		}
	}()
	return func() uint64 {
		// Stop the loop.
		close(stopc)
		wg.Wait()
		return size
	}
}
