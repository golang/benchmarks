// Copyright ©2014 The bíogo Authors. All rights reserved.
// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package igor

import (
	"log"

	"github.com/biogo/biogo/align/pals"
	"github.com/biogo/biogo/seq"
	"github.com/biogo/graph"
)

// GroupConfig specifies Group behaviour
type GroupConfig struct {
	// PileDiff specifies the acceptable fractional difference between
	// piles for assignment to the same group.
	PileDiff float64
	// ImageDiff specifies the acceptable fractional difference between
	// an image and its containing pile when considering a pile for
	// assignment based on that image.
	ImageDiff float64

	// When Classic is true, Group will run a reasonable approximation
	// of the original C PILER family grouping.
	Classic bool
}

// Group clusters the input pile collection based on the existence of satisfactory
// image alignments according to the provided config. Piles with nil locations are
// ignored. Checks are performed to ensure that the produced clusters can be
// unambiguously assigned to a DNA strand.
func Group(clust [][]*pals.Pile, cfg GroupConfig) []graph.Nodes {
	g := newPileGraph()

	for _, c := range clust {
		for _, pile := range c {
			if pile == nil || pile.Loc == nil || len(pile.Images) == 0 {
				continue
			}
			g.insert(pile)

			for _, im := range pile.Images {
				partner := im.Mate().Location().(*pals.Pile)
				if partner.Loc == nil {
					continue
				}
				if !cfg.Classic { // We already know these are true when cfg.Classic is true.
					// Confirm that piles are within cfg.PileDiff in length.
					if !within(cfg.PileDiff, min(pile.Len(), partner.Len()), max(pile.Len(), partner.Len())) {
						continue
					}
					// Confirm that images are within cfg.ImageDiff of their piles in length.
					if !within(cfg.ImageDiff, im.Len(), pile.Len()) || !within(cfg.ImageDiff, im.Mate().Len(), partner.Len()) {
						continue
					}
				}
				g.insert(partner)

				fPile := feature{pile.Name(), pile.Start(), pile.End()}
				fPartner := feature{partner.Name(), partner.Start(), partner.End()}
				if pile.Node != partner.Node && fPile != fPartner {
					err := g.connect(pile, partner, im.Pair.Strand)
					if err != nil {
						log.Fatalf("igor: internal error: %v", err)
					}
				}
			}
		}
	}

	for p, n := range g.poisoned {
		if n > 1 {
			// Removing poisioned node.
			g.delete(p)
		}
		// May still have a poisoned node.
	}

	cc := g.connectedComponents(func(e graph.Edge) bool {
		te := e.(*twistEdge)
		if te.twist == seq.None {
			return false
		}
		h := e.Head().(*pals.Pile)
		t := e.Tail().(*pals.Pile)
		switch {
		case h.Strand == 0 && t.Strand == 0:
			h.Strand = 1
			t.Strand = te.twist
		case t.Strand == 0:
			t.Strand = h.Strand * te.twist
		case h.Strand == 0:
			h.Strand = t.Strand * te.twist
		default:
			if h.Strand != t.Strand*te.twist {
				te.conflict = true
			}
		}
		return true
	})

	for i := 0; i < len(cc); i++ {
	loop:
		for _, n := range cc[i] {
			for _, e := range n.Edges() {
				if e.(*twistEdge).conflict {
					cc[i] = cc[len(cc)-1]
					cc = cc[:len(cc)-1]
					i--
					break loop
				}
			}
		}
	}

	for _, c := range cc {
		for _, n := range c {
			n.(*pals.Pile).Node = nil
		}
	}

	return cc
}

type feature struct {
	name     string
	from, to int
}

type twistEdge struct {
	graph.Edge
	twist    seq.Strand
	conflict bool
}

type pileGraph struct {
	g     *graph.Undirected
	nodes map[feature]graph.Node
	edges map[[2]*pals.Pile]*twistEdge

	poisoned map[*pals.Pile]int
}

func newPileGraph() pileGraph {
	return pileGraph{
		g: graph.NewUndirected(),

		nodes: make(map[feature]graph.Node),
		edges: make(map[[2]*pals.Pile]*twistEdge),

		poisoned: make(map[*pals.Pile]int),
	}
}

func (g pileGraph) insert(p *pals.Pile) {
	if p.Node != nil {
		return
	}
	f := feature{p.Loc.Name(), p.Start(), p.End()}
	if n, exists := g.nodes[f]; exists {
		p.Node = g.g.NewNode()
		g.g.Add(p)
		e := &twistEdge{
			Edge:  graph.NewEdge(),
			twist: seq.Plus,
		}
		g.g.ConnectWith(n, p, e)
		return
	}
	p.Node = g.g.NewNode()
	g.g.Add(p)
	g.nodes[f] = p
}

func (g pileGraph) connect(pile, partner *pals.Pile, twist seq.Strand) error {
	var (
		e  *twistEdge
		ok bool
	)
	if e, ok = g.edges[[2]*pals.Pile{pile, partner}]; !ok {
		e, ok = g.edges[[2]*pals.Pile{partner, pile}]
	}
	if ok && e.twist != twist {
		if e.twist != seq.None {
			g.poisoned[pile]++
			g.poisoned[partner]++
		}
		e.twist = seq.None
		return nil
	}

	e = &twistEdge{
		Edge:  graph.NewEdge(),
		twist: twist,
	}
	g.edges[[2]*pals.Pile{pile, partner}] = e
	return g.g.ConnectWith(pile, partner, e)
}

func (g pileGraph) delete(p *pals.Pile) error {
	return g.g.Delete(p)
}

func (g pileGraph) connectedComponents(fn func(e graph.Edge) bool) []graph.Nodes {
	return graph.ConnectedComponents(g.g, fn)
}
