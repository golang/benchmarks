// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build linux
// +build linux

package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/benchmarks/sweet/benchmarks/internal/cgroups"
	"golang.org/x/benchmarks/sweet/benchmarks/internal/driver"
	"golang.org/x/benchmarks/sweet/benchmarks/internal/pool"
)

const (
	ip   = "127.0.0.1"
	port = "8081"
	host = "http://" + ip + ":" + port
)

func httpGet(ctx context.Context, url string) (*http.Response, error) {
	r, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(r)
}

type httpServer struct {
	duration time.Duration
}

func (h httpServer) name() string {
	return "GVisorHTTP"
}

type worker struct {
	lat []time.Duration
}

func newWorker() *worker {
	return &worker{
		lat: make([]time.Duration, 0, 100000),
	}
}

func (w *worker) Run(_ context.Context) error {
	start := time.Now()
	resp, err := http.Get(host)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		resp, err := http.Get(host + "/" + scanner.Text())
		if err != nil {
			return err
		}
		resp.Body.Close()
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	dur := time.Now().Sub(start)
	w.lat = append(w.lat, dur)
	return nil
}

func (w *worker) Close() error {
	return nil
}

func (b httpServer) run(cfg *config, out io.Writer) (err error) {
	procs := runtime.GOMAXPROCS(-1)
	clientProcs := procs / 4
	if clientProcs == 0 {
		clientProcs = 1
	}
	serverProcs := procs - clientProcs
	if serverProcs == 0 {
		serverProcs = 1
	}
	runtime.GOMAXPROCS(clientProcs)
	defer runtime.GOMAXPROCS(procs)
	clients := clientProcs

	baseSrvCmd := cfg.runscCmd(
		"-rootless", "do", "-ip", ip,
		workloadsPath(cfg.assetsDir, "http"),
		"-host", ip,
		"-port", port,
		"-assets", filepath.Join(cfg.assetsDir, "http", "assets"),
		"-procs", strconv.Itoa(serverProcs),
	)
	baseSrvCmd.Stdout = out
	baseSrvCmd.Stderr = out
	srvCmd, err := cgroups.WrapCommand(baseSrvCmd, "test-http-server.scope")
	if err != nil {
		return err
	}
	ctx := context.Background()
	defer func() {
		if r := srvCmd.Process.Signal(os.Interrupt); r != nil {
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to force shut down server: %v\n", r)
			} else {
				err = r
			}
		}
		if r := srvCmd.Wait(); r != nil {
			ee, ok := r.(*exec.ExitError)
			if ok {
				status := ee.ProcessState.Sys().(syscall.WaitStatus)
				if status.Signaled() && status.Signal() == os.Interrupt {
					return
				}
			}
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: failed to wait for server: %v\n", r)
				return
			}
			err = r
			return
		}
	}()

	err = driver.RunBenchmark(b.name()+"Startup", func(d *driver.B) error {
		if err := srvCmd.Start(); err != nil {
			return err
		}
		// Poll until the server is ready to serve, up to a maximum in case of a bug.
		const timeout = 30 * time.Second
		start := time.Now()
		for time.Now().Sub(start) < timeout {
			resp, err := httpGet(ctx, host)
			if err == nil {
				resp.Body.Close()
				break
			}
		}
		if time.Now().Sub(start) >= timeout {
			return fmt.Errorf("server startup timed out")
		}
		return nil
	}, driver.DoTime(true))

	workers := make([]pool.Worker, 0, clients)
	for i := 0; i < clients; i++ {
		workers = append(workers, newWorker())
	}

	// Run the benchmark for b.duration.
	ctx, cancel := context.WithTimeout(ctx, b.duration)
	defer cancel()
	p := pool.New(ctx, workers)
	return driver.RunBenchmark(b.name(), func(d *driver.B) error {
		if err := p.Run(); err != nil {
			return err
		}
		d.StopTimer()

		// Test is done, bring all latency measurements together.
		latencies := make([]time.Duration, 0, len(workers)*100000)
		for _, w := range workers {
			latencies = append(latencies, w.(*worker).lat...)
		}
		sort.Slice(latencies, func(i, j int) bool {
			return latencies[i] < latencies[j]
		})

		// Sort and report percentiles.
		p50 := latencies[len(latencies)*50/100]
		p90 := latencies[len(latencies)*90/100]
		p99 := latencies[len(latencies)*99/100]
		d.Report("p50-latency-ns", uint64(p50))
		d.Report("p90-latency-ns", uint64(p90))
		d.Report("p99-latency-ns", uint64(p99))

		// Report throughput.
		lengthS := float64(b.duration) / float64(time.Second)
		reqsPerSec := float64(len(latencies)) / lengthS
		d.Report("ops/s", uint64(reqsPerSec))

		// Report the average request latency.
		d.Ops(len(latencies))
		d.Report(driver.StatTime, uint64((int(b.duration)*clients)/len(latencies)))
		return nil
	}, driver.DoTime(true), driver.DoAvgRSS(srvCmd.RSSFunc()))
}
