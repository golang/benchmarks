// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package driver

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/google/pprof/profile"
	"golang.org/x/benchmarks/sweet/common/diagnostics"
	sprofile "golang.org/x/benchmarks/sweet/common/profile"
)

// There are three ways of gathering diagnostic profiles, in order of
// preference:
//
// - In-process: The process coordinating the benchmark and running the
// benchmarked code are the same, and driver.B takes care of diagnostic
// collection.
//
// - Subprocess self collection: The benchmarked code is running in a subprocess
// that has the ability to collect diagnostics using a command line flag. For
// this, we use [Diagnostic.NewFile] and pass the name of the file to the
// subprocess.
//
// - Subprocess HTTP collection: The benchmarked code is an HTTP server with
// net/http/pprof endpoints. We use [Diagnostic] in conjunction with
// [server.FetchDiagnostic].
//
// TODO: Can we better consolidate the last case into Diagnostics?

type Diagnostics struct {
	name string

	once      sync.Once
	tmpDir    string
	tmpDirErr error
}

func NewDiagnostics(name string) *Diagnostics {
	return &Diagnostics{name: name}
}

func safeFileName(name string) string {
	// The following characters are disallowed by either VFAT, NTFS, APFS, or
	// most Unix file systems:
	//
	// 0x00–0x1F 0x7F " * / : < > ? \ |
	//
	// We use % for escaping, so we also escape it.

	const bad = (1<<0x20 - 1) | 1<<'"' | 1<<'*' | 1<<'/' | 1<<':' | 1<<'<' | 1<<'>' | 1<<'?' | 1<<'\\' | 1<<'|' | 1<<'%'
	const badLo uint64 = bad & 0xFFFFFFFFFFFFFFFF
	const badHi uint64 = bad >> 64

	var buf strings.Builder
	for i := 0; i < len(name); i++ {
		ch := name[i]
		if ch >= 0x7F || (badLo>>ch)&1 != 0 || (ch >= 64 && (badHi>>(ch-64))&1 != 0) {
			fmt.Fprintf(&buf, "%%%02x", ch)
		} else {
			buf.WriteByte(ch)
		}
	}
	return buf.String()
}

// Commit combines all individually committed diagnostic files into the final
// output files. If there are multiple diagnostic files with the same type and
// name, it merges them into a single file. If b != nil, it adds metrics for
// diagnostic file sizes to b.
func (d *Diagnostics) Commit(b *B) error {
	// Commit is usually used in a defer, so log the error.
	err := d.commit1(b)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	return err
}

func (d *Diagnostics) commit1(b *B) error {
	if d.tmpDir == "" {
		// No diagnostics were created.
		return nil
	}

	allEntries, err := os.ReadDir(d.tmpDir)
	if err != nil {
		return err
	}

	// Bucket the file names.
	type mergeKey struct {
		typ  diagnostics.Type
		name string
	}
	toMerge := make(map[mergeKey][]string)
	var toDelete []string
	for _, entry := range allEntries {
		fileName := entry.Name()
		path := filepath.Join(d.tmpDir, fileName)

		typ, name, committed := parseDiagnosticPath(fileName)

		if !committed {
			// Uncommitted. Delete this one.
			toDelete = append(toDelete, path)
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return err
		} else if info.Size() == 0 {
			// Skip zero-sized files, otherwise the pprof package
			// will call it a parsing error.
			toDelete = append(toDelete, path)
			continue
		}

		// Add to the merge list.
		k := mergeKey{typ, name}
		toMerge[k] = append(toMerge[k], path)
	}

	// Process each merge list.
	var errs []error
	anyTrace := false
	var traceBytes int64
	for k, paths := range toMerge {
		if err, outPath, deleteInputs := d.merge(k.typ, k.name, paths); err != nil {
			errs = append(errs, err)
		} else {
			if deleteInputs {
				toDelete = append(toDelete, paths...)
			}
			if k.typ == diagnostics.Trace {
				anyTrace = true
				if st, err := os.Stat(outPath); err == nil {
					traceBytes = st.Size()
				}
			}
		}
	}
	if b != nil && anyTrace {
		// Report metric for diagnostic size.
		b.Report("trace-bytes", uint64(traceBytes))
	}

	// Delete all of the temporary files.
	for _, path := range toDelete {
		errs = append(errs, os.Remove(path))
	}
	errs = append(errs, os.Remove(d.tmpDir))

	return errors.Join(errs...)
}

