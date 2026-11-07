//go:build windows

package main

// Reiter "Dialoge": Gespraeche von NPCs ansehen, neue anlegen und vorhandene
// aendern. Gespeichert wird als Infos.m0x/.p0x in der Mod (wie bei G3IQ eine
// .info-Datei je Gespraech), die Texte kommen in die stringtable.ini.

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
	"unsafe"

	"g3viewer/g3"
	"g3viewer/lang"
)

const (
	idTabDlg = 330 + iota
	idDlgSearch
	idDlgNPCs
	idDlgList
	idDlgNew
	idDlgDel
	idDlgType
	idDlgPerm
	idDlgCond
	idDlgText
	idDlgApply
	idDlgQuest
	idDlgPhase
	idDlgCondNot
	idDlgParent
	idDlgClear
	idDlgNeed
	idDlgGold
	idDlgDisable
)

const (
	EM_SETSEL       = 0x00B1
	ES_WANTRETURN   = 0x1000
	MB_ICONQUESTION = 0x20
	BN_CLICKED      = 0
)

// Eigene Gespraeche beginnen mit diesem Namen.
const ownInfoPrefix = "G3V_"

type dlgItem struct {
	info  *g3.Info
	path  string
	depth int // Tiefe in der Unterauswahl (0 = Hauptauswahl)
}

type dlgUI struct {
	npcLbl, search, npcs       uintptr
	listLbl, list, btnNew, del uintptr
	typeLbl, typ, perm         uintptr
	condLbl, cond, textLbl     uintptr
	condNotLbl, condNot        uintptr
	text, apply, help          uintptr
	questLbl, quest            uintptr
	phaseLbl, phase            uintptr
	parentLbl, parent, clear   uintptr // Unterauswahl: uebergeordneter Dialog, schliesst Unterauswahl
	needLbl, need, goldLbl     uintptr // Held muss Gegenstaende haben, Dialog kostet Gold
	gold                       uintptr
	disable                    uintptr  // Dialog im Spiel abschalten
	parents                    []string // Namen der Auswahl "Unter" (Index 0 = keiner)
	qnames                     []string // Quests der Auswahl (Index 0 = keine)
	phases                     []int    // Bedingungsarten der Phasen-Auswahl
	owners                     []string
	owner                      string
	keepOwner                  bool // beim naechsten Wechsel in den Reiter nicht auf den gewaehlten Welt-NPC umschalten
	items                      []dlgItem
	cur                        int
	conds                      []string // Ereignisse der Bedingungsliste (Index 0 = keine)
	snap                       string   // Inhalt der Felder beim Anzeigen
	changed, loading           bool
	note                       string

	infos   map[string]*g3.Info // Pfad (klein) -> Gespraech (Spiel + Mod + Aenderungen)
	paths   map[string]string   // Pfad (klein) -> Pfad wie im Archiv
	byOwner map[string][]string // owner (klein) -> Pfade
	strs    map[string][]string // INFO_-Texte der stringtable.ini
}

func (a *app) createDlgControls(mk func(ex uint32, class, text string, style uint32, id int) uintptr) {
	d := &a.dlg
	d.cur = -1
	a.hTabDlg = mk(0, "BUTTON", T("Dialoge"), BS_AUTORADIOBUTTON|BS_PUSHLIKE, idTabDlg)
	d.npcLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	d.search = mk(WS_EX_CLIENTEDGE, "EDIT", "", ES_AUTOHSCROLL|WS_TABSTOP, idDlgSearch)
	d.npcs = mk(WS_EX_CLIENTEDGE, "LISTBOX", "", WS_VSCROLL|LBS_NOTIFY|LBS_NOINTEGRALHEIGHT|WS_TABSTOP, idDlgNPCs)
	d.listLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	d.list = mk(WS_EX_CLIENTEDGE, "LISTBOX", "", WS_VSCROLL|LBS_NOTIFY|LBS_NOINTEGRALHEIGHT|WS_TABSTOP, idDlgList)
	d.btnNew = mk(0, "BUTTON", "", BS_PUSHBUTTON|WS_TABSTOP, idDlgNew)
	d.del = mk(0, "BUTTON", "", BS_PUSHBUTTON|WS_TABSTOP, idDlgDel)
	d.typeLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	d.typ = mk(0, "COMBOBOX", "", CBS_DROPDOWNLIST|WS_VSCROLL|WS_TABSTOP, idDlgType)
	d.perm = mk(0, "BUTTON", "", BS_AUTOCHECKBOX|WS_TABSTOP, idDlgPerm)
	d.condLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	d.cond = mk(WS_EX_CLIENTEDGE, "EDIT", "", ES_AUTOHSCROLL|WS_TABSTOP, idDlgCond)
	d.condNotLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	d.condNot = mk(WS_EX_CLIENTEDGE, "EDIT", "", ES_AUTOHSCROLL|WS_TABSTOP, idDlgCondNot)
	d.questLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	d.quest = mk(0, "COMBOBOX", "", CBS_DROPDOWNLIST|WS_VSCROLL|WS_TABSTOP, idDlgQuest)
	d.phaseLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	d.phase = mk(0, "COMBOBOX", "", CBS_DROPDOWNLIST|WS_VSCROLL|WS_TABSTOP, idDlgPhase)
	d.parentLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	d.parent = mk(0, "COMBOBOX", "", CBS_DROPDOWNLIST|WS_VSCROLL|WS_TABSTOP, idDlgParent)
	d.clear = mk(0, "BUTTON", "", BS_AUTOCHECKBOX|WS_TABSTOP, idDlgClear)
	d.disable = mk(0, "BUTTON", "", BS_AUTOCHECKBOX|WS_TABSTOP, idDlgDisable)
	d.needLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	d.need = mk(WS_EX_CLIENTEDGE, "EDIT", "", ES_AUTOHSCROLL|WS_TABSTOP, idDlgNeed)
	d.goldLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	d.gold = mk(WS_EX_CLIENTEDGE, "EDIT", "", ES_AUTOHSCROLL|WS_TABSTOP, idDlgGold)
	d.textLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	d.text = mk(WS_EX_CLIENTEDGE, "EDIT", "", ES_MULTILINE|ES_WANTRETURN|ES_AUTOVSCROLL|WS_VSCROLL|WS_TABSTOP, idDlgText)
	d.apply = mk(0, "BUTTON", "", BS_PUSHBUTTON|WS_TABSTOP, idDlgApply)
	d.help = mk(WS_EX_CLIENTEDGE, "EDIT", "", ES_MULTILINE|ES_READONLY|ES_AUTOVSCROLL|WS_VSCROLL, 0)
	a.applyDlgTexts()
}

