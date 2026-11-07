//go:build windows

package main

import (
	"fmt"
	"math"
	"syscall"
	"unsafe"
)

// Minimale OpenGL-Anbindung (fixed function + VBOs + DXT-Texturen) ohne cgo.

var opengl32 = syscall.NewLazyDLL("opengl32.dll")

func glp(name string) *syscall.LazyProc { return opengl32.NewProc(name) }

var (
	pwglCreateContext  = glp("wglCreateContext")
	pwglMakeCurrent    = glp("wglMakeCurrent")
	pwglDeleteContext  = glp("wglDeleteContext")
	pwglGetProcAddress = glp("wglGetProcAddress")

	pglClear              = glp("glClear")
	pglClearColor         = glp("glClearColor")
	pglViewport           = glp("glViewport")
	pglMatrixMode         = glp("glMatrixMode")
	pglLoadMatrixf        = glp("glLoadMatrixf")
	pglMultMatrixf        = glp("glMultMatrixf")
	pglLoadIdentity       = glp("glLoadIdentity")
	pglPushMatrix         = glp("glPushMatrix")
	pglPopMatrix          = glp("glPopMatrix")
	pglEnable             = glp("glEnable")
	pglDisable            = glp("glDisable")
	pglDepthFunc          = glp("glDepthFunc")
	pglDepthMask          = glp("glDepthMask")
	pglBlendFunc          = glp("glBlendFunc")
	pglAlphaFunc          = glp("glAlphaFunc")
	pglEnableClientState  = glp("glEnableClientState")
	pglDisableClientState = glp("glDisableClientState")
	pglVertexPointer      = glp("glVertexPointer")
	pglNormalPointer      = glp("glNormalPointer")
	pglTexCoordPointer    = glp("glTexCoordPointer")
	pglDrawElements       = glp("glDrawElements")
	pglGenTextures        = glp("glGenTextures")
	pglBindTexture        = glp("glBindTexture")
	pglTexImage2D         = glp("glTexImage2D")
	pglTexParameteri      = glp("glTexParameteri")
	pglDeleteTextures     = glp("glDeleteTextures")
	pglLightfv            = glp("glLightfv")
	pglLightModelfv       = glp("glLightModelfv")
	pglLightModeli        = glp("glLightModeli")
	pglColor4fv           = glp("glColor4fv")
	pglBegin              = glp("glBegin")
	pglEnd                = glp("glEnd")
	pglVertex3fv          = glp("glVertex3fv")
	pglNormal3fv          = glp("glNormal3fv")
	pglLineWidth          = glp("glLineWidth")
	pglGetString          = glp("glGetString")
	pglGetIntegerv        = glp("glGetIntegerv")
	pglColorMaterial      = glp("glColorMaterial")
	pglShadeModel         = glp("glShadeModel")
	pglFogi               = glp("glFogi")
	pglFogfv              = glp("glFogfv")
	pglPixelStorei        = glp("glPixelStorei")
	pglGetError           = glp("glGetError")
	pglTexEnvi            = glp("glTexEnvi")
	pglPolygonMode        = glp("glPolygonMode")
	pglHint               = glp("glHint")
	pglCullFace           = glp("glCullFace")

	glGenBuffersPtr, glBindBufferPtr, glBufferDataPtr, glDeleteBuffersPtr uintptr
	glCompressedTexImage2DPtr, wglSwapIntervalPtr                         uintptr
	hasCompressed                                                         bool
)

