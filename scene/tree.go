package scene

import (
	"bytes"
	"encoding/binary"
	"hash/fnv"
	"math"
	"math/rand"
	"regexp"
	"strings"
)

// TreeInfo sind die aus einer .spt gelesenen Angaben.
type TreeInfo struct {
	Bark      string     // Rindentextur (ohne Endung)
	Composite string     // Laub-Atlas (ohne Endung)
	Leaf      [4]float32 // u0, v0, u1, v1 im Atlas
	Kind      string     // "broadleaf", "conifer", "palm", "bush", "willow", "dead"
}

var reTga = regexp.MustCompile(`[A-Za-z0-9_]+\.tga`)
var reComp = regexp.MustCompile(`G3_Speedtree_Misc_Composite_(\d+)_Diffuse_01`)

// ParseSpt liest die fuer die Darstellung noetigen Angaben einer SpeedTree-Datei.
func ParseSpt(name string, d []byte) TreeInfo {
	var ti TreeInfo
	low := strings.ToLower(name)
	if m := reTga.FindAll(d, -1); len(m) > 0 {
		for _, s := range m {
			if strings.Contains(strings.ToLower(string(s)), "bark") {
				ti.Bark = strings.TrimSuffix(string(s), ".tga")
				break
			}
		}
	}
	comp := "02"
	if m := reComp.FindSubmatch(d); m != nil {
		comp = string(m[1])
		ti.Composite = string(m[0])
	} else {
		ti.Composite = "G3_Speedtree_Misc_Composite_02_Diffuse_01"
	}
	switch {
	case strings.Contains(low, "palm"):
		ti.Kind = "palm"
	case strings.Contains(low, "fir") || strings.Contains(low, "pine") || strings.Contains(low, "cypress") || strings.Contains(low, "spruce"):
		ti.Kind = "conifer"
	case strings.Contains(low, "willow") || strings.Contains(low, "broadleaf_05"):
		ti.Kind = "willow"
	case strings.Contains(low, "bush") || strings.Contains(low, "_s_"):
		ti.Kind = "bush"
	case strings.Contains(low, "dead") || strings.Contains(low, "dry"):
		ti.Kind = "dead"
	default:
		ti.Kind = "broadleaf"
	}
	// Bereich des Laubs im Atlas: aus der Datei, wenn vorhanden
	full := true
	if i := bytes.Index(d, []byte(".dds#N")); i >= 0 {
		tok := make([]byte, 4)
		binary.LittleEndian.PutUint32(tok, 20005)
		if j := bytes.Index(d[i:], tok); j >= 0 && i+j+36 <= len(d) {
			p := i + j + 4
			var f [8]float32
			for k := range f {
				f[k] = math.Float32frombits(binary.LittleEndian.Uint32(d[p+k*4:]))
			}
			u0 := min(f[0], f[2], f[4], f[6])
			u1 := max(f[0], f[2], f[4], f[6])
			v0 := min(f[1], f[3], f[5], f[7])
			v1 := max(f[1], f[3], f[5], f[7])
			if u1-u0 < 0.99 || v1-v0 < 0.99 {
				full = false
				ti.Leaf = [4]float32{u0, v0, u1, v1}
			}
		}
	}
	if full {
		ti.Leaf = defaultLeaf(comp, ti.Kind)
	}
	return ti
}

// defaultLeaf waehlt einen passenden Laubbereich je Atlas und Baumart.
func defaultLeaf(comp, kind string) [4]float32 {
	c := func(x, y, w, h float32) [4]float32 {
		return [4]float32{x * 0.25, y * 0.25, (x + w) * 0.25, (y + h) * 0.25}
	}
	switch comp {
	case "01":
		return c(0, 2, 1, 1)
	case "02":
		if kind == "conifer" {
			return c(1, 1, 1, 1)
		}
		return c(0, 1, 1, 1)
	case "03":
		return c(0, 2, 2, 2)
	case "04":
		if kind == "palm" {
			return c(3, 0, 1, 2)
		}
		return c(0, 0, 1, 1)
	case "05":
		if kind == "palm" {
			return c(1, 1, 1, 1)
		}
		return c(0, 0, 1, 4)
	case "06":
		return c(0, 1, 1, 1)
	}
	return c(0, 1, 1, 1)
}

