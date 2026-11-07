package scene

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"

	"g3viewer/g3"
)

// VertexFloats: Position(3), Normale(3), UV(2), Farbe (RGBA als 4 Byte)
const VertexFloats = 9

var whiteRGBA = math.Float32frombits(0xFFFFFFFF)

// CPUSub ist ein fertig aufbereitetes Teilmesh zum Hochladen.
type CPUSub struct {
	Vertices []float32 // VertexFloats pro Vertex
	Indices  []uint32
	Tex      string     // Texturschluessel ("" = keine)
	Scale    [2]float32 // UV-Skalierung (0 = keine)
	Tex2     string     // zweite, ueber Vertex-Alpha eingeblendete Textur
	Scale2   [2]float32
	Blend    int
	MaskRef  int
	Special  int // SpecialWater, SpecialBarrier: im Spiel nur per Shader sichtbar
}

// Sonderflaechen, die das Spiel mit eigenen Shadern zeichnet
const (
	SpecialNone = iota
	SpecialWater
	SpecialBarrier
)

// meshSpecial erkennt Wasser- und Barriere-Modelle am Namen.
func meshSpecial(mesh string) int {
	l := strings.ToLower(mesh)
	switch {
	case strings.Contains(l, "magic_barrier"):
		return SpecialBarrier
	case strings.Contains(l, "oceanmesh"), strings.Contains(l, "rivermesh"), strings.Contains(l, "lakemesh"), strings.Contains(l, "_water_"):
		return SpecialWater
	}
	return SpecialNone
}

// materialSpecial erkennt Wasser- und Barriere-Materialien am Namen.
func materialSpecial(mat string) int {
	l := strings.ToLower(mat)
	switch {
	case strings.Contains(l, "magic_barrier"):
		return SpecialBarrier
	case strings.Contains(l, "water_ocean"), strings.Contains(l, "water_river"), strings.Contains(l, "water_lake"), strings.Contains(l, "rivermesh_depth"):
		return SpecialWater
	}
	return SpecialNone
}

// CPUModel ist ein geladenes Modell.
type CPUModel struct {
	Key      string
	Subs     []CPUSub
	Min, Max Vec3
	Err      string
}

// CPUTex ist eine geladene Textur.
type CPUTex struct {
	Key string
	Img *g3.Image
	Err string
}

// Result kommt aus dem Lader: entweder Modell oder Textur.
type Result struct {
	Model *CPUModel
	Tex   *CPUTex
}

// Loader laedt Modelle und Texturen im Hintergrund, nach Prioritaet.
type Loader struct {
	vfs        *g3.VFS
	mu         sync.Mutex
	cond       *sync.Cond
	want       []string        // nach Prioritaet sortiert (wichtigstes zuerst)
	busy       map[string]bool // in Arbeit oder fertig
	texSent    map[string]bool
	matMu      sync.Mutex
	mats       map[string]*matInfo
	Out        chan Result
	closed     bool
	Pending    int
	SpecialLog func(string) // optional: meldet erkannte Wasser-/Barriereflaechen
}

// NewLoader startet n Arbeits-Goroutinen.
func NewLoader(v *g3.VFS, workers int) *Loader {
	l := &Loader{vfs: v, busy: map[string]bool{}, texSent: map[string]bool{}, mats: map[string]*matInfo{},
		Out: make(chan Result, 512)}
	l.cond = sync.NewCond(&l.mu)
	for i := 0; i < workers; i++ {
		go l.work()
	}
	return l
}

// Want ersetzt die Wunschliste (wichtigstes zuerst). Schon geladene oder
// in Arbeit befindliche Schluessel werden ignoriert.
func (l *Loader) Want(keys []string) {
	l.mu.Lock()
	l.want = l.want[:0]
	for _, k := range keys {
		if !l.busy[k] {
			l.want = append(l.want, k)
		}
	}
	l.Pending = len(l.want)
	l.mu.Unlock()
	l.cond.Broadcast()
}

// Forget erlaubt erneutes Laden eines entladenen Modells.
func (l *Loader) Forget(key string) {
	l.mu.Lock()
	delete(l.busy, key)
	l.mu.Unlock()
}

// ForgetTex erlaubt erneutes Senden einer entladenen Textur.
func (l *Loader) ForgetTex(key string) {
	l.mu.Lock()
	delete(l.texSent, key)
	l.mu.Unlock()
}

// Close beendet die Arbeiter.
func (l *Loader) Close() {
	l.mu.Lock()
	l.closed = true
	l.mu.Unlock()
	l.cond.Broadcast()
}

