// Copyright (C) 2015 Michael Fogleman.
// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ptbench

import (
	"image"

	"github.com/fogleman/pt/pt"
)

type Gopher struct {
	mesh *pt.Mesh
}

func Load(meshPath string) (*Gopher, error) {
	gopher := pt.GlossyMaterial(pt.Black, 1.2, pt.Radians(30))
	mesh, err := pt.LoadOBJ(meshPath, gopher)
	if err != nil {
		return nil, err
	}
	return &Gopher{
		mesh: mesh,
	}, nil
}

func (g *Gopher) Render(iter int) image.Image {
	scene := pt.Scene{}

	// create materials
	wall := pt.GlossyMaterial(pt.HexColor(0xFCFAE1), 1.5, pt.Radians(10))
	light := pt.LightMaterial(pt.White, 80)

	// add walls and lights
	scene.Add(pt.NewCube(pt.V(-10, -1, -10), pt.V(-2, 10, 10), wall))
	scene.Add(pt.NewCube(pt.V(-10, -1, -10), pt.V(10, 0, 10), wall))
	scene.Add(pt.NewSphere(pt.V(4, 10, 1), 1, light))

	g.mesh.Transform(pt.Rotate(pt.V(0, 1, 0), pt.Radians(-10)))
	g.mesh.SmoothNormals()
	g.mesh.FitInside(pt.Box{pt.V(-1, 0, -1), pt.V(1, 2, 1)}, pt.V(0.5, 0, 0.5))
	scene.Add(g.mesh)

	// position camera
	camera := pt.LookAt(pt.V(4, 1, 0), pt.V(0, 0.9, 0), pt.V(0, 1, 0), 40)

	// render the scene
	sampler := pt.NewSampler(16, 16)
	renderer := pt.NewRenderer(&scene, &camera, sampler, 1024, 1024)
	renderer.Verbose = false

	// perform iter iterations of rendering
	for j := 0; j < iter; j++ {
		renderer.Render()
	}
	return renderer.Buffer.Image(pt.ColorChannel)
}
