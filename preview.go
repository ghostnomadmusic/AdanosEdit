//go:build windows

package main

// Kleine, sich drehende 3D-Vorschau des in der Ergebnisliste gewaehlten
// Objekts (oben links im 3D-Fenster).

import (
	"math"
	"strings"
	"time"

	"g3viewer/g3"
	"g3viewer/scene"
)

var pglScissor = glp("glScissor")

const GL_SCISSOR_TEST = 0x0C11

func glScissor(x, y, w, h int32) {
	pglScissor.Call(uintptr(x), uintptr(y), uintptr(w), uintptr(h))
}

// preview beschreibt das Vorschau-Objekt (ein oder mehrere Modelle).
type preview struct {
	parts []prevPart
	start float64 // Startzeit (fuer die Drehung)
}

type prevPart struct {
	key   string     // Modellschluessel
	world scene.Mat4 // Lage relativ zum Objekt (ohne Verschiebung des Objekts)
}

// setPreviewInstance zeigt das Modell einer Welt-Instanz in der Vorschau.
// Bei Figuren werden die Kinder (Koerper, Kopf ...) mit angezeigt.
func (a *app) setPreviewInstance(in *scene.Instance) {
	if in == nil {
		a.prev = nil
		return
	}
	p := &preview{start: a.secsNow()}
	base := scene.Vec3{in.World[12], in.World[13], in.World[14]}
	add := func(x *scene.Instance) {
		if x.ModelKey == "" || strings.HasPrefix(strings.ToLower(x.ModelKey), "g3_hero_skeleton") {
			return
		}
		w := x.World
		w[12], w[13], w[14] = w[12]-base[0], w[13]-base[1], w[14]-base[2]
		p.parts = append(p.parts, prevPart{key: x.ModelKey, world: w})
	}
	add(in)
	if in.Ent.Kind == g3.KindNPC && a.world != nil {
		if wf, err := a.editFile(in); err == nil {
			for _, c := range wf.Children(in.Ent.Index) {
				if ii, ok := a.ed.inst[wf.Path][c]; ok && int(ii) < len(a.world.Inst) {
					add(&a.world.Inst[ii])
				}
			}
		}
	}
	if len(p.parts) == 0 {
		a.prev = nil
		return
	}
	a.prev = p
}

