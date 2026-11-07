//go:build windows

package main

// Wiederherstellen (Strg+Y): Jeder Rueckgaengig-Schritt kennt die Objekte,
// die seine Aktion veraendert hat (gesammelt ueber syncFile). Vor dem
// Rueckgaengigmachen merkt sich der Editor deren Zustand; Strg+Y setzt ihn
// wieder ein. Eine neue Aktion leert die Wiederherstellen-Liste.

import (
	"g3viewer/g3"
)

type entKey struct {
	wf *g3.WorldFile
	k  int
}

type entState struct {
	entKey
	raw []byte
	del bool
	ent g3.Entity
}

type redoEntry struct {
	keys  []entKey
	state []entState // Zustand nach der Aktion
}

// noteChanged merkt sich die in syncFile abgeglichenen Objekte.
func (a *app) noteChanged(wf *g3.WorldFile, changed map[int]bool) {
	a.alignUndo()
	if a.ed.pending == nil {
		a.ed.pending = map[entKey]bool{}
	}
	for k := range changed {
		a.ed.pending[entKey{wf, k}] = true
	}
}

// alignUndo ordnet neu angelegten Rueckgaengig-Schritten die zuletzt
// veraenderten Objekte zu. Neue Schritte aus einer Aktion leeren die
// Wiederherstellen-Liste.
func (a *app) alignUndo() {
	ed := a.ed
	if len(ed.undoKeys) > len(ed.undo) {
		ed.undoKeys = ed.undoKeys[:len(ed.undo)]
	}
	if len(ed.undoKeys) == len(ed.undo) {
		return
	}
	var keys []entKey
	for k := range ed.pending {
		keys = append(keys, k)
	}
	for len(ed.undoKeys) < len(ed.undo)-1 {
		ed.undoKeys = append(ed.undoKeys, nil)
	}
	ed.undoKeys = append(ed.undoKeys, keys)
	ed.pending = nil
	if !ed.redoPush {
		ed.redo = nil
	}
	ed.redoPush = false
}

func snapshotEnts(keys []entKey) []entState {
	out := make([]entState, 0, len(keys))
	for _, key := range keys {
		if key.k < 0 || key.k >= len(key.wf.Ents) {
			continue
		}
		out = append(out, entState{key, key.wf.RawCopy(key.k), key.wf.Deleted[key.k], *key.wf.Ents[key.k]})
	}
	return out
}

func (a *app) restoreEnts(st []entState) {
	files := map[*g3.WorldFile]bool{}
	for _, s := range st {
		s.wf.RestoreRaw(s.k, s.raw)
		s.wf.Deleted[s.k] = s.del
		*s.wf.Ents[s.k] = s.ent
		files[s.wf] = true
	}
	for wf := range files {
		a.syncFile(wf)
	}
	a.ed.pending = nil
}

// popUndo nimmt den letzten Schritt und merkt ihn fuer Strg+Y.
func (a *app) popUndo() func() {
	a.alignUndo()
	ed := a.ed
	n := len(ed.undo) - 1
	f, keys := ed.undo[n], ed.undoKeys[n]
	ed.undo, ed.undoKeys = ed.undo[:n], ed.undoKeys[:n]
	after := snapshotEnts(keys)
	return func() {
		ed.pending = nil
		f()
		ed.pending = nil
		if len(keys) > 0 {
			ed.redo = append(ed.redo, redoEntry{keys, after})
		}
	}
}

// redoLast: Strg+Y.
func (a *app) redoLast() {
	a.alignUndo()
	ed := a.ed
	if len(ed.redo) == 0 {
		a.flash(T("Nichts zum Wiederherstellen."))
		return
	}
	r := ed.redo[len(ed.redo)-1]
	ed.redo = ed.redo[:len(ed.redo)-1]
	before := snapshotEnts(r.keys)
	a.restoreEnts(r.state)
	ed.undo = append(ed.undo, func() { a.restoreEnts(before) })
	ed.undoKeys = append(ed.undoKeys, r.keys)
	if a.selected >= 0 && a.world != nil && a.world.Inst[a.selected].Hidden {
		a.select_(-1, false)
	} else if a.selected >= 0 {
		a.select_(a.selected, false)
	}
	a.flash(T("Wiederhergestellt (Strg+Z macht es wieder rückgängig)."))
}
