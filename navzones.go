//go:build windows

package main

// Eigene Zonen in der Navigationskarte (G3_World_01/NavigationMap.xnav).
//
// Gothic 3 kennt nur Zonen, die in dieser Karte stehen. Beim Speichern baut
// AdanosEdit die Karte deshalb jedes Mal neu: aus der Karte des Spiels ohne
// Mods (Projects_compiled.pak) plus allen Zonen der Welt, die dort fehlen.
// Die Originaldateien bleiben unveraendert; die Karte liegt in der Mod.

import (
	"fmt"
	"log"
	"path/filepath"
	"strings"

	"g3viewer/g3"
)

var navBaseCache *g3.NavMap
var navBaseErr error

// navBase liefert die Navigationskarte des Spiels ohne Mods.
func (a *app) navBase() (*g3.NavMap, error) {
	if navBaseCache != nil || navBaseErr != nil {
		return navBaseCache, navBaseErr
	}
	d, err := g3.BaseFile(filepath.Join(a.gothicDir, "Data"), "Projects_compiled", g3.NavMapPath)
	if err == nil {
		navBaseCache, err = g3.ParseNavMap(d)
	}
	if err != nil {
		log.Println("Navigationskarte:", err)
	}
	navBaseErr = err
	return navBaseCache, navBaseErr
}

// zoneNavInfo berechnet Mittelpunkt (Welt) und Radius einer Zone.
func (a *app) zoneNavInfo(ii int) (g3.NavZoneInfo, bool) {
	in := &a.world.Inst[ii]
	wf, err := a.editFile(in)
	if err != nil {
		return g3.NavZoneInfo{}, false
	}
	k := in.Ent.Index
	off, r, err := wf.ZoneCircle(k)
	if err != nil {
		return g3.NavZoneInfo{}, false
	}
	m := wf.Ents[k].World
	c := [3]float32{
		off[0]*m[0] + off[1]*m[4] + off[2]*m[8] + m[12],
		off[0]*m[1] + off[1]*m[5] + off[2]*m[9] + m[13],
		off[0]*m[2] + off[1]*m[6] + off[2]*m[10] + m[14],
	}
	return g3.NavZoneInfo{GUID: wf.Ents[k].GUID, Center: c, Radius: r}, true
}

// buildModNavMap liefert die Navigationskarte fuer die Mod (nil = keine
// eigenen Zonen, die Mod braucht keine Karte) und Hinweise.
func (a *app) buildModNavMap() ([]byte, []string) {
	if a.world == nil {
		return nil, nil
	}
	base, err := a.navBase()
	if err != nil {
		return nil, []string{fmt.Sprintf(T("Achtung: Die Navigationskarte des Spiels konnte nicht gelesen werden (%s). Eigene Zonen werden im Spiel nicht erkannt."), err.Error())}
	}
	var zones []g3.NavZoneInfo
	var names, outside []string
	seen := map[[16]byte]bool{}
	for i := range a.world.Inst {
		in := &a.world.Inst[i]
		if in.Hidden || !hasClass(in.Ent, "gCNavZone_PS") || base.HasZone(in.Ent.GUID) || seen[in.Ent.GUID] {
			continue
		}
		seen[in.Ent.GUID] = true
		z, ok := a.zoneNavInfo(i)
		if !ok {
			continue
		}
		if !base.InGrid(z) {
			outside = append(outside, in.Ent.Name)
			continue
		}
		zones = append(zones, z)
		names = append(names, in.Ent.Name)
	}
	var notes []string
	if len(outside) > 0 {
		notes = append(notes, fmt.Sprintf(T("Achtung: Diese Zonen liegen außerhalb der Navigationskarte und werden im Spiel nicht erkannt: %s"), strings.Join(outside, ", ")))
	}
	if len(zones) == 0 {
		return nil, notes
	}
	out, err := base.AddZones(zones)
	if err != nil {
		return nil, append(notes, fmt.Sprintf(T("Achtung: Die eigenen Zonen konnten nicht in die Navigationskarte eingetragen werden (%s). Sie werden im Spiel nicht erkannt."), err.Error()))
	}
	log.Printf("Navigationskarte: %d eigene Zone(n) eingetragen: %s", len(zones), strings.Join(names, ", "))
	notes = append(notes, fmt.Sprintf(T("Eigene Zonen (%s) wurden in die Navigationskarte der Mod eingetragen."), strings.Join(names, ", ")))
	return out, notes
}

// otherNavMapMods nennt andere aktive Archive, die ebenfalls eine
// Navigationskarte mitbringen (nur eine kann gelten).
func (a *app) otherNavMapMods(own ...string) []string {
	var out []string
	for _, ar := range a.vfs.Archives {
		if !strings.EqualFold(strings.TrimSuffix(filepath.Base(ar.Path), filepath.Ext(ar.Path)), "Projects_compiled") {
			continue
		}
		ext := strings.ToLower(filepath.Ext(ar.Path))
		if ext == ".pak" || (len(ext) == 4 && ext[1] == 'p') {
			continue
		}
		skip := false
		for _, o := range own {
			if o != "" && strings.EqualFold(filepath.Clean(o), filepath.Clean(ar.Path)) {
				skip = true
			}
		}
		if skip || strings.EqualFold(filepath.Dir(ar.Path), filepath.Join(a.gothicDir, "Data")) == false {
			continue
		}
		for _, e := range ar.Entries {
			if strings.EqualFold(e.Name, g3.NavMapPath) && e.Size > 0 {
				out = append(out, filepath.Base(ar.Path))
				break
			}
		}
	}
	return out
}
