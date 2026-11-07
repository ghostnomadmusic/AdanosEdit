package scene

import (
	"math"
	"testing"
)

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-3 }

func TestViewBasics(t *testing.T) {
	c := Camera{Pos: Vec3{100, 50, 200}, FovY: 60}
	v := c.View()
	// Punkt 10 vor der Kamera -> GL eye z = -10
	p := v.TransformPoint(Vec3{100, 50, 210})
	if !near(p[0], 0) || !near(p[1], 0) || !near(p[2], -10) {
		t.Fatal("vorne", p)
	}
	// rechts (+X bei Yaw 0) -> eye x > 0
	p = v.TransformPoint(Vec3{105, 50, 210})
	if p[0] <= 0 {
		t.Fatal("rechts", p)
	}
	// oben
	p = v.TransformPoint(Vec3{100, 60, 210})
	if p[1] <= 0 {
		t.Fatal("oben", p)
	}
	c.Yaw = math.Pi / 2 // Blick nach +X, rechts ist -Z
	v = c.View()
	p = v.TransformPoint(Vec3{110, 50, 195})
	if !near(p[2], -10) || p[0] <= 0 {
		t.Fatal("yaw", p)
	}
	inv, ok := v.Inverse()
	if !ok {
		t.Fatal("inv")
	}
	q := inv.TransformPoint(v.TransformPoint(Vec3{1, 2, 3}))
	if !near(q[0], 1) || !near(q[1], 2) || !near(q[2], 3) {
		t.Fatal("inverse", q)
	}
}

func TestFrustumAndRay(t *testing.T) {
	c := Camera{Pos: Vec3{0, 0, 0}, FovY: 60}
	pv := Perspective(60, 1.5, 10, 10000).Mul(c.View())
	f := FrustumFrom(pv)
	if !f.SphereVisible(Vec3{0, 0, 500}, 1) {
		t.Fatal("vorne sichtbar")
	}
	if f.SphereVisible(Vec3{0, 0, -500}, 1) {
		t.Fatal("hinten unsichtbar")
	}
	if f.SphereVisible(Vec3{5000, 0, 500}, 1) {
		t.Fatal("seitlich unsichtbar")
	}
	o, d := c.Ray(150, 50, 300, 100) // Bildmitte
	if !near(d[2], 1) {
		t.Fatal("ray", d)
	}
	_, d = c.Ray(300, 50, 300, 100) // rechter Rand
	if d[0] <= 0 {
		t.Fatal("ray rechts", d)
	}
	if tt, ok := RayBox(o, Vec3{0, 0, 1}, Vec3{-1, -1, 9}, Vec3{1, 1, 11}); !ok || !near(tt, 9) {
		t.Fatal("raybox", tt, ok)
	}
}