func (a *app) applyDlgTexts() {
	d := &a.dlg
	setText(a.hTabDlg, T("Dialoge"))
	setText(d.npcLbl, T("NPC (Name suchen):"))
	setText(d.btnNew, T("Neuer Dialog"))
	setText(d.typeLbl, T("Art:"))
	setText(d.perm, T("Immer wieder wählbar"))
	setText(d.disable, T("Deaktiviert (erscheint im Spiel nicht)"))
	setText(d.condLbl, T("Erst nach:"))
	setText(d.condNotLbl, T("Nicht nach:"))
	setText(d.questLbl, T("Quest:"))
	setText(d.phaseLbl, T("Wann:"))
	setText(d.textLbl, T("Ablauf (eine Zeile pro Satz oder Aktion):"))
	setText(d.parentLbl, T("Unter:"))
	setText(d.clear, T("Schließt die Unterauswahl"))
	setText(d.needLbl, T("Held hat:"))
	setText(d.goldLbl, T("Kostet:"))
	setText(d.apply, T("Übernehmen"))
	d.loading = true
	comboFill(d.typ, []string{T("Auswahl im Gespräch (Held fragt)"), T("NPC spricht den Helden von selbst an"), T("Thema – öffnet eine Unterauswahl")}, comboSel(d.typ))
	d.loading = false
	a.updateDlgButtons()
	a.updateDlgHelp()
	if d.infos != nil {
		a.fillDlgList()
	}
}

func (a *app) dlgControls() []uintptr {
	d := &a.dlg
	return []uintptr{d.npcLbl, d.search, d.npcs, d.listLbl, d.list, d.btnNew, d.del, d.typeLbl, d.typ, d.perm,
		d.condLbl, d.cond, d.textLbl, d.text, d.apply, d.help, d.questLbl, d.quest, d.phaseLbl, d.phase,
		d.condNotLbl, d.condNot, d.parentLbl, d.parent, d.clear, d.needLbl, d.need, d.goldLbl, d.gold, d.disable}
}

// layoutDlg ordnet die Elemente des Dialog-Reiters an.
func (a *app) layoutDlg(x, y, w, bottom int) {
	d := &a.dlg
	lh := a.scale(20)
	ch := a.scale(22)
	row := ch + a.scale(6)
	// feste Hoehen: Beschriftungen, Suche, Knoepfe, 5 Feldzeilen, Haken, Uebernehmen
	fixed := lh + row + lh + a.scale(32) + 7*row + 2*(ch+a.scale(4)) + lh + a.scale(4) + a.scale(32) + a.scale(12)
	avail := bottom - y - fixed
	if avail < a.scale(260) {
		avail = a.scale(260)
	}
	nh := avail * 24 / 100
	lh2 := avail * 22 / 100
	helpH := avail * 14 / 100
	if nh < a.scale(64) {
		nh = a.scale(64)
	}
	if lh2 < a.scale(60) {
		lh2 = a.scale(60)
	}
	if helpH < a.scale(44) {
		helpH = a.scale(44)
	}
	th := avail - nh - lh2 - helpH
	if th < a.scale(90) {
		th = a.scale(90)
	}
	moveWindow(d.npcLbl, x, y, w, lh)
	y += lh
	moveWindow(d.search, x, y, w, ch+a.scale(2))
	y += row
	moveWindow(d.npcs, x, y, w, nh)
	y += nh + a.scale(4)
	moveWindow(d.listLbl, x, y, w, lh)
	y += lh
	moveWindow(d.list, x, y, w, lh2)
	y += lh2 + a.scale(4)
	bw := (w - a.scale(6)) / 2
	moveWindow(d.btnNew, x, y, bw, a.scale(26))
	moveWindow(d.del, x+bw+a.scale(6), y, w-bw-a.scale(6), a.scale(26))
	y += a.scale(32)
	lw := a.scale(80)
	moveWindow(d.typeLbl, x, y+a.scale(4), lw, lh)
	moveWindow(d.typ, x+lw, y, w-lw, a.scale(200))
	y += row
	moveWindow(d.condLbl, x, y+a.scale(4), lw, lh)
	moveWindow(d.cond, x+lw, y, w-lw, ch+a.scale(2))
	y += row
	moveWindow(d.condNotLbl, x, y+a.scale(4), lw, lh)
	moveWindow(d.condNot, x+lw, y, w-lw, ch+a.scale(2))
	y += row
	moveWindow(d.questLbl, x, y+a.scale(4), lw, lh)
	moveWindow(d.quest, x+lw, y, w-lw, a.scale(400))
	y += row
	moveWindow(d.phaseLbl, x, y+a.scale(4), lw, lh)
	moveWindow(d.phase, x+lw, y, w-lw, a.scale(240))
	y += row
	moveWindow(d.parentLbl, x, y+a.scale(4), lw, lh)
	moveWindow(d.parent, x+lw, y, w-lw, a.scale(300))
	y += row
	gw, glw := a.scale(60), a.scale(48)
	moveWindow(d.needLbl, x, y+a.scale(4), lw, lh)
	moveWindow(d.need, x+lw, y, w-lw-glw-gw-a.scale(6), ch+a.scale(2))
	moveWindow(d.goldLbl, x+w-glw-gw, y+a.scale(4), glw, lh)
	moveWindow(d.gold, x+w-gw, y, gw, ch+a.scale(2))
	y += row
	pw := (w - a.scale(6)) / 2
	moveWindow(d.clear, x+pw+a.scale(6), y, w-pw-a.scale(6), ch)
	moveWindow(d.perm, x, y, pw, ch)
	y += ch + a.scale(4)
	moveWindow(d.disable, x, y, w, ch)
	y += ch + a.scale(4)
	moveWindow(d.textLbl, x, y, w, lh)
	y += lh
	moveWindow(d.text, x, y, w, th)
	y += th + a.scale(4)
	moveWindow(d.apply, x, y, w, a.scale(28))
	y += a.scale(32)
	moveWindow(d.help, x, y, w, max(bottom-y, a.scale(40)))
}

