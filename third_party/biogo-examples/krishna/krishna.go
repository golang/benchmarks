// Copyright ©2014 The bíogo Authors. All rights reserved.
// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package krishna

import (
	"fmt"
	"sync"

	"github.com/biogo/biogo/align/pals"
	"github.com/biogo/biogo/align/pals/filter"
	"github.com/biogo/biogo/morass"
)

type Params struct {
	TmpChunkSize int
	MinHitLen    int
	MinHitId     float64
	TubeOffset   int
	AlignConc    bool
	TmpConc      bool
}

// Krishna is a pure Go implementation of Edgar and Myers PALS tool.
// This version of krishna is modified from its original form and only
// computes alignment for a sequence against itself.
type Krishna struct {
	params Params
	target *pals.Packed
	pa     [2]*pals.PALS
}

func New(seqPath, tmpDir string, params Params) (*Krishna, error) {
	target, err := packSequence(seqPath)
	if err != nil {
		return nil, err
	}
	if target.Len() == 0 {
		return nil, fmt.Errorf("target sequence is zero length")
	}
	// Allocate morass before since these can be somewhat large
	// and single large allocations tend to be noisy.
	m1, err := morass.New(filter.Hit{}, "krishna_", tmpDir, params.TmpChunkSize, params.TmpConc)
	if err != nil {
		return nil, err
	}
	m2, err := morass.New(filter.Hit{}, "krishna_", tmpDir, params.TmpChunkSize, params.TmpConc)
	if err != nil {
		return nil, err
	}
	pa := [2]*pals.PALS{
		pals.New(target.Seq, target.Seq, true, m1, params.TubeOffset, nil, nil),
		pals.New(target.Seq, target.Seq, true, m2, params.TubeOffset, nil, nil),
	}
	return &Krishna{params, target, pa}, nil
}

//
// Returns a cleanup function, and an error. The cleanup function should be
// called before program exit, if not nil.
func (k *Krishna) Run(writer *pals.Writer) error {
	if err := k.pa[0].Optimise(k.params.MinHitLen, k.params.MinHitId); err != nil {
		return err
	}
	if err := k.pa[0].BuildIndex(); err != nil {
		return err
	}
	k.pa[1].Share(k.pa[0])

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, comp := range [...]bool{false, true} {
		wg.Add(1)
		go func(i int, p *pals.PALS, comp bool) {
			defer wg.Done()
			hits, err := p.Align(comp)
			if err != nil {
				errs[i] = err
				return
			}
			_, err = writeDPHits(writer, k.target, k.target, hits, comp)
			if err != nil {
				errs[i] = err
				return
			}
		}(i, k.pa[i], comp)
		if !k.params.AlignConc {
			// Block until it's done if we don't want to run
			// the alignment processing concurrently.
			wg.Wait()
		}
	}
	wg.Wait()

	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func (k *Krishna) CleanUp() {
	for _, p := range k.pa {
		p.CleanUp()
	}
}
