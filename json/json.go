// Copyright 2013 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// JSON benchmark marshals and unmarshals ~2MB json string
// with a tree-like object hierarchy, in 4*GOMAXPROCS goroutines.

package json

import (
	"bytes"
	"compress/bzip2"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/ioutil"

	"golang.org/x/benchmarks/driver"
)

func init() {
	driver.Register("json", benchmark)
}

func benchmark() driver.Result {
	return driver.Benchmark(benchmarkN)
}

func benchmarkN(N uint64) {
	driver.Parallel(N, 4, func() {
		var r Response
		if err := json.Unmarshal(jsonbytes, &r); err != nil {
			panic(err)
		}
		if _, err := json.Marshal(&jsondata); err != nil {
			panic(err)
		}
	})
}

var (
	jsonbytes = makeBytes()
	jsondata  = makeData()
)

func makeBytes() []byte {
	var r io.Reader
	r = bytes.NewReader(bytes.Replace(jsonbz2_base64, []byte{'\n'}, nil, -1))
	r = base64.NewDecoder(base64.StdEncoding, r)
	r = bzip2.NewReader(r)
	b, err := ioutil.ReadAll(r)
	if err != nil {
		panic(err)
	}
	return b
}

func makeData() Response {
	var v Response
	if err := json.Unmarshal(jsonbytes, &v); err != nil {
		panic(err)
	}
	return v
}

type Response struct {
	Tree     *Node  `json:"tree"`
	Username string `json:"username"`
}

type Node struct {
	Name     string  `json:"name"`
	Kids     []*Node `json:"kids"`
	CLWeight float64 `json:"cl_weight"`
	Touches  int     `json:"touches"`
	MinT     int64   `json:"min_t"`
	MaxT     int64   `json:"max_t"`
	MeanT    int64   `json:"mean_t"`
}
