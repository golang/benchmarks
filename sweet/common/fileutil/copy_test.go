// Copyright 2026 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package fileutil

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func TestCopyFile(t *testing.T) {
	tempDir := t.TempDir()

	srcPath := filepath.Join(tempDir, "src.txt")
	srcContent := []byte("hello world 1234567890")
	if err := os.WriteFile(srcPath, srcContent, 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("NewFile", func(t *testing.T) {
		dstPath := filepath.Join(tempDir, "dst_new.txt")
		if err := CopyFile(dstPath, srcPath, nil, nil); err != nil {
			t.Fatalf("CopyFile failed: %v", err)
		}
		got, err := os.ReadFile(dstPath)
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if !bytes.Equal(got, srcContent) {
			t.Fatalf("got %q, want %q", got, srcContent)
		}
	})

	t.Run("DstExistsSameContent", func(t *testing.T) {
		dstPath := filepath.Join(tempDir, "dst_same.txt")
		if err := os.WriteFile(dstPath, srcContent, 0644); err != nil {
			t.Fatal(err)
		}

		fi, err := os.Stat(dstPath)
		if err != nil {
			t.Fatal(err)
		}
		modTime := fi.ModTime()

		if err := CopyFile(dstPath, srcPath, nil, nil); err != nil {
			t.Fatalf("CopyFile failed: %v", err)
		}

		got, err := os.ReadFile(dstPath)
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if !bytes.Equal(got, srcContent) {
			t.Fatalf("got %q, want %q", got, srcContent)
		}

		fi2, err := os.Stat(dstPath)
		if err != nil {
			t.Fatal(err)
		}
		if !fi2.ModTime().Equal(modTime) {
			t.Errorf("file was modified/rewritten, modTime changed from %v to %v", modTime, fi2.ModTime())
		}
	})

	t.Run("DstExistsDifferentContentSameSize", func(t *testing.T) {
		dstPath := filepath.Join(tempDir, "dst_diff_same_size.txt")
		diffContent := []byte("hello world 0987654321") // same length as srcContent
		if err := os.WriteFile(dstPath, diffContent, 0644); err != nil {
			t.Fatal(err)
		}

		if err := CopyFile(dstPath, srcPath, nil, nil); err != nil {
			t.Fatalf("CopyFile failed: %v", err)
		}

		got, err := os.ReadFile(dstPath)
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if !bytes.Equal(got, srcContent) {
			t.Fatalf("got %q, want %q", got, srcContent)
		}
	})

	t.Run("DstExistsDifferentSize", func(t *testing.T) {
		dstPath := filepath.Join(tempDir, "dst_diff_size.txt")
		diffContent := []byte("short content")
		if err := os.WriteFile(dstPath, diffContent, 0644); err != nil {
			t.Fatal(err)
		}

		if err := CopyFile(dstPath, srcPath, nil, nil); err != nil {
			t.Fatalf("CopyFile failed: %v", err)
		}

		got, err := os.ReadFile(dstPath)
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if !bytes.Equal(got, srcContent) {
			t.Fatalf("got %q, want %q", got, srcContent)
		}
	})

	t.Run("SameSrcAndDst", func(t *testing.T) {
		filePath := filepath.Join(tempDir, "same.txt")
		if err := os.WriteFile(filePath, srcContent, 0644); err != nil {
			t.Fatal(err)
		}

		if err := CopyFile(filePath, filePath, nil, nil); err != nil {
			t.Fatalf("CopyFile failed: %v", err)
		}

		got, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if !bytes.Equal(got, srcContent) {
			t.Fatalf("file content corrupted; got %q, want %q", got, srcContent)
		}
	})

	t.Run("SrcFS", func(t *testing.T) {
		mapFS := fstest.MapFS{
			"virtual.txt": &fstest.MapFile{
				Data: srcContent,
				Mode: 0644,
			},
		}

		dstPath := filepath.Join(tempDir, "dst_virtual.txt")
		if err := CopyFile(dstPath, "virtual.txt", nil, mapFS); err != nil {
			t.Fatalf("CopyFile failed: %v", err)
		}

		got, err := os.ReadFile(dstPath)
		if err != nil {
			t.Fatalf("ReadFile failed: %v", err)
		}
		if !bytes.Equal(got, srcContent) {
			t.Fatalf("got %q, want %q", got, srcContent)
		}

		// Copy again when dst exists and has same content
		if err := CopyFile(dstPath, "virtual.txt", nil, mapFS); err != nil {
			t.Fatalf("CopyFile second time failed: %v", err)
		}
	})

	t.Run("LargeFiles", func(t *testing.T) {
		largeSrc := filepath.Join(tempDir, "large_src.dat")
		largeDst := filepath.Join(tempDir, "large_dst.dat")

		// Create 150KB data (> 64KB chunk buffer)
		data := make([]byte, 150*1024)
		for i := range data {
			data[i] = byte(i % 251)
		}
		if err := os.WriteFile(largeSrc, data, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(largeDst, data, 0644); err != nil {
			t.Fatal(err)
		}

		fi, err := os.Stat(largeDst)
		if err != nil {
			t.Fatal(err)
		}
		modTime := fi.ModTime()

		if err := CopyFile(largeDst, largeSrc, nil, nil); err != nil {
			t.Fatalf("CopyFile failed: %v", err)
		}

		fi2, err := os.Stat(largeDst)
		if err != nil {
			t.Fatal(err)
		}
		if !fi2.ModTime().Equal(modTime) {
			t.Errorf("large file was unnecessarily rewritten")
		}

		// Mutate largeDst at offset 100,000
		dataDiff := make([]byte, len(data))
		copy(dataDiff, data)
		dataDiff[100000] ^= 0xFF
		if err := os.WriteFile(largeDst, dataDiff, 0644); err != nil {
			t.Fatal(err)
		}

		if err := CopyFile(largeDst, largeSrc, nil, nil); err != nil {
			t.Fatalf("CopyFile failed: %v", err)
		}

		got, err := os.ReadFile(largeDst)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, data) {
			t.Fatalf("CopyFile failed to overwrite modified large file")
		}
	})
}
