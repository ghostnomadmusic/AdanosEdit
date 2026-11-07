//go:build windows

package main

// Dialoge deaktivieren: Die Mod liefert eine eigene Fassung des Dialogs mit,
// die zusaetzlich ein Ereignis voraussetzt, das nie gesetzt wird. Der Dialog
// erscheint dann im Spiel nicht mehr; sonst bleibt er unveraendert.
// Wieder einschalten entfernt das Ereignis (bzw. die ganze geaenderte
// Fassung, wenn sonst nichts geaendert wurde).

import (
	"fmt"
	"sort"
	"strings"

	"g3viewer/g3"
)

// disabledEvent wird von keinem Dialog gesetzt.
const disabledEvent = "G3V_Deaktiviert"

func isDisabled(in *g3.Info) bool {
	for _, e := range splitEvents(in.CondPlayerKnows) {
		if e == disabledEvent {
			return true
		}
	}
	return false
}

// withoutDisabled liefert die Ereignisliste ohne die Sperre.
func withoutDisabled(s string) string {
	var out []string
	for _, e := range splitEvents(s) {
		if e != disabledEvent {
			out = append(out, e)
		}
	}
	return strings.Join(out, ";")
}

// withDisabled haengt die Sperre an eine Ereignisliste an.
func withDisabled(s string) string {
	s = withoutDisabled(s)
	if s == "" {
		return disabledEvent
	}
	return s + ";" + disabledEvent
}

// dlgDependents beschreibt, was am Dialog haengt (fuer die Warnung).
func (a *app) dlgDependents(in *g3.Info) []string {
	d := &a.dlg
	var out []string
	// Unterdialoge (auch tiefer verschachtelte) verschwinden mit
	group := map[*g3.Info]bool{in: true}
	for changed := true; changed; {
		changed = false
		for _, o := range d.infos {
			if group[o] || o.Parent == "" || !strings.EqualFold(o.Owner, in.Owner) {
				continue
			}
			for g := range group {
				if strings.EqualFold(o.Parent, g.Name) {
					group[o], changed = true, true
					break
				}
			}
		}
	}
	kids := len(group) - 1
	if kids > 0 {
		out = append(out, fmt.Sprintf(T("Er öffnet %d Unterdialog(e) – sie erscheinen dann ebenfalls nicht mehr."), kids))
	}
	// Quests, die der Dialog startet
	started := map[string]bool{}
	sets := map[string]bool{}
	for g := range group {
		if g.Quest != "" && g.ConditionType == g3.CondActivator {
			started[g.Quest] = true
		}
		for _, c := range g.Script {
			if c.Cmd == "RunQuest" && c.ID1 != "" {
				started[c.ID1] = true
			}
			if c.Cmd == "SetGameEvent" && c.ID1 != "" {
				sets[c.ID1] = true
			}
		}
	}
	var qs []string
	for q := range started {
		qs = append(qs, q)
	}
	sort.Strings(qs)
	for _, q := range qs {
		other, uses := 0, 0
		for _, o := range d.infos {
			if group[o] || isDisabled(o) {
				continue
			}
			runs := strings.EqualFold(o.Quest, q) && o.ConditionType == g3.CondActivator
			for _, c := range o.Script {
				if c.Cmd == "RunQuest" && strings.EqualFold(c.ID1, q) {
					runs = true
				}
			}
			if runs {
				other++
			} else if strings.EqualFold(o.Quest, q) {
				uses++
			}
		}
		follow := 0
		for _, qq := range a.qst.quests {
			for _, f := range splitEvents(qq.Get("FinishedQuests")) {
				if strings.EqualFold(f, q) {
					follow++
				}
			}
		}
		s := fmt.Sprintf(T("Er startet die Quest „%s“."), a.questTitle(q))
		if other > 0 {
			s += " " + fmt.Sprintf(T("Sie lässt sich aber auch über %d andere(n) Dialog(e) starten."), other)
		} else {
			s += " " + T("Kein anderer Dialog startet sie – sie ist dann nicht mehr zu bekommen.")
			if uses > 0 {
				s += " " + fmt.Sprintf(T("%d weitere Dialoge hängen an dieser Quest und erscheinen dann nicht mehr."), uses)
			}
			if follow > 0 {
				s += " " + fmt.Sprintf(T("%d Folgequest(s) setzen sie voraus."), follow)
			}
		}
		out = append(out, s)
	}
	// Ereignisse, auf die andere Dialoge warten
	var evs []string
	for ev := range sets {
		n, other := 0, false
		for _, o := range d.infos {
			if group[o] {
				continue
			}
			for _, c := range o.Script {
				if c.Cmd == "SetGameEvent" && c.ID1 == ev && !isDisabled(o) {
					other = true
				}
			}
			for _, e := range splitEvents(o.CondPlayerKnows) {
				if e == ev && !isDisabled(o) {
					n++
				}
			}
		}
		if n > 0 && !other {
			evs = append(evs, fmt.Sprintf(T("Er setzt das Ereignis „%s“, das %d andere(r) Dialog(e) voraussetzen – sie erscheinen dann nicht mehr."), ev, n))
		}
	}
	sort.Strings(evs)
	return append(out, evs...)
}

