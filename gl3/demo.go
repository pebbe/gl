package main

import (
	"github.com/go-gl/gl/all-core/gl"
	"github.com/go-gl/glfw/v3.1/glfw"

	"errors"
	"fmt"
	"image"
	"image/draw"
	_ "image/png"
	"log"
	"math"
	"os"
	"runtime"
	"strings"
	"time"
	"unsafe"
)

var (
	vector_glsl1 = `
#version 110

attribute vec2 position;

void main()
{
    gl_Position = vec4(position, 0.0, 1.0);
}
` + "\x00"

	fragment_glsl1 = `
#version 110

void main()
{
    gl_FragColor = vec4(0, 0, 0, 0);
}
` + "\x00"

	vector_glsl2 = `
#version 110

uniform float xmul;
uniform float ymul;
uniform float sn;
uniform float cs;

attribute vec3 vertexColor;
attribute vec2 position;

varying vec3 color;

void main()
{
    gl_Position = vec4(xmul * (cs*position[0] + sn*position[1]), ymul * (sn*position[0] - cs*position[1]), 0.0, 1.0);
    color = vertexColor;
}
` + "\x00"

	fragment_glsl2 = `
#version 110

varying vec3 color;

void main()
{
    gl_FragColor = vec4(color, 0);
}
` + "\x00"
)

//
// Global data used by render
//

type tUniforms struct {
	xmul int32
	ymul int32
	sin  int32
	cos  int32
}

type tAttributes struct {
	position int32
	color    int32
	color3   int32
}

type gResources struct {
	vertexBuffer1   uint32
	elementBuffer1  uint32
	vertexShader1   uint32
	fragmentShader1 uint32
	program1        uint32
	attributes1     tAttributes

	vertexBuffer2   uint32
	elementBuffer2  uint32
	colorBuffer2    uint32
	vertexShader2   uint32
	fragmentShader2 uint32
	program2        uint32
	uniforms2       tUniforms
	attributes2     tAttributes

	vertexBuffer3  uint32
	elementBuffer3 uint32
	colorBuffer3   uint32
	len3           int32
}

//
// Functions for creating OpenGL objects:
//

func makeBuffer(target uint32, bufferData unsafe.Pointer, bufferSize int) uint32 {
	var buffer uint32
	gl.GenBuffers(1, &buffer)
	gl.BindBuffer(target, buffer)
	gl.BufferData(target, bufferSize, bufferData, gl.STATIC_DRAW)
	return buffer
}

func makeTexture(filename string) uint32 {
	fp, err := os.Open(filename)
	x(err)
	img, _, err := image.Decode(fp)
	fp.Close()
	x(err)

	rgba := image.NewRGBA(img.Bounds())
	if rgba.Stride != rgba.Rect.Size().X*4 {
		x(errors.New("unsupported stride"))
	}

	draw.Draw(rgba, rgba.Bounds(), img, image.Point{0, 0}, draw.Src)

	var texture uint32
	gl.GenTextures(1, &texture)
	gl.BindTexture(gl.TEXTURE_2D, texture)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexImage2D(
		gl.TEXTURE_2D, 0, // target, level
		gl.RGB8,                   // internal format
		int32(rgba.Rect.Size().X), // width
		int32(rgba.Rect.Size().Y), // height
		0,                         // border
		gl.RGBA, gl.UNSIGNED_BYTE, // external format, type
		gl.Ptr(rgba.Pix)) // pixels

	return texture
}

func makeShader(shaderType uint32, source string) uint32 {
	shader := gl.CreateShader(shaderType)

	csource := gl.Str(source)
	gl.ShaderSource(shader, 1, &csource, nil)
	gl.CompileShader(shader)

	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &logLength)

		log := strings.Repeat("\x00", int(logLength+1))
		gl.GetShaderInfoLog(shader, logLength, nil, gl.Str(log))

		x(fmt.Errorf("failed to compile %v: %v", source, log))
	}

	return shader
}

func makeProgram(vertexShader uint32, fragmentShader uint32) uint32 {

	program := gl.CreateProgram()

	gl.AttachShader(program, vertexShader)
	gl.AttachShader(program, fragmentShader)
	gl.LinkProgram(program)

	var status int32
	gl.GetProgramiv(program, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetProgramiv(program, gl.INFO_LOG_LENGTH, &logLength)

		log := strings.Repeat("\x00", int(logLength+1))
		gl.GetProgramInfoLog(program, logLength, nil, gl.Str(log))

		x(errors.New(fmt.Sprintf("failed to link program: %v", log)))
	}

	return program
}