func (d *Diagnostics) merge(typ diagnostics.Type, subName string, paths []string) (err error, outPath string, deleteInputs bool) {
	if len(paths) > 1 && !typ.CanMerge() {
		return fmt.Errorf("found %d > 1 %s files, but this diagnostic cannot be merged", len(paths), typ), "", false
	}

	// Create the output file.
	name := d.name
	if subName != "" {
		name += "-" + subName
	}
	outFile, err := os.CreateTemp(diag.ResultsDir, safeFileName(name)+"-*-"+typ.FileName())
	if err != nil {
		return err, "", false
	}
	outPath = outFile.Name()

	if len(paths) == 1 {
		// Simply rename it to the final path.
		outFile.Close()
		if err := os.Rename(paths[0], outPath); err != nil {
			return err, "", false
		}
	} else if len(paths) > 1 {
		defer outFile.Close()

		// Otherwise, merge the profiles.
		var profiles []*profile.Profile
		for _, path := range paths {
			p, err := sprofile.ReadPprof(path)
			if err != nil {
				return err, "", false
			}
			profiles = append(profiles, p)
		}

		p, err := profile.Merge(profiles)
		if err != nil {
			return fmt.Errorf("error merging profiles: %w", err), "", false
		}

		err = p.Write(outFile)
		if err == nil {
			err = outFile.Close()
		}
		if err != nil {
			return fmt.Errorf("error writing profile %s: %s", outPath, err), "", false
		}

		// Now we can delete all of the input files.
		deleteInputs = true
	}

	return nil, outPath, deleteInputs
}

type DiagnosticFile struct {
	*os.File
}

// getTmpDir returns the directory for storing uncommitted diagnostics files.
func (d *Diagnostics) getTmpDir() (string, error) {
	d.once.Do(func() {
		// Create the uncommitted results directory.
		d.tmpDir, d.tmpDirErr = os.MkdirTemp(diag.ResultsDir, safeFileName(d.name)+"-*.tmp")
	})
	return d.tmpDir, d.tmpDirErr
}

// Create is shorthand for CreateNamed(typ, "").
func (d *Diagnostics) Create(typ diagnostics.Type) (*DiagnosticFile, error) {
	return d.CreateNamed(typ, "")
}

// CreateNamed returns a new file that a diagnostic can be written to. If this
// type of diagnostic can be merged, this can be called multiple times with the
// same type and name and Commit will merge all of the files. The caller must
// close this file. Diagnostic files are temporary until the caller calls
// [DiagnosticFile.Commit] to indicate they are ready for merging into the final
// output.
func (d *Diagnostics) CreateNamed(typ diagnostics.Type, name string) (*DiagnosticFile, error) {
	if !DiagnosticEnabled(typ) {
		return nil, nil
	}

	tmpDir, err := d.getTmpDir()
	if err != nil {
		return nil, err
	}

	// Construct diagnostic file name. This path must be parsable by
	// parseDiagnosticPath.
	if strings.Contains(string(typ), "-") {
		// To later parse the file name, we assume there's no "-".
		panic("diagnostic type contains '-'")
	}
	pattern := string(typ) + "-*"
	if name != "" {
		pattern += "-" + safeFileName(name)
	}
	// Mark this as uncommitted.
	pattern += ".tmp"

	// Create file.
	f, err := os.CreateTemp(tmpDir, pattern)
	if err != nil {
		return nil, err
	}

	return &DiagnosticFile{f}, nil
}

func parseDiagnosticPath(fileName string) (typ diagnostics.Type, name string, committed bool) {
	// Check whether its committed.
	committed = !strings.HasSuffix(fileName, ".tmp")
	fileName = strings.TrimSuffix(fileName, ".tmp")

	// Get the type.
	typString, rest, _ := strings.Cut(fileName, "-")
	typ = diagnostics.Type(typString)

	// Drop the CreateTemp junk, leaving only the name. If there's no "-", then
	// there's no name, so we let this set name to "".
	_, name, _ = strings.Cut(rest, "-")

	return
}

// Commit indicates that diagnostic file f is ready to be merged into the final
// output. For a diagnostic that cannot be truncated, this should only be called
// when the file has been fully written.
func (f *DiagnosticFile) Commit() {
	path := f.Name()
	if !strings.HasSuffix(path, ".tmp") {
		panic("temporary diagnostic file does not end in .tmp: " + path)
	}
	newPath := strings.TrimSuffix(path, ".tmp")
	if err := os.Rename(path, newPath); err != nil {
		// If rename fails, something is *horribly* wrong.
		panic(fmt.Sprintf("failed to rename %q to %q: %s", path, newPath, err))
	}
}