// --- Daten ---------------------------------------------------------------

// stCol ist die Spalte der stringtable.ini fuer die Sprache der Oberflaeche.
func stCol() int {
	switch lang.Code() {
	case "en":
		return 0
	case "ru":
		return 8
	case "pl":
		return 7
	}
	return 3
}

// loadDialogs liest alle Gespraeche und die Texte (einmalig).
func (a *app) loadDialogs() {
	d := &a.dlg
	if d.infos != nil || a.vfs == nil {
		return
	}
	d.infos = map[string]*g3.Info{}
	d.paths = map[string]string{}
	for _, e := range a.vfs.List("Infos") {
		if !strings.HasSuffix(strings.ToLower(e.Name), ".info") {
			continue
		}
		b, err := e.Read()
		if err != nil {
			continue
		}
		d.infos[strings.ToLower(e.Name)] = g3.ParseInfo(b)
		d.paths[strings.ToLower(e.Name)] = e.Name
	}
	for p, b := range a.ed.extra["Infos"] {
		if b == nil {
			delete(d.infos, strings.ToLower(p))
		} else {
			d.infos[strings.ToLower(p)] = g3.ParseInfo(b)
			d.paths[strings.ToLower(p)] = p
		}
	}
	d.strs = map[string][]string{}
	if st := a.currentStringtable(); st != nil {
		u := make([]uint16, len(st)/2)
		for i := range u {
			u[i] = uint16(st[2*i]) | uint16(st[2*i+1])<<8
		}
		s := string(utf16.Decode(u))
		for _, line := range strings.Split(s, "\n") {
			if !strings.HasPrefix(line, "INFO_") && !strings.HasPrefix(line, "FO_") && !strings.HasPrefix(line, "QUEST_") {
				continue
			}
			k, v, ok := strings.Cut(strings.TrimRight(line, "\r"), "=")
			if ok {
				d.strs[k] = strings.Split(strings.TrimSuffix(v, ";"), ";;")
			}
		}
	}
	// Texte der bearbeiteten Mod (stringtableMod.ini): Sie stehen nur dann in
	// der Texttabelle des Spiels, wenn genau diese Mod integriert ist.
	for k, v := range a.modStrings() {
		d.strs[k] = strings.Split(strings.TrimSuffix(v, ";"), ";;")
	}
	for k, v := range a.ed.strs {
		d.strs[k] = v
	}
	a.indexDialogs()
}

func (a *app) indexDialogs() {
	d := &a.dlg
	d.byOwner = map[string][]string{}
	for p, in := range d.infos {
		o := strings.ToLower(in.Owner)
		d.byOwner[o] = append(d.byOwner[o], p)
	}
}

// infoText liefert den Text eines Schluessels in der Sprache der Oberflaeche.
func (a *app) infoText(key string) (string, bool) {
	v, ok := a.dlg.strs[key]
	if !ok {
		return "", false
	}
	c := stCol()
	if c < len(v) && v[c] != "" {
		return v[c], true
	}
	if len(v) > 3 && v[3] != "" {
		return v[3], true
	}
	if len(v) > 0 {
		return v[0], true
	}
	return "", true
}

// modInfoArchive ist das Dialog-Archiv der bearbeiteten Mod.
func (a *app) modInfoArchive() *g3.Pak {
	mds := a.findMDS()
	if mds == "" {
		return nil
	}
	dir := a.mdsModDir(mds)
	if dir == "" {
		return nil
	}
	p := a.modArchive(dir, "Infos")
	for _, ar := range a.vfs.Archives {
		if strings.EqualFold(ar.Path, p) {
			return ar
		}
	}
	return nil
}

// originalInfo liest ein Gespraech aus den Archiven des Spiels (ohne die Mod).
func (a *app) originalInfo(path string) *g3.Info {
	mod := a.modInfoArchive()
	for i := len(a.vfs.Archives) - 1; i >= 0; i-- {
		ar := a.vfs.Archives[i]
		if ar == mod {
			continue
		}
		if !strings.EqualFold(strings.TrimSuffix(filepath.Base(ar.Path), filepath.Ext(ar.Path)), "Infos") {
			continue
		}
		for _, e := range ar.Entries {
			if strings.EqualFold(e.Name, path) {
				if b, err := e.Read(); err == nil {
					return g3.ParseInfo(b)
				}
			}
		}
	}
	return nil
}

// isModified: Original-Gespraech, das in der Mod geaendert wurde.
func (a *app) isModified(path string) bool {
	if b, ok := a.ed.extra["Infos"][path]; ok {
		return b != nil
	}
	if m := a.modInfoArchive(); m != nil {
		for _, e := range m.Entries {
			if strings.EqualFold(e.Name, path) {
				return true
			}
		}
	}
	return false
}

func isOwnInfo(in *g3.Info) bool { return strings.HasPrefix(in.Name, ownInfoPrefix) }

// --- Oberflaeche -----------------------------------------------------------

// fillDlgNPCs listet die benannten NPCs der Welt.
func (a *app) fillDlgNPCs() {
	d := &a.dlg
	a.loadDialogs()
	sendMessage(d.npcs, LB_RESETCONTENT, 0, 0)
	d.owners = nil
	if a.world == nil {
		return
	}
	q := strings.ToLower(strings.TrimSpace(getText(d.search)))
	seen := map[string]bool{}
	for i := range a.world.Inst {
		in := &a.world.Inst[i]
		e := in.Ent
		if e.Kind != g3.KindNPC || e.Name == "" || in.Hidden {
			continue
		}
		l := strings.ToLower(e.Name)
		if seen[l] || (q != "" && !strings.Contains(l, q)) {
			continue
		}
		seen[l] = true
		d.owners = append(d.owners, e.Name)
	}
	sort.Slice(d.owners, func(i, j int) bool { return strings.ToLower(d.owners[i]) < strings.ToLower(d.owners[j]) })
	sel := -1
	for i, o := range d.owners {
		n := len(d.byOwner[strings.ToLower(o)])
		s := o
		if n > 0 {
			s = fmt.Sprintf("%s   (%d)", o, n)
		}
		sendMessage(d.npcs, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(s))))
		if strings.EqualFold(o, d.owner) {
			sel = i
		}
	}
	if sel >= 0 {
		sendMessage(d.npcs, LB_SETCURSEL, uintptr(sel), 0)
	}
}