// toggleDisabled schaltet den gewaehlten Dialog im Spiel ab bzw. wieder an.
func (a *app) toggleDisabled() {
	d := &a.dlg
	if d.loading {
		return
	}
	on := isChecked(d.disable)
	it := a.curDialog()
	if it == nil || isDisabled(it.info) == on {
		return
	}
	revert := func() {
		d.loading = true
		setChecked(d.disable, !on)
		d.loading = false
	}
	if loadConfig()["SaveMode"] == "data" {
		revert()
		messageBox(a.hwnd, T("Dialoge brauchen eigene Archive. Das geht nur mit dem Mod Starter.\n\nStelle dafür in AdanosEdit.ini SaveMode=mds ein."), T(appTitleDE), MB_ICONWARNING)
		return
	}
	// offene Aenderungen an den Feldern zuerst uebernehmen
	if !a.commitDialog() {
		revert()
		return
	}
	it = a.curDialog()
	if it == nil {
		return
	}
	if on {
		a.loadQuests()
		deps := a.dlgDependents(it.info)
		msg := T("Diesen Dialog deaktivieren? Er erscheint im Spiel dann nicht mehr (gilt in einem neuen Spiel).")
		icon := uintptr(MB_ICONQUESTION)
		if len(deps) > 0 {
			msg += "\n\n" + T("Achtung:") + "\n• " + strings.Join(deps, "\n• ")
			icon = MB_ICONWARNING
		}
		if messageBoxRet(a.hwnd, msg, T(appTitleDE), MB_YESNO|icon) != IDYES {
			revert()
			return
		}
	}
	in := *it.info
	if on {
		in.CondPlayerKnows = withDisabled(in.CondPlayerKnows)
	} else {
		in.CondPlayerKnows = withoutDisabled(in.CondPlayerKnows)
	}
	lp := strings.ToLower(it.path)
	restored := false
	if !on && !isOwnInfo(&in) {
		// nichts sonst geaendert? Dann einfach wieder das Original
		if o := a.originalInfo(it.path); o != nil && string(o.Bytes()) == string(in.Bytes()) {
			a.ed.setExtra("Infos", it.path, nil)
			d.infos[lp] = o
			restored = true
		}
	}
	if !restored {
		a.ed.setExtra("Infos", it.path, in.Bytes())
		np := &in
		d.infos[lp] = np
	}
	d.paths[lp] = it.path
	if on {
		d.note = "✓ " + T("Deaktiviert. Zum Spielen speichern und die Mod im Mod Starter neu integrieren.")
	} else {
		d.note = "✓ " + T("Wieder aktiv. Zum Spielen speichern und die Mod im Mod Starter neu integrieren.")
	}
	cur := d.cur
	a.indexDialogs()
	a.fillDlgList()
	d.cur = cur
	sendMessage(d.list, LB_SETCURSEL, uintptr(cur), 0)
	note := d.note
	a.showDialog()
	d.note = note
	a.updateDlgHelp()
	a.updateTitle()
}
