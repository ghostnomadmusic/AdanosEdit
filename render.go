//go:build windows

package main

import (
	"math"
	"sort"
	"unsafe"

	"g3viewer/g3"
	"g3viewer/scene"
)

var pglReadPixels = glp("glReadPixels")

type gpuSub struct {
	first   int32 // Byte-Offset in den Indexpuffer
	count   int32
	texKey  string
	tex     *gpuTex
	scale   [2]float32
	tex2Key string
	tex2    *gpuTex
	scale2  [2]float32
	blend   int
	alpha   float32
	special int // scene.SpecialWater / SpecialBarrier
}

type gpuModel struct {
	key      string
	vbo, ibo uint32
	subs     []gpuSub
	failed   bool
	err      string
	bytes    int
	lastUsed float64
	min, max scene.Vec3
}

type gpuTex struct {
	id     uint32
	refs   int
	bytes  int
	failed bool
}

type renderer struct {
	models     []*gpuModel
	modelIndex map[string]int32
	textures   map[string]*gpuTex
	gpuBytes   int
	cube       struct{ vbo, ibo uint32 }
	frame      uint32

	// pro Frame gesammelte Listen
	opaque  []int32
	blended []int32
	markers []int32

	showTextures bool
	wireframe    bool
	failedCount  int
}

func newRenderer() *renderer {
	r := &renderer{modelIndex: map[string]int32{}, textures: map[string]*gpuTex{}, showTextures: true}
	r.initCube()
	return r
}

func (r *renderer) initCube() {
	// Wuerfel -1..1 mit Normalen, 24 Vertices
	faces := [6][3]float32{{1, 0, 0}, {-1, 0, 0}, {0, 1, 0}, {0, -1, 0}, {0, 0, 1}, {0, 0, -1}}
	var v []float32
	var idx []uint32
	for fi, n := range faces {
		// zwei Achsen senkrecht zur Normalen
		var a, b [3]float32
		switch {
		case n[0] != 0:
			a, b = [3]float32{0, 1, 0}, [3]float32{0, 0, 1}
		case n[1] != 0:
			a, b = [3]float32{1, 0, 0}, [3]float32{0, 0, 1}
		default:
			a, b = [3]float32{1, 0, 0}, [3]float32{0, 1, 0}
		}
		for _, s := range [4][2]float32{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}} {
			p := [3]float32{n[0] + a[0]*s[0] + b[0]*s[1], n[1] + a[1]*s[0] + b[1]*s[1], n[2] + a[2]*s[0] + b[2]*s[1]}
			v = append(v, p[0], p[1], p[2], n[0], n[1], n[2], 0, 0, whiteF)
		}
		base := uint32(fi * 4)
		idx = append(idx, base, base+1, base+2, base, base+2, base+3)
	}
	r.cube.vbo = glGenBuffer()
	glBindBuffer(GL_ARRAY_BUFFER, r.cube.vbo)
	glBufferData(GL_ARRAY_BUFFER, len(v)*4, unsafe.Pointer(&v[0]))
	r.cube.ibo = glGenBuffer()
	glBindBuffer(GL_ELEMENT_ARRAY_BUFFER, r.cube.ibo)
	glBufferData(GL_ELEMENT_ARRAY_BUFFER, len(idx)*4, unsafe.Pointer(&idx[0]))
}