// selectDlgNPC waehlt den NPC, dessen Gespraeche gezeigt werden.
func (a *app) selectDlgNPC() {
	d := &a.dlg
	i := comboSelList(d.npcs)
	if i < 0 || i >= len(d.owners) {
		return
	}
	if !a.commitDialog() {
		return
	}
	d.owner = d.owners[i]
	d.cur = -1
	a.fillDlgList()
	a.showDialog()
	// NPC in der Welt anzeigen
	for k := range a.world.Inst {
		if strings.EqualFold(a.world.Inst[k].Ent.Name, d.owner) && !a.world.Inst[k].Hidden {
			a.select_(int32(k), true)
			break
		}
	}
}

// dlgTitle bildet die Zeile in der Liste der Gespraeche.
func (a *app) dlgTitle(it dlgItem) string {
	in := it.info
	t := ""
	for _, c := range in.Script {
		if c.Cmd == "Say" && (c.Ent1 == "player" || t == "") {
			if s, ok := a.infoText(c.Text); ok && s != "" {
				t = s
				if c.Ent1 == "player" {
					break
				}
			}
		}
	}
	if t == "" {
		t = in.Name
	}
	if r := []rune(t); len(r) > 48 {
		t = string(r[:47]) + "…"
	}
	pre := "• "
	if isDisabled(in) {
		pre = "⊘ "
	}
	switch {
	case isDisabled(in):
	case in.InfoType == g3.InfoImportant:
		pre = "» "
	case in.InfoType == g3.InfoParent:
		pre = "▸ "
	}
	if in.GoldCost > 0 {
		t += "  " + fmt.Sprintf(T("(%d Gold)"), in.GoldCost)
	}
	switch {
	case isOwnInfo(in):
	case a.isModified(it.path):
		t += "   " + T("(geändert)")
	default:
		t += "   " + T("(Original)")
	}
	return pre + t
}

// fillDlgList listet die Gespraeche des gewaehlten NPCs.
func (a *app) fillDlgList() {
	d := &a.dlg
	sendMessage(d.list, LB_RESETCONTENT, 0, 0)
	d.items = nil
	if d.owner == "" {
		setText(d.listLbl, T("Dialoge:"))
		a.updateDlgButtons()
		return
	}
	setText(d.listLbl, fmt.Sprintf(T("Dialoge von %s:"), d.owner))
	d.items = a.dlgOrdered(d.owner)
	for _, it := range d.items {
		sendMessage(d.list, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(strings.Repeat("      ", it.depth)+a.dlgTitle(it)))))
	}
	if d.cur >= 0 && d.cur < len(d.items) {
		sendMessage(d.list, LB_SETCURSEL, uintptr(d.cur), 0)
	}
	a.updateDlgButtons()
}

func (a *app) curDialog() *dlgItem {
	d := &a.dlg
	if d.cur >= 0 && d.cur < len(d.items) {
		return &d.items[d.cur]
	}
	return nil
}

func (a *app) updateDlgButtons() {
	d := &a.dlg
	it := a.curDialog()
	en := func(h uintptr, v bool) {
		x := uintptr(0)
		if v {
			x = 1
		}
		pEnableWindow.Call(h, x)
	}
	en(d.btnNew, d.owner != "")
	setText(d.del, T("Dialog löschen"))
	switch {
	case it == nil:
		en(d.del, false)
	case isOwnInfo(it.info):
		en(d.del, true)
	case a.isModified(it.path):
		setText(d.del, T("Original wiederherstellen"))
		en(d.del, true)
	default:
		en(d.del, false)
	}
	for _, h := range []uintptr{d.typ, d.perm, d.cond, d.condNot, d.text, d.apply, d.quest, d.disable} {
		en(h, it != nil)
	}
	en(d.phase, it != nil && comboSel(d.quest) > 0)
}

// showDialog zeigt das gewaehlte Gespraech in den Feldern.
func (a *app) showDialog() {
	d := &a.dlg
	d.loading = true
	defer func() { d.loading = false }()
	d.changed = false
	it := a.curDialog()
	a.updateDlgButtons()
	if it == nil {
		setText(d.text, "")
		setText(d.cond, "")
		setText(d.condNot, "")
		setChecked(d.perm, false)
		setChecked(d.clear, false)
		setChecked(d.disable, false)
		setText(d.need, "")
		setText(d.gold, "")
		comboFill(d.parent, []string{T("(keiner – Hauptauswahl)")}, 0)
		a.updateDlgHelp()
		return
	}
	in := it.info
	ti := 0
	switch in.InfoType {
	case g3.InfoImportant:
		ti = 1
	case g3.InfoParent:
		ti = 2
	}
	a.fillDlgParents(it)
	setChecked(d.clear, in.ClearChildren)
	setText(d.need, heroItemsText(in))
	if in.GoldCost > 0 {
		setText(d.gold, strconv.Itoa(in.GoldCost))
	} else {
		setText(d.gold, "")
	}
	sendMessage(d.typ, CB_SETCURSEL, uintptr(ti), 0)
	setChecked(d.perm, in.Permanent)
	// Bedingungen: Spielereignisse (eigene Dialoge setzen ihr eigenes Ereignis)
	setText(d.cond, withoutDisabled(in.CondPlayerKnows))
	setChecked(d.disable, isDisabled(in))
	setText(d.condNot, in.CondPlayerKnowsNot)
	a.fillDlgQuest(in)
	setText(d.text, a.scriptText(in))
	d.snap = a.dlgSnapshot()
	d.note = ""
	a.updateDlgHelp()
}

// scriptCtx verbindet die Umwandlung mit den Texten und Gegenstaenden.
func (a *app) scriptCtx() scriptCtx {
	return scriptCtx{
		text: a.infoText,
		exists: func(k string) bool {
			_, ok := a.dlg.strs[k]
			return ok
		},
		items: a.itemNames(),
	}
}

