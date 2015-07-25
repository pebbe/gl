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
	v_glsl = `
#version 110

attribute vec2 position;

varying vec2 texcoord;

void main()
{
    gl_Position = vec4(position, 0.0, 1.0);
    texcoord = position * vec2(0.5) + vec2(0.5);
}
` + "\x00"

	f_glsl = `
#version 110

uniform float fade_factor;
uniform sampler2D textures[2];

varying vec2 texcoord;

void main()
{
    gl_FragColor = mix(
        texture2D(textures[0], texcoord),
        texture2D(textures[1], texcoord),
        fade_factor
    );
}
` + "\x00"
)

//
// Global data used by render
//

type tUniforms struct {
	fadeFactor int32
	textures   [2]int32
}

type tAttributes struct {
	position int32
}

type gResources struct {
	vertexBuffer  uint32
	elementBuffer uint32

	textures [2]uint32

	vertexShader   uint32
	fragmentShader uint32
	program        uint32

	uniforms tUniforms

	attributes tAttributes

	fadeFactor float32
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

	// upside down
	// draw.Draw(rgba, rgba.Bounds(), img, image.Point{0, 0}, draw.Src)

	// flip upside down image upside up
	ib := img.Bounds()
	width := ib.Max.X
	height := ib.Max.Y
	for h := 0; h < height; h++ {
		r := image.Rect(0, height-1-h, width, height-h)
		draw.Draw(rgba, r, img, image.Point{0, h}, draw.Src)
	}

	var texture uint32
	gl.GenTextures(1, &texture)
	gl.BindTexture(gl.TEXTURE_2D, texture)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexImage2D(
		gl.TEXTURE_2D, 0, // target, level
		gl.RGBA,                   // internal format
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
	gVertexBufferData = []float32{
		-1.0, -1.0,
		1.0, -1.0,
		-1.0, 1.0,
		1.0, 1.0,
	}
	gElementBufferData = []uint32{0, 1, 2, 3}
)

//
// Load and create all of our resources
//

func makeResources() *gResources {
	r := gResources{
		vertexBuffer:  makeBuffer(gl.ARRAY_BUFFER, gl.Ptr(gVertexBufferData), 4*len(gVertexBufferData)),
		elementBuffer: makeBuffer(gl.ELEMENT_ARRAY_BUFFER, gl.Ptr(gElementBufferData), 4*len(gElementBufferData)),
	}

	r.textures[0] = makeTexture("hello1.png")
	r.textures[1] = makeTexture("hello2.png")

	r.vertexShader = makeShader(gl.VERTEX_SHADER, v_glsl)
	r.fragmentShader = makeShader(gl.FRAGMENT_SHADER, f_glsl)
	r.program = makeProgram(r.vertexShader, r.fragmentShader)

	r.uniforms.fadeFactor = gl.GetUniformLocation(r.program, gl.Str("fade_factor\x00"))
	r.uniforms.textures[0] = gl.GetUniformLocation(r.program, gl.Str("textures[0]\x00"))
	r.uniforms.textures[1] = gl.GetUniformLocation(r.program, gl.Str("textures[1]\x00"))

	r.attributes.position = gl.GetAttribLocation(r.program, gl.Str("position\x00"))

	return &r
}

//
// Update:
//

var start = time.Now()

func updateFadeFactor(r *gResources) {
	r.fadeFactor = float32(math.Sin(time.Since(start).Seconds())*.5 + 0.5)
}

func render(w *glfw.Window, r *gResources) {

	width, height := w.GetFramebufferSize()
	gl.Viewport(0, 0, int32(width), int32(height))
	gl.Clear(gl.COLOR_BUFFER_BIT)

	gl.MatrixMode(gl.PROJECTION)
	gl.LoadIdentity()

	gl.MatrixMode(gl.MODELVIEW)
	gl.LoadIdentity()

	////////////////

	gl.UseProgram(r.program)

	gl.Uniform1f(r.uniforms.fadeFactor, r.fadeFactor)

	gl.ActiveTexture(gl.TEXTURE0)
	gl.BindTexture(gl.TEXTURE_2D, r.textures[0])
	gl.Uniform1i(r.uniforms.textures[0], 0)

	gl.ActiveTexture(gl.TEXTURE1)
	gl.BindTexture(gl.TEXTURE_2D, r.textures[1])
	gl.Uniform1i(r.uniforms.textures[1], 1)

	gl.BindBuffer(gl.ARRAY_BUFFER, r.vertexBuffer)
	gl.VertexAttribPointer(
		uint32(r.attributes.position), /* attribute */
		2,               /* size */
		gl.FLOAT,        /* type */
		false,           /* normalized? */
		8,               /* stride */
		gl.PtrOffset(0)) /* array buffer offset */

	gl.EnableVertexAttribArray(uint32(r.attributes.position))

	gl.BindBuffer(gl.ELEMENT_ARRAY_BUFFER, r.elementBuffer)
	gl.DrawElements(
		gl.TRIANGLE_STRIP, /* mode */
		4,                 /* count */
		gl.UNSIGNED_INT,   /* type */
		gl.PtrOffset(0))   /* element array buffer offset */

	gl.DisableVertexAttribArray(uint32(r.attributes.position))

}

func main() {
	err := glfw.Init()
	if err != nil {
		panic(err)
	}
	defer glfw.Terminate()

	glfw.WindowHint(glfw.Resizable, glfw.False)
	w, err := glfw.CreateWindow(400, 300, "Hello World", nil, nil)
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

	gl.ClearColor(1, 1, 1, 0)
	fmt.Println("Press 'q' to quit")
	for !w.ShouldClose() {
		time.Sleep(10 * time.Millisecond)

		updateFadeFactor(r)
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
