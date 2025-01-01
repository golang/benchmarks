// Copyright ©2014 The bíogo Authors. All rights reserved.
// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package turner

import (
	"sort"

	"github.com/biogo/biogo/align/pals"
	"github.com/biogo/biogo/feat"
	"github.com/biogo/store/interval"
)

type pairings struct {
	interval interval.IntRange
	id       uintptr

	loc
	data []*pals.Feature
}

type loc struct{ feat.Feature }

func (p *pairings) Start() int { return p.interval.Start }
func (p *pairings) End() int   { return p.interval.End }
func (p *pairings) Len() int   { return p.interval.End - p.interval.Start }

func (p *pairings) ID() uintptr              { return p.id }
func (p *pairings) Range() interval.IntRange { return p.interval }
func (p *pairings) Overlap(b interval.IntRange) bool {
	return b.End >= p.Start() && b.Start <= p.End()
}

type byDepth []*pairings

func (p byDepth) Len() int { return len(p) }
func (p byDepth) Less(i, j int) bool {
	return len(p[i].data) > len(p[j].data)
}
func (p byDepth) Swap(i, j int) { p[i], p[j] = p[j], p[i] }

type simple interval.IntRange

func (i simple) Range() interval.IntRange { return interval.IntRange(i) }
func (i simple) ID() uintptr              { return 0 }
func (i simple) Overlap(b interval.IntRange) bool {
	return b.End >= i.Start && b.Start <= i.End
}

// Cluster performs a clustering of images in the pile p to create a collection
// of piles where image ends are within h times the images length of the ends of
// the generated sub-pile. Clustering is seeded by the set of unique image intervals
// in order of decreasing depth.
func Cluster(p *pals.Pile, h float64) []*pals.Pile {
	pm := make(map[interval.IntRange][]*pals.Feature)
	for _, fv := range p.Images {
		i := interval.IntRange{Start: fv.Start(), End: fv.End()}
		c := pm[i]
		c = append(c, fv)
		pm[i] = c
	}

	pl := make([]*pairings, 0, len(pm))
	for iv, data := range pm {
		pl = append(pl, &pairings{interval: iv, loc: loc{p}, data: data})
	}

	var t interval.IntTree
	for i, pe := range pl {
		pe.id = uintptr(i)
		t.Insert(pe, true)
	}
	t.AdjustRanges()

	var cl []*pals.Pile

	sort.Sort(byDepth(pl))
	for _, pe := range pl {
		if _, ok := pm[interval.IntRange{Start: pe.Start(), End: pe.End()}]; !ok {
			continue
		}
		thr := int(h * float64(pe.interval.End-pe.interval.Start))
		from := pe.End()
		to := pe.Start()
		var (
			mf []*pals.Feature
			dm []interval.IntInterface
		)
		t.DoMatching(func(iv interval.IntInterface) (done bool) {
			r := iv.Range()
			if abs(pe.Start()-r.Start) <= thr && abs(pe.End()-r.End) <= thr {
				ivp := iv.(*pairings)
				dm = append(dm, iv)
				mf = append(mf, ivp.data...)

				if r.Start < from {
					from = r.Start
				}
				if r.End > to {
					to = r.End
				}
			}
			return
		}, simple{pe.Start() - thr, pe.End() + thr})
		if len(mf) == 0 {
			continue
		}
		for _, de := range dm {
			t.Delete(de, true)
			delete(pm, de.Range())
		}
		t.AdjustRanges()
		cl = append(cl, &pals.Pile{
			From:   from,
			To:     to,
			Loc:    pe.Location(),
			Images: mf,
		})
	}

	return cl
}

func abs(a int) int {
	if a != 0 && a == -a {
		panic("weird number")
	}
	if a < 0 {
		return -a
	}
	return a
}

// Range returns the start and end positions for all the members of p.
func Range(p []*pals.Pile) (start, end int) {
	if len(p) == 0 {
		return
	}
	start, end = p[0].Start(), p[0].End()
	for _, pi := range p[1:] {
		if s := pi.Start(); s < start {
			start = s
		}
		if e := pi.End(); e > end {
			end = e
		}
	}
	return start, end
}

// Volume returns the number of bases contributing to the pile p.
func Volume(p *pals.Pile) int {
	var m int
	for _, im := range p.Images {
		m += im.Len()
	}
	return m
}

// ByVolume allows a collection of pals.Pile to be sorted by volume.
type ByVolume []*pals.Pile

func (p ByVolume) Len() int           { return len(p) }
func (p ByVolume) Less(i, j int) bool { return Volume(p[i]) > Volume(p[j]) }
func (p ByVolume) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }

// ByDepth allows a collection of pals.Pile to be sorted by the number
// images in each pile.
type ByDepth []*pals.Pile

func (p ByDepth) Len() int           { return len(p) }
func (p ByDepth) Less(i, j int) bool { return len(p[i].Images) > len(p[j].Images) }
func (p ByDepth) Swap(i, j int)      { p[i], p[j] = p[j], p[i] }