func (a *app) scriptText(in *g3.Info) string { return scriptText(in, a.scriptCtx()) }

func (a *app) parseScript(text string, in *g3.Info) ([]g3.InfoCmd, map[string]string, []string, error) {
	return parseScript(text, in, a.scriptCtx())
}

// itemNames liefert die Namen aller Gegenstands-Vorlagen (klein).
func (a *app) itemNames() map[string]bool {
	if len(a.templates) == 0 {
		return nil
	}
	if a.itemSet == nil {
		a.itemSet = map[string]bool{}
		for _, t := range a.templates {
			if strings.HasPrefix(strings.ToLower(t.t.Path), "items/") || strings.HasPrefix(t.t.Name, "It_") {
				a.itemSet[strings.ToLower(t.t.Name)] = true
			}
		}
	}
	return a.itemSet
}

// commitDialog uebernimmt Aenderungen am gewaehlten Gespraech (falls vorhanden).
// false = Fehler, die Auswahl soll bleiben.
func (a *app) commitDialog() bool {
	d := &a.dlg
	if !d.changed {
		return true
	}
	// nur getippt und wieder geloescht? Dann gibt es nichts zu uebernehmen.
	if a.dlgSnapshot() == d.snap {
		d.changed = false
		return true
	}
	msg := a.applyDialog()
	if msg == "" {
		return true
	}
	if messageBoxRet(a.hwnd, msg+"\n\n"+T("Änderungen an diesem Dialog verwerfen?"), T(appTitleDE), MB_YESNO|MB_ICONWARNING) != IDYES {
		return false
	}
	// verwerfen: noch nie uebernommene neue Dialoge entfernen
	if it := a.curDialog(); it != nil {
		if _, ok := a.ed.extra["Infos"][it.path]; !ok && isOwnInfo(it.info) && a.modHas(it.path) == false {
			delete(d.infos, strings.ToLower(it.path))
			a.indexDialogs()
		}
	}
	d.changed = false
	d.note = ""
	return true
}

// modHas prueft, ob das Dialog-Archiv der Mod die Datei enthaelt.
func (a *app) modHas(path string) bool {
	if m := a.modInfoArchive(); m != nil {
		for _, e := range m.Entries {
			if strings.EqualFold(e.Name, path) {
				return true
			}
		}
	}
	return false
}

// onApplyDialog: Knopf "Uebernehmen".
func (a *app) onApplyDialog() {
	if msg := a.applyDialog(); msg != "" {
		messageBox(a.hwnd, msg, T(appTitleDE), MB_ICONWARNING)
	}
}

// applyDialog uebernimmt die Felder in das gewaehlte Gespraech.
// Rueckgabe: Fehlermeldung ("" = in Ordnung).
func (a *app) applyDialog() string {
	d := &a.dlg
	it := a.curDialog()
	if it == nil {
		return ""
	}
	fail := func(msg string) string {
		d.note = "⚠ " + msg
		a.updateDlgHelp()
		return msg
	}
	if loadConfig()["SaveMode"] == "data" {
		return fail(T("Dialoge brauchen eigene Archive. Das geht nur mit dem Mod Starter.\n\nStelle dafür in AdanosEdit.ini SaveMode=mds ein."))
	}
	old := it.info
	in := *old
	script, strs, warn, err := a.parseScript(getText(d.text), old)
	if err != nil {
		return fail(err.Error())
	}
	if len(script) == 0 {
		return fail(T("Der Dialog ist leer."))
	}
	typ := g3.InfoInfo
	switch comboSel(d.typ) {
	case 1:
		typ = g3.InfoImportant
	case 2:
		typ = g3.InfoParent
	}
	if (typ == g3.InfoInfo || typ == g3.InfoParent) && (isOwnInfo(old) || old.InfoType != g3.InfoInfo) {
		first := ""
		for _, c := range script {
			if c.Cmd == "Say" {
				first = c.Ent1
				break
			}
		}
		if first != "player" {
			return fail(T("Bei „Auswahl im Gespräch“ muss der erste Satz vom Helden kommen – er erscheint als Auswahl im Gespräch."))
		}
	}
	if old.InfoType == g3.InfoInfo || old.InfoType == g3.InfoImportant || old.InfoType == g3.InfoParent || isOwnInfo(old) {
		in.InfoType = typ
	}
	in.Permanent = isChecked(d.perm)
	if typ == g3.InfoParent {
		in.Permanent = true // ein Thema bleibt waehlbar
	}
	// Unterauswahl
	in.Parent = ""
	if pi := comboSel(d.parent); pi > 0 && pi < len(d.parents) {
		in.Parent = d.parents[pi]
	}
	in.ClearChildren = isChecked(d.clear)
	// Held muss Gegenstaende haben / Dialog kostet Gold
	cont, items, amounts, msgItems := a.parseHeroItems(getText(d.need))
	if msgItems != "" {
		return fail(msgItems)
	}
	in.CondItemContainer, in.CondItems, in.CondItemAmounts = cont, items, amounts
	in.GoldCost = 0
	if s := strings.TrimSpace(getText(d.gold)); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 0 {
			return fail(fmt.Sprintf(T("„%s“ ist keine gültige Zahl."), s))
		}
		in.GoldCost = v
	}
	in.CondPlayerKnows = eventList(withoutDisabled(getText(d.cond)))
	if isDisabled(old) {
		in.CondPlayerKnows = withDisabled(in.CondPlayerKnows) // bleibt abgeschaltet
	}
	in.CondPlayerKnowsNot = eventList(getText(d.condNot))
	var warnEv []string
	known := a.knownEvents()
	for _, c := range script {
		if c.Cmd == "SetGameEvent" {
			known[c.ID1] = true
		}
	}
	for _, ev := range append(splitEvents(in.CondPlayerKnows), splitEvents(in.CondPlayerKnowsNot)...) {
		if !known[ev] && ev != disabledEvent {
			warnEv = append(warnEv, fmt.Sprintf(T("Das Ereignis „%s“ wird von keinem Dialog gesetzt – die Bedingung kann nie eintreten (außer du setzt es selbst mit > Ereignis)."), ev))
		}
	}
	// Quest und Zeitpunkt
	if qi := comboSel(d.quest); qi > 0 && qi < len(d.qnames) {
		in.Quest = d.qnames[qi]
		if pi := comboSel(d.phase); pi >= 0 && pi < len(d.phases) {
			in.ConditionType = d.phases[pi]
		}
	} else {
		if old.Quest != "" {
			in.ConditionType = g3.CondGeneral
		}
		in.Quest = ""
	}
	if isOwnInfo(&in) {
		// Markierung fuer "Erst nach"
		script = placeMarker(script, in.Name)
	}
	in.Script = script
	// Dialoge, auf die sich die Bedingungen beziehen, muessen ihr Ereignis setzen
	// (auch bei anderen NPCs; bei aelteren Dialogen fehlte die Markierung).
	refs := map[string]bool{}
	for _, ev := range append(splitEvents(in.CondPlayerKnows), splitEvents(in.CondPlayerKnowsNot)...) {
		refs[ev] = true
	}
	for lp, o := range d.infos {
		if refs[o.Name] && isOwnInfo(o) && o != old && (!hasMarker(o) || !markerOK(o)) {
			pre := *o
			pre.Script = placeMarker(pre.Script, pre.Name)
			pp := d.paths[lp]
			if pp == "" {
				pp = g3.InfoPath(pre.Name)
			}
			a.ed.setExtra("Infos", pp, pre.Bytes())
			d.infos[lp] = &pre
		}
	}
	for k, t := range strs {
		a.ed.setString(k, []string{t})
		v := make([]string, len(g3.StringLangs))
		for i := range v {
			v[i] = t
		}
		d.strs[k] = v
	}
	a.ed.setExtra("Infos", it.path, in.Bytes())
	np := &in
	d.infos[strings.ToLower(it.path)] = np
	d.paths[strings.ToLower(it.path)] = it.path
	it.info = np
	d.changed = false
	d.note = ""
	warn = append(warn, warnEv...)
	if len(warn) > 0 {
		d.note = "⚠ " + strings.Join(warn, "\r\n⚠ ")
	} else {
		d.note = "✓ " + T("Übernommen. Zum Spielen speichern und die Mod im Mod Starter neu integrieren.")
	}
	cur := d.cur
	a.fillDlgList()
	d.cur = cur
	sendMessage(d.list, LB_SETCURSEL, uintptr(cur), 0)
	a.updateDlgButtons()
	a.updateDlgHelp()
	a.updateTitle()
	return ""
}