const (
	GL_COLOR_BUFFER_BIT              = 0x4000
	GL_DEPTH_BUFFER_BIT              = 0x0100
	GL_TRIANGLES                     = 0x0004
	GL_LINES                         = 0x0001
	GL_LINE_LOOP                     = 0x0002
	GL_QUADS                         = 0x0007
	GL_MODELVIEW                     = 0x1700
	GL_PROJECTION                    = 0x1701
	GL_DEPTH_TEST                    = 0x0B71
	GL_CULL_FACE                     = 0x0B44
	GL_BLEND                         = 0x0BE2
	GL_ALPHA_TEST                    = 0x0BC0
	GL_TEXTURE_2D                    = 0x0DE1
	GL_LIGHTING                      = 0x0B50
	GL_LIGHT0                        = 0x4000
	GL_LIGHT1                        = 0x4001
	GL_NORMALIZE                     = 0x0BA1
	GL_COLOR_MATERIAL                = 0x0B57
	GL_FOG                           = 0x0B60
	GL_LEQUAL                        = 0x0203
	GL_LESS                          = 0x0201
	GL_GREATER                       = 0x0204
	GL_SRC_ALPHA                     = 0x0302
	GL_ONE_MINUS_SRC_ALPHA           = 0x0303
	GL_ONE                           = 1
	GL_ZERO                          = 0
	GL_DST_COLOR                     = 0x0306
	GL_VERTEX_ARRAY                  = 0x8074
	GL_NORMAL_ARRAY                  = 0x8075
	GL_TEXTURE_COORD_ARRAY           = 0x8078
	GL_FLOAT                         = 0x1406
	GL_UNSIGNED_INT                  = 0x1405
	GL_UNSIGNED_BYTE                 = 0x1401
	GL_RGBA                          = 0x1908
	GL_BGRA                          = 0x80E1
	GL_TEXTURE_MIN_FILTER            = 0x2801
	GL_TEXTURE_MAG_FILTER            = 0x2800
	GL_TEXTURE_WRAP_S                = 0x2802
	GL_TEXTURE_WRAP_T                = 0x2803
	GL_REPEAT                        = 0x2901
	GL_LINEAR                        = 0x2601
	GL_LINEAR_MIPMAP_LINEAR          = 0x2703
	GL_TEXTURE_MAX_LEVEL             = 0x813D
	GL_POSITION                      = 0x1203
	GL_AMBIENT                       = 0x1200
	GL_DIFFUSE                       = 0x1201
	GL_SPECULAR                      = 0x1202
	GL_LIGHT_MODEL_AMBIENT           = 0x0B53
	GL_LIGHT_MODEL_TWO_SIDE          = 0x0B52
	GL_FRONT_AND_BACK                = 0x0408
	GL_FRONT                         = 0x0404
	GL_BACK                          = 0x0405
	GL_AMBIENT_AND_DIFFUSE           = 0x1602
	GL_SMOOTH                        = 0x1D01
	GL_FOG_MODE                      = 0x0B65
	GL_FOG_START                     = 0x0B63
	GL_FOG_END                       = 0x0B64
	GL_FOG_COLOR                     = 0x0B66
	GL_LINEAR_FOG                    = 0x2601
	GL_UNPACK_ALIGNMENT              = 0x0CF5
	GL_TEXTURE_ENV                   = 0x2300
	GL_TEXTURE_ENV_MODE              = 0x2200
	GL_MODULATE                      = 0x2100
	GL_LINE                          = 0x1B01
	GL_FILL                          = 0x1B02
	GL_VENDOR                        = 0x1F00
	GL_RENDERER                      = 0x1F01
	GL_VERSION                       = 0x1F02
	GL_EXTENSIONS                    = 0x1F03
	GL_ARRAY_BUFFER                  = 0x8892
	GL_ELEMENT_ARRAY_BUFFER          = 0x8893
	GL_STATIC_DRAW                   = 0x88E4
	GL_COMPRESSED_RGBA_S3TC_DXT1_EXT = 0x83F1
	GL_COMPRESSED_RGBA_S3TC_DXT3_EXT = 0x83F2
	GL_COMPRESSED_RGBA_S3TC_DXT5_EXT = 0x83F3
	GL_PERSPECTIVE_CORRECTION_HINT   = 0x0C50
	GL_NICEST                        = 0x1102
)

func f32arg(f float32) uintptr { return uintptr(math.Float32bits(f)) }