// uploadTexture laedt eine Textur auf die Grafikkarte.
func (r *renderer) uploadTexture(t *scene.CPUTex) {
	gt := r.textures[t.Key]
	if gt == nil {
		gt = &gpuTex{}
		r.textures[t.Key] = gt
	}
	if gt.id != 0 {
		return
	}
	if t.Img == nil || len(t.Img.Levels) == 0 {
		gt.failed = true
		return
	}
	img := t.Img
	if img.Format != g3.TexBGRA8 && !hasCompressed {
		gt.failed = true
		return
	}
	id := glGenTexture()
	glBindTexture(GL_TEXTURE_2D, id)
	glPixelStorei(GL_UNPACK_ALIGNMENT, 1)
	w, h := img.Width, img.Height
	levels := 0
	for i, lv := range img.Levels {
		if len(lv) == 0 {
			break
		}
		switch img.Format {
		case g3.TexDXT1:
			glCompressedTexImage2D(GL_TEXTURE_2D, int32(i), GL_COMPRESSED_RGBA_S3TC_DXT1_EXT, int32(w), int32(h), lv)
		case g3.TexDXT3:
			glCompressedTexImage2D(GL_TEXTURE_2D, int32(i), GL_COMPRESSED_RGBA_S3TC_DXT3_EXT, int32(w), int32(h), lv)
		case g3.TexDXT5:
			glCompressedTexImage2D(GL_TEXTURE_2D, int32(i), GL_COMPRESSED_RGBA_S3TC_DXT5_EXT, int32(w), int32(h), lv)
		case g3.TexBGRA8:
			glTexImage2D(GL_TEXTURE_2D, int32(i), GL_RGBA, int32(w), int32(h), GL_BGRA, GL_UNSIGNED_BYTE, lv)
		}
		gt.bytes += len(lv)
		levels++
		w, h = max(w/2, 1), max(h/2, 1)
	}
	if levels > 1 {
		glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER, GL_LINEAR_MIPMAP_LINEAR)
	} else {
		glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MIN_FILTER, GL_LINEAR)
	}
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAX_LEVEL, int32(levels-1))
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_MAG_FILTER, GL_LINEAR)
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_S, GL_REPEAT)
	glTexParameteri(GL_TEXTURE_2D, GL_TEXTURE_WRAP_T, GL_REPEAT)
	gt.id = id
	r.gpuBytes += gt.bytes
}

// uploadModel laedt ein Modell auf die Grafikkarte.
func (r *renderer) uploadModel(m *scene.CPUModel, now float64) {
	if _, ok := r.modelIndex[m.Key]; ok {
		return
	}
	gm := &gpuModel{key: m.Key, lastUsed: now, min: m.Min, max: m.Max}
	if m.Err != "" || len(m.Subs) == 0 {
		gm.failed = true
		gm.err = m.Err
		r.failedCount++
	} else {
		var verts []float32
		var idx []uint32
		for _, s := range m.Subs {
			base := uint32(len(verts) / scene.VertexFloats)
			first := int32(len(idx) * 4)
			verts = append(verts, s.Vertices...)
			for _, i := range s.Indices {
				idx = append(idx, i+base)
			}
			alpha := float32(s.MaskRef) / 255
			if alpha < 0.05 {
				alpha = 0.05
			}
			if alpha > 0.95 {
				alpha = 0.95
			}
			gs := gpuSub{first: first, count: int32(len(s.Indices)), texKey: s.Tex, blend: s.Blend, alpha: alpha,
				scale: s.Scale, tex2Key: s.Tex2, scale2: s.Scale2, special: s.Special}
			gs.tex = r.texRef(s.Tex)
			gs.tex2 = r.texRef(s.Tex2)
			gm.subs = append(gm.subs, gs)
		}
		if len(verts) == 0 || len(idx) == 0 {
			gm.failed = true
		} else {
			gm.vbo = glGenBuffer()
			glBindBuffer(GL_ARRAY_BUFFER, gm.vbo)
			glBufferData(GL_ARRAY_BUFFER, len(verts)*4, unsafe.Pointer(&verts[0]))
			gm.ibo = glGenBuffer()
			glBindBuffer(GL_ELEMENT_ARRAY_BUFFER, gm.ibo)
			glBufferData(GL_ELEMENT_ARRAY_BUFFER, len(idx)*4, unsafe.Pointer(&idx[0]))
			gm.bytes = len(verts)*4 + len(idx)*4
			r.gpuBytes += gm.bytes
		}
	}
	r.modelIndex[m.Key] = int32(len(r.models))
	r.models = append(r.models, gm)
}

func (r *renderer) texRef(key string) *gpuTex {
	if key == "" {
		return nil
	}
	gt := r.textures[key]
	if gt == nil {
		gt = &gpuTex{}
		r.textures[key] = gt
	}
	gt.refs++
	return gt
}

func (r *renderer) texUnref(key string, t *gpuTex) (freed bool) {
	if t == nil {
		return false
	}
	t.refs--
	if t.refs <= 0 {
		if t.id != 0 {
			glDeleteTexture(t.id)
			r.gpuBytes -= t.bytes
		}
		delete(r.textures, key)
		return true
	}
	return false
}