// newDialog legt ein neues Gespraech fuer den gewaehlten NPC an.
func (a *app) newDialog() {
	d := &a.dlg
	if d.owner == "" || !a.commitDialog() {
		return
	}
	maxSort := 90000
	names := map[string]bool{}
	for _, in := range d.infos {
		names[strings.ToLower(in.Name)] = true
		if isOwnInfo(in) && in.SortID > maxSort {
			maxSort = in.SortID
		}
	}
	base := a.ownPrefix() + npcInternalName(d.owner)
	name := ""
	for i := 1; ; i++ {
		name = fmt.Sprintf("%s_%02d", base, i)
		if !names[strings.ToLower(name)] {
			break
		}
	}
	in := &g3.Info{Name: name, SortID: maxSort + 1, Owner: d.owner, ConditionType: g3.CondGeneral,
		InfoType: g3.InfoInfo, Folder: "G3Viewer/"}
	p := g3.InfoPath(name)
	d.infos[strings.ToLower(p)] = in
	d.paths[strings.ToLower(p)] = p
	a.indexDialogs()
	a.fillDlgList()
	for i, it := range d.items {
		if it.info == in {
			d.cur = i
		}
	}
	sendMessage(d.list, LB_SETCURSEL, uintptr(d.cur), 0)
	a.showDialog()
	setText(d.text, T("Held: Wer bist du?")+"\r\n"+fmt.Sprintf(T("NPC: Ich bin %s."), d.owner))
	d.changed = true
	pSetFocus.Call(d.text)
	sendMessage(d.text, EM_SETSEL, 0, ^uintptr(0))
}

// deleteDialog loescht ein eigenes Gespraech bzw. stellt ein Original wieder her.
func (a *app) deleteDialog() {
	d := &a.dlg
	it := a.curDialog()
	if it == nil {
		return
	}
	own := isOwnInfo(it.info)
	msg := fmt.Sprintf(T("Den Dialog „%s“ löschen?"), strings.TrimPrefix(strings.TrimPrefix(a.dlgTitle(*it), "• "), "» "))
	if !own {
		msg = T("Die Änderungen an diesem Original-Dialog verwerfen?")
	}
	var kids []string
	if own {
		for lp, o := range d.infos {
			if strings.EqualFold(o.Parent, it.info.Name) && strings.EqualFold(o.Owner, it.info.Owner) {
				kids = append(kids, lp)
			}
		}
		if len(kids) > 0 {
			msg += "\n\n" + fmt.Sprintf(T("Er hat %d Unterdialog(e). Sie kommen danach in die Hauptauswahl."), len(kids))
		}
	}
	if messageBoxRet(a.hwnd, msg, T(appTitleDE), MB_YESNO|MB_ICONQUESTION) != IDYES {
		return
	}
	for _, lp := range kids {
		k := *d.infos[lp]
		k.Parent = ""
		pp := d.paths[lp]
		if pp == "" {
			pp = g3.InfoPath(k.Name)
		}
		a.ed.setExtra("Infos", pp, k.Bytes())
		d.infos[lp] = &k
	}
	a.ed.setExtra("Infos", it.path, nil)
	if own {
		delete(d.infos, strings.ToLower(it.path))
		d.cur = -1
	} else if o := a.originalInfo(it.path); o != nil {
		d.infos[strings.ToLower(it.path)] = o
	}
	d.changed = false
	a.indexDialogs()
	a.fillDlgList()
	a.showDialog()
	a.fillDlgNPCsKeep()
	a.updateTitle()
}

// fillDlgNPCsKeep aktualisiert die NPC-Liste (Anzahl der Dialoge).
func (a *app) fillDlgNPCsKeep() {
	top := sendMessage(a.dlg.npcs, LB_GETTOPINDEX, 0, 0)
	a.fillDlgNPCs()
	sendMessage(a.dlg.npcs, LB_SETTOPINDEX, top, 0)
}

