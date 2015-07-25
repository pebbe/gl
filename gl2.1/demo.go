package main

import (
	"github.com/go-gl/gl/v2.1/gl"
	"github.com/go-gl/glfw/v3.1/glfw"

	"fmt"
	"math"
	"runtime"
	"time"
)

func init() {
	// This is needed to arrange that main() runs on main thread.
	// See documentation for functions that are only allowed to be called from the main thread.
	runtime.LockOSThread()
}

func main() {
	err := glfw.Init()
	if err != nil {
		panic(err)
	}
	defer glfw.Terminate()

	w, err := glfw.CreateWindow(640, 480, "Testing", nil, nil)
	if err != nil {
		panic(err)
	}

	w.MakeContextCurrent()
	glfw.SwapInterval(1)

	w.SetCharCallback(charCallBack)

	if err := gl.Init(); err != nil {
		panic(err)
	}

	setupScene(w)
	for !w.ShouldClose() {
		// Do OpenGL stuff.
		time.Sleep(10 * time.Millisecond)
		drawScene(w)

		w.SwapBuffers()
		glfw.PollEvents()
	}
}

func charCallBack(w *glfw.Window, char rune) {
	fmt.Printf("%c\n", char)
	if char == 'q' {
		w.SetShouldClose(true)
	}
}

func setupScene(w *glfw.Window) {
	gl.ClearColor(.5, .5, .5, 0)
}

func drawScene(w *glfw.Window) {
	width, height := w.GetFramebufferSize()
	ratio := float32(width) / float32(height)
	var x1, x2, y1, y2 float32
	if ratio > 1 {
		x1, x2, y1, y2 = -ratio, ratio, -1, 1
	} else {
		x1, x2, y1, y2 = -1, 1, -1/ratio, 1/ratio
	}

	gl.Viewport(0, 0, int32(width), int32(height))
	gl.Clear(gl.COLOR_BUFFER_BIT)

	// Applies subsequent matrix operations to the projection matrix stack
	gl.MatrixMode(gl.PROJECTION)

	gl.LoadIdentity()                                                   // replace the current matrix with the identity matrix
	gl.Ortho(float64(x1), float64(x2), float64(y1), float64(y2), 1, -1) // multiply the current matrix with an orthographic matrix

	// Applies subsequent matrix operations to the modelview matrix stack
	gl.MatrixMode(gl.MODELVIEW)

	gl.LoadIdentity()

	gl.LineWidth(1)
	gl.Begin(gl.LINE)   // delimit the vertices of a primitive or a group of like primitives
	gl.Color3f(0, 0, 0) // set the current color
	gl.Vertex3f(0, y1, 0)
	gl.Vertex3f(0, y2, 0)
	gl.Vertex3f(x1, 0, 0)
	gl.Vertex3f(x2, 0, 0)
	gl.End()

	gl.Rotatef(float32(glfw.GetTime()*50), 0, 0, 1) // multiply the current matrix by a rotation matrix

	s := float32(.95)

	gl.Begin(gl.TRIANGLES)
	gl.Color3f(1, 0, 0)  // set the current color
	gl.Vertex3f(0, s, 0) // specify a vertex
	gl.Color3f(0, 1, 0)
	gl.Vertex3f(s*.866, s*-0.5, 0)
	gl.Color3f(0, 0, 1)
	gl.Vertex3f(s*-.866, s*-0.5, 0)
	gl.End()

	gl.LineWidth(5)
	gl.Begin(gl.LINE_LOOP)
	for i := float64(0); i < 2*math.Pi; i += .05 {
		r, g, b := hsb2rgb(float32(i/(2*math.Pi)), 1, 1)
		gl.Color3f(r, g, b)
		gl.Vertex3f(s*float32(math.Sin(i)), s*float32(math.Cos(i)), 0)
	}
	gl.End()

}

func hsb2rgb(h, s, b float32) (float32, float32, float32) {
	c := b * s
	h *= 6
	x := c * float32(1-math.Abs(math.Mod(float64(h), 2)-1))
	if h < 1 {
		return c, x, 0
	}
	if h < 2 {
		return x, c, 0
	}
	if h < 3 {
		return 0, c, x
	}
	if h < 4 {
		return 0, x, c
	}
	if h < 5 {
		return x, 0, c
	}
	return c, 0, x
}