// unloadModel gibt Grafikspeicher frei. Liefert Texturen, deren Referenzen auf 0 fielen.
func (r *renderer) unloadModel(i int32) (freedTex []string) {
	gm := r.models[i]
	if gm == nil {
		return nil
	}
	if gm.vbo != 0 {
		glDeleteBuffer(gm.vbo)
		glDeleteBuffer(gm.ibo)
	}
	r.gpuBytes -= gm.bytes
	for _, s := range gm.subs {
		if r.texUnref(s.texKey, s.tex) {
			freedTex = append(freedTex, s.texKey)
		}
		if r.texUnref(s.tex2Key, s.tex2) {
			freedTex = append(freedTex, s.tex2Key)
		}
	}
	if gm.failed {
		r.failedCount--
	}
	delete(r.modelIndex, gm.key)
	r.models[i] = nil
	return freedTex
}

// model liefert das geladene Modell einer Instanz (oder nil).
func (r *renderer) model(in *scene.Instance) *gpuModel {
	if in.Model >= 0 {
		if int(in.Model) < len(r.models) && r.models[in.Model] != nil && r.models[in.Model].key == in.ModelKey {
			return r.models[in.Model]
		}
		in.Model = -1
	}
	if i, ok := r.modelIndex[in.ModelKey]; ok {
		in.Model = i
		return r.models[i]
	}
	return nil
}

type viewState struct {
	cam        scene.Camera
	proj, view scene.Mat4
	width      int
	height     int
	viewDist   float32
	show       map[scene.MarkerKind]bool
	now        float64
	water      bool // Wasser anzeigen
	barrier    bool // magische Barriere anzeigen
}

var skyColor = [4]float32{0.62, 0.72, 0.82, 1}

func markerColor(k scene.MarkerKind) [4]float32 {
	switch k {
	case scene.MarkNPC:
		return [4]float32{0.9, 0.2, 0.15, 1}
	case scene.MarkItem:
		return [4]float32{0.95, 0.8, 0.1, 1}
	case scene.MarkTree:
		return [4]float32{0.2, 0.6, 0.2, 1}
	case scene.MarkLight:
		return [4]float32{1, 0.95, 0.6, 1}
	case scene.MarkAnimated:
		return [4]float32{0.9, 0.5, 0.2, 1}
	case scene.MarkZone:
		return [4]float32{0.15, 0.8, 0.55, 1} // gruen: Zone
	case scene.MarkArena:
		return [4]float32{0.85, 0.3, 0.9, 1} // violett: Arena
	case scene.MarkPoint:
		return [4]float32{0.35, 0.65, 1, 1} // hellblau: Routinepunkt
	}
	return [4]float32{0.6, 0.6, 0.65, 1} // grau: sonstige Hilfsobjekte
}

// markerBox liefert die Groesse des Platzhalters. NPCs bekommen einen Kasten
// in Menschengroesse (ca. 70 x 180 x 70 cm) ab den Fuessen; ihr Suchbereich
// (Min/Max) bleibt groesser, damit die geladene Figur hineinpasst.
func markerBox(in *scene.Instance) (scene.Vec3, scene.Vec3) {
	if in.Marker == scene.MarkNPC && in.Ent != nil {
		p := scene.Vec3(in.Ent.Pos())
		return p.Add(scene.Vec3{-35, 0, -35}), p.Add(scene.Vec3{35, 180, 35})
	}
	return in.Min, in.Max
}