const (
	LB_GETTOPINDEX = 0x018E
	LB_SETTOPINDEX = 0x0197
)

func (a *app) updateDlgHelp() {
	d := &a.dlg
	var b strings.Builder
	if d.note != "" {
		b.WriteString(d.note + "\r\n\r\n")
	}
	b.WriteString(T("So schreibst du den Ablauf:\r\nHeld: Was gibt es Neues?\r\nNPC: Nicht viel.\r\nNPC (SD_Angry): Und jetzt verschwinde!\r\n\r\nAktionen beginnen mit >:\r\n> Geben It_Gold 50  (NPC gibt dem Helden)\r\n> Nehmen It_Apple 2  (Held gibt dem NPC)\r\n> Erfahrung 100\r\n> Ereignis Name  (setzt ein Spielereignis)\r\n> Ende  (beendet das Gespräch)\r\n> !Befehl;Wer;Wen;ID1;ID2;Text  (jeder Befehl aus G3IQ)\r\n\r\nBei „Auswahl im Gespräch“ ist der erste Satz des Helden der Text in der Auswahl. „Erst nach“ zeigt den Dialog erst, wenn der gewählte Dialog geführt wurde. » = NPC spricht an, • = Auswahl.\r\nNeue Sätze haben keine Sprachaufnahme; sie erscheinen als Untertitel. Unveränderte Original-Sätze behalten ihre Stimme."))
	b.WriteString("\r\n\r\n" + T("> Tagesablauf Name Ablauf  (schickt einen NPC in einen anderen Tagesablauf, angelegt im Reiter NPCs; „npc“ = der Gesprächspartner)"))
	b.WriteString("\r\n\r\n" + T("> Handel an / > Lehren an  (öffnet beim NPC die Auswahl „Handeln“ bzw. „Lehren“; „aus“ schließt sie wieder). Was er lehrt, legst du im Reiter NPCs mit „Händler / Lehrer …“ fest."))
	b.WriteString("\r\n\r\n" + T("Verzweigte Gespräche: Bei „Unter“ einen anderen Dialog desselben NPCs wählen – dieser Dialog erscheint dann erst als Unterauswahl, nachdem der andere gewählt wurde. Die Art „Thema“ ist ein reiner Menüpunkt (z. B. „Erzähl mir von der Stadt.“), der immer wählbar bleibt und seine Unterdialoge öffnet. „Schließt die Unterauswahl“: Nach diesem Dialog geht es zurück zur Hauptauswahl (z. B. bei „Das war alles.“). In der Liste stehen Unterdialoge eingerückt unter ihrem Dialog."))
	b.WriteString("\r\n\r\n" + T("„Held hat“: Der Dialog erscheint nur, wenn der Held diese Gegenstände dabeihat, z. B. „It_Gold 100; It_Apple 2“ (er gibt sie dabei nicht ab – dafür > Nehmen verwenden). „Kostet“: So viel Gold zieht das Spiel dem Helden ab, wenn er diese Auswahl nimmt (wie bei Lehrern im Originalspiel)."))
	b.WriteString("\r\n\r\n" + T("„Deaktiviert“: Der Dialog (auch ein Original-Dialog) erscheint im Spiel nicht mehr; in der Liste steht ⊘ davor. Vorher zeigt AdanosEdit, was daran hängt (Quests, Unterdialoge, andere Dialoge). Häkchen entfernen schaltet ihn wieder ein."))
	b.WriteString("\r\n\r\n" + T("„Erst nach“ und „Nicht nach“: Tippe einen Teil des NPC-Namens oder Dialogs (z. B. „Mortimer“) – die Liste zeigt eigene Dialoge aller NPCs und alle Ereignisse des Spiels. „Erst nach“: Der Dialog erscheint erst, wenn das Ereignis gesetzt ist (der gewählte Dialog geführt wurde). „Nicht nach“: Er verschwindet, sobald es gesetzt ist. Mehrere Ereignisse lassen sich wie im Originalspiel mit ; trennen."))
	setText(d.help, b.String())
}

// onDlgTab wird beim Wechsel auf den Dialog-Reiter aufgerufen.
func (a *app) onDlgTab() {
	defer func() { a.dlg.keepOwner = false }()
	a.fillDlgNPCs()
	a.fillDlgList()
	a.showDialog()
	// gewaehlten NPC der Welt uebernehmen
	if a.selected >= 0 && a.world != nil && int(a.selected) < len(a.world.Inst) {
		e := a.world.Inst[a.selected].Ent
		if a.dlg.keepOwner {
			// gerade erzeugte Quest-Dialoge zeigen, nicht den gewaehlten NPC
		} else if e.Kind == g3.KindNPC && e.Name != "" && !strings.EqualFold(e.Name, a.dlg.owner) && a.commitDialog() {
			a.dlg.owner = e.Name
			a.dlg.cur = -1
			a.fillDlgNPCs()
			a.fillDlgList()
			a.showDialog()
		}
	}
}

// hasMarker: der Dialog setzt beim Fuehren sein eigenes Ereignis.
func hasMarker(in *g3.Info) bool {
	for _, c := range in.Script {
		if c.Cmd == "SetGameEvent" && c.ID1 == in.Name {
			return true
		}
	}
	return false
}

// fixMarkers ergaenzt die Ereignis-Markierung bei eigenen Dialogen, auf die
// ein anderer Dialog mit "Erst nach" wartet (vor dem Speichern).
func (a *app) fixMarkers() {
	d := &a.dlg
	if d.infos == nil && a.vfs != nil && a.modInfoArchive() != nil {
		a.loadDialogs()
	}
	if d.infos == nil {
		return
	}
	need := map[string]bool{}
	for _, in := range d.infos {
		for _, ev := range append(splitEvents(in.CondPlayerKnows), splitEvents(in.CondPlayerKnowsNot)...) {
			need[ev] = true
		}
	}
	for lp, in := range d.infos {
		if !isOwnInfo(in) || !hasMarker(in) && !need[in.Name] || hasMarker(in) && markerOK(in) {
			continue
		}
		p := d.paths[lp]
		if p == "" {
			p = g3.InfoPath(in.Name)
		}
		fx := *in
		fx.Script = placeMarker(fx.Script, fx.Name)
		a.ed.setExtra("Infos", p, fx.Bytes())
		d.infos[lp] = &fx
	}
}