func (l *Loader) work() {
	for {
		l.mu.Lock()
		for len(l.want) == 0 && !l.closed {
			l.cond.Wait()
		}
		if l.closed {
			l.mu.Unlock()
			return
		}
		key := l.want[0]
		l.want = l.want[1:]
		l.Pending = len(l.want)
		if l.busy[key] {
			l.mu.Unlock()
			continue
		}
		l.busy[key] = true
		l.mu.Unlock()

		m := l.loadModel(key)
		l.Out <- Result{Model: m}
	}
}

// ModelKey bildet den Schluessel aus Mesh-Name und Materialvariante.
func ModelKey(mesh string, matSwitch int) string {
	k := strings.ToLower(mesh)
	if matSwitch > 0 {
		k += "|" + strconv.Itoa(matSwitch)
	}
	return k
}

func splitKey(key string) (string, int) {
	if i := strings.LastIndex(key, "|"); i >= 0 {
		n, _ := strconv.Atoi(key[i+1:])
		return key[:i], n
	}
	return key, 0
}

type matInfo struct {
	mat    *g3.Material
	recipe g3.Recipe
	hasRec bool
}

func (l *Loader) material(name string) *matInfo {
	k := strings.ToLower(name)
	l.matMu.Lock()
	m, ok := l.mats[k]
	l.matMu.Unlock()
	if ok {
		return m
	}
	e := l.vfs.FindBase("_compiledMaterial", name)
	if e != nil {
		if d, err := e.Read(); err == nil {
			m = &matInfo{}
			m.mat, _ = g3.ParseMaterial(d)
			if sh, err := g3.ParseShader(d); err == nil {
				m.recipe, m.hasRec = sh.Recipe()
			}
			if m.mat == nil {
				m = nil
			}
		}
	}
	l.matMu.Lock()
	l.mats[k] = m
	l.matMu.Unlock()
	return m
}

// resolveTexture sucht die passende .ximg (inkl. Materialvariante _S1.._Sn).
func (l *Loader) resolveTexture(name string, sw int) string {
	if name == "" {
		return ""
	}
	low := strings.ToLower(name)
	if sw > 0 && strings.HasSuffix(low, "_s1") {
		alt := low[:len(low)-1] + strconv.Itoa(sw+1)
		if l.vfs.FindBase("_compiledImage", alt+".ximg") != nil {
			return alt
		}
	}
	if l.vfs.FindBase("_compiledImage", low+".ximg") != nil {
		return low
	}
	return ""
}

func (l *Loader) sendTexture(key string) {
	l.mu.Lock()
	if l.texSent[key] {
		l.mu.Unlock()
		return
	}
	l.texSent[key] = true
	l.mu.Unlock()
	t := &CPUTex{Key: key}
	e := l.vfs.FindBase("_compiledImage", key+".ximg")
	if e == nil {
		t.Err = "nicht gefunden"
	} else if d, err := e.Read(); err != nil {
		t.Err = err.Error()
	} else if img, err := g3.ParseXimg(d); err != nil {
		t.Err = err.Error()
	} else {
		t.Img = img
	}
	l.Out <- Result{Tex: t}
}