// collect bestimmt die sichtbaren Instanzen.
func (r *renderer) collect(w *scene.World, vs *viewState) {
	r.frame++
	r.opaque = r.opaque[:0]
	r.blended = r.blended[:0]
	r.markers = r.markers[:0]
	if w == nil {
		return
	}
	fr := scene.FrustumFrom(vs.proj.Mul(vs.view))
	pxScale := float32(vs.height) / 2 / float32(math.Tan(float64(vs.cam.FovY)*math.Pi/360))
	cam := vs.cam.Pos
	w.Query(cam, vs.viewDist, func(i int32) {
		in := &w.Inst[i]
		if in.Frame == r.frame {
			return
		}
		in.Frame = r.frame
		if in.Hidden {
			return
		}
		d := in.Center.Sub(cam).Len() - in.Radius
		if d > vs.viewDist {
			return
		}
		if d > 0 && in.Radius/d*pxScale < 1.5 {
			return
		}
		if !fr.SphereVisible(in.Center, in.Radius) {
			return
		}
		if in.Marker != scene.MarkNone && !vs.show[in.Marker] {
			return
		}
		if in.ModelKey != "" {
			m := r.model(in)
			if m == nil || m.failed || len(m.subs) == 0 {
				// Figur noch nicht geladen oder ohne Geometrie: Platzhalter zeigen
				if in.Marker != scene.MarkNone && in.Marker != scene.MarkTree && (m == nil || m.failed) {
					r.markers = append(r.markers, i)
				}
				return
			}
			m.lastUsed = vs.now
			fixOrigin(in, m)
			hasBlend, hasOpaque := false, false
			for _, s := range m.subs {
				if isBlended(s.blend) {
					hasBlend = true
				} else {
					hasOpaque = true
				}
			}
			if hasOpaque {
				r.opaque = append(r.opaque, i)
			}
			if hasBlend {
				r.blended = append(r.blended, i)
			}
			return
		}
		if vs.show[in.Marker] {
			r.markers = append(r.markers, i)
		}
	})
	// transparente Objekte von hinten nach vorne
	sort.Slice(r.blended, func(a, b int) bool {
		da := w.Inst[r.blended[a]].Center.Sub(cam).Len()
		db := w.Inst[r.blended[b]].Center.Sub(cam).Len()
		return da > db
	})
}

func isBlended(b int) bool {
	switch b {
	case g3.BlendAlphaBlend, g3.BlendAlphaModulate, g3.BlendTranslucent, g3.BlendModulate, g3.BlendDarken, g3.BlendBrighten:
		return true
	}
	return false
}

func setBlend(b int) {
	switch b {
	case g3.BlendModulate, g3.BlendDarken:
		glBlendFunc(GL_DST_COLOR, GL_ZERO)
	case g3.BlendBrighten:
		glBlendFunc(GL_ONE, GL_ONE)
	default:
		glBlendFunc(GL_SRC_ALPHA, GL_ONE_MINUS_SRC_ALPHA)
	}
}

const vstride = scene.VertexFloats * 4

func bindModel(m *gpuModel) {
	glBindBuffer(GL_ARRAY_BUFFER, m.vbo)
	glVertexPointer(3, GL_FLOAT, vstride, 0)
	glNormalPointer(GL_FLOAT, vstride, 12)
	glTexCoordPointer(2, GL_FLOAT, vstride, 24)
	glColorPointer(4, GL_UNSIGNED_BYTE, vstride, 32)
	glBindBuffer(GL_ELEMENT_ARRAY_BUFFER, m.ibo)
}

var curTexScale [2]float32

// setTexScale setzt die Texturmatrix (UV-Skalierung aus dem Material).
func setTexScale(sc [2]float32) {
	if sc[0] == 0 || sc[1] == 0 {
		sc = [2]float32{1, 1}
	}
	if sc == curTexScale {
		return
	}
	curTexScale = sc
	m := [16]float32{sc[0], 0, 0, 0, 0, sc[1], 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}
	glMatrixMode(GL_TEXTURE)
	glLoadMatrixf(&m)
	glMatrixMode(GL_MODELVIEW)
}

