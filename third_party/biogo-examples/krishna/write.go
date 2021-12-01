// Copyright ©2014 The bíogo Authors. All rights reserved.
// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package krishna

import (
	"sync"

	"github.com/biogo/biogo/align/pals"
	"github.com/biogo/biogo/align/pals/dp"
)

var wlock = &sync.Mutex{}

func writeDPHits(w *pals.Writer, target, query *pals.Packed, hits []dp.Hit, comp bool) (n int, err error) {
	wlock.Lock()
	defer wlock.Unlock()

	for _, hit := range hits {
		pair, err := pals.NewPair(target, query, hit, comp)
		if err != nil {
			return n, err
		} else {
			ln, err := w.Write(pair)
			n += ln
			if err != nil {
				return n, err
			}
		}
	}

	return
}
