// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package generators

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gomodule/redigo/redis"

	"golang.org/x/benchmarks/sweet/common"
	"golang.org/x/benchmarks/sweet/common/fileutil"
	"golang.org/x/benchmarks/sweet/common/log"
	"golang.org/x/benchmarks/sweet/harnesses"
)

// Tile38 is a dynamic assets Generator for the tile38 benchmark.
type Tile38 struct{}

// Generate starts from static assets to generate a persistent store
// for Tile38 that will be passed to the server for benchmarking.
//
// The persistent store is created from gen-data/allCountries.txt,
// which is a TSV file listing points of interest around the globe,
// and gen-data/countries.geojson, which is a GeoJSON file that
// describe countries' borders. Both of these are used to populate
// a running Tile38 server which is downloaded and built on the
// fly, using the same version of Tile38 that will be benchmarked.
//
// The resulting persistent store is placed in the data directory
// in the output directory.
//
// This generator also copies over the static assets used to generate
// the dynamic assets.
func (_ Tile38) Generate(cfg *common.GenConfig) error {
	if cfg.AssetsDir != cfg.OutputDir {
		// Copy over the datasets which are used to generate
		// the server's persistent data.
		if err := os.MkdirAll(filepath.Join(cfg.OutputDir, "gen-data", "geonames"), 0755); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(cfg.OutputDir, "gen-data", "datahub"), 0755); err != nil {
			return err
		}
		err := copyFiles(cfg.OutputDir, cfg.AssetsDir, []string{
			"gen-data/geonames/allCountries.txt",
			"gen-data/geonames/LICENSE",
			"gen-data/datahub/countries.geojson",
			"gen-data/datahub/LICENSE",
			"gen-data/README.md",
		})
		if err != nil {
			return err
		}
	}

	// Create a temporary directory where we can put the Tile38
	// source and build it.
	tmpDir, err := ioutil.TempDir("", "tile38-gen")
	if err != nil {
		return err
	}

	// In order to generate the assets, we need a working Tile38
	// server. Use the harness code to get the source.
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, os.ModePerm); err != nil {
		return err
	}
	if err := (harnesses.Tile38{}).Get(srcDir); err != nil {
		return err
	}

	// Add the Go tool to PATH, since tile38's Makefile doesn't provide enough
	// visibility into how tile38 is built to allow us to pass this information
	// directly.
	env := cfg.GoTool.Env.Prefix("PATH", filepath.Join(filepath.Dir(cfg.GoTool.Tool))+":")

	// Build Tile38.
	cmd := exec.Command("make", "-C", srcDir)
	cmd.Env = env.Collapse()
	if err := cmd.Run(); err != nil {
		return err
	}

	// Launch the server.
	//
	// Generate the datastore in the tmp directory and copy it
	// over later, otherwise if cfg.OutputDir == cfg.AssetsDir, then
	// we might launch the server with an old database.
	serverPath := filepath.Join(srcDir, "tile38-server")
	tmpDataPath := filepath.Join(srcDir, "tile38-data")
	var buf bytes.Buffer
	srvCmd, err := launchServer(serverPath, tmpDataPath, &buf)
	if err != nil {
		log.Printf("=== Server stdout+stderr ===")
		for _, line := range strings.Split(buf.String(), "\n") {
			log.Printf(line)
		}
		return fmt.Errorf("error: starting server: %w", err)
	}

	// Clean up the server process after we're done.
	defer func() {
		if r := srvCmd.Process.Signal(os.Interrupt); r != nil {
			if err == nil {
				err = r
			} else {
				fmt.Fprintf(os.Stderr, "failed to shut down server: %v\n", r)
			}
			return
		}
		if _, r := srvCmd.Process.Wait(); r != nil {
			if err == nil {
				err = r
			} else if r != nil {
				fmt.Fprintf(os.Stderr, "failed to wait for server to exit: %v\n", r)
			}
			return
		}
		if err != nil && buf.Len() != 0 {
			log.Printf("=== Server stdout+stderr ===")
			for _, line := range strings.Split(buf.String(), "\n") {
				log.Printf(line)
			}
		}
		if err == nil {
			// Copy database to the output directory.
			// We cannot do this until we've stopped the
			// server because the data might not have been
			// written back yet. An interrupt should have
			// the server shut down gracefully.
			err = fileutil.CopyDir(
				filepath.Join(cfg.OutputDir, "data"),
				tmpDataPath,
				nil,
			)
		}
	}()

	// Connect to the server and feed it data.
	c, err := redis.Dial("tcp", ":9851")
	if err != nil {
		return err
	}
	defer c.Close()

	// Store GeoJSON of countries.
	genDataDir := filepath.Join(cfg.AssetsDir, "gen-data")
	if err := storeGeoJSON(c, filepath.Join(genDataDir, "datahub", "countries.geojson")); err != nil {
		return err
	}

	// Feed the server points-of-interest.
	f, err := os.Open(filepath.Join(genDataDir, "geonames", "allCountries.txt"))
	if err != nil {
		return err
	}
	defer f.Close()

	// allCountries.txt is a TSV file with a fixed number ofcolumns per row
	// (line). What we need to pull out of it is a unique ID, and the
	// coordinates for the point-of-interest.
	const (
		columnsPerLine = 19
		idColumn       = 0
		latColumn      = 4
		lonColumn      = 5
	)
	s := tsvScanner(f)

	var item int
	var obj geoObj
	for s.Scan() {
		// Each iteration of this loop is another cell in the
		// TSV file.
		switch item % columnsPerLine {
		case idColumn:
			i, err := strconv.ParseInt(s.Text(), 10, 64)
			if err != nil {
				return err
			}
			obj.id = i
		case latColumn:
			f, err := strconv.ParseFloat(s.Text(), 64)
			if err != nil {
				return err
			}
			obj.lat = f
		case lonColumn:
			f, err := strconv.ParseFloat(s.Text(), 64)
			if err != nil {
				return err
			}
			obj.lon = f
		}
		item++

		// We finished off another row, which means obj
		// should be correctly populated.
		if item%columnsPerLine == 0 {
			if err := storeGeoObj(c, &obj); err != nil {
				return err
			}
		}
	}
	return s.Err()
}