func (r *renderer) setupFrame(vs *viewState, pick bool) {
	glViewport(0, 0, int32(vs.width), int32(vs.height))
	if pick {
		glClearColor(0, 0, 0, 1)
	} else {
		glClearColor(skyColor[0], skyColor[1], skyColor[2], 1)
	}
	glClear(GL_COLOR_BUFFER_BIT | GL_DEPTH_BUFFER_BIT)
	glMatrixMode(GL_PROJECTION)
	p := [16]float32(vs.proj)
	glLoadMatrixf(&p)
	glMatrixMode(GL_MODELVIEW)
	v := [16]float32(vs.view)
	glLoadMatrixf(&v)
	glEnable(GL_DEPTH_TEST)
	glDepthFunc(GL_LEQUAL)
	glDepthMask(true)
	glDisable(GL_CULL_FACE)
	glEnableClientState(GL_VERTEX_ARRAY)
	glEnableClientState(GL_NORMAL_ARRAY)
	glEnableClientState(GL_TEXTURE_COORD_ARRAY)
	glDisableClientState(GL_COLOR_ARRAY)
	glDisable(GL_BLEND)
	glDisable(GL_ALPHA_TEST)
	curTexScale = [2]float32{}
	setTexScale([2]float32{1, 1})
	if pick {
		glDisable(GL_LIGHTING)
		glDisable(GL_TEXTURE_2D)
		glDisable(GL_FOG)
		glDisable(GL_DITHER)
		glPolygonMode(GL_FRONT_AND_BACK, GL_FILL)
		return
	}
	glEnable(GL_DITHER)
	if r.wireframe {
		glPolygonMode(GL_FRONT_AND_BACK, GL_LINE)
	} else {
		glPolygonMode(GL_FRONT_AND_BACK, GL_FILL)
	}
	// Sonne (Richtung, w=0) im Weltraum; wird mit der Viewmatrix transformiert.
	sun := scene.Vec3{0.35, 0.85, 0.4}.Norm()
	pos := [4]float32{sun[0], sun[1], sun[2], 0}
	glLightfv(GL_LIGHT0, GL_POSITION, &pos)
	amb := [4]float32{0.45, 0.45, 0.47, 1}
	dif := [4]float32{0.65, 0.63, 0.6, 1}
	zero := [4]float32{0, 0, 0, 1}
	glLightfv(GL_LIGHT0, GL_AMBIENT, &zero)
	glLightfv(GL_LIGHT0, GL_DIFFUSE, &dif)
	glLightModelfv(GL_LIGHT_MODEL_AMBIENT, &amb)
	glLightModeli(GL_LIGHT_MODEL_TWO_SIDE, 1)
	glEnable(GL_LIGHTING)
	glEnable(GL_LIGHT0)
	glEnable(GL_NORMALIZE)
	glEnable(GL_COLOR_MATERIAL)
	glColorMaterial(GL_FRONT_AND_BACK, GL_AMBIENT_AND_DIFFUSE)
	glShadeModel(GL_SMOOTH)
	glTexEnvi(GL_TEXTURE_ENV, GL_TEXTURE_ENV_MODE, GL_MODULATE)
	glEnable(GL_FOG)
	glFogi(GL_FOG_MODE, GL_LINEAR_FOG)
	fc := skyColor
	glFogfv(GL_FOG_COLOR, &fc)
	glFog(vs.viewDist*0.55, vs.viewDist)
}

const GL_DITHER = 0x0BD0

var whiteF = math.Float32frombits(0xFFFFFFFF)

var white = [4]float32{1, 1, 1, 1}
var grey = [4]float32{0.72, 0.72, 0.7, 1}

func (r *renderer) bindTex(t *gpuTex) bool {
	if !r.showTextures || t == nil || t.id == 0 {
		glDisable(GL_TEXTURE_2D)
		glColor4fv(&grey)
		return false
	}
	glEnable(GL_TEXTURE_2D)
	glBindTexture(GL_TEXTURE_2D, t.id)
	glColor4fv(&white)
	return true
}