func glClear(m uint32) { pglClear.Call(uintptr(m)) }
func glClearColor(r, g, b, a float32) {
	pglClearColor.Call(f32arg(r), f32arg(g), f32arg(b), f32arg(a))
}
func glViewport(x, y, w, h int32) { pglViewport.Call(uintptr(x), uintptr(y), uintptr(w), uintptr(h)) }
func glMatrixMode(m uint32)       { pglMatrixMode.Call(uintptr(m)) }
func glLoadMatrixf(m *[16]float32) {
	pglLoadMatrixf.Call(uintptr(unsafe.Pointer(&m[0])))
}
func glMultMatrixf(m *[16]float32) {
	pglMultMatrixf.Call(uintptr(unsafe.Pointer(&m[0])))
}
func glLoadIdentity()                   { pglLoadIdentity.Call() }
func glPushMatrix()                     { pglPushMatrix.Call() }
func glPopMatrix()                      { pglPopMatrix.Call() }
func glEnable(c uint32)                 { pglEnable.Call(uintptr(c)) }
func glDisable(c uint32)                { pglDisable.Call(uintptr(c)) }
func glDepthFunc(f uint32)              { pglDepthFunc.Call(uintptr(f)) }
func glDepthMask(b bool)                { pglDepthMask.Call(boolArg(b)) }
func glBlendFunc(s, d uint32)           { pglBlendFunc.Call(uintptr(s), uintptr(d)) }
func glAlphaFunc(f uint32, ref float32) { pglAlphaFunc.Call(uintptr(f), f32arg(ref)) }
func glEnableClientState(c uint32)      { pglEnableClientState.Call(uintptr(c)) }
func glDisableClientState(c uint32)     { pglDisableClientState.Call(uintptr(c)) }
func glVertexPointer(size int32, typ uint32, stride int32, off uintptr) {
	pglVertexPointer.Call(uintptr(size), uintptr(typ), uintptr(stride), off)
}
func glNormalPointer(typ uint32, stride int32, off uintptr) {
	pglNormalPointer.Call(uintptr(typ), uintptr(stride), off)
}
func glTexCoordPointer(size int32, typ uint32, stride int32, off uintptr) {
	pglTexCoordPointer.Call(uintptr(size), uintptr(typ), uintptr(stride), off)
}
func glDrawElements(mode uint32, count int32, typ uint32, off uintptr) {
	pglDrawElements.Call(uintptr(mode), uintptr(count), uintptr(typ), off)
}
func glGenTexture() uint32 {
	var t uint32
	pglGenTextures.Call(1, uintptr(unsafe.Pointer(&t)))
	return t
}
func glBindTexture(target, t uint32) { pglBindTexture.Call(uintptr(target), uintptr(t)) }
func glTexParameteri(target, p uint32, v int32) {
	pglTexParameteri.Call(uintptr(target), uintptr(p), uintptr(v))
}
func glDeleteTexture(t uint32) {
	pglDeleteTextures.Call(1, uintptr(unsafe.Pointer(&t)))
}
func glLightfv(light, p uint32, v *[4]float32) {
	pglLightfv.Call(uintptr(light), uintptr(p), uintptr(unsafe.Pointer(&v[0])))
}
func glLightModelfv(p uint32, v *[4]float32) {
	pglLightModelfv.Call(uintptr(p), uintptr(unsafe.Pointer(&v[0])))
}
func glLightModeli(p uint32, v int32) { pglLightModeli.Call(uintptr(p), uintptr(v)) }
func glColor4fv(c *[4]float32)        { pglColor4fv.Call(uintptr(unsafe.Pointer(&c[0]))) }
func glBegin(m uint32)                { pglBegin.Call(uintptr(m)) }
func glEnd()                          { pglEnd.Call() }
func glVertex3fv(v *[3]float32)       { pglVertex3fv.Call(uintptr(unsafe.Pointer(&v[0]))) }
func glNormal3fv(v *[3]float32)       { pglNormal3fv.Call(uintptr(unsafe.Pointer(&v[0]))) }
func glLineWidth(w float32)           { pglLineWidth.Call(f32arg(w)) }
func glColorMaterial(face, mode uint32) {
	pglColorMaterial.Call(uintptr(face), uintptr(mode))
}
func glShadeModel(m uint32)    { pglShadeModel.Call(uintptr(m)) }
func glFogi(p uint32, v int32) { pglFogi.Call(uintptr(p), uintptr(v)) }
func glFogfv(p uint32, v *[4]float32) {
	pglFogfv.Call(uintptr(p), uintptr(unsafe.Pointer(&v[0])))
}
func glPixelStorei(p uint32, v int32) { pglPixelStorei.Call(uintptr(p), uintptr(v)) }
func glGetError() uint32 {
	r, _, _ := pglGetError.Call()
	return uint32(r)
}
func glTexEnvi(t, p uint32, v int32)  { pglTexEnvi.Call(uintptr(t), uintptr(p), uintptr(v)) }
func glPolygonMode(face, mode uint32) { pglPolygonMode.Call(uintptr(face), uintptr(mode)) }
func glHint(t, m uint32)              { pglHint.Call(uintptr(t), uintptr(m)) }
func glCullFace(m uint32)             { pglCullFace.Call(uintptr(m)) }

