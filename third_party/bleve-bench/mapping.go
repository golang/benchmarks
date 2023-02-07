//  Copyright (c) 2018 Couchbase, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// 		http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build !aix && !plan9
// +build !aix,!plan9

package blevebench

import (
	"github.com/blevesearch/bleve"
	"github.com/blevesearch/bleve/mapping"
)

// ArticleMapping returns a mapping for indexing wikipedia articles
// in a manner similar to that done by the Apache Lucene nightly
// benchmarks.
func ArticleMapping() mapping.IndexMapping {
	standard := bleve.NewTextFieldMapping()
	standard.Store = false
	standard.IncludeInAll = false
	standard.IncludeTermVectors = false
	standard.Analyzer = "standard"

	keyword := bleve.NewTextFieldMapping()
	keyword.Store = false
	keyword.IncludeInAll = false
	keyword.IncludeTermVectors = false
	keyword.Analyzer = "keyword"

	article := bleve.NewDocumentMapping()
	article.AddFieldMappingsAt("Title", keyword)
	article.AddFieldMappingsAt("Text", standard)

	disabled := bleve.NewDocumentDisabledMapping()
	article.AddSubDocumentMapping("Other", disabled)

	mapping := bleve.NewIndexMapping()
	mapping.DefaultMapping = article
	mapping.DefaultField = "Other"
	mapping.DefaultAnalyzer = "standard"

	return mapping
}

type Article struct {
	Title, Text string
}
