// Licensed to Apache Software Foundation (ASF) under one or more contributor
// license agreements. See the NOTICE file distributed with
// this work for additional information regarding copyright
// ownership. Apache Software Foundation (ASF) licenses this file to you under
// the Apache License, Version 2.0 (the "License"); you may
// not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

// Package analyzer provides analyzers for indexing and searching.
package analyzer

import (
	"bytes"
	"unicode"

	"github.com/blugelabs/bluge/analysis"
	"github.com/blugelabs/bluge/analysis/analyzer"
	"github.com/blugelabs/bluge/analysis/tokenizer"

	"github.com/apache/skywalking-banyandb/pkg/index"
)

// Analyzers is a map that associates each IndexRule_Analyzer type with a corresponding Analyzer.
var Analyzers map[string]*analysis.Analyzer

func init() {
	Analyzers = map[string]*analysis.Analyzer{
		index.AnalyzerKeyword:  analyzer.NewKeywordAnalyzer(),
		index.AnalyzerSimple:   analyzer.NewSimpleAnalyzer(),
		index.AnalyzerStandard: analyzer.NewStandardAnalyzer(),
		index.AnalyzerURL:      NewURLAnalyzer(),
	}
}

// NewURLAnalyzer creates a new URL analyzer.
func NewURLAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Tokenizer: tokenizer.NewCharacterTokenizer(func(r rune) bool {
			return unicode.IsLetter(r) || unicode.IsNumber(r)
		}),
		TokenFilters: []analysis.TokenFilter{
			newAlphanumericFilter(),
		},
	}
}

type alphanumericFilter struct{}

func newAlphanumericFilter() *alphanumericFilter {
	return &alphanumericFilter{}
}

func (f *alphanumericFilter) Filter(input analysis.TokenStream) analysis.TokenStream {
	for _, token := range input {
		termRunes := []rune{}
		for _, r := range bytes.Runes(token.Term) {
			if unicode.IsLetter(r) || unicode.IsNumber(r) {
				termRunes = append(termRunes, r)
			}
		}
		token.Term = analysis.BuildTermFromRunes(termRunes)
	}
	return input
}