// placeMarker setzt die Ereignis-Markierung direkt hinter den ersten Satz.
// (Der erste Eintrag des Ablaufs liefert den Text in der Auswahl des
// Gespraechs; stuende die Markierung vorne, waere die Auswahl leer.)
func placeMarker(sc []g3.InfoCmd, name string) []g3.InfoCmd {
	var out []g3.InfoCmd
	for _, c := range sc {
		if !(c.Cmd == "SetGameEvent" && c.ID1 == name) {
			out = append(out, c)
		}
	}
	at := 0
	if len(out) > 0 && out[0].Cmd == "Say" {
		at = 1
	}
	m := g3.InfoCmd{Cmd: "SetGameEvent", ID1: name}
	return append(out[:at], append([]g3.InfoCmd{m}, out[at:]...)...)
}

// markerOK: Markierung steht an der richtigen Stelle.
func markerOK(in *g3.Info) bool {
	p := placeMarker(in.Script, in.Name)
	if len(p) != len(in.Script) {
		return false
	}
	for i := range p {
		if p[i] != in.Script[i] {
			return false
		}
	}
	return true
}

// questPhases sind die Zeitpunkte, zu denen ein Gespraech einer Quest erscheint.
var questPhases = []struct {
	label string
	ct    int
}{
	{"Quest vergeben (startet die Quest)", g3.CondActivator},
	{"Solange die Quest läuft", g3.CondRunning},
	{"Quest abgeben (Aufgabe erfüllt)", g3.CondDelivery},
	{"Nach erfolgreicher Quest", g3.CondSuccess},
	{"Wenn die Quest gescheitert ist", g3.CondFailed},
	{"Quest angeboten, noch nicht angenommen", g3.CondOpen},
}

// fillDlgQuest fuellt Quest- und Zeitpunkt-Auswahl fuer ein Gespraech.
func (a *app) fillDlgQuest(in *g3.Info) {
	d := &a.dlg
	a.loadQuests()
	items := []string{T("(keine Quest)")}
	d.qnames = []string{""}
	sel := 0
	add := func(name, label string) {
		d.qnames = append(d.qnames, name)
		items = append(items, label)
		if strings.EqualFold(name, in.Quest) {
			sel = len(d.qnames) - 1
		}
	}
	own, other := a.questNames()
	for _, n := range own {
		add(n, a.questTitle(n))
	}
	for _, n := range other {
		add(n, n)
	}
	if in.Quest != "" && sel == 0 {
		add(in.Quest, in.Quest)
	}
	comboFill(d.quest, items, sel)
	items = nil
	d.phases = nil
	psel := 0
	for _, p := range questPhases {
		if p.ct == in.ConditionType {
			psel = len(d.phases)
		}
		d.phases = append(d.phases, p.ct)
		items = append(items, T(p.label))
	}
	if in.Quest != "" && d.phases[psel] != in.ConditionType {
		d.phases = append(d.phases, in.ConditionType)
		items = append(items, fmt.Sprintf(T("Andere Bedingung (%d)"), in.ConditionType))
		psel = len(d.phases) - 1
	}
	comboFill(d.phase, items, psel)
	en := uintptr(0)
	if sel > 0 {
		en = 1
	}
	pEnableWindow.Call(d.phase, en)
}

// splitEvents zerlegt eine Ereignisliste ("A;B").
func splitEvents(s string) []string {
	var out []string
	for _, e := range strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == ',' || r == ' ' }) {
		if e != "" {
			out = append(out, e)
		}
	}
	return out
}

// eventList normalisiert eine Eingabe zu "A;B".
func eventList(s string) string { return strings.Join(splitEvents(s), ";") }

// knownEvents: alle Ereignisse, die irgendein Dialog setzt (inkl. der
// Markierungen eigener Dialoge).
func (a *app) knownEvents() map[string]bool {
	m := map[string]bool{}
	for _, in := range a.dlg.infos {
		if isOwnInfo(in) {
			m[in.Name] = true
		}
		for _, c := range in.Script {
			if c.Cmd == "SetGameEvent" && c.ID1 != "" {
				m[c.ID1] = true
			}
		}
	}
	return m
}

// eventItems liefert die Vorschlaege fuer "Erst nach" / "Nicht nach":
// eigene Dialoge aller NPCs und alle Ereignisse des Spiels.
func (a *app) eventItems() []acItem {
	a.loadDialogs()
	var own, other []acItem
	seen := map[string]bool{}
	for lp, in := range a.dlg.infos {
		if !isOwnInfo(in) {
			continue
		}
		p := a.dlg.paths[lp]
		label := in.Name + "  –  " + in.Owner + ": " + strings.TrimPrefix(strings.TrimPrefix(a.dlgTitle(dlgItem{info: in, path: p}), "• "), "» ")
		own = append(own, acItem{name: in.Name, label: label, lower: strings.ToLower(in.Name), labelLower: strings.ToLower(label)})
		seen[in.Name] = true
	}
	for ev := range a.knownEvents() {
		if seen[ev] {
			continue
		}
		other = append(other, acItem{name: ev, label: ev, lower: strings.ToLower(ev), labelLower: strings.ToLower(ev), prio: 1})
	}
	sort.Slice(own, func(i, j int) bool { return own[i].labelLower < own[j].labelLower })
	sort.Slice(other, func(i, j int) bool { return other[i].lower < other[j].lower })
	return append(own, other...)
}

// dlgSnapshot fasst den Inhalt aller Felder eines Dialogs zusammen.
func (a *app) dlgSnapshot() string {
	d := &a.dlg
	return fmt.Sprintf("%d|%v|%s|%s|%d|%d|%s|%d|%v|%s|%s", comboSel(d.typ), isChecked(d.perm), getText(d.cond), getText(d.condNot),
		comboSel(d.quest), comboSel(d.phase), getText(d.text), comboSel(d.parent), isChecked(d.clear), getText(d.need), getText(d.gold))
}
