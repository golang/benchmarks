// Copyright 2014 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// HTTP is a benchmark that examines client/server http performance.
package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"time"

	"golang.org/x/benchmarks/driver"
)

func main() {
	driver.Main(benchmark)
}

func benchmark() driver.Result {
	return driver.Benchmark(benchmarkHTTPImpl)
}

const procs = 4

func benchmarkHTTPImpl(N uint64) {
	driver.Parallel(N, procs, func() {
		t0 := time.Now()
		makeOneRequest()
		driver.LatencyNote(t0)
	})
}

func makeOneRequest() bool {
	res, err := client.Get(server.Addr)
	if err != nil {
		// Under heavy load with GOMAXPROCS>>1, it frequently fails
		// with transient failures like:
		// "dial tcp: cannot assign requested address"
		// or:
		// "ConnectEx tcp: Only one usage of each socket address
		// (protocol/network address/port) is normally permitted".
		// So we just log and continue,
		// otherwise significant fraction of benchmarks will fail.
		log.Printf("Get: %v", err)
		return false
	}
	defer res.Body.Close()
	b, err := ioutil.ReadAll(res.Body)
	if err != nil {
		log.Fatalf("ReadAll: %v", err)
	}
	if s := string(b); s != "Hello world.\n" {
		log.Fatalf("Got body: " + s)
	}
	return true
}

var (
	server *http.Server
	client *http.Client
)

func init() {
	// These environment variables affect net/http behavior,
	// ensure that we get predictable results regardless of environment on the machine.
	os.Setenv("HTTP_PROXY", "")
	os.Setenv("http_proxy", "")
	os.Setenv("NO_PROXY", "")
	os.Setenv("no_proxy", "")

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if l, err = net.Listen("tcp6", "[::1]:0"); err != nil {
			log.Fatalf("failed to listen: %v", err)
		}
	}
	server = &http.Server{
		Addr:           "http://" + l.Addr().String(),
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   10 * time.Second,
		MaxHeaderBytes: 1 << 20,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprintf(w, "Hello world.\n")
		}),
	}
	go server.Serve(l)

	client = &http.Client{
		Transport: &http.Transport{
			// just what default client uses
			Proxy: http.ProxyFromEnvironment,
			// this leads to more stable numbers
			MaxIdleConnsPerHost: procs * runtime.GOMAXPROCS(0),
		},
	}

	if !makeOneRequest() {
		log.Fatalf("server is not listening")
	}
}
