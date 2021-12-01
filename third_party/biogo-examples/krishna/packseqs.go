// Copyright ©2014 The bíogo Authors. All rights reserved.
// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package krishna

import (
	"os"
	"path/filepath"

	"github.com/biogo/biogo/align/pals"
	"github.com/biogo/biogo/alphabet"
	"github.com/biogo/biogo/io/seqio/fasta"
	"github.com/biogo/biogo/seq"
	"github.com/biogo/biogo/seq/linear"
)

func packSequence(fileName string) (*pals.Packed, error) {
	_, name := filepath.Split(fileName)
	packer := pals.NewPacker(name)

	file, err := os.Open(fileName)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	template := &linear.Seq{Annotation: seq.Annotation{Alpha: alphabet.DNA}}
	seqFile := fasta.NewReader(file, template)

	var seq seq.Sequence
	for {
		seq, err = seqFile.Read()
		if err != nil {
			break
		}
		_, err = packer.Pack(seq.(*linear.Seq))
		if err != nil {
			return nil, err
		}
	}
	return packer.FinalisePack(), nil
}