// BuildTree erzeugt ein Baummodell im Einheitsraum (x,z in -0.5..0.5, y in 0..1).
func BuildTree(key string, ti TreeInfo) *CPUModel {
	h := fnv.New64a()
	h.Write([]byte(key))
	rng := rand.New(rand.NewSource(int64(h.Sum64())))
	cm := &CPUModel{Key: key, Min: Vec3{-0.5, 0, -0.5}, Max: Vec3{0.5, 1, 0.5}}

	var bark, leaf []float32
	var bi, li []uint32
	vert := func(dst *[]float32, p, n Vec3, u, v float32) uint32 {
		idx := uint32(len(*dst) / VertexFloats)
		*dst = append(*dst, p[0], p[1], p[2], n[0], n[1], n[2], u, v, whiteRGBA)
		return idx
	}
	// Stamm: sich verjuengender Zylinder
	trunkTop, r0, r1 := float32(0.55), float32(0.035), float32(0.012)
	switch ti.Kind {
	case "palm":
		trunkTop, r0, r1 = 0.9, 0.025, 0.018
	case "conifer":
		trunkTop, r0, r1 = 0.85, 0.03, 0.006
	case "bush":
		trunkTop, r0, r1 = 0.25, 0.02, 0.008
	case "dead":
		trunkTop, r0, r1 = 0.9, 0.035, 0.008
	}
	const sides = 8
	for i := 0; i <= sides; i++ {
		a := float32(i) / sides * 2 * math.Pi
		cs, sn := float32(math.Cos(float64(a))), float32(math.Sin(float64(a)))
		n := Vec3{cs, 0, sn}
		u := float32(i) / sides * 2
		vert(&bark, Vec3{cs * r0, 0, sn * r0}, n, u, 0)
		vert(&bark, Vec3{cs * r1, trunkTop, sn * r1}, n, u, trunkTop*6)
	}
	for i := 0; i < sides; i++ {
		b := uint32(i * 2)
		bi = append(bi, b, b+1, b+3, b, b+3, b+2)
	}
	// Laubkarten
	l := ti.Leaf
	card := func(c Vec3, size float32, yaw, tilt float32, center Vec3) {
		cy, sy := float32(math.Cos(float64(yaw))), float32(math.Sin(float64(yaw)))
		ct, st := float32(math.Cos(float64(tilt))), float32(math.Sin(float64(tilt)))
		right := Vec3{cy, 0, sy}.Scale(size / 2)
		up := Vec3{-sy * st, ct, cy * st}.Scale(size / 2)
		n := c.Sub(center).Norm()
		if n.Len() == 0 {
			n = Vec3{0, 1, 0}
		}
		p0 := vert(&leaf, c.Sub(right).Sub(up), n, l[0], l[3])
		vert(&leaf, c.Add(right).Sub(up), n, l[2], l[3])
		vert(&leaf, c.Add(right).Add(up), n, l[2], l[1])
		vert(&leaf, c.Sub(right).Add(up), n, l[0], l[1])
		li = append(li, p0, p0+1, p0+2, p0, p0+2, p0+3)
	}
	rnd := func(a, b float32) float32 { return a + rng.Float32()*(b-a) }
	switch ti.Kind {
	case "palm":
		top := Vec3{0, trunkTop, 0}
		for i := 0; i < 9; i++ {
			yaw := float32(i)/9*2*math.Pi + rnd(-0.2, 0.2)
			dir := Vec3{float32(math.Cos(float64(yaw))), 0, float32(math.Sin(float64(yaw)))}
			// Wedel als nach aussen haengende Karte
			c := top.Add(dir.Scale(0.22)).Add(Vec3{0, -0.05, 0})
			card(c, 0.5, yaw+math.Pi/2, 1.2, top.Sub(Vec3{0, 0.3, 0}))
		}
	case "conifer":
		layers := 9
		for k := 0; k < layers; k++ {
			t := float32(k) / float32(layers-1)
			y := 0.2 + t*0.78
			rad := (1 - t) * 0.45
			n := 4 + int((1-t)*6)
			for i := 0; i < n; i++ {
				yaw := float32(i)/float32(n)*2*math.Pi + rnd(0, 1)
				dir := Vec3{float32(math.Cos(float64(yaw))), 0, float32(math.Sin(float64(yaw)))}
				c := Vec3{0, y, 0}.Add(dir.Scale(rad * 0.55))
				card(c, rad*1.1+0.08, yaw+math.Pi/2, rnd(0.9, 1.3), Vec3{0, y + 0.1, 0})
			}
		}
	case "dead":
		// nur Stamm
	default:
		center := Vec3{0, 0.62, 0}
		radius := Vec3{0.45, 0.35, 0.45}
		count := 42
		size := float32(0.42)
		if ti.Kind == "bush" {
			center, radius, count, size = Vec3{0, 0.5, 0}, Vec3{0.45, 0.45, 0.45}, 26, 0.55
		}
		if ti.Kind == "willow" {
			center, radius, size = Vec3{0, 0.6, 0}, Vec3{0.42, 0.38, 0.42}, 0.5
		}
		for i := 0; i < count; i++ {
			// Punkt im Ellipsoid, zur Oberflaeche hin verteilt
			var p Vec3
			for {
				p = Vec3{rnd(-1, 1), rnd(-1, 1), rnd(-1, 1)}
				if p.Len() <= 1 {
					break
				}
			}
			s := float32(math.Sqrt(float64(p.Len())))*0.4 + 0.6
			p = p.Norm().Scale(s)
			c := center.Add(Vec3{p[0] * radius[0], p[1] * radius[1], p[2] * radius[2]})
			card(c, size*rnd(0.8, 1.2), rnd(0, 2*math.Pi), rnd(-0.6, 0.6), center)
		}
	}
	if len(bi) > 0 {
		cm.Subs = append(cm.Subs, CPUSub{Vertices: bark, Indices: bi, Tex: strings.ToLower(ti.Bark), Blend: 0, MaskRef: 128})
	}
	if len(li) > 0 {
		cm.Subs = append(cm.Subs, CPUSub{Vertices: leaf, Indices: li, Tex: strings.ToLower(ti.Composite), Blend: 1, MaskRef: 80})
	}
	return cm
}
