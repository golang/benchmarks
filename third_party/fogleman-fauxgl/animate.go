// Copyright (C) 2017 Michael Fogleman.
// Copyright 2021 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package animatebench

import (
	"flag"
	"image"

	. "github.com/fogleman/fauxgl"
	"github.com/nfnt/resize"
)

const (
	scale  = 4   // optional supersampling
	width  = 800 // output width in pixels
	height = 800 // output height in pixels
	fovy   = 30  // vertical field of view in degrees
	near   = 1   // near clipping plane
	far    = 10  // far clipping plane
)

var (
	eye        = V(4, 4, 2)                  // camera position
	center     = V(0, 0, 0)                  // view center position
	up         = V(0, 0, 1)                  // up vector
	light      = V(0.25, 0.5, 1).Normalize() // light direction
	color      = HexColor("#FEB41C")         // object color
	background = HexColor("#24221F")         // background color
)

type RotateAnimation struct {
	mesh    *Mesh
	matrix  Matrix
	context *Context
}

func Load(meshPath string) (*RotateAnimation, error) {
	// load a mesh
	mesh, err := LoadSTL(flag.Arg(0))
	if err != nil {
		return nil, err
	}

	// fit mesh in a bi-unit cube centered at the origin
	mesh.BiUnitCube()

	// create transformation matrix and light direction
	aspect := float64(width) / float64(height)
	matrix := LookAt(eye, center, up).Perspective(fovy, aspect, near, far)

	return &RotateAnimation{
		mesh:    mesh,
		matrix:  matrix,
		context: NewContext(width*scale, height*scale),
	}, nil
}

// RenderNext renders the next step in the animation, returning the resulting
// image. Each step rotates the input mesh by 5 degrees.
func (a *RotateAnimation) RenderNext() image.Image {
	// render
	a.context.ClearDepthBuffer()
	a.context.ClearColorBufferWith(background)
	shader := NewPhongShader(a.matrix, light, eye)
	shader.ObjectColor = color
	shader.DiffuseColor = Gray(0.9)
	shader.SpecularColor = Gray(0.25)
	shader.SpecularPower = 100
	a.context.Shader = shader
	a.context.DrawMesh(a.mesh)

	// resize image for anti-aliasing
	image := a.context.Image()
	image = resize.Resize(width, height, image, resize.Bilinear)

	a.mesh.Transform(Rotate(up, Radians(5)))

	return image
}
