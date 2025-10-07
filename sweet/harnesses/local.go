// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package harnesses

import (
	"os/exec"
	"path/filepath"

	"golang.org/x/benchmarks/sweet/common"
	"golang.org/x/benchmarks/sweet/common/log"
)

type localBenchHarness struct {
	binName   string
	genArgs   func(cfg *common.Config, rcfg *common.RunConfig) []string
	beforeRun func(cfg *common.Config, rcfg *common.RunConfig) error
}

func (h *localBenchHarness) CheckPrerequisites() error {
	return nil
}

func (h *localBenchHarness) Get(_ *common.GetConfig) error {
	return nil
}

func (h *localBenchHarness) Build(cfg *common.Config, bcfg *common.BuildConfig) error {
	return cfg.GoTool(bcfg.BuildLog).BuildPath(bcfg.BenchDir, filepath.Join(bcfg.BinDir, h.binName))
}

func (h *localBenchHarness) Run(cfg *common.Config, rcfg *common.RunConfig) error {
	if h.beforeRun != nil {
		if err := h.beforeRun(cfg, rcfg); err != nil {
			return err
		}
	}
	cmd := exec.Command(
		filepath.Join(rcfg.BinDir, h.binName),
		append(rcfg.Args, h.genArgs(cfg, rcfg)...)...,
	)
	cmd.Env = cfg.ExecEnv.Collapse()
	cmd.Stdout = rcfg.Results
	cmd.Stderr = rcfg.Log
	log.TraceCommand(cmd, false)
	return cmd.Run()
}

func BiogoIgor() common.Harness {
	return &localBenchHarness{
		binName: "biogo-igor-bench",
		genArgs: func(cfg *common.Config, rcfg *common.RunConfig) []string {
			return []string{
				filepath.Join(rcfg.AssetsDir, "Homo_sapiens.GRCh38.dna.chromosome.22.gff"),
			}
		},
	}
}

func BiogoKrishna() common.Harness {
	return &localBenchHarness{
		binName: "biogo-krishna-bench",
		genArgs: func(cfg *common.Config, rcfg *common.RunConfig) []string {
			return []string{
				"-alignconc",
				"-tmp", rcfg.TmpDir,
				"-tmpconc",
				filepath.Join(rcfg.AssetsDir, "Mus_musculus.GRCm38.dna.nonchromosomal.fa"),
			}
		},
	}
}

func BleveIndex() common.Harness {
	return &localBenchHarness{
		binName: "bleve-index-bench",
		genArgs: func(cfg *common.Config, rcfg *common.RunConfig) []string {
			var args []string
			if rcfg.Short {
				args = []string{
					"-documents", "10",
					"-batch-size", "10",
				}
			} else {
				args = []string{
					"-documents", "1000",
					"-batch-size", "100",
				}
			}
			return append(args, filepath.Join(rcfg.AssetsDir, "enwiki-20080103-pages-articles.xml.bz2"))
		},
	}
}

func GopherLua() common.Harness {
	return &localBenchHarness{
		binName: "gopher-lua-bench",
		genArgs: func(cfg *common.Config, rcfg *common.RunConfig) []string {
			args := []string{
				filepath.Join(rcfg.AssetsDir, "k-nucleotide.lua"),
				filepath.Join(rcfg.AssetsDir, "input.txt"),
			}
			if rcfg.Short {
				args = append([]string{"-short"}, args...)
			}
			return args
		},
	}
}

func Markdown() common.Harness {
	return &localBenchHarness{
		binName: "markdown-bench",
		genArgs: func(cfg *common.Config, rcfg *common.RunConfig) []string {
			return []string{
				rcfg.AssetsDir,
			}
		},
	}
}
