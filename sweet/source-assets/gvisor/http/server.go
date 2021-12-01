// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"text/template"
	"time"
)

var (
	host       string
	port       int
	procs      int
	assetsRoot string
)

func init() {
	flag.StringVar(&host, "host", "localhost", "host to serve on")
	flag.IntVar(&port, "port", 8081, "port to serve on")
	flag.IntVar(&procs, "procs", runtime.GOMAXPROCS(-1), "how many processors to use")
	flag.StringVar(&assetsRoot, "assets", "./assets", "directory to serve assets from")
}

var frontPage = template.Must(template.New("front").Parse(`
{{- range . -}}
  {{.}}
{{end -}}
`))

func main() {
	flag.Parse()
	runtime.GOMAXPROCS(procs)

	var images []string
	filepath.Walk(assetsRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			log.Printf("warning: failed to walk %s: %v", path, err)
			return nil
		}
		if info.IsDir() {
			// Ignore directories.
			return nil
		}
		relPath, _ := filepath.Rel(assetsRoot, path)
		switch filepath.Ext(path) {
		case ".jpg", ".png", ".gif":
			images = append(images, relPath)
		default:
		}
		return nil
	})
	for i, img := range images {
		images[i] = filepath.Join("/static", img)
		log.Printf("Image: %s", images[i])
	}

	ctx := context.Background()
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	// Set up a done channel.
	http.Handle("/static/", http.FileServer(http.Dir(assetsRoot)))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if err := frontPage.Execute(w, images); err != nil {
			http.Error(w, fmt.Sprintf("Internal server error: %s", err.Error()), 500)
			return
		}
	})

	server := &http.Server{Addr: fmt.Sprintf("%s:%d", host, port), Handler: http.DefaultServeMux}
	go func() {
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error listening/serving: %s", err)
		}
	}()

	// Wait for a signal to stop.
	<-ctx.Done()

	// Shut down the server cleanly.
	exitCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(exitCtx); err != nil {
		log.Printf("Clean shutdown failed: %v", err)
		return
	}
	log.Print("Shut down successfully. Goodbye!")
}