func launchServer(serverBin, dataPath string, out io.Writer) (*exec.Cmd, error) {
	// Start up the server.
	srvCmd := exec.Command(serverBin,
		"-d", dataPath,
		"-h", "127.0.0.1",
		"-p", "9851",
	)
	srvCmd.Stdout = out
	srvCmd.Stderr = out
	if err := srvCmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start server: %w", err)
	}

	// Poll until the server is ready to serve, up to 120 seconds.
	var err error
	start := time.Now()
	for time.Now().Sub(start) < 120*time.Second {
		var c redis.Conn
		c, err = redis.Dial("tcp", ":9851")
		if err == nil {
			c.Close()
			return srvCmd, nil
		}
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("timeout trying to connect to server: %w", err)
}

// tsvScanner returns a bufio.Scanner that emits a cell in
// a TSV stream for each call to Scan.
func tsvScanner(f io.Reader) *bufio.Scanner {
	s := bufio.NewScanner(f)
	s.Split(func(data []byte, atEOF bool) (advance int, token []byte, err error) {
		// Skip leading tab or newline (1).
		start := 0

		// Scan until tab or newline, marking end of value.
		for width, i := 0, start; i < len(data); i += width {
			var r rune
			r, width = utf8.DecodeRune(data[i:])
			if r == '\t' || r == '\n' {
				return i + width, data[start:i], nil
			}
		}
		// If we're at EOF, we have a final, non-empty, non-terminated word. Return it.
		if atEOF && len(data) > start {
			return len(data), data[start:], nil
		}
		// Request more data.
		return start, nil, nil
	})
	return s
}

// geoObj represents a single point on a globe with a unique ID
// indicating it as a point-of-interest.
type geoObj struct {
	id       int64
	lat, lon float64
}

// storeGeoObj writes a new point to a Tile38 database.
func storeGeoObj(c redis.Conn, g *geoObj) error {
	_, err := c.Do("SET", "key:bench", "id:"+strconv.FormatInt(g.id, 10), "POINT",
		strconv.FormatFloat(g.lat, 'f', 5, 64),
		strconv.FormatFloat(g.lon, 'f', 5, 64),
	)
	return err
}

// storeGeoJSON writes an entire GeoJSON object (which may contain many polygons)
// to a Tile38 database.
func storeGeoJSON(c redis.Conn, jsonFile string) error {
	b, err := ioutil.ReadFile(jsonFile)
	if err != nil {
		return err
	}
	_, err = c.Do("SET", "key:bench", "id:countries", "OBJECT", string(b))
	return err
}
