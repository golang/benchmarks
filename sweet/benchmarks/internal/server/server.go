// Copyright 2023 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package server

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"golang.org/x/benchmarks/sweet/benchmarks/internal/driver"
	"golang.org/x/benchmarks/sweet/common/diagnostics"
)

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
