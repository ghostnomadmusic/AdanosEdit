package g3

import (
	"encoding/binary"
	"math"
)

// ShaderNode ist ein Element des Shader-Graphen eines Materials.
type ShaderNode struct {
	Class    string
	Token    [16]byte
	Tex      string     // eCColorSrcSampler: Texturname (ohne Pfad/Endung)
	TexCoord Proxy      // eCColorSrcSampler: Texturkoordinaten-Quelle
	Src      [3]Proxy   // Blend: Quelle1, Quelle2, Faktor; Combiner: Quelle1, Quelle2
	Combine  int        // Combiner-Typ (0 Add, 1 Sub, 2 Mul, 3 Max, 4 Min)
	Scale    [2]float32 // eCTexCoordSrcScale
	Color    [4]float32 // eCColorSrcConstant
	Props    PropSet
}

// Proxy verweist auf ein anderes Element (Komponente + Token).
type Proxy struct {
	Comp  int
	Token [16]byte
	Valid bool
}

// Shader ist der ausgewertete Graph.
type Shader struct {
	Class     string
	Diffuse   Proxy
	Opacity   Proxy
	Nodes     []ShaderNode
	Blend     int
	MaskRef   int
	Transform int
	ByToken   map[[16]byte]*ShaderNode
}

func (r *reader) proxy() Proxy {
	var p Proxy
	p.Comp = int(r.u32())
	copy(p.Token[:], r.bytes(16))
	p.Valid = r.u32() != 0
	return p
}

// subClassHeader liest den Kopf eines eingebetteten Objekts und seine
// Eigenschaften. Liefert Klasse, Eigenschaften, Klassenversion und Ende.
func (r *reader) subClass() (string, PropSet, int, int) {
	r.skip(6)
	name := r.entry()
	r.skip(5)
	ps := PropSet{Class: name}
	end := r.propsBody(&ps)
	cv := int(r.u16())
	return name, ps, cv, end
}

func (r *reader) elementBase(n *ShaderNode) {
	r.u16()
	copy(n.Token[:], r.bytes(16))
	r.u32()
	r.skip(16)
}

func vec2Prop(ps *PropSet, name string) ([2]float32, bool) {
	p := ps.Get(name)
	if p == nil || len(p.Data) < 8 {
		return [2]float32{}, false
	}
	return [2]float32{math.Float32frombits(binary.LittleEndian.Uint32(p.Data)), math.Float32frombits(binary.LittleEndian.Uint32(p.Data[4:]))}, true
}

func enumProp(ps *PropSet, name string) (int, bool) {
	p := ps.Get(name)
	if p == nil || len(p.Data) < 6 {
		return 0, false
	}
	return int(binary.LittleEndian.Uint32(p.Data[2:])), true
}

// ParseShader liest den Shader-Graphen eines .xshmat.
func ParseShader(d []byte) (s *Shader, err error) {
	defer recoverParse(&err)
	r, cname := openResource(d)
	if cname != "eCResourceShaderMaterial_PS" {
		fail("kein Material (%s)", cname)
	}
	s = &Shader{ByToken: map[[16]byte]*ShaderNode{}, MaskRef: 128}
	name, ps, cv, end := r.subClass()
	s.Class = name
	if v, ok := enumProp(&ps, "BlendMode"); ok {
		s.Blend = v
	}
	if p := ps.Get("MaskReference"); p != nil && len(p.Data) > 0 {
		s.MaskRef = int(p.Data[0])
	}
	if v, ok := enumProp(&ps, "TransformationType"); ok {
		s.Transform = v
	}
	nproxy := 0
	switch name {
	case "eCShaderDefault":
		nproxy = 6
		if cv > 1 {
			nproxy = 7
		}
	case "eCShaderSkin":
		nproxy = 7
	case "eCShaderLeaf":
		nproxy = 4
	case "eCShaderWater":
		nproxy = 6
		if cv > 1 {
			nproxy = 7
		}
	case "eCShaderParticle":
		nproxy = 1
		if cv > 1 {
			nproxy = 2
		}
	default:
		r.p = end
		return s, nil
	}
	for i := 0; i < nproxy; i++ {
		p := r.proxy()
		if i == 0 {
			s.Diffuse = p
		}
		if i == 1 && (name == "eCShaderDefault" || name == "eCShaderSkin") {
			s.Opacity = p
		}
	}
	r.u16()
	var base ShaderNode
	r.elementBase(&base)
	n := int(r.u32())
	if n > 1000 {
		fail("zu viele Shader-Elemente")
	}
	for i := 0; i < n; i++ {
		en, eps, _, eend := r.subClass()
		node := ShaderNode{Class: en, Props: eps}
		switch en {
		case "eCColorSrcSampler":
			node.TexCoord = r.proxy()
			r.u32()
			r.u16()
			r.elementBase(&node)
			node.Tex = stripTex(r.stringValue(eps.Get("ImageFilePath")))
		case "eCColorSrcBlend":
			node.Src[0], node.Src[1], node.Src[2] = r.proxy(), r.proxy(), r.proxy()
			r.u16()
			r.elementBase(&node)
		case "eCColorSrcCombiner":
			node.Src[0], node.Src[1] = r.proxy(), r.proxy()
			r.u16()
			r.elementBase(&node)
			node.Combine, _ = enumProp(&eps, "CombinerType")
		case "eCColorSrcVertexColor", "eCColorSrcConstant":
			r.u16()
			r.elementBase(&node)
			if p := eps.Get("Color"); p != nil && len(p.Data) >= 12 {
				for k := 0; k < 3; k++ {
					node.Color[k] = math.Float32frombits(binary.LittleEndian.Uint32(p.Data[k*4:]))
				}
				node.Color[3] = 1
			}
		case "eCTexCoordSrcScale":
			node.Scale, _ = vec2Prop(&eps, "Scale")
			// Token liegt am Ende der Daten (Element-Basis)
			if eend-40 >= r.p {
				copy(node.Token[:], r.d[eend-36:eend-20])
			}
		default:
			if eend-36 >= r.p {
				copy(node.Token[:], r.d[eend-36:eend-20])
			}
		}
		r.p = eend
		s.Nodes = append(s.Nodes, node)
	}
	for i := range s.Nodes {
		s.ByToken[s.Nodes[i].Token] = &s.Nodes[i]
	}
	return s, nil
}