//
// Data used to seed our vertex array and element array buffers:
//

var (
	gVertexBufferData1 = []float32{
		-1.0, 0.0,
		1.0, 0.0,
		0.0, -1.0,
		0.0, 1.0,
	}
	gElementBufferData1 = []uint32{0, 1, 2, 3}
)

var (
	gVertexBufferData2 = []float32{
		0.0, 1.0,
		0.866, -0.5,
		-0.866, -0.5,
	}
	gElementBufferData2 = []uint32{0, 1, 2}
	gColorBufferData2   = []float32{
		1, 0, 0,
		0, 1, 0,
		0, 0, 1,
	}
)

//
// Load and create all of our resources
//

func makeResources() *gResources {
	r := gResources{
		vertexBuffer1:  makeBuffer(gl.ARRAY_BUFFER, gl.Ptr(gVertexBufferData1), 4*len(gVertexBufferData1)),
		elementBuffer1: makeBuffer(gl.ELEMENT_ARRAY_BUFFER, gl.Ptr(gElementBufferData1), 4*len(gElementBufferData1)),
		vertexBuffer2:  makeBuffer(gl.ARRAY_BUFFER, gl.Ptr(gVertexBufferData2), 4*len(gVertexBufferData2)),
		elementBuffer2: makeBuffer(gl.ELEMENT_ARRAY_BUFFER, gl.Ptr(gElementBufferData2), 4*len(gElementBufferData2)),
		colorBuffer2:   makeBuffer(gl.ARRAY_BUFFER, gl.Ptr(gColorBufferData2), 4*len(gColorBufferData2)),
	}

	r.vertexShader1 = makeShader(gl.VERTEX_SHADER, vector_glsl1)
	r.fragmentShader1 = makeShader(gl.FRAGMENT_SHADER, fragment_glsl1)
	r.program1 = makeProgram(r.vertexShader1, r.fragmentShader1)

	r.vertexShader2 = makeShader(gl.VERTEX_SHADER, vector_glsl2)
	r.fragmentShader2 = makeShader(gl.FRAGMENT_SHADER, fragment_glsl2)
	r.program2 = makeProgram(r.vertexShader2, r.fragmentShader2)

	r.uniforms2.xmul = gl.GetUniformLocation(r.program2, gl.Str("xmul\x00"))
	r.uniforms2.ymul = gl.GetUniformLocation(r.program2, gl.Str("ymul\x00"))
	r.uniforms2.sin = gl.GetUniformLocation(r.program2, gl.Str("sn\x00"))
	r.uniforms2.cos = gl.GetUniformLocation(r.program2, gl.Str("cs\x00"))

	r.attributes1.position = gl.GetAttribLocation(r.program1, gl.Str("position\x00"))
	r.attributes2.position = gl.GetAttribLocation(r.program2, gl.Str("position\x00"))
	r.attributes2.color = gl.GetAttribLocation(r.program2, gl.Str("vertexColor\x00"))

	gColorBufferData3 := make([]float32, 0, 126*3)
	gVertexBufferData3 := make([]float32, 0, 126*2)
	gElementBufferData3 := make([]uint32, 0, 126)
	r.len3 = 0
	for i := float64(0); i < 2*math.Pi; i += .05 {
		rd, g, b := hsb2rgb(float32(i/(2*math.Pi)), 1, 1)
		gColorBufferData3 = append(gColorBufferData3, rd, g, b)
		gVertexBufferData3 = append(gVertexBufferData3, float32(math.Sin(i)), float32(math.Cos(i)))
		gElementBufferData3 = append(gElementBufferData3, uint32(r.len3))
		r.len3++
	}
	r.vertexBuffer3 = makeBuffer(gl.ARRAY_BUFFER, gl.Ptr(gVertexBufferData3), 4*len(gVertexBufferData3))
	r.elementBuffer3 = makeBuffer(gl.ELEMENT_ARRAY_BUFFER, gl.Ptr(gElementBufferData3), 4*len(gElementBufferData3))
	r.colorBuffer3 = makeBuffer(gl.ARRAY_BUFFER, gl.Ptr(gColorBufferData3), 4*len(gColorBufferData3))

	return &r
}

var start = time.Now()