func (l *Loader) loadModel(key string) *CPUModel {
	if strings.HasPrefix(key, "tree|") {
		return l.loadTree(key)
	}
	mesh, sw := splitKey(key)
	cm := &CPUModel{Key: key}
	var m *g3.Mesh
	if strings.HasSuffix(mesh, ".fxa") || strings.HasSuffix(mesh, ".xact") {
		name := strings.TrimSuffix(strings.TrimSuffix(mesh, ".fxa"), ".xact") + ".xact"
		e := l.vfs.FindBase("_compiledAnimation", name)
		if e == nil {
			cm.Err = "Figur nicht gefunden: " + name
			return cm
		}
		d, err := e.Read()
		if err == nil {
			m, err = g3.ParseActor(d)
		}
		if err != nil {
			cm.Err = name + ": " + err.Error()
			return cm
		}
	} else {
		e := l.vfs.FindBase("_compiledMesh", mesh)
		if e == nil {
			cm.Err = "Mesh nicht gefunden: " + mesh
			return cm
		}
		d, err := e.Read()
		if err != nil {
			cm.Err = err.Error()
			return cm
		}
		if strings.HasSuffix(mesh, ".xlmsh") {
			names, err := g3.ParseLodMesh(d)
			if err != nil || len(names) == 0 {
				cm.Err = fmt.Sprintf("LoD-Mesh %s: %v", mesh, err)
				return cm
			}
			e = l.vfs.FindBase("_compiledMesh", names[0])
			if e == nil {
				cm.Err = "Mesh nicht gefunden: " + names[0]
				return cm
			}
			if d, err = e.Read(); err != nil {
				cm.Err = err.Error()
				return cm
			}
		}
		m, err = g3.ParseMesh(d)
		if err != nil {
			cm.Err = mesh + ": " + err.Error()
			return cm
		}
	}
	cm.Min, cm.Max = Vec3(m.Min), Vec3(m.Max)
	ms := meshSpecial(mesh)
	for _, s := range m.Subs {
		mi := l.material(s.Material)
		cs := CPUSub{Indices: s.Index, Blend: g3.BlendNormal, MaskRef: 128}
		overAlpha := false
		if mi != nil {
			cs.Blend = mi.mat.Blend
			cs.MaskRef = mi.mat.MaskRef
			if mi.hasRec {
				cs.Tex = l.resolveTexture(mi.recipe.Base.Tex, sw)
				cs.Scale = mi.recipe.Base.Scale
				if mi.recipe.HasOver && s.Color != nil {
					cs.Tex2 = l.resolveTexture(mi.recipe.Over.Tex, sw)
					cs.Scale2 = mi.recipe.Over.Scale
					overAlpha = cs.Tex2 != ""
				}
			}
			if cs.Tex == "" {
				cs.Tex = l.resolveTexture(mi.mat.Diffuse, sw)
				cs.Scale = [2]float32{}
			}
		}
		if sp := materialSpecial(s.Material); sp == SpecialBarrier || sp == SpecialWater && cs.Tex == "" {
			// Wasser mit eigener Farbtextur (z. B. in Faessern) bleibt normal
			cs.Special = sp
		} else if ms != SpecialNone && (mi == nil || mi.mat.Diffuse == "" || ms == SpecialBarrier) {
			cs.Special = ms
		}
		if cs.Special != SpecialNone {
			// eigene Darstellung (halbtransparente Flaeche)
			if l.SpecialLog != nil {
				l.SpecialLog(fmt.Sprintf("%s: Material %s, Blend %d, Textur %q", mesh, s.Material, cs.Blend, cs.Tex))
			}
			cs.Blend = g3.BlendAlphaBlend
			cs.Tex, cs.Tex2 = "", ""
		} else if cs.Blend == g3.BlendInvisible {
			continue
		}
		nv := len(s.Pos) / 3
		v := make([]float32, nv*VertexFloats)
		for i := 0; i < nv; i++ {
			o := v[i*VertexFloats : (i+1)*VertexFloats]
			o[0], o[1], o[2] = s.Pos[i*3], s.Pos[i*3+1], s.Pos[i*3+2]
			if s.Normal != nil {
				o[3], o[4], o[5] = s.Normal[i*3], s.Normal[i*3+1], s.Normal[i*3+2]
			} else {
				o[4] = 1
			}
			if s.UV != nil {
				o[6], o[7] = s.UV[i*2], s.UV[i*2+1]
			}
			rgba := uint32(0xFFFFFFFF)
			if overAlpha {
				// Mischfaktor aus dem Alpha der Vertexfarbe (D3DCOLOR: A in Bits 24..31)
				a := s.Color[i] >> 24
				rgba = 0x00FFFFFF | a<<24
			}
			o[8] = math.Float32frombits(rgba)
		}
		cs.Vertices = v
		if cs.Tex != "" {
			l.sendTexture(cs.Tex)
		}
		if cs.Tex2 != "" {
			l.sendTexture(cs.Tex2)
		}
		cm.Subs = append(cm.Subs, cs)
	}
	return cm
}

func (l *Loader) loadTree(key string) *CPUModel {
	name := strings.TrimPrefix(key, "tree|")
	cm := &CPUModel{Key: key}
	e := l.vfs.FindBase("Speedtrees", name)
	if e == nil {
		cm.Err = "Baum nicht gefunden: " + name
		return cm
	}
	d, err := e.Read()
	if err != nil {
		cm.Err = err.Error()
		return cm
	}
	ti := ParseSpt(name, d)
	tm := BuildTree(key, ti)
	for i := range tm.Subs {
		t := l.resolveTexture(tm.Subs[i].Tex, 0)
		tm.Subs[i].Tex = t
		if t != "" {
			l.sendTexture(t)
		}
	}
	return tm
}
