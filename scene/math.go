// Package scene enthaelt plattformunabhaengige Szenenlogik: Mathematik,
// Kamera, raeumlicher Index und Picking.
package scene

import "math"

type Vec3 [3]float32

func (a Vec3) Add(b Vec3) Vec3      { return Vec3{a[0] + b[0], a[1] + b[1], a[2] + b[2]} }
func (a Vec3) Sub(b Vec3) Vec3      { return Vec3{a[0] - b[0], a[1] - b[1], a[2] - b[2]} }
func (a Vec3) Scale(s float32) Vec3 { return Vec3{a[0] * s, a[1] * s, a[2] * s} }
func (a Vec3) Dot(b Vec3) float32   { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
func (a Vec3) Cross(b Vec3) Vec3 {
	return Vec3{a[1]*b[2] - a[2]*b[1], a[2]*b[0] - a[0]*b[2], a[0]*b[1] - a[1]*b[0]}
}
func (a Vec3) Len() float32 { return float32(math.Sqrt(float64(a.Dot(a)))) }
func (a Vec3) Norm() Vec3 {
	l := a.Len()
	if l < 1e-12 {
		return Vec3{}
	}
	return a.Scale(1 / l)
}

// Mat4 im OpenGL-Speicherlayout (spaltenweise). Eine Gothic-Weltmatrix
// (Zeilenvektoren, DirectX) hat exakt dasselbe Speicherlayout.
type Mat4 [16]float32

func Identity() Mat4 { return Mat4{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1} }

// Mul liefert a*b (erst b, dann a anwenden, OpenGL-Konvention).
func (a Mat4) Mul(b Mat4) Mat4 {
	var r Mat4
	for c := 0; c < 4; c++ {
		for rr := 0; rr < 4; rr++ {
			var s float32
			for k := 0; k < 4; k++ {
				s += a[k*4+rr] * b[c*4+k]
			}
			r[c*4+rr] = s
		}
	}
	return r
}

// TransformPoint wendet die Matrix auf einen Punkt an.
func (m Mat4) TransformPoint(p Vec3) Vec3 {
	return Vec3{
		m[0]*p[0] + m[4]*p[1] + m[8]*p[2] + m[12],
		m[1]*p[0] + m[5]*p[1] + m[9]*p[2] + m[13],
		m[2]*p[0] + m[6]*p[1] + m[10]*p[2] + m[14],
	}
}

// TransformDir wendet nur den 3x3-Anteil an.
func (m Mat4) TransformDir(p Vec3) Vec3 {
	return Vec3{
		m[0]*p[0] + m[4]*p[1] + m[8]*p[2],
		m[1]*p[0] + m[5]*p[1] + m[9]*p[2],
		m[2]*p[0] + m[6]*p[1] + m[10]*p[2],
	}
}

// Inverse berechnet die allgemeine Inverse (ok=false wenn singulaer).
func (m Mat4) Inverse() (Mat4, bool) {
	var a [16]float64
	for i := range a {
		a[i] = float64(m[i])
	}
	var inv [16]float64
	inv[0] = a[5]*a[10]*a[15] - a[5]*a[11]*a[14] - a[9]*a[6]*a[15] + a[9]*a[7]*a[14] + a[13]*a[6]*a[11] - a[13]*a[7]*a[10]
	inv[4] = -a[4]*a[10]*a[15] + a[4]*a[11]*a[14] + a[8]*a[6]*a[15] - a[8]*a[7]*a[14] - a[12]*a[6]*a[11] + a[12]*a[7]*a[10]
	inv[8] = a[4]*a[9]*a[15] - a[4]*a[11]*a[13] - a[8]*a[5]*a[15] + a[8]*a[7]*a[13] + a[12]*a[5]*a[11] - a[12]*a[7]*a[9]
	inv[12] = -a[4]*a[9]*a[14] + a[4]*a[10]*a[13] + a[8]*a[5]*a[14] - a[8]*a[6]*a[13] - a[12]*a[5]*a[10] + a[12]*a[6]*a[9]
	inv[1] = -a[1]*a[10]*a[15] + a[1]*a[11]*a[14] + a[9]*a[2]*a[15] - a[9]*a[3]*a[14] - a[13]*a[2]*a[11] + a[13]*a[3]*a[10]
	inv[5] = a[0]*a[10]*a[15] - a[0]*a[11]*a[14] - a[8]*a[2]*a[15] + a[8]*a[3]*a[14] + a[12]*a[2]*a[11] - a[12]*a[3]*a[10]
	inv[9] = -a[0]*a[9]*a[15] + a[0]*a[11]*a[13] + a[8]*a[1]*a[15] - a[8]*a[3]*a[13] - a[12]*a[1]*a[11] + a[12]*a[3]*a[9]
	inv[13] = a[0]*a[9]*a[14] - a[0]*a[10]*a[13] - a[8]*a[1]*a[14] + a[8]*a[2]*a[13] + a[12]*a[1]*a[10] - a[12]*a[2]*a[9]
	inv[2] = a[1]*a[6]*a[15] - a[1]*a[7]*a[14] - a[5]*a[2]*a[15] + a[5]*a[3]*a[14] + a[13]*a[2]*a[7] - a[13]*a[3]*a[6]
	inv[6] = -a[0]*a[6]*a[15] + a[0]*a[7]*a[14] + a[4]*a[2]*a[15] - a[4]*a[3]*a[14] - a[12]*a[2]*a[7] + a[12]*a[3]*a[6]
	inv[10] = a[0]*a[5]*a[15] - a[0]*a[7]*a[13] - a[4]*a[1]*a[15] + a[4]*a[3]*a[13] + a[12]*a[1]*a[7] - a[12]*a[3]*a[5]
	inv[14] = -a[0]*a[5]*a[14] + a[0]*a[6]*a[13] + a[4]*a[1]*a[14] - a[4]*a[2]*a[13] - a[12]*a[1]*a[6] + a[12]*a[2]*a[5]
	inv[3] = -a[1]*a[6]*a[11] + a[1]*a[7]*a[10] + a[5]*a[2]*a[11] - a[5]*a[3]*a[10] - a[9]*a[2]*a[7] + a[9]*a[3]*a[6]
	inv[7] = a[0]*a[6]*a[11] - a[0]*a[7]*a[10] - a[4]*a[2]*a[11] + a[4]*a[3]*a[10] + a[8]*a[2]*a[7] - a[8]*a[3]*a[6]
	inv[11] = -a[0]*a[5]*a[11] + a[0]*a[7]*a[9] + a[4]*a[1]*a[11] - a[4]*a[3]*a[9] - a[8]*a[1]*a[7] + a[8]*a[3]*a[5]
	inv[15] = a[0]*a[5]*a[10] - a[0]*a[6]*a[9] - a[4]*a[1]*a[10] + a[4]*a[2]*a[9] + a[8]*a[1]*a[6] - a[8]*a[2]*a[5]
	det := a[0]*inv[0] + a[1]*inv[4] + a[2]*inv[8] + a[3]*inv[12]
	if math.Abs(det) < 1e-20 {
		return Identity(), false
	}
	var r Mat4
	for i := range inv {
		r[i] = float32(inv[i] / det)
	}
	return r, true
}

// Perspective erzeugt eine OpenGL-Projektionsmatrix.
func Perspective(fovYDeg, aspect, near, far float32) Mat4 {
	f := float32(1 / math.Tan(float64(fovYDeg)*math.Pi/360))
	var m Mat4
	m[0] = f / aspect
	m[5] = f
	m[10] = (far + near) / (near - far)
	m[11] = -1
	m[14] = 2 * far * near / (near - far)
	return m
}

// Camera im Gothic-Koordinatensystem (linkshaendig, Y oben, Einheit cm).
type Camera struct {
	Pos        Vec3
	Yaw, Pitch float32 // Bogenmass; Yaw 0 = Blick nach +Z
	FovY       float32
}

func (c *Camera) Forward() Vec3 {
	cp := float32(math.Cos(float64(c.Pitch)))
	return Vec3{float32(math.Sin(float64(c.Yaw))) * cp, float32(math.Sin(float64(c.Pitch))), float32(math.Cos(float64(c.Yaw))) * cp}
}

// Right zeigt im linkshaendigen System nach rechts.
func (c *Camera) Right() Vec3 {
	return Vec3{float32(math.Cos(float64(c.Yaw))), 0, -float32(math.Sin(float64(c.Yaw)))}
}

// Up: im linkshaendigen System ist forward x right (Standard-Kreuzprodukt) oben.
func (c *Camera) Up() Vec3 { return c.Forward().Cross(c.Right()).Norm() }

// View liefert die OpenGL-Viewmatrix. Die Z-Achse wird gespiegelt, damit
// die linkshaendigen Gothic-Daten in OpenGL korrekt (nicht gespiegelt) erscheinen.
func (c *Camera) View() Mat4 {
	f, r, u := c.Forward(), c.Right(), c.Up()
	var m Mat4
	// Zeilen der Rotation: r, u, -f
	m[0], m[4], m[8] = r[0], r[1], r[2]
	m[1], m[5], m[9] = u[0], u[1], u[2]
	m[2], m[6], m[10] = -f[0], -f[1], -f[2]
	m[12] = -r.Dot(c.Pos)
	m[13] = -u.Dot(c.Pos)
	m[14] = f.Dot(c.Pos)
	m[15] = 1
	return m
}

// LookAt richtet die Kamera auf einen Punkt aus.
func (c *Camera) LookAt(target Vec3) {
	d := target.Sub(c.Pos).Norm()
	c.Pitch = float32(math.Asin(float64(clamp(d[1], -1, 1))))
	c.Yaw = float32(math.Atan2(float64(d[0]), float64(d[2])))
}

func clamp(v, a, b float32) float32 {
	if v < a {
		return a
	}
	if v > b {
		return b
	}
	return v
}

// Ray liefert den Sichtstrahl durch einen Bildschirmpunkt (0..w, 0..h).
func (c *Camera) Ray(x, y, w, h float32) (Vec3, Vec3) {
	aspect := w / h
	t := float32(math.Tan(float64(c.FovY) * math.Pi / 360))
	nx := (2*x/w - 1) * t * aspect
	ny := (1 - 2*y/h) * t
	f, r := c.Forward(), c.Right()
	u := f.Cross(r).Norm()
	d := f.Add(r.Scale(nx)).Add(u.Scale(ny)).Norm()
	return c.Pos, d
}

// Frustum als sechs Ebenen (a,b,c,d) in Weltkoordinaten (Gothic-Raum).
type Frustum [6][4]float32

// FrustumFrom erzeugt das Frustum aus Projektion*View (OpenGL-Matrizen,
// View enthaelt bereits die Z-Spiegelung; Weltpunkte im Gothic-Raum).
func FrustumFrom(pv Mat4) Frustum {
	row := func(i int) [4]float32 { return [4]float32{pv[i], pv[4+i], pv[8+i], pv[12+i]} }
	r0, r1, r2, r3 := row(0), row(1), row(2), row(3)
	var f Frustum
	for i := 0; i < 4; i++ {
		f[0][i] = r3[i] + r0[i]
		f[1][i] = r3[i] - r0[i]
		f[2][i] = r3[i] + r1[i]
		f[3][i] = r3[i] - r1[i]
		f[4][i] = r3[i] + r2[i]
		f[5][i] = r3[i] - r2[i]
	}
	for i := range f {
		l := float32(math.Sqrt(float64(f[i][0]*f[i][0] + f[i][1]*f[i][1] + f[i][2]*f[i][2])))
		if l > 0 {
			for j := 0; j < 4; j++ {
				f[i][j] /= l
			}
		}
	}
	return f
}

// SphereVisible prueft eine Kugel gegen das Frustum.
func (f *Frustum) SphereVisible(c Vec3, r float32) bool {
	for i := 0; i < 6; i++ {
		if f[i][0]*c[0]+f[i][1]*c[1]+f[i][2]*c[2]+f[i][3] < -r {
			return false
		}
	}
	return true
}

// RayBox schneidet einen Strahl mit einer achsparallelen Box. Liefert t>=0.
func RayBox(o, d, mn, mx Vec3) (float32, bool) {
	tmin, tmax := float32(-math.MaxFloat32), float32(math.MaxFloat32)
	for a := 0; a < 3; a++ {
		if math.Abs(float64(d[a])) < 1e-9 {
			if o[a] < mn[a] || o[a] > mx[a] {
				return 0, false
			}
			continue
		}
		inv := 1 / d[a]
		t1 := (mn[a] - o[a]) * inv
		t2 := (mx[a] - o[a]) * inv
		if t1 > t2 {
			t1, t2 = t2, t1
		}
		if t1 > tmin {
			tmin = t1
		}
		if t2 < tmax {
			tmax = t2
		}
		if tmin > tmax {
			return 0, false
		}
	}
	if tmax < 0 {
		return 0, false
	}
	if tmin < 0 {
		return 0, true
	}
	return tmin, true
}

// RayTriangle (Moeller-Trumbore), liefert t.
func RayTriangle(o, d, a, b, c Vec3) (float32, bool) {
	e1, e2 := b.Sub(a), c.Sub(a)
	p := d.Cross(e2)
	det := e1.Dot(p)
	if det > -1e-9 && det < 1e-9 {
		return 0, false
	}
	inv := 1 / det
	s := o.Sub(a)
	u := s.Dot(p) * inv
	if u < 0 || u > 1 {
		return 0, false
	}
	q := s.Cross(e1)
	v := d.Dot(q) * inv
	if v < 0 || u+v > 1 {
		return 0, false
	}
	t := e2.Dot(q) * inv
	if t < 0 {
		return 0, false
	}
	return t, true
}