func render(w *glfw.Window, r *gResources) {

	ra := float32(.95)

	width, height := w.GetFramebufferSize()
	ratio := float32(width) / float32(height)
	d := time.Since(start).Seconds()
	sin := float32(math.Sin(d))
	cos := float32(math.Cos(d))
	var xmul, ymul float32
	if ratio > 1 {
		xmul, ymul = ra/ratio, ra
	} else {
		xmul, ymul = ra, ra*ratio
	}

	gl.Viewport(0, 0, int32(width), int32(height))
	gl.Clear(gl.COLOR_BUFFER_BIT)

	////////////////

	gl.UseProgram(r.program1)

	gl.BindBuffer(gl.ARRAY_BUFFER, r.vertexBuffer1)

	gl.VertexAttribPointer(
		uint32(r.attributes1.position), // attribute
		2,               // size
		gl.FLOAT,        // type
		false,           // normalized?
		8,               // stride
		gl.PtrOffset(0)) // array buffer offset
	gl.EnableVertexAttribArray(uint32(r.attributes1.position))

	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, r.elementBuffer1)

	gl.LineWidth(1)
	gl.DrawElements(
		gl.LINES,        // mode
		4,               // count/
		gl.UNSIGNED_INT, // type
		gl.PtrOffset(0)) // element array buffer offset

	gl.DisableVertexAttribArray(uint32(r.attributes1.position))

	////////////////

	gl.UseProgram(r.program2)

	gl.Uniform1f(r.uniforms2.xmul, xmul)
	gl.Uniform1f(r.uniforms2.ymul, ymul)
	gl.Uniform1f(r.uniforms2.sin, sin)
	gl.Uniform1f(r.uniforms2.cos, cos)

	gl.BindBuffer(gl.ARRAY_BUFFER, r.vertexBuffer2)

	gl.VertexAttribPointer(
		uint32(r.attributes2.position), // attribute
		2,               // size
		gl.FLOAT,        // type
		false,           // normalized?
		8,               // stride
		gl.PtrOffset(0)) // array buffer offset
	gl.EnableVertexAttribArray(uint32(r.attributes2.position))

	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, r.elementBuffer2)

	gl.EnableVertexAttribArray(uint32(r.attributes2.color))
	gl.BindBuffer(gl.ARRAY_BUFFER, r.colorBuffer2)
	gl.VertexAttribPointer(
		uint32(r.attributes2.color), // attribute
		3,               // size
		gl.FLOAT,        // type
		false,           // normalized?
		0,               // stride
		gl.PtrOffset(0)) // array buffer offset

	gl.DrawElements(
		gl.TRIANGLES,    // mode
		3,               // count/
		gl.UNSIGNED_INT, // type
		gl.PtrOffset(0)) // element array buffer offset

	////////////////

	gl.BindBuffer(gl.ARRAY_BUFFER, r.vertexBuffer3)

	gl.VertexAttribPointer(
		uint32(r.attributes2.position), // attribute
		2,               // size
		gl.FLOAT,        // type
		false,           // normalized?
		8,               // stride
		gl.PtrOffset(0)) // array buffer offset

	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, r.elementBuffer3)

	gl.BindBuffer(gl.ARRAY_BUFFER, r.colorBuffer3)

	gl.VertexAttribPointer(
		uint32(r.attributes2.color), // attribute
		3,               // size
		gl.FLOAT,        // type
		false,           // normalized?
		0,               // stride
		gl.PtrOffset(0)) // array buffer offset

	gl.LineWidth(5)
	gl.DrawElements(
		gl.LINE_LOOP,    // mode
		r.len3,          // count/
		gl.UNSIGNED_INT, // type
		gl.PtrOffset(0)) // element array buffer offset

	gl.DisableVertexAttribArray(uint32(r.attributes2.color))
	gl.DisableVertexAttribArray(uint32(r.attributes2.position))

}

func main() {
	err := glfw.Init()
	if err != nil {
		panic(err)
	}
	defer glfw.Terminate()

	w, err := glfw.CreateWindow(640, 480, "Testing 3+", nil, nil)
	if err != nil {
		panic(err)
	}

	w.MakeContextCurrent()
	glfw.SwapInterval(1)

	w.SetCharCallback(charCallBack)

	if err := gl.Init(); err != nil {
		panic(err)
	}

	r := makeResources()

	gl.ClearColor(.5, .5, .5, 0)
	fmt.Println("Press 'q' to quit")
	for !w.ShouldClose() {
		time.Sleep(10 * time.Millisecond)

		render(w, r)

		w.SwapBuffers()
		glfw.PollEvents()
	}
}

func charCallBack(w *glfw.Window, char rune) {
	if char == 'q' {
		w.SetShouldClose(true)
	}
}

func init() {
	// This is needed to arrange that main() runs on main thread.
	// See documentation for functions that are only allowed to be called from the main thread.
	runtime.LockOSThread()
}

func x(err error) {
	if err != nil {
		log.Fatalln(err)
	}
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