// draw zeichnet die gesammelten Listen. pickColors != nil: Auswahl-Pass.
func (r *renderer) draw(w *scene.World, vs *viewState, pick bool, pickList *[]int32) {
	r.setupFrame(vs, pick)
	if w == nil {
		return
	}
	pickID := func(i int32) {
		*pickList = append(*pickList, i)
		n := len(*pickList)
		c := [4]float32{float32(n&255) / 255, float32(n>>8&255) / 255, float32(n>>16&255) / 255, 1}
		glColor4fv(&c)
	}
	// undurchsichtige und maskierte Teile
	for _, i := range r.opaque {
		in := &w.Inst[i]
		m := r.model(in)
		if m == nil {
			continue
		}
		wm := [16]float32(in.World)
		glPushMatrix()
		glMultMatrixf(&wm)
		bindModel(m)
		if pick {
			pickID(i)
		}
		for si := range m.subs {
			s := &m.subs[si]
			if isBlended(s.blend) {
				continue
			}
			if !pick {
				textured := r.bindTex(s.tex)
				setTexScale(s.scale)
				if s.blend == g3.BlendMasked && textured {
					glEnable(GL_ALPHA_TEST)
					glAlphaFunc(GL_GREATER, s.alpha)
				} else {
					glDisable(GL_ALPHA_TEST)
				}
			}
			glDrawElements(GL_TRIANGLES, s.count, GL_UNSIGNED_INT, uintptr(s.first))
			if !pick && s.tex2 != nil && s.tex2.id != 0 && r.showTextures {
				// zweite Bodentextur ueber Vertex-Alpha einblenden
				r.bindTex(s.tex2)
				setTexScale(s.scale2)
				glEnableClientState(GL_COLOR_ARRAY)
				glEnable(GL_BLEND)
				glBlendFunc(GL_SRC_ALPHA, GL_ONE_MINUS_SRC_ALPHA)
				glDepthMask(false)
				glDrawElements(GL_TRIANGLES, s.count, GL_UNSIGNED_INT, uintptr(s.first))
				glDepthMask(true)
				glDisable(GL_BLEND)
				glDisableClientState(GL_COLOR_ARRAY)
			}
		}
		glPopMatrix()
	}
	glDisable(GL_ALPHA_TEST)
	// Platzhalter
	var outlines []int32
	if len(r.markers) > 0 {
		glDisable(GL_TEXTURE_2D)
		glBindBuffer(GL_ARRAY_BUFFER, r.cube.vbo)
		glVertexPointer(3, GL_FLOAT, vstride, 0)
		glNormalPointer(GL_FLOAT, vstride, 12)
		glTexCoordPointer(2, GL_FLOAT, vstride, 24)
		glColorPointer(4, GL_UNSIGNED_BYTE, vstride, 32)
		glBindBuffer(GL_ELEMENT_ARRAY_BUFFER, r.cube.ibo)
		for _, i := range r.markers {
			in := &w.Inst[i]
			mn, mx := markerBox(in)
			h := mx.Sub(mn).Scale(0.5)
			c0 := mn.Add(mx).Scale(0.5)
			m := [16]float32{h[0], 0, 0, 0, 0, h[1], 0, 0, 0, 0, h[2], 0, c0[0], c0[1], c0[2], 1}
			if in.Marker == scene.MarkNPC && !pick {
				// NPC ohne zeichenbare Figur: nur als Umriss in Menschengroesse
				outlines = append(outlines, i)
				continue
			}
			if pick {
				pickID(i)
			} else {
				c := markerColor(in.Marker)
				glColor4fv(&c)
			}
			glPushMatrix()
			glMultMatrixf(&m)
			glDrawElements(GL_TRIANGLES, 36, GL_UNSIGNED_INT, 0)
			glPopMatrix()
		}
	}
	if len(outlines) > 0 {
		glBindBuffer(GL_ARRAY_BUFFER, 0)
		glBindBuffer(GL_ELEMENT_ARRAY_BUFFER, 0)
		glDisableClientState(GL_VERTEX_ARRAY)
		glDisableClientState(GL_NORMAL_ARRAY)
		glDisableClientState(GL_TEXTURE_COORD_ARRAY)
		glDisableClientState(GL_COLOR_ARRAY)
		glDisable(GL_LIGHTING)
		glDisable(GL_TEXTURE_2D)
		glLineWidth(2)
		for _, i := range outlines {
			mn, mx := markerBox(&w.Inst[i])
			c := markerColor(scene.MarkNPC)
			glColor4fv(&c)
			boxEdges(mn, mx)
		}
		glLineWidth(1)
		glEnable(GL_LIGHTING)
		glEnableClientState(GL_VERTEX_ARRAY)
		glEnableClientState(GL_NORMAL_ARRAY)
		glEnableClientState(GL_TEXTURE_COORD_ARRAY)
	}
	// transparente Teile
	if len(r.blended) > 0 {
		if !pick {
			glEnable(GL_BLEND)
			glDepthMask(false)
		}
		for _, i := range r.blended {
			in := &w.Inst[i]
			m := r.model(in)
			if m == nil {
				continue
			}
			wm := [16]float32(in.World)
			glPushMatrix()
			glMultMatrixf(&wm)
			bindModel(m)
			if pick {
				pickID(i)
			}
			for si := range m.subs {
				s := &m.subs[si]
				if !isBlended(s.blend) {
					continue
				}
				if s.special != scene.SpecialNone {
					// Wasser und Barriere: einfache halbtransparente Flaeche,
					// nicht anklickbar
					if pick || s.special == scene.SpecialWater && !vs.water || s.special == scene.SpecialBarrier && !vs.barrier {
						continue
					}
					glDisable(GL_TEXTURE_2D)
					glDisable(GL_LIGHTING)
					c := [4]float32{0.16, 0.36, 0.46, 0.6}
					if s.special == scene.SpecialBarrier {
						c = [4]float32{0.35, 0.5, 1, 0.32}
					}
					glColor4fv(&c)
					glBlendFunc(GL_SRC_ALPHA, GL_ONE_MINUS_SRC_ALPHA)
					glDrawElements(GL_TRIANGLES, s.count, GL_UNSIGNED_INT, uintptr(s.first))
					glEnable(GL_LIGHTING)
					continue
				}
				if !pick {
					r.bindTex(s.tex)
					setTexScale(s.scale)
					setBlend(s.blend)
				}
				glDrawElements(GL_TRIANGLES, s.count, GL_UNSIGNED_INT, uintptr(s.first))
			}
			glPopMatrix()
		}
		glDepthMask(true)
		glDisable(GL_BLEND)
	}
	glBindBuffer(GL_ARRAY_BUFFER, 0)
	glBindBuffer(GL_ELEMENT_ARRAY_BUFFER, 0)
}