// Node liefert das Element zu einem Proxy.
func (s *Shader) Node(p Proxy) *ShaderNode {
	if s == nil {
		return nil
	}
	return s.ByToken[p.Token]
}

// Layer ist eine darstellbare Texturebene.
type Layer struct {
	Tex   string
	Scale [2]float32 // 0 = keine Skalierung
}

// Recipe ist die vereinfachte Darstellung eines Materials:
// Grundtextur und optional eine zweite, ueber die Vertexfarbe eingeblendete.
type Recipe struct {
	Base, Over Layer
	OverComp   int // Komponente der Vertexfarbe als Mischfaktor
	HasOver    bool
}

func isDetailTex(t string) bool {
	l := toLower(t)
	return contains(l, "detailmap") || contains(l, "dirtmap") || contains(l, "_normal") || contains(l, "_specular")
}

func (s *Shader) layer(p Proxy, depth int) (Layer, bool) {
	n := s.Node(p)
	if n == nil || depth > 16 {
		return Layer{}, false
	}
	switch n.Class {
	case "eCColorSrcSampler":
		if n.Tex == "" {
			return Layer{}, false
		}
		l := Layer{Tex: n.Tex}
		if tc := s.Node(n.TexCoord); tc != nil && n.TexCoord.Valid && tc.Class == "eCTexCoordSrcScale" {
			l.Scale = tc.Scale
		}
		return l, true
	case "eCColorSrcCombiner", "eCColorSrcBlend":
		a, oka := s.layer(n.Src[0], depth+1)
		b, okb := s.layer(n.Src[1], depth+1)
		switch {
		case oka && okb && isDetailTex(a.Tex) && !isDetailTex(b.Tex):
			return b, true
		case oka:
			return a, true
		case okb:
			return b, true
		}
	}
	return Layer{}, false
}

// findBlend sucht unterhalb von p einen Blend mit Vertexfarbe als Faktor.
func (s *Shader) findBlend(p Proxy, depth int) *ShaderNode {
	n := s.Node(p)
	if n == nil || depth > 16 {
		return nil
	}
	if n.Class == "eCColorSrcBlend" {
		if f := s.Node(n.Src[2]); f != nil && f.Class == "eCColorSrcVertexColor" {
			return n
		}
	}
	if n.Class == "eCColorSrcCombiner" || n.Class == "eCColorSrcBlend" {
		if b := s.findBlend(n.Src[0], depth+1); b != nil {
			return b
		}
		return s.findBlend(n.Src[1], depth+1)
	}
	return nil
}

// Recipe wertet den Diffuse-Zweig aus.
func (s *Shader) Recipe() (Recipe, bool) {
	var rc Recipe
	if s == nil || !s.Diffuse.Valid {
		return rc, false
	}
	if b := s.findBlend(s.Diffuse, 0); b != nil {
		l1, ok1 := s.layer(b.Src[0], 0)
		l2, ok2 := s.layer(b.Src[1], 0)
		if ok1 && ok2 {
			return Recipe{Base: l1, Over: l2, OverComp: b.Src[2].Comp, HasOver: true}, true
		}
	}
	l, ok := s.layer(s.Diffuse, 0)
	if !ok {
		return rc, false
	}
	rc.Base = l
	return rc, true
}

func toLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 32
		}
	}
	return string(b)
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