func glGetString(name uint32) string {
	p, _, _ := pglGetString.Call(uintptr(name))
	if p == 0 {
		return ""
	}
	var b []byte
	for i := 0; i < 100000; i++ {
		c := *(*byte)(unsafe.Pointer(p + uintptr(i)))
		if c == 0 {
			break
		}
		b = append(b, c)
	}
	return string(b)
}

func boolArg(b bool) uintptr {
	if b {
		return 1
	}
	return 0
}

func glFog(start, end float32) {
	v := [4]float32{start}
	glFogfv(GL_FOG_START, &v)
	v = [4]float32{end}
	glFogfv(GL_FOG_END, &v)
}

func getProc(name string) uintptr {
	p, _, _ := pwglGetProcAddress.Call(uintptr(unsafe.Pointer(syscall.StringBytePtr(name))))
	if p == 0 || p == 1 || p == 2 || p == 3 || p == ^uintptr(0) {
		return 0
	}
	return p
}

// loadExtensions laedt die Funktionen jenseits von OpenGL 1.1.
func loadExtensions() error {
	pick := func(names ...string) uintptr {
		for _, n := range names {
			if p := getProc(n); p != 0 {
				return p
			}
		}
		return 0
	}
	glGenBuffersPtr = pick("glGenBuffers", "glGenBuffersARB")
	glBindBufferPtr = pick("glBindBuffer", "glBindBufferARB")
	glBufferDataPtr = pick("glBufferData", "glBufferDataARB")
	glDeleteBuffersPtr = pick("glDeleteBuffers", "glDeleteBuffersARB")
	glCompressedTexImage2DPtr = pick("glCompressedTexImage2D", "glCompressedTexImage2DARB")
	wglSwapIntervalPtr = pick("wglSwapIntervalEXT")
	hasCompressed = glCompressedTexImage2DPtr != 0
	if glGenBuffersPtr == 0 || glBindBufferPtr == 0 || glBufferDataPtr == 0 {
		return fmt.Errorf("die Grafikkarte bzw. der Treiber unterstuetzt keine Vertex-Buffer (OpenGL 1.5)")
	}
	return nil
}

func glGenBuffer() uint32 {
	var b uint32
	syscall.SyscallN(glGenBuffersPtr, 1, uintptr(unsafe.Pointer(&b)))
	return b
}
func glBindBuffer(target, b uint32) { syscall.SyscallN(glBindBufferPtr, uintptr(target), uintptr(b)) }
func glBufferData(target uint32, size int, data unsafe.Pointer) {
	syscall.SyscallN(glBufferDataPtr, uintptr(target), uintptr(size), uintptr(data), GL_STATIC_DRAW)
}
func glDeleteBuffer(b uint32) {
	if glDeleteBuffersPtr != 0 {
		syscall.SyscallN(glDeleteBuffersPtr, 1, uintptr(unsafe.Pointer(&b)))
	}
}
func glCompressedTexImage2D(target uint32, level int32, format uint32, w, h int32, data []byte) {
	syscall.SyscallN(glCompressedTexImage2DPtr, uintptr(target), uintptr(level), uintptr(format),
		uintptr(w), uintptr(h), 0, uintptr(len(data)), uintptr(unsafe.Pointer(&data[0])))
}
func glTexImage2D(target uint32, level int32, internal int32, w, h int32, format, typ uint32, data []byte) {
	pglTexImage2D.Call(uintptr(target), uintptr(level), uintptr(internal), uintptr(w), uintptr(h), 0,
		uintptr(format), uintptr(typ), uintptr(unsafe.Pointer(&data[0])))
}
func wglSwapInterval(n int) {
	if wglSwapIntervalPtr != 0 {
		syscall.SyscallN(wglSwapIntervalPtr, uintptr(n))
	}
}

func glColorPointer(size int32, typ uint32, stride int32, off uintptr) {
	pglColorPointer.Call(uintptr(size), uintptr(typ), uintptr(stride), off)
}

var pglColorPointer = glp("glColorPointer")

const (
	GL_COLOR_ARRAY = 0x8076
	GL_TEXTURE     = 0x1702
)