// drawSelection zeichnet die Box des ausgewaehlten Objekts.
func drawSelection(in *scene.Instance) {
	if in == nil {
		return
	}
	glDisable(GL_LIGHTING)
	glDisable(GL_TEXTURE_2D)
	glDisable(GL_FOG)
	glDisable(GL_DEPTH_TEST)
	glPolygonMode(GL_FRONT_AND_BACK, GL_FILL)
	glDisableClientState(GL_VERTEX_ARRAY)
	glDisableClientState(GL_NORMAL_ARRAY)
	glDisableClientState(GL_TEXTURE_COORD_ARRAY)
	glLineWidth(2)
	c := [4]float32{1, 0.85, 0.1, 1}
	glColor4fv(&c)
	mn, mx := in.Min, in.Max
	if in.Marker == scene.MarkNPC {
		mn, mx = markerBox(in)
	}
	boxEdges(mn, mx)
	glEnable(GL_DEPTH_TEST)
	glLineWidth(1)
}

// readPick liest die Objekt-ID unter dem Mauszeiger.
func readPick(x, y, h int, list []int32) int32 {
	var px [4]byte
	pglReadPixels.Call(uintptr(x), uintptr(h-1-y), 1, 1, GL_RGBA, GL_UNSIGNED_BYTE, uintptr(unsafe.Pointer(&px[0])))
	n := int(px[0]) | int(px[1])<<8 | int(px[2])<<16
	if n <= 0 || n > len(list) {
		return -1
	}
	return list[n-1]
}

// readWorldAt liest die Tiefe am Bildpunkt (x, y von oben) und rechnet sie in
// einen Weltpunkt um.
func readWorldAt(vs *viewState, x, y int) (scene.Vec3, bool) {
	var depth float32
	pglReadPixels.Call(uintptr(x), uintptr(vs.height-1-y), 1, 1, 0x1902, GL_FLOAT, uintptr(unsafe.Pointer(&depth)))
	if depth >= 0.99999 || depth <= 0 {
		return scene.Vec3{}, false
	}
	inv, ok := vs.proj.Mul(vs.view).Inverse()
	if !ok {
		return scene.Vec3{}, false
	}
	nx := (float32(x)+0.5)/float32(vs.width)*2 - 1
	ny := 1 - (float32(y)+0.5)/float32(vs.height)*2
	nz := depth*2 - 1
	px := inv[0]*nx + inv[4]*ny + inv[8]*nz + inv[12]
	py := inv[1]*nx + inv[5]*ny + inv[9]*nz + inv[13]
	pz := inv[2]*nx + inv[6]*ny + inv[10]*nz + inv[14]
	pw := inv[3]*nx + inv[7]*ny + inv[11]*nz + inv[15]
	if pw == 0 {
		return scene.Vec3{}, false
	}
	return scene.Vec3{px / pw, py / pw, pz / pw}, true
}