// setPreviewParts zeigt eine Figur aus einzelnen Vorlagen (Koerper, Kopf ...).
func (a *app) setPreviewParts(ts ...*g3.Template) {
	p := &preview{start: a.secsNow()}
	if a.prev != nil {
		p.start = a.prev.start
	}
	for _, t := range ts {
		if t == nil || t.Mesh == "" {
			continue
		}
		e := &g3.Entity{Name: t.Name, Mesh: t.Mesh, Kind: t.Kind, NodeMin: t.LocalMin, NodeMax: t.LocalMax}
		e.World = [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
		in, _ := scene.MakeInstance(e, "")
		if in.ModelKey != "" {
			p.parts = append(p.parts, prevPart{key: in.ModelKey, world: in.World})
		}
	}
	if len(p.parts) == 0 {
		a.prev = nil
		return
	}
	a.prev = p
}

// setPreviewTemplate zeigt das Modell einer Vorlage in der Vorschau.
func (a *app) setPreviewTemplate(t *g3.Template) {
	a.prev = nil
	a.setPreviewParts(t)
}

// glMul multipliziert zwei Matrizen im OpenGL-Spaltenformat (a*b).
func glMul(a, b [16]float32) [16]float32 {
	var r [16]float32
	for c := 0; c < 4; c++ {
		for row := 0; row < 4; row++ {
			var s float32
			for k := 0; k < 4; k++ {
				s += a[k*4+row] * b[c*4+k]
			}
			r[c*4+row] = s
		}
	}
	return r
}

func glTranslate(x, y, z float32) [16]float32 {
	return [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, x, y, z, 1}
}

func glRotY(a float64) [16]float32 {
	c, s := float32(math.Cos(a)), float32(math.Sin(a))
	return [16]float32{c, 0, -s, 0, 0, 1, 0, 0, s, 0, c, 0, 0, 0, 0, 1}
}

func glRotX(a float64) [16]float32 {
	c, s := float32(math.Cos(a)), float32(math.Sin(a))
	return [16]float32{1, 0, 0, 0, 0, c, s, 0, 0, -s, c, 0, 0, 0, 0, 1}
}

// previewKeys liefert die zu ladenden Modellschluessel (fuer den Lader).
func (a *app) previewKeys() []string {
	if a.prev == nil {
		return nil
	}
	var k []string
	for _, p := range a.prev.parts {
		k = append(k, p.key)
	}
	return k
}

// drawPreview zeichnet die Vorschau in die linke obere Ecke.
func (a *app) drawPreview(secs float64) {
	p := a.prev
	if p == nil || a.ren == nil {
		return
	}
	type loaded struct {
		m *gpuModel
		w scene.Mat4
	}
	var ms []loaded
	for _, pp := range p.parts {
		i, ok := a.ren.modelIndex[pp.key]
		if !ok {
			continue
		}
		m := a.ren.models[i]
		if m == nil || m.failed || m.vbo == 0 {
			continue
		}
		m.lastUsed = secs
		ms = append(ms, loaded{m, pp.world})
	}
	if len(ms) == 0 {
		return
	}
	size := int32(a.scale(230))
	if size > int32(a.height)/2 {
		size = int32(a.height) / 2
	}
	margin := int32(a.scale(8))
	x0, y0 := margin, int32(a.height)-size-margin
	// Hintergrund
	glEnable(GL_SCISSOR_TEST)
	glScissor(x0-2, y0-2, size+4, size+4)
	glClearColor(0.85, 0.75, 0.3, 1)
	glClear(GL_COLOR_BUFFER_BIT)
	glScissor(x0, y0, size, size)
	glClearColor(0.16, 0.17, 0.19, 1)
	glClear(GL_COLOR_BUFFER_BIT | GL_DEPTH_BUFFER_BIT)
	glViewport(x0, y0, size, size)

	// Begrenzung aller Teile
	bmn := scene.Vec3{math.MaxFloat32, math.MaxFloat32, math.MaxFloat32}
	bmx := scene.Vec3{-math.MaxFloat32, -math.MaxFloat32, -math.MaxFloat32}
	for _, l := range ms {
		mn, mx := l.m.min, l.m.max
		if !(mn[0] <= mx[0] && mn[1] <= mx[1] && mn[2] <= mx[2]) {
			mn, mx = scene.Vec3{-100, -100, -100}, scene.Vec3{100, 100, 100}
		}
		for c := 0; c < 8; c++ {
			q := mn
			if c&1 != 0 {
				q[0] = mx[0]
			}
			if c&2 != 0 {
				q[1] = mx[1]
			}
			if c&4 != 0 {
				q[2] = mx[2]
			}
			t := l.w.TransformPoint(q)
			for k := 0; k < 3; k++ {
				bmn[k] = min(bmn[k], t[k])
				bmx[k] = max(bmx[k], t[k])
			}
		}
	}
	ctr := bmn.Add(bmx).Scale(0.5)
	rad := bmx.Sub(bmn).Len() * 0.5
	if rad < 1 {
		rad = 1
	}
	const fov = 35.0
	dist := rad / float32(math.Sin(fov/2*math.Pi/180)) * 1.05
	proj := [16]float32(scene.Perspective(fov, 1, dist*0.05, dist+rad*2))
	glMatrixMode(GL_PROJECTION)
	glLoadMatrixf(&proj)
	glMatrixMode(GL_MODELVIEW)
	// Licht im Kameraraum (von vorne oben)
	glLoadIdentity()
	light := [4]float32{0.3, 0.6, 0.75, 0}
	glLightfv(GL_LIGHT0, GL_POSITION, &light)
	ang := (secs - p.start) * 0.8
	// Kamera blickt entlang -Z (OpenGL); Spiegelung der Z-Achse wie in der Weltansicht
	flip := [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, -1, 0, 0, 0, 0, 1}
	mv := glTranslate(0, 0, -dist)
	mv = glMul(mv, glRotX(0.35))
	mv = glMul(mv, flip)
	mv = glMul(mv, glRotY(ang))
	mv = glMul(mv, glTranslate(-ctr[0], -ctr[1], -ctr[2]))
	glDisable(GL_FOG)
	glEnable(GL_LIGHTING)
	glEnable(GL_LIGHT0)
	glEnable(GL_DEPTH_TEST)
	glDepthFunc(GL_LEQUAL)
	glEnableClientState(GL_VERTEX_ARRAY)
	glEnableClientState(GL_NORMAL_ARRAY)
	glEnableClientState(GL_TEXTURE_COORD_ARRAY)
	glDisableClientState(GL_COLOR_ARRAY)
	glDepthMask(true)
	glPolygonMode(GL_FRONT_AND_BACK, GL_FILL)
	for pass := 0; pass < 2; pass++ {
		if pass == 1 {
			glEnable(GL_BLEND)
			glDepthMask(false)
		}
		for _, l := range ms {
			m := l.m
			wm := glMul(mv, [16]float32(l.w))
			glLoadMatrixf(&wm)
			bindModel(m)
			for si := range m.subs {
				s := &m.subs[si]
				if isBlended(s.blend) != (pass == 1) {
					continue
				}
				textured := a.ren.bindTex(s.tex)
				setTexScale(s.scale)
				if pass == 0 && s.blend == g3.BlendMasked && textured {
					glEnable(GL_ALPHA_TEST)
					glAlphaFunc(GL_GREATER, s.alpha)
				} else {
					glDisable(GL_ALPHA_TEST)
				}
				if pass == 1 {
					setBlend(s.blend)
				}
				glDrawElements(GL_TRIANGLES, s.count, GL_UNSIGNED_INT, uintptr(s.first))
			}
		}
	}
	glDepthMask(true)
	glDisable(GL_BLEND)
	glDisable(GL_ALPHA_TEST)
	glBindBuffer(GL_ARRAY_BUFFER, 0)
	glBindBuffer(GL_ELEMENT_ARRAY_BUFFER, 0)
	glDisable(GL_SCISSOR_TEST)
	glViewport(0, 0, int32(a.width), int32(a.height))
}

func (a *app) secsNow() float64 { return time.Since(a.startTime).Seconds() }
