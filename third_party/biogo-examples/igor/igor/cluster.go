// Copyright ©2014 The bíogo Authors. All rights reserved.
// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package igor

import (
	"sort"
	"sync"

	"golang.org/x/benchmarks/third_party/biogo-examples/igor/turner"

	"github.com/biogo/biogo/align/pals"
	"github.com/biogo/store/interval"
	"github.com/biogo/store/step"
)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func within(alpha float64, short, long int) bool {
	return float64(short) >= float64(long)*(1-alpha)
}

func overlap(a, b interval.IntRange) int {
	return max(0, max(a.End-b.Start, b.End-a.Start))
}

type pileInterval struct {
	p  *pals.Pile
	id uintptr
}

func (pi pileInterval) ID() uintptr { return pi.id }
func (pi pileInterval) Overlap(b interval.IntRange) bool {
	return pi.p.Start() < b.End && pi.p.End() > b.Start
}
func (pi pileInterval) Range() interval.IntRange {
	return interval.IntRange{Start: pi.p.Start(), End: pi.p.End()}
}

// ClusterConfig specifies Cluster behaviour.
type ClusterConfig struct {
	// BandWidth specifies the maximum fractional distance between
	// endpoints of images being clustered into sub-piles. See
	// turner.Cluster for details.
	BandWidth float64

	// RequiredCover specifies the target coverage fraction for
	// each input pile. If RequiredCover is greater than 1, all
	// all sub-piles are retained depending on the values of
	// KeepOverlaps and OverlapThresh.
	RequiredCover float64

	// OverlapStrictness specifies the clustering behaviour.
	// If OverlapStrictness is zero, all clusters are passed
	// returned. If set to one, clusters containing clusters
	// with greater depth are rejected. If set to two, contained
	// features overlapping by more than OverlapThresh fraction
	// of the smaller pile are are discarded.
	OverlapStrictness byte
	OverlapThresh     float64

	// Procs specifies the number of independent clustering
	// instances to run in parallel. If zero, only single threaded
	// operation is performed.
	Procs int
}

// Cluster performs sub-pile clustering according to the config provided.
// The number of sub-piles and a collection of piles broken into sub-piles is
// returned.
func Cluster(piles []*pals.Pile, cfg ClusterConfig) (int, [][]*pals.Pile) {
	procs := cfg.Procs
	if procs < 1 {
		procs = 1
	}
	type workItem struct {
		i int
		p *pals.Pile
	}
	var wg sync.WaitGroup
	clust := make([][]*pals.Pile, len(piles))
	work := make([]chan workItem, 0, procs)
	ready := make(chan int, procs)
	// skipLock protect writes/reads to p.Loc which is abused as a flag to
	// allow Group to know which piles to ignore in the grouping phase.
	var skipLock sync.Mutex
	for i := 0; i < procs; i++ {
		wg.Add(1)
		work = append(work, make(chan workItem))
		go func(id int) {
			for {
				ready <- id
				w := <-work[id]
				i, p := w.i, w.p
				if p == nil {
					wg.Done()
					return
				}

				skipLock.Lock()
				loc := p.Loc
				skipLock.Unlock()
				if loc == nil {
					return
				}

				tc := turner.Cluster(p, cfg.BandWidth)

				sv, err := step.New(p.Start(), p.End(), step.Int(0))
				if err != nil {
					panic(err)
				}
				sort.Sort(turner.ByDepth(tc))
				var (
					t        interval.IntTree
					accepted int
				)
				for j, c := range tc {
					if cfg.OverlapStrictness > 0 {
						pi := pileInterval{c, uintptr(j)}
						for _, iv := range t.Get(pi) {
							r := iv.Range()
							pir := pi.Range()
							discard := func() {
								c = nil
								pile := iv.(pileInterval).p
								skipLock.Lock()
								pile.Loc = nil
								for _, im := range pile.Images {
									im.Location().(*pals.Pile).Loc = nil
								}
								skipLock.Unlock()
							}
							switch cfg.OverlapStrictness {
							case 1:
								if (pir.Start <= r.Start && pir.End > r.End) || (pir.Start < r.Start && pir.End >= r.End) {
									discard()
								}
							case 2:
								if within(cfg.OverlapThresh, overlap(r, pir), min(pi.p.Len(), r.End-r.Start)) {
									discard()
								}
							default:
								panic("illegal strictness value")
							}
						}
						if c == nil {
							tc[j] = nil
							continue
						}
						t.Insert(pi, false)
					}

					accepted++
					sv.ApplyRange(c.Start(), c.End(), func(e step.Equaler) step.Equaler {
						return e.(step.Int) + step.Int(len(c.Images))
					})
					var cov int
					sv.Do(func(start, end int, e step.Equaler) {
						if e.(step.Int) > 1 {
							cov += end - start
						}
					})
					if !within(cfg.RequiredCover, cov, p.Len()) {
						skipLock.Lock()
						for _, dc := range tc[j+1:] {
							dc.Loc = nil
							for _, im := range dc.Images {
								im.Location().(*pals.Pile).Loc = nil
							}
						}
						skipLock.Unlock()
						tc = tc[:j+1]
						break
					}
				}
				clust[i] = tc
			}
		}(i)
	}
	for i, p := range piles {
		id := <-ready
		work[id] <- workItem{i, p}
	}
	// Send nil to all to signal completion, and wait for all
	// workers to quit gracefully.
	for i := 0; i < procs; i++ {
		work[i] <- workItem{}
	}
	wg.Wait()

	var n int
	for _, c := range clust {
		n += len(c)
	}
	return n, clust
}