// readCenter liest die Tiefe in der Bildmitte und rechnet sie in einen Weltpunkt um.
func readCenter(vs *viewState) (scene.Vec3, bool) {
	var depth float32
	pglReadPixels.Call(uintptr(vs.width/2), uintptr(vs.height/2), 1, 1, 0x1902, GL_FLOAT, uintptr(unsafe.Pointer(&depth)))
	if depth >= 0.99999 || depth <= 0 {
		return scene.Vec3{}, false
	}
	inv, ok := vs.proj.Mul(vs.view).Inverse()
	if !ok {
		return scene.Vec3{}, false
	}
	z := depth*2 - 1
	x, y, w := inv[8]*z+inv[12], inv[9]*z+inv[13], inv[11]*z+inv[15]
	zz := inv[10]*z + inv[14]
	if w == 0 {
		return scene.Vec3{}, false
	}
	return scene.Vec3{x / w, y / w, zz / w}, true
}

// boxEdges zeichnet die zwoelf Kanten eines achsenparallelen Kastens.
func boxEdges(mn, mx scene.Vec3) {
	corner := func(i int) [3]float32 {
		p := mn
		if i&1 != 0 {
			p[0] = mx[0]
		}
		if i&2 != 0 {
			p[1] = mx[1]
		}
		if i&4 != 0 {
			p[2] = mx[2]
		}
		return [3]float32(p)
	}
	edges := [12][2]int{{0, 1}, {2, 3}, {4, 5}, {6, 7}, {0, 2}, {1, 3}, {4, 6}, {5, 7}, {0, 4}, {1, 5}, {2, 6}, {3, 7}}
	glBegin(GL_LINES)
	for _, e := range edges {
		a, b := corner(e[0]), corner(e[1])
		glVertex3fv(&a)
		glVertex3fv(&b)
	}
	glEnd()
}

// fixOrigin: Manche Objekte (z. B. die Barriere ueber Vengard) haben in den
// Weltdaten eine Matrix ohne Verschiebung, ihr Weltbereich liegt aber an der
// richtigen Stelle. Dann wird das Modell in diesen Bereich gesetzt.
func fixOrigin(in *scene.Instance, m *gpuModel) {
	w := &in.World
	if w[12] != 0 || w[13] != 0 || w[14] != 0 || in.Center.Len() < 1000 {
		return
	}
	mc := m.min.Add(m.max).Scale(0.5)
	// Modellmitte mit der Drehung/Skalierung der Matrix in die Welt bringen
	wc := scene.Vec3{w[0]*mc[0] + w[4]*mc[1] + w[8]*mc[2], w[1]*mc[0] + w[5]*mc[1] + w[9]*mc[2], w[2]*mc[0] + w[6]*mc[1] + w[10]*mc[2]}
	t := in.Center.Sub(wc)
	w[12], w[13], w[14] = t[0], t[1], t[2]
}

// drawZoneOutline zeichnet den Umriss einer Zone als Prisma (unten, oben, Kanten).
func drawZoneOutline(loop []scene.Vec3, top, bottom float32, c [4]float32) {
	glDisable(GL_LIGHTING)
	glDisable(GL_TEXTURE_2D)
	glDisable(GL_FOG)
	glDisableClientState(GL_VERTEX_ARRAY)
	glDisableClientState(GL_NORMAL_ARRAY)
	glDisableClientState(GL_TEXTURE_COORD_ARRAY)
	glLineWidth(3)
	glColor4fv(&c)
	ring := func(dy float32) {
		glBegin(GL_LINES)
		for i := range loop {
			p, q := loop[i], loop[(i+1)%len(loop)]
			a := [3]float32{p[0], p[1] + dy, p[2]}
			b := [3]float32{q[0], q[1] + dy, q[2]}
			glVertex3fv(&a)
			glVertex3fv(&b)
		}
		glEnd()
	}
	ring(-bottom)
	ring(top)
	glBegin(GL_LINES)
	for _, p := range loop {
		a := [3]float32{p[0], p[1] - bottom, p[2]}
		b := [3]float32{p[0], p[1] + top, p[2]}
		glVertex3fv(&a)
		glVertex3fv(&b)
	}
	glEnd()
	glLineWidth(1)
}
