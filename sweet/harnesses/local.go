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
	noStdout  bool
}

func (h *localBenchHarness) CheckPrerequisites() error {
	return nil
}

func (h *localBenchHarness) Get(_ string) error {
	return nil
}

func (h *localBenchHarness) Build(cfg *common.Config, bcfg *common.BuildConfig) error {
	return cfg.GoTool().BuildPath(bcfg.BenchDir, filepath.Join(bcfg.BinDir, h.binName))
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
	if !h.noStdout {
		cmd.Stdout = rcfg.Results
	}
	cmd.Stderr = rcfg.Results
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
			return []string{
				"-batch-size", "100",
				"-documents", "1000",
				filepath.Join(rcfg.AssetsDir, "enwiki-20080103-pages-articles.xml.bz2"),
			}
		},
	}
}

func BleveQuery() common.Harness {
	return &localBenchHarness{
		binName: "bleve-query-bench",
		genArgs: func(cfg *common.Config, rcfg *common.RunConfig) []string {
			return []string{
				filepath.Join(rcfg.AssetsDir, "index"),
			}
		},
		beforeRun: func(cfg *common.Config, rcfg *common.RunConfig) error {
			// Make sure all the index passed to the benchmark is writeable.
			indexPath := filepath.Join(rcfg.AssetsDir, "index")
			return makeWriteable(indexPath)
		},
	}
}

func FoglemanFauxGL() common.Harness {
	return &localBenchHarness{
		binName: "fogleman-fauxgl-bench",
		genArgs: func(cfg *common.Config, rcfg *common.RunConfig) []string {
			return []string{
				filepath.Join(rcfg.AssetsDir, "3dbenchy.stl"),
			}
		},
		noStdout: true,
	}
}

func FoglemanPT() common.Harness {
	return &localBenchHarness{
		binName: "fogleman-pt-bench",
		genArgs: func(cfg *common.Config, rcfg *common.RunConfig) []string {
			return []string{
				"-iter", "1",
				filepath.Join(rcfg.AssetsDir, "gopher.obj"),
			}
		},
		noStdout: true,
	}
}

func GopherLua() common.Harness {
	return &localBenchHarness{
		binName: "gopher-lua-bench",
		genArgs: func(cfg *common.Config, rcfg *common.RunConfig) []string {
			return []string{
				filepath.Join(rcfg.AssetsDir, "k-nucleotide.lua"),
				filepath.Join(rcfg.AssetsDir, "input.txt"),
			}
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
