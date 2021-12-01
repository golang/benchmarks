// Copyright ©2014 The bíogo Authors. All rights reserved.
// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package igor

import (
	"encoding/json"
	"io"

	"github.com/biogo/biogo/align/pals"
	"github.com/biogo/biogo/seq"
	"github.com/biogo/graph"
)

func WriteJSON(cc []graph.Nodes, w io.Writer) error {
	type feat struct {
		C string
		S int
		E int
		O seq.Strand
	}
	var (
		a feat
		f []feat
		j = json.NewEncoder(w)
	)

	seen := make(map[feat]struct{})
	var fi int
	for _, fam := range cc {
		for _, p := range fam {
			pile := p.(*pals.Pile)

			a.C = pile.Location().Name()
			a.S = pile.Start()
			a.E = pile.End()
			a.O = pile.Strand
			if _, ok := seen[a]; !ok && pile.Loc != nil {
				seen[a] = struct{}{}
				f = append(f, a)
			}
		}
		switch len(f) {
		case 0, 1:
			continue
		default:
			for i := 0; i < len(f); {
				if f[i].O == seq.None {
					f[i], f = f[len(f)-1], f[:len(f)-1]
				} else {
					i++
				}
			}
		}
		if len(f) < 2 {
			continue
		}
		err := j.Encode(f)
		if err != nil {
			return err
		}
		f = f[:0]
		fi++
	}
	return nil
}
