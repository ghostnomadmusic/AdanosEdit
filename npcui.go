//go:build windows

package main

// Reiter "NPCs": neuen NPC nach dem Vorbild eines vorhandenen erstellen
// (Name, Kopf, Haare, Bart, Kleidung, Haendlerwaren, Routinepunkt).

import (
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unsafe"

	"g3viewer/g3"
	"g3viewer/scene"
)

const (
	BS_PUSHLIKE = 0x1000
	SW_HIDE     = 0
)

const (
	idTabWorld = 300 + iota
	idTabNPC
	idNpcSearch
	idNpcList
	idNpcName
	idNpcHead
	idNpcHair
	idNpcBeard
	idNpcBody
	idNpcTrade
	idNpcPoint
	idNpcCreate
	idNpcRelax
	idNpcSleep
	idNpcPickWork
	idNpcPickRelax
	idNpcPickSleep
	idNpcWeap
	idNpcLoot
	idNpcPickp
	idNpcItems
	idNpcEquip
	idNpcAlign
	idNpcEncl
	idNpcRout
	idNpcTeach
)

// Vorlagen fuer den NPC-Baukasten (beim Einlesen der Vorlagen gefuellt).
type npcPartLists struct {
	heads, hairs, beards, bodies, points []*g3.Template
	traders                              []string
	chestSets                            []string       // Schatzsets fuer Truhen
	zone                                 *g3.Template   // Vorlage fuer Zonen (Nav_Zone)
	perks                                []*g3.Template // Faehigkeiten (Skills/.../Perk_*), fuer Lehrer
	weaponry, plunder, pickpocket        []string
	byName                               map[string]*g3.Template
}

var npcParts npcPartLists

// collectNPCPart ordnet eine Vorlage den Listen des NPC-Baukastens zu.
func collectNPCPart(l *npcPartLists, t *g3.Template) {
	if l.byName == nil {
		l.byName = map[string]*g3.Template{}
	}
	p := strings.ToLower(t.Path)
	n := t.Name
	if n == "Nav_Zone" && strings.HasPrefix(p, "navigation/") {
		l.zone = t
	}
	if strings.HasPrefix(p, "skills/") && strings.HasPrefix(n, "Perk_") {
		l.perks = append(l.perks, t)
	}
	switch {
	case strings.HasPrefix(p, "npc/__master_humans/") && t.Problem == "":
		l.byName[n] = t
		switch {
		case strings.HasPrefix(n, "Head_"):
			l.heads = append(l.heads, t)
		case strings.HasPrefix(n, "Hair_"):
			l.hairs = append(l.hairs, t)
		case strings.HasPrefix(n, "Beard_"):
			l.beards = append(l.beards, t)
		case strings.HasPrefix(n, "Body_"):
			l.bodies = append(l.bodies, t)
		}
	case strings.HasPrefix(p, "navigation/navigation/nav_freepoints_") && t.Problem == "" && !strings.HasPrefix(n, "__"):
		l.byName[n] = t
		l.points = append(l.points, t)
	case strings.HasPrefix(p, "treasure/trader/") && strings.HasPrefix(n, "TS_Trader"):
		l.traders = append(l.traders, n)
	case strings.HasPrefix(p, "treasure/") && strings.HasPrefix(n, "TS_Weaponry_"):
		l.weaponry = append(l.weaponry, n)
	case strings.HasPrefix(p, "treasure/") && strings.HasPrefix(n, "TS_Plunder_"):
		l.plunder = append(l.plunder, n)
	case strings.HasPrefix(p, "treasure/") && strings.HasPrefix(n, "TS_PickPocket_"):
		l.pickpocket = append(l.pickpocket, n)
	case strings.HasPrefix(p, "treasure/") && (strings.HasPrefix(n, "TS_Treasure_") || strings.HasPrefix(n, "TS_Unique_")):
		l.chestSets = append(l.chestSets, n)
	}
}

func sortParts(l *npcPartLists) {
	for _, s := range [][]*g3.Template{l.heads, l.hairs, l.beards, l.bodies, l.points} {
		sort.Slice(s, func(i, j int) bool { return s[i].Name < s[j].Name })
	}
	sort.Strings(l.traders)
	sort.Strings(l.weaponry)
	sort.Strings(l.plunder)
	sort.Strings(l.pickpocket)
	sort.Strings(l.chestSets)
}

// npcUI haelt die Bedienelemente des NPC-Reiters.
type npcUI struct {
	baseLbl, search, list                         uintptr
	nameLbl, name                                 uintptr
	headLbl, head, hairLbl, hair, beardLbl, beard uintptr
	bodyLbl, body, tradeLbl, trade, pointLbl      uintptr
	point, create, info                           uintptr
	relaxLbl, relax, sleepLbl, sleep              uintptr
	routLbl, pickWork, pickRelax, pickSleep       uintptr
	rout                                          uintptr  // Auswahl des Tagesablaufs (Start, weitere …)
	routNames                                     []string // Ablaeufe des gewaehlten NPCs
	routName                                      string   // bearbeiteter Ablauf ("" = aktueller)
	routNPC                                       int32
	routCur                                       string // Ablauf beim Spielstart
	routDel                                       string // per Auswahl loeschbarer Ablauf ("" = keiner)
	weapLbl, weap, lootLbl, loot                  uintptr
	pickpLbl, pickp, itemsLbl, items, equip       uintptr
	teach                                         uintptr // Haendler / Lehrer …
	alignLbl, align, encl                         uintptr
	enclaves                                      []enclaveRef
	pick                                          int   // 1..3: naechster Klick in der Welt setzt diesen Platz
	pickNPC                                       int32 // NPC, dessen Tagesablauf geaendert wird
	bases                                         []baseRef
	cur                                           *baseRef       // gewaehltes Vorbild
	baseParts                                     map[int]string // Platz -> Vorlagenname des Vorbilds
	hairList, beardList                           []*g3.Template
	filled                                        bool
}

func (a *app) createNPCControls(mk func(ex uint32, class, text string, style uint32, id int) uintptr) {
	n := &a.npc
	a.hTabWorld = mk(0, "BUTTON", T("Welt"), BS_AUTORADIOBUTTON|BS_PUSHLIKE|WS_GROUP|WS_TABSTOP, idTabWorld)
	a.hTabNPC = mk(0, "BUTTON", T("NPCs"), BS_AUTORADIOBUTTON|BS_PUSHLIKE, idTabNPC)
	setChecked(a.hTabWorld, true)
	n.baseLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	n.search = mk(WS_EX_CLIENTEDGE, "EDIT", "", ES_AUTOHSCROLL|WS_TABSTOP, idNpcSearch)
	n.list = mk(WS_EX_CLIENTEDGE, "LISTBOX", "", WS_VSCROLL|LBS_NOTIFY|LBS_NOINTEGRALHEIGHT|WS_TABSTOP, idNpcList)
	n.nameLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	n.name = mk(WS_EX_CLIENTEDGE, "EDIT", "", ES_AUTOHSCROLL|WS_TABSTOP, idNpcName)
	combo := func(id int) uintptr {
		return mk(0, "COMBOBOX", "", CBS_DROPDOWNLIST|WS_VSCROLL|WS_TABSTOP, id)
	}
	n.headLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	n.head = combo(idNpcHead)
	n.hairLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	n.hair = combo(idNpcHair)
	n.beardLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	n.beard = combo(idNpcBeard)
	n.bodyLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	n.body = combo(idNpcBody)
	n.tradeLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	n.trade = combo(idNpcTrade)
	n.pointLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	n.point = combo(idNpcPoint)
	n.weapLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	n.weap = combo(idNpcWeap)
	n.lootLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	n.loot = combo(idNpcLoot)
	n.pickpLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	n.pickp = combo(idNpcPickp)
	n.alignLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	n.align = combo(idNpcAlign)
	n.encl = combo(idNpcEncl)
	n.itemsLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	n.items = mk(WS_EX_CLIENTEDGE, "EDIT", "", ES_AUTOHSCROLL|WS_TABSTOP, idNpcItems)
	n.equip = mk(0, "BUTTON", "", BS_PUSHBUTTON|WS_TABSTOP, idNpcEquip)
	n.teach = mk(0, "BUTTON", "", BS_PUSHBUTTON|WS_TABSTOP, idNpcTeach)
	n.relaxLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	n.relax = combo(idNpcRelax)
	n.sleepLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	n.sleep = combo(idNpcSleep)
	n.routLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	n.rout = combo(idNpcRout)
	n.routNPC = -1
	n.pickWork = mk(0, "BUTTON", "", BS_PUSHBUTTON|WS_TABSTOP, idNpcPickWork)
	n.pickRelax = mk(0, "BUTTON", "", BS_PUSHBUTTON|WS_TABSTOP, idNpcPickRelax)
	n.pickSleep = mk(0, "BUTTON", "", BS_PUSHBUTTON|WS_TABSTOP, idNpcPickSleep)
	n.create = mk(0, "BUTTON", "", BS_PUSHBUTTON|WS_TABSTOP, idNpcCreate)
	n.info = mk(WS_EX_CLIENTEDGE, "EDIT", "", ES_MULTILINE|ES_READONLY|ES_AUTOVSCROLL|WS_VSCROLL, 0)
	a.applyNPCTexts()
}

func (a *app) applyNPCTexts() {
	n := &a.npc
	setText(a.hTabWorld, T("Welt"))
	setText(a.hTabNPC, T("NPCs"))
	setText(n.baseLbl, T("Vorbild-NPC aus der Welt (Name suchen):"))
	setText(n.nameLbl, T("Name:"))
	setText(n.headLbl, T("Kopf:"))
	setText(n.hairLbl, T("Haare:"))
	setText(n.beardLbl, T("Bart:"))
	setText(n.bodyLbl, T("Kleidung:"))
	setText(n.tradeLbl, T("Händler:"))
	setText(n.pointLbl, T("Arbeit:"))
	setText(n.relaxLbl, T("Freizeit:"))
	setText(n.weapLbl, T("Waffen:"))
	setText(n.lootLbl, T("Beute:"))
	setText(n.pickpLbl, T("Taschen:"))
	setText(n.itemsLbl, T("Gegenst.:"))
	setText(n.alignLbl, T("Gruppe:"))
	setText(n.equip, T("↑ Auf gewählten NPC anwenden"))
	setText(n.teach, T("Händler / Lehrer …"))
	setText(n.sleepLbl, T("Schlaf:"))
	setText(n.routLbl, T("Tagesablauf des gewählten NPCs ändern:"))
	a.fillRoutineCombo()
	setText(n.pickWork, T("Arbeit …"))
	setText(n.pickRelax, T("Freizeit …"))
	setText(n.pickSleep, T("Schlaf …"))
	setText(n.create, T("NPC in der Bildmitte erstellen"))
	if n.filled {
		a.fillNPCCombos()
	}
	a.updateNPCInfo()
}

func (a *app) worldControls() []uintptr {
	l := []uintptr{a.hModeWorld, a.hModeTpl, a.hSearchLbl, a.hSearch, a.hResults, a.hPlace,
		a.hDistLbl, a.hDistMinus, a.hDistPlus, a.hEditLbl, a.hLblX, a.hLblY, a.hLblZ, a.hLblRot,
		a.hEdX, a.hEdY, a.hEdZ, a.hEdRot, a.hApply, a.hCopy, a.hDelete, a.hUndo, a.hClearNPC, a.hChest, a.hInfoLbl, a.hInfo}
	for _, h := range a.hChk {
		l = append(l, h)
	}
	return l
}

func (a *app) npcControls() []uintptr {
	n := &a.npc
	return []uintptr{n.baseLbl, n.search, n.list, n.nameLbl, n.name, n.headLbl, n.head, n.hairLbl, n.hair,
		n.beardLbl, n.beard, n.bodyLbl, n.body, n.tradeLbl, n.trade, n.pointLbl, n.point, n.create, n.info,
		n.relaxLbl, n.relax, n.sleepLbl, n.sleep, n.routLbl, n.rout, n.pickWork, n.pickRelax, n.pickSleep,
		n.weapLbl, n.weap, n.lootLbl, n.loot, n.pickpLbl, n.pickp, n.itemsLbl, n.items, n.equip, n.teach,
		n.alignLbl, n.align, n.encl}
}

// setTab wechselt zwischen den Reitern Welt (0) und NPCs (1).
func (a *app) setTab(t int) {
	a.hideAC()
	a.tab = t
	setChecked(a.hTabWorld, t == 0)
	setChecked(a.hTabNPC, t == 1)
	setChecked(a.hTabDlg, t == 2)
	setChecked(a.hTabQst, t == 3)
	show := func(l []uintptr, v bool) {
		c := uintptr(SW_HIDE)
		if v {
			c = SW_SHOW
		}
		for _, h := range l {
			pShowWindow.Call(h, c)
		}
	}
	show(a.worldControls(), t == 0)
	show(a.npcControls(), t == 1)
	show(a.dlgControls(), t == 2)
	show(a.qstControls(), t == 3)
	if t != 2 {
		a.commitDialog()
	}
	if t != 3 {
		a.commitQuest()
	}
	if t == 1 {
		if !a.npc.filled {
			a.fillNPCCombos()
		}
		if len(a.npc.enclaves) == 0 {
			a.fillEnclaves()
		}
		a.searchNPCBases()
	}
	if t == 2 {
		a.onDlgTab()
	}
	if t == 3 {
		a.onQstTab()
	}
	a.layout()
	// alles neu zeichnen (sonst bleiben beim Wechsel Reste stehen)
	pRedrawWindow.Call(a.hwnd, 0, 0, 0x0001|0x0004|0x0080) // RDW_INVALIDATE|RDW_ERASE|RDW_ALLCHILDREN
}

// layoutNPC ordnet die Elemente des NPC-Reiters an; liefert die naechste y-Position.
func (a *app) layoutNPC(x, y, w, bottom int) {
	n := &a.npc
	lh := a.scale(20)
	ch := a.scale(22)
	gap := a.scale(4)
	moveWindow(n.baseLbl, x, y, w, lh)
	y += lh
	moveWindow(n.search, x, y, w, ch+a.scale(2))
	y += ch + a.scale(6)
	// feste Hoehe der Felder unterhalb der Liste
	rows := 14
	formH := (rows+1)*(ch+a.scale(6)) + a.scale(34) + a.scale(54) + a.scale(34) + a.scale(70)
	listH := bottom - y - formH
	if listH < a.scale(80) {
		listH = a.scale(80)
	}
	moveWindow(n.list, x, y, w, listH)
	y += listH + a.scale(8)
	lw := a.scale(70)
	row := func(lbl, ctl uintptr, combo bool) {
		moveWindow(lbl, x, y+a.scale(4), lw, lh)
		hh := ch + a.scale(2)
		if combo {
			hh = a.scale(320)
		}
		moveWindow(ctl, x+lw, y, w-lw, hh)
		y += ch + a.scale(6)
	}
	row(n.nameLbl, n.name, false)
	row(n.headLbl, n.head, true)
	row(n.hairLbl, n.hair, true)
	row(n.beardLbl, n.beard, true)
	row(n.bodyLbl, n.body, true)
	// Gruppe: Ausrichtung und Enklave nebeneinander
	moveWindow(n.alignLbl, x, y+a.scale(4), lw, lh)
	hw := (w - lw - a.scale(4)) / 2
	moveWindow(n.align, x+lw, y, hw, a.scale(320))
	moveWindow(n.encl, x+lw+hw+a.scale(4), y, w-lw-hw-a.scale(4), a.scale(320))
	y += ch + a.scale(6)
	row(n.tradeLbl, n.trade, true)
	row(n.weapLbl, n.weap, true)
	row(n.lootLbl, n.loot, true)
	row(n.pickpLbl, n.pickp, true)
	row(n.itemsLbl, n.items, false)
	ew := (w - lw) * 3 / 5
	moveWindow(n.equip, x+lw, y-a.scale(2), ew, a.scale(26))
	moveWindow(n.teach, x+lw+ew+a.scale(4), y-a.scale(2), w-lw-ew-a.scale(4), a.scale(26))
	y += a.scale(30)
	row(n.pointLbl, n.point, true)
	row(n.relaxLbl, n.relax, true)
	row(n.sleepLbl, n.sleep, true)
	y += gap
	moveWindow(n.create, x, y, w, a.scale(30))
	y += a.scale(34)
	moveWindow(n.routLbl, x, y, w, lh)
	y += lh
	moveWindow(n.rout, x, y, w, a.scale(240))
	y += ch + a.scale(6)
	bw := (w - a.scale(8)) / 3
	moveWindow(n.pickWork, x, y, bw, a.scale(26))
	moveWindow(n.pickRelax, x+bw+a.scale(4), y, bw, a.scale(26))
	moveWindow(n.pickSleep, x+2*bw+a.scale(8), y, w-2*bw-a.scale(8), a.scale(26))
	y += a.scale(34)
	ih := bottom - y
	if ih < a.scale(40) {
		ih = a.scale(40)
	}
	moveWindow(n.info, x, y, w, ih)
}

func comboFill(h uintptr, items []string, sel int) {
	sendMessage(h, CB_RESETCONTENT, 0, 0)
	for _, s := range items {
		sendMessage(h, CB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(s))))
	}
	if sel < 0 {
		sel = 0
	}
	sendMessage(h, CB_SETCURSEL, uintptr(sel), 0)
}

func comboSel(h uintptr) int { return int(sendMessage(h, CB_GETCURSEL, 0, 0)) }

// headSuffix liefert den Typ eines Kopfes ("Fat_01_S00" aus "Head_Fat_01_S00").
func headSuffix(head string) string { return strings.TrimPrefix(head, "Head_") }

// fillNPCCombos fuellt die Auswahlfelder (Koepfe, Haare ... passend zum Kopf).
func (a *app) fillNPCCombos() {
	n := &a.npc
	p := &npcParts
	if p.byName == nil {
		return
	}
	keep := T("(wie Vorbild)")
	sel := comboSel(n.head)
	items := []string{keep}
	for _, t := range p.heads {
		items = append(items, t.Name)
	}
	comboFill(n.head, items, sel)
	items = []string{keep}
	for _, t := range p.bodies {
		items = append(items, t.Name)
	}
	comboFill(n.body, items, comboSel(n.body))
	items = []string{T("(wie Vorbild)"), T("(kein Händler)")}
	items = append(items, p.traders...)
	comboFill(n.trade, items, comboSel(n.trade))
	al := []string{T("(wie Vorbild)")}
	for _, l := range alignNames {
		al = append(al, T(l))
	}
	comboFill(n.align, al, comboSel(n.align))
	a.fillEnclaves()
	for _, x := range []struct {
		h    uintptr
		list []string
	}{{n.weap, p.weaponry}, {n.loot, p.plunder}, {n.pickp, p.pickpocket}} {
		it := []string{T("(wie Vorbild)"), T("(keine)")}
		it = append(it, x.list...)
		comboFill(x.h, it, comboSel(x.h))
	}
	items = []string{T("(wie Vorbild)")}
	def := 0
	for i, t := range p.points {
		items = append(items, t.Name)
		if t.Name == "Stand" {
			def = i + 1
		}
	}
	ps := comboSel(n.point)
	if !n.filled || ps < 0 {
		ps = def
	}
	comboFill(n.point, items, ps)
	// Freizeit und Schlaf: Vorgabe Lagerfeuer bzw. Schlafen am Boden
	fill2 := func(h uintptr, want string) {
		it := []string{T("(wie Arbeit)")}
		d := 0
		for i, t := range p.points {
			it = append(it, t.Name)
			if t.Name == want {
				d = i + 1
			}
		}
		sel := comboSel(h)
		if !n.filled || sel < 0 {
			sel = d
		}
		comboFill(h, it, sel)
	}
	fill2(n.relax, "Sit_Campfire")
	fill2(n.sleep, "Sleep_Ground")
	n.filled = true
	a.fillHairBeard()
}

// fillHairBeard zeigt nur Haare und Baerte, die zum gewaehlten Kopf passen.
func (a *app) fillHairBeard() {
	n := &a.npc
	p := &npcParts
	head := n.baseParts[g3.SlotHead]
	if i := comboSel(n.head); i > 0 && i-1 < len(p.heads) {
		head = p.heads[i-1].Name
	}
	suf := headSuffix(head)
	filter := func(src []*g3.Template) []*g3.Template {
		var out []*g3.Template
		for _, t := range src {
			if suf == "" || strings.HasSuffix(t.Name, "_"+suf) {
				out = append(out, t)
			}
		}
		return out
	}
	n.hairList = filter(p.hairs)
	n.beardList = filter(p.beards)
	items := []string{T("(wie Vorbild)"), T("(keine)")}
	for _, t := range n.hairList {
		items = append(items, t.Name)
	}
	comboFill(n.hair, items, 0)
	items = []string{T("(wie Vorbild)"), T("(keiner)")}
	for _, t := range n.beardList {
		items = append(items, t.Name)
	}
	comboFill(n.beard, items, 0)
}

// searchNPCBases listet menschliche NPCs der Welt als Vorbilder - auch solche,
// die in der Mod geloescht wurden (aus der Originalwelt).
func (a *app) searchNPCBases() {
	n := &a.npc
	sendMessage(n.list, LB_RESETCONTENT, 0, 0)
	n.bases = nil
	if a.world == nil {
		return
	}
	q := strings.ToLower(strings.TrimSpace(getText(n.search)))
	for i := range a.world.Inst {
		in := &a.world.Inst[i]
		e := in.Ent
		if e.Kind != g3.KindNPC || e.Name == "" || in.Hidden || !strings.EqualFold(e.Mesh, "G3_Hero_Skeleton.FXA") ||
			!strings.HasSuffix(strings.ToLower(e.Source), ".lrentdat") || strings.HasPrefix(e.Name, "PC_Hero") {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(e.Name), q) {
			continue
		}
		n.bases = append(n.bases, baseRef{inst: int32(i), name: e.Name, sector: in.Sector})
	}
	for _, r := range a.removedBases() {
		if q == "" || strings.Contains(strings.ToLower(r.name), q) {
			n.bases = append(n.bases, r)
		}
	}
	sort.SliceStable(n.bases, func(x, y int) bool { return strings.ToLower(n.bases[x].name) < strings.ToLower(n.bases[y].name) })
	if len(n.bases) > 3000 {
		n.bases = n.bases[:3000]
	}
	for _, r := range n.bases {
		sec := r.sector
		if j := strings.LastIndex(sec, "/"); j >= 0 {
			sec = sec[j+1:]
		}
		label := r.name + "   (" + sec + ")"
		if r.inst < 0 {
			label = r.name + "   " + T("(gelöscht – nur als Vorbild)")
		}
		sendMessage(n.list, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(label))))
	}
}

// selectNPCBase waehlt das Vorbild aus und liest seine Ausruestung.
func (a *app) selectNPCBase() {
	n := &a.npc
	i := comboSelList(n.list)
	if i < 0 || i >= len(n.bases) {
		return
	}
	r := n.bases[i]
	n.cur = &r
	n.baseParts = map[int]string{}
	if wf, idx, err := a.baseFile(&r); err == nil {
		for _, c := range wf.Children(idx) {
			ce := wf.Ents[c]
			switch {
			case strings.HasPrefix(ce.Name, "Head_"):
				n.baseParts[g3.SlotHead] = ce.Name
			case strings.HasPrefix(ce.Name, "Hair_"):
				n.baseParts[g3.SlotHair] = ce.Name
			case strings.HasPrefix(ce.Name, "Beard_"):
				n.baseParts[g3.SlotBeard] = ce.Name
			case strings.HasPrefix(ce.Name, "Body_"):
				n.baseParts[g3.SlotBody] = ce.Name
			}
		}
	}
	if r.inst >= 0 {
		a.select_(r.inst, true)
	}
	if wf, idx, err := a.baseFile(&r); err == nil {
		a.fillItemsFrom(wf, idx)
	}
	a.fillHairBeard()
	a.npcPreview()
	a.updateNPCInfo()
}

func comboSelList(h uintptr) int { return int(sendMessage(h, LB_GETCURSEL, 0, 0)) }

func (a *app) updateNPCInfo() {
	n := &a.npc
	var b strings.Builder
	if n.cur != nil {
		fmt.Fprintf(&b, T("Vorbild: %s\r\n"), n.cur.name)
		for _, s := range []int{g3.SlotHead, g3.SlotHair, g3.SlotBeard, g3.SlotBody} {
			if v := n.baseParts[s]; v != "" {
				fmt.Fprintf(&b, "  %s\r\n", v)
			}
		}
		b.WriteString("\r\n")
	} else {
		b.WriteString(T("Wähle oben einen NPC als Vorbild. Der neue NPC übernimmt seine Werte, Gilde, Stimme, Kampfstil und Ausrüstung; Name und Aussehen stellst du hier ein.") + "\r\n\r\n")
	}
	b.WriteString(T("Arbeit, Freizeit und Schlaf: Gothic 3 schickt den NPC tagsüber zum Arbeitsplatz, abends zum Freizeitplatz und nachts zum Schlafplatz. Die Plätze werden als Routinepunkte neben dem NPC angelegt (sichtbar mit „Hilfsobjekte“) und lassen sich wie andere Objekte verschieben."))
	b.WriteString("\r\n\r\n" + T("Waffen, Beute und Taschen sind Schatzsets: Daraus erzeugt Gothic 3 die Waffen des NPCs, die Beute nach seinem Tod und was man ihm stehlen kann. Gegenstände sind feste Gegenstände im Inventar, z. B. „It_Apple 3; It_Key_Ardea 1“.") + " " + T("Steht beim Anwenden etwas im Feld „Name“, wird es der angezeigte Name des gewählten NPCs."))
	a.fillRoutineCombo()
	if a.selected >= 0 && a.world != nil && int(a.selected) < len(a.world.Inst) && a.world.Inst[a.selected].Ent.Kind == g3.KindNPC {
		b.WriteString("\r\n\r\n" + a.routineText(a.selected))
		b.WriteString(a.inventoryText(a.selected) + "\r\n")
		b.WriteString(T("Mit den Knöpfen „Arbeit …“, „Freizeit …“, „Schlaf …“ und einem Klick in der Welt (Bank, Bett, Amboss, Routinepunkt …) änderst du die Plätze dieses NPCs."))
		b.WriteString(" " + T("Ein Klick auf den Boden legt dort einen neuen Routinepunkt an."))
		b.WriteString("\r\n\r\n" + T("Weitere Tagesabläufe: Im Auswahlfeld über den Knöpfen „Neuer Tagesablauf …“ wählen, Namen vergeben (z. B. BeiJohn) und die Plätze setzen. Der NPC wechselt erst dorthin, wenn ein Dialog es befiehlt – z. B. im Abgabe-Dialog einer Quest mit der Zeile:") + "\r\n> " + T("Tagesablauf") + " " + a.world.Inst[a.selected].Ent.Name + " BeiJohn")
	}
	setText(n.info, b.String())
}

// npcInternalName bildet aus einem Anzeigenamen einen Entity-Namen.
func npcInternalName(s string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(s) {
		switch {
		case r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)) || r == '_':
			b.WriteRune(r)
		case r == ' ' || r == '-':
			b.WriteRune('_')
		case r == 'ä':
			b.WriteString("ae")
		case r == 'ö':
			b.WriteString("oe")
		case r == 'ü':
			b.WriteString("ue")
		case r == 'Ä':
			b.WriteString("Ae")
		case r == 'Ö':
			b.WriteString("Oe")
		case r == 'Ü':
			b.WriteString("Ue")
		case r == 'ß':
			b.WriteString("ss")
		}
	}
	return b.String()
}

// currentStringtable liefert die aktuelle stringtable.ini (inkl. eigener Aenderungen).
func (a *app) currentStringtable() []byte {
	if d, err := os.ReadFile(a.stringtablePath()); err == nil {
		return d
	}
	if e := a.vfs.Find("Strings", "stringtable.ini"); e != nil {
		if d, err := e.Read(); err == nil {
			return d
		}
	}
	return nil
}

// createNPC legt den neuen NPC an.
func (a *app) createNPC() {
	n := &a.npc
	if a.world == nil || n.cur == nil {
		a.flash(T("Bitte zuerst einen Vorbild-NPC auswählen."))
		return
	}
	if loadConfig()["SaveMode"] == "data" {
		messageBox(a.hwnd, T("Neue NPCs brauchen mehrere Archive (Welt und Texte). Das geht nur mit dem Mod Starter.\n\nStelle dafür in AdanosEdit.ini SaveMode=mds ein."), T(appTitleDE), MB_ICONINFORMATION)
		return
	}
	disp := strings.TrimSpace(getText(n.name))
	name := npcInternalName(disp)
	if name == "" {
		a.flash(T("Bitte einen Namen für den neuen NPC eingeben."))
		return
	}
	for i := range a.world.Inst {
		if strings.EqualFold(a.world.Inst[i].Ent.Name, name) && !a.world.Inst[i].Hidden {
			messageBox(a.hwnd, fmt.Sprintf(T("Den Namen „%s“ gibt es in der Welt schon. Bitte einen anderen wählen."), name), T(appTitleDE), MB_ICONWARNING)
			return
		}
	}
	// Nur das Originalspiel und die bearbeitete Mod zaehlen - Namen aus
	// anderen Mods (die gerade im Spiel integriert sein koennen) nicht.
	st := a.baseStringtable()
	if st == nil {
		a.flash(T("Die Texttabelle (stringtable.ini) wurde nicht gefunden."))
		return
	}
	_, inGame := g3.StringEntry(st, "FO_"+name)
	_, inMod := a.modStrings()["FO_"+name]
	if (inGame || inMod) && npcParts.byName[name] == nil {
		if messageBoxRet(a.hwnd, fmt.Sprintf(T("Für „%s“ gibt es schon einen Anzeigenamen im Spiel. Trotzdem verwenden (er wird überschrieben)?"), name), T(appTitleDE), MB_YESNO|MB_ICONWARNING) != IDYES {
			return
		}
	}
	src, srcIdx, err := a.baseFile(n.cur)
	if err != nil {
		a.flash(fmt.Sprintf(T("Dieses Objekt kann nicht bearbeitet werden: %s"), T(err.Error())))
		return
	}
	dst, err := a.editFilePath(newObjectFile)
	if err != nil {
		a.flash(fmt.Sprintf(T("Neue Objekte können nicht angelegt werden: %s"), T(err.Error())))
		return
	}
	p := &npcParts
	parts := map[int]*g3.Template{}
	if i := comboSel(n.head); i > 0 && i-1 < len(p.heads) {
		parts[g3.SlotHead] = p.heads[i-1]
	}
	if i := comboSel(n.body); i > 0 && i-1 < len(p.bodies) {
		parts[g3.SlotBody] = p.bodies[i-1]
	}
	switch i := comboSel(n.hair); {
	case i == 1:
		parts[g3.SlotHair] = nil
	case i > 1 && i-2 < len(n.hairList):
		parts[g3.SlotHair] = n.hairList[i-2]
	}
	switch i := comboSel(n.beard); {
	case i == 1:
		parts[g3.SlotBeard] = nil
	case i > 1 && i-2 < len(n.beardList):
		parts[g3.SlotBeard] = n.beardList[i-2]
	}
	spec := g3.NPCSpec{Name: name, Src: src, SrcIdx: srcIdx, Parts: parts}
	spec.Treasure = a.treasureChange()
	spec.Enclave, spec.Align = a.factionChange()
	if items, ok := a.parseItemsField(); !ok {
		return
	} else {
		spec.Items = items
	}
	if i := comboSel(n.trade); i > 1 && i-2 < len(p.traders) {
		spec.Trader = true
		spec.TradeSet = p.traders[i-2]
	}
	if i := comboSel(n.point); i > 0 && i-1 < len(p.points) {
		spec.Points[0] = p.points[i-1]
	}
	if i := comboSel(n.relax); i > 0 && i-1 < len(p.points) {
		spec.Points[1] = p.points[i-1]
	}
	if i := comboSel(n.sleep); i > 0 && i-1 < len(p.points) {
		spec.Points[2] = p.points[i-1]
	}
	if spec.Points[0] == nil && (spec.Points[1] != nil || spec.Points[2] != nil) {
		messageBox(a.hwnd, T("Bei „Arbeit“ bitte einen Platz wählen, wenn Freizeit oder Schlaf gesetzt werden."), T(appTitleDE), MB_ICONINFORMATION)
		return
	}
	// Position: Bildmitte auf dem Boden, zur Kamera gedreht
	var pos scene.Vec3
	if c, ok := a.centerPoint(); ok {
		pos = c
	} else {
		pos = a.cam.Pos.Add(a.cam.Forward().Scale(400))
	}
	f := a.cam.Forward()
	yaw := math.Atan2(float64(-f[0]), float64(-f[2]))
	s, c := float32(math.Sin(yaw)), float32(math.Cos(yaw))
	spec.World = [16]float32{c, 0, -s, 0, 0, 1, 0, 0, s, 0, c, 0, pos[0], pos[1], pos[2], 1}
	created, err := dst.CreateNPC(spec)
	if err != nil {
		a.flash(fmt.Sprintf(T("Der NPC konnte nicht angelegt werden: %s"), err.Error()))
		return
	}
	sector := strings.TrimPrefix(newObjectFile[:strings.LastIndex(newObjectFile, "/")], "G3_World_01/")
	if a.ed.inst[dst.Path] == nil {
		a.ed.inst[dst.Path] = map[int]int32{}
	}
	var insts []int32
	for _, k := range created {
		if dst.Deleted[k] {
			continue
		}
		if ii := a.world.Add(dst.Ents[k], sector); ii >= 0 {
			a.ed.inst[dst.Path][k] = ii
			insts = append(insts, ii)
		}
	}
	dst.Changed = map[int]bool{}
	for _, k := range created {
		dst.Changed[k] = true
	}
	a.syncFile(dst)
	// Anzeigename im Spiel
	a.ed.setString("FO_"+name, []string{disp})
	npc := created[0]
	var points []int
	for _, t := range spec.Points {
		if t != nil {
			points = append(points, created[len(created)-1-len(points)])
		}
	}
	a.ed.undo = append(a.ed.undo, func() {
		dst.Delete(npc)
		for _, pt := range points {
			dst.Delete(pt)
		}
		a.syncFile(dst)
		a.select_(-1, false)
	})
	if ii, ok := a.ed.inst[dst.Path][npc]; ok {
		a.select_(ii, false)
	}
	a.flash(fmt.Sprintf(T("NPC „%s“ angelegt. Verschieben mit Alt + linker Maustaste, Strg+Z entfernt ihn wieder. Speichern nicht vergessen."), disp))
	setText(n.name, "")
	pSetFocus.Call(a.glHwnd)
}

// npcPreview zeigt die gewaehlte Kombination aus Koerper und Kopf.
func (a *app) npcPreview() {
	n := &a.npc
	p := &npcParts
	pick := func(h uintptr, list []*g3.Template, off, slot int) *g3.Template {
		if i := comboSel(h); i >= off && i-off < len(list) {
			return list[i-off]
		}
		if off == 2 && comboSel(h) == 1 {
			return nil // "(keine)"
		}
		return p.byName[n.baseParts[slot]]
	}
	body := pick(n.body, p.bodies, 1, g3.SlotBody)
	head := pick(n.head, p.heads, 1, g3.SlotHead)
	hair := pick(n.hair, n.hairList, 2, g3.SlotHair)
	beard := pick(n.beard, n.beardList, 2, g3.SlotBeard)
	a.setPreviewParts(body, head, hair, beard)
}

// --- Tagesablauf vorhandener NPCs ---------------------------------------------

var routineNames = []string{"Arbeit", "Freizeit", "Schlaf"}

// startRoutinePick: der naechste Klick in der Welt setzt den Platz k (1..3)
// fuer den gewaehlten NPC.
func (a *app) startRoutinePick(k int) {
	n := &a.npc
	if a.world == nil || a.selected < 0 || int(a.selected) >= len(a.world.Inst) || a.world.Inst[a.selected].Ent.Kind != g3.KindNPC {
		a.flash(T("Bitte zuerst einen NPC in der Welt anklicken (oder oben in der Liste wählen)."))
		return
	}
	if _, err := a.editFile(&a.world.Inst[a.selected]); err != nil {
		a.flash(fmt.Sprintf(T("Dieses Objekt kann nicht bearbeitet werden: %s"), T(err.Error())))
		return
	}
	n.pick = k
	n.pickNPC = a.selected
	a.flash(fmt.Sprintf(T("Klicke jetzt in der Welt auf den neuen Platz für „%s“ von %s (z. B. Bank, Bett, Amboss oder Routinepunkt). Esc bricht ab."),
		T(routineNames[k-1]), a.world.Inst[a.selected].Ent.Name))
	pSetFocus.Call(a.glHwnd)
}

// isRoutinePlace: Objekt taugt als Platz fuer einen Tagesablauf.
func isRoutinePlace(e *g3.Entity) bool {
	for _, c := range e.Classes {
		if c == "gCAIHelper_FreePoint_PS" || c == "gCInteraction_PS" {
			return e.Kind != g3.KindNPC
		}
	}
	return false
}

// assignRoutinePoint setzt den beim Klick gewaehlten Platz. Ein Klick auf den
// Boden (kein Platz) legt dort einen neuen Routinepunkt an.
func (a *app) assignRoutinePoint(id int32, hit scene.Vec3, hitOK bool) {
	n := &a.npc
	k := n.pick
	n.pick = 0
	if (id < 0 || int(id) >= len(a.world.Inst)) && !hitOK {
		a.flash(T("Abgebrochen."))
		return
	}
	var undoPoint func()
	var target *g3.Entity
	if id >= 0 && int(id) < len(a.world.Inst) && isRoutinePlace(a.world.Inst[id].Ent) {
		target = a.world.Inst[id].Ent
	} else if hitOK {
		ii, undo, err := a.newRoutinePoint(hit)
		if err != nil {
			a.flash(fmt.Sprintf(T("Anlegen nicht möglich: %s"), T(err.Error())))
			return
		}
		target = a.world.Inst[ii].Ent
		undoPoint = undo
	} else {
		n.pick = k
		a.flash(fmt.Sprintf(T("„%s“ ist kein Platz für einen Tagesablauf. Bitte eine Bank, ein Bett, einen Amboss o. Ä. oder einen Routinepunkt anklicken (Esc bricht ab)."), a.world.Inst[id].Ent.Name))
		return
	}
	in := &a.world.Inst[n.pickNPC]
	wf, err := a.editFile(in)
	if err != nil {
		a.flash(fmt.Sprintf(T("Dieses Objekt kann nicht bearbeitet werden: %s"), T(err.Error())))
		return
	}
	idx := in.Ent.Index
	raw := wf.RawCopy(idx)
	rname := n.routName
	list, cur, err := wf.Routines(idx)
	if err == nil {
		if rname == "" {
			rname = cur
		}
		found := false
		for _, r := range list {
			if strings.EqualFold(r.Name, rname) {
				r.Points[k-1] = target.GUID
				err = wf.SetRoutine(idx, r)
				found = true
				break
			}
		}
		if !found && err == nil {
			err = fmt.Errorf(T("Tagesablauf %s fehlt"), rname)
		}
	}
	if err != nil {
		if undoPoint != nil {
			undoPoint()
		}
		a.flash(fmt.Sprintf(T("Der Tagesablauf konnte nicht geändert werden: %s"), err.Error()))
		return
	}
	a.syncFile(wf)
	a.ed.undo = append(a.ed.undo, func() {
		wf.RestoreRaw(idx, raw)
		a.syncFile(wf)
		if undoPoint != nil {
			undoPoint()
		}
	})
	a.flash(fmt.Sprintf(T("%s: „%s“ ist jetzt der Platz für %s (Tagesablauf %s). Strg+Z macht es rückgängig."), in.Ent.Name, target.Name, T(routineNames[k-1]), rname))
	a.select_(n.pickNPC, false)
	a.updateNPCInfo()
}

// newRoutinePoint legt einen Routinepunkt ("Stand") an der Stelle p an.
func (a *app) newRoutinePoint(p scene.Vec3) (int32, func(), error) {
	var tpl *g3.Template
	for _, t := range npcParts.points {
		if t.Name == "Stand" {
			tpl = t
		}
	}
	if tpl == nil && len(npcParts.points) > 0 {
		tpl = npcParts.points[0]
	}
	if tpl == nil {
		return -1, nil, errors.New(T("Keine Vorlage für Routinepunkte gefunden."))
	}
	wf, err := a.editFilePath(newObjectFile)
	if err != nil {
		return -1, nil, err
	}
	world := [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, p[0], p[1], p[2], 1}
	ci, err := wf.AddFromTemplate(tpl, world)
	if err != nil {
		return -1, nil, err
	}
	sector := strings.TrimPrefix(newObjectFile[:strings.LastIndex(newObjectFile, "/")], "G3_World_01/")
	if a.ed.inst[wf.Path] == nil {
		a.ed.inst[wf.Path] = map[int]int32{}
	}
	ii := a.world.Add(wf.Ents[ci], sector)
	if ii < 0 {
		wf.Delete(ci)
		a.syncFile(wf)
		return -1, nil, errors.New(T("Das Objekt konnte nicht angezeigt werden (keine gültige Größe)."))
	}
	a.ed.inst[wf.Path][ci] = ii
	wf.Changed = map[int]bool{ci: true}
	a.syncFile(wf)
	return ii, func() {
		wf.Delete(ci)
		a.syncFile(wf)
	}, nil
}

// fillRoutineCombo zeigt die Tagesablaeufe des gewaehlten NPCs.
func (a *app) fillRoutineCombo() {
	n := &a.npc
	if n.rout == 0 {
		return
	}
	var names []string
	cur := ""
	sel := a.selected
	if a.world != nil && sel >= 0 && int(sel) < len(a.world.Inst) && a.world.Inst[sel].Ent.Kind == g3.KindNPC {
		if wf, err := a.editFile(&a.world.Inst[sel]); err == nil {
			if list, c, err := wf.Routines(a.world.Inst[sel].Ent.Index); err == nil {
				cur = c
				for _, r := range list {
					names = append(names, r.Name)
				}
			}
		}
	}
	if sel != n.routNPC {
		n.routNPC = sel
		n.routName = ""
	}
	n.routNames = names
	n.routCur = cur
	n.routDel = ""
	var items []string
	si := 0
	for i, r := range names {
		l := r
		if strings.EqualFold(r, cur) {
			l += "  " + T("(beim Spielstart)")
		}
		items = append(items, l)
		if n.routName != "" && strings.EqualFold(r, n.routName) || n.routName == "" && strings.EqualFold(r, cur) {
			si = i
		}
	}
	if len(names) > 0 {
		items = append(items, T("Neuer Tagesablauf …"))
		if si < len(names) && !strings.EqualFold(names[si], cur) {
			n.routDel = names[si]
			items = append(items, fmt.Sprintf(T("Tagesablauf „%s“ löschen …"), names[si]))
		}
	} else {
		items = []string{T("(zuerst einen NPC in der Welt anklicken)")}
	}
	comboFill(n.rout, items, si)
}

// onRoutineSelect: anderer Tagesablauf gewaehlt (oder neuer angelegt).
func (a *app) onRoutineSelect() {
	n := &a.npc
	i := comboSel(n.rout)
	if i < 0 || len(n.routNames) == 0 {
		return
	}
	if i < len(n.routNames) {
		n.routName = n.routNames[i]
		a.updateNPCInfo()
		a.fillRoutineCombo() // Loeschen-Eintrag fuer den gewaehlten Ablauf
		return
	}
	if i > len(n.routNames) {
		a.deleteRoutine()
		return
	}
	// neuer Tagesablauf
	sel := a.selected
	if a.world == nil || sel < 0 || int(sel) >= len(a.world.Inst) {
		return
	}
	in := &a.world.Inst[sel]
	name, ok := a.inputBox(T("Neuer Tagesablauf"), T("Name des neuen Tagesablaufs (nur Buchstaben, Ziffern und _), z. B. BeiJohn:"), "")
	name = strings.TrimSpace(name)
	if !ok || name == "" {
		a.fillRoutineCombo()
		return
	}
	for _, r := range name {
		if !(r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)) || r == '_') {
			messageBox(a.hwnd, T("Bitte nur Buchstaben (ohne Umlaute), Ziffern und _ verwenden."), T(appTitleDE), MB_ICONWARNING)
			a.fillRoutineCombo()
			return
		}
	}
	wf, err := a.editFile(in)
	if err != nil {
		a.flash(fmt.Sprintf(T("Dieses Objekt kann nicht bearbeitet werden: %s"), T(err.Error())))
		return
	}
	idx := in.Ent.Index
	list, cur, err := wf.Routines(idx)
	if err != nil {
		a.flash(fmt.Sprintf(T("Der Tagesablauf konnte nicht geändert werden: %s"), err.Error()))
		return
	}
	for _, r := range list {
		if strings.EqualFold(r.Name, name) {
			n.routName = r.Name
			a.updateNPCInfo()
			return
		}
	}
	// Vorgabe: Plaetze des aktuellen Ablaufs
	nr := g3.Routine{Name: name}
	for _, r := range list {
		if strings.EqualFold(r.Name, cur) {
			nr.Points = r.Points
		}
	}
	raw := wf.RawCopy(idx)
	if err := wf.SetRoutine(idx, nr); err != nil {
		a.flash(fmt.Sprintf(T("Der Tagesablauf konnte nicht geändert werden: %s"), err.Error()))
		return
	}
	a.syncFile(wf)
	a.ed.undo = append(a.ed.undo, func() {
		wf.RestoreRaw(idx, raw)
		a.syncFile(wf)
	})
	n.routName = name
	a.updateNPCInfo()
	a.flash(fmt.Sprintf(T("Tagesablauf „%s“ angelegt. Setze jetzt mit „Arbeit …“, „Freizeit …“ und „Schlaf …“ seine Plätze."), name))
}

// deleteRoutine loescht den gewaehlten Tagesablauf des NPCs (nicht den beim
// Spielstart).
func (a *app) deleteRoutine() {
	n := &a.npc
	name := n.routDel
	sel := a.selected
	if name == "" || a.world == nil || sel < 0 || int(sel) >= len(a.world.Inst) {
		a.fillRoutineCombo()
		return
	}
	in := &a.world.Inst[sel]
	// Dialoge, die in diesen Ablauf schicken
	a.loadDialogs()
	var uses []string
	for _, info := range a.dlg.infos {
		for _, c := range info.Script {
			if c.Cmd == "SetRoutine" && strings.EqualFold(c.ID1, name) &&
				(strings.EqualFold(c.Ent1, in.Ent.Name) || (c.Ent1 == "npc" || c.Ent1 == "") && strings.EqualFold(info.Owner, in.Ent.Name)) {
				uses = append(uses, info.Name)
				break
			}
		}
	}
	msg := fmt.Sprintf(T("Tagesablauf „%s“ von %s löschen?"), name, a.displayName(in.Ent.Name))
	if len(uses) > 0 {
		msg += "\n\n" + fmt.Sprintf(T("Achtung: %d Dialog(e) schicken den NPC in diesen Tagesablauf (%s). Dort wirkt die Zeile „> Tagesablauf …“ danach nicht mehr."), len(uses), strings.Join(uses, ", "))
	}
	if messageBoxRet(a.hwnd, msg, T(appTitleDE), MB_YESNO|MB_ICONQUESTION) != IDYES {
		a.fillRoutineCombo()
		return
	}
	wf, err := a.editFile(in)
	if err != nil {
		a.flash(fmt.Sprintf(T("Dieses Objekt kann nicht bearbeitet werden: %s"), T(err.Error())))
		return
	}
	idx := in.Ent.Index
	raw := wf.RawCopy(idx)
	if err := wf.RemoveRoutine(idx, name); err != nil {
		a.flash(fmt.Sprintf(T("Der Tagesablauf konnte nicht geändert werden: %s"), T(err.Error())))
		a.fillRoutineCombo()
		return
	}
	wf.Changed = map[int]bool{idx: true}
	a.syncFile(wf)
	a.ed.undo = append(a.ed.undo, func() {
		wf.RestoreRaw(idx, raw)
		wf.Changed = map[int]bool{idx: true}
		a.syncFile(wf)
	})
	n.routName = ""
	a.updateNPCInfo()
	a.fillRoutineCombo()
	a.flash(fmt.Sprintf(T("Tagesablauf „%s“ gelöscht. Strg+Z holt ihn zurück."), name))
}

// routineText beschreibt den Tagesablauf eines NPCs.
func (a *app) routineText(i int32) string {
	if a.world == nil || i < 0 || int(i) >= len(a.world.Inst) {
		return ""
	}
	in := &a.world.Inst[i]
	wf, err := a.editFile(in)
	if err != nil {
		return ""
	}
	list, cur, err := wf.Routines(in.Ent.Index)
	if err != nil {
		return ""
	}
	if a.guidInst == nil || a.guidN != len(a.world.Inst) {
		a.guidN = len(a.world.Inst)
		a.guidInst = map[[16]byte]int32{}
		for k := range a.world.Inst {
			a.guidInst[a.world.Inst[k].Ent.GUID] = int32(k)
		}
	}
	var b strings.Builder
	for _, r := range list {
		title := fmt.Sprintf(T("Tagesablauf von %s:"), in.Ent.Name)
		if len(list) > 1 || !strings.EqualFold(r.Name, cur) {
			title = fmt.Sprintf(T("Tagesablauf „%s“ von %s:"), r.Name, in.Ent.Name)
			if strings.EqualFold(r.Name, cur) {
				title += " " + T("(beim Spielstart)")
			}
		}
		b.WriteString(title + "\r\n")
		for k, g := range r.Points {
			name := T("(keiner)")
			if g != ([16]byte{}) {
				name = "?"
				if ii, ok := a.guidInst[g]; ok {
					name = a.world.Inst[ii].Ent.Name
				}
			}
			fmt.Fprintf(&b, "  %s: %s\r\n", T(routineNames[k]), name)
		}
	}
	return b.String()
}

// --- Inventar -------------------------------------------------------------------

// treasureChange liefert die gewaehlten Schatzsets (nur geaenderte Arten).
func (a *app) treasureChange() map[string]string {
	n := &a.npc
	p := &npcParts
	ch := map[string]string{}
	for _, x := range []struct {
		h    uintptr
		kind string
		list []string
	}{{n.trade, "TS_Trader", p.traders}, {n.weap, "TS_Weaponry", p.weaponry}, {n.loot, "TS_Plunder", p.plunder}, {n.pickp, "TS_PickPocket", p.pickpocket}} {
		i := comboSel(x.h)
		switch {
		case i == 1:
			ch[x.kind] = ""
		case i > 1 && i-2 < len(x.list):
			ch[x.kind] = x.list[i-2]
		}
	}
	return ch
}

// itemTemplates: Gegenstands-Vorlagen nach Name (klein) und nach GUID.
func (a *app) itemTemplates() (byName map[string]*g3.Template, byGUID map[[16]byte]*g3.Template) {
	byName = map[string]*g3.Template{}
	byGUID = map[[16]byte]*g3.Template{}
	for _, e := range a.templates {
		t := e.t
		if !strings.HasPrefix(strings.ToLower(t.Path), "items/") && !strings.HasPrefix(t.Name, "It_") {
			continue
		}
		byName[strings.ToLower(t.Name)] = t
		var g [16]byte
		copy(g[:], t.RefGUID[:16])
		byGUID[g] = t
	}
	return
}

// parseItemsField liest das Feld "Gegenstaende" (z. B. "It_Apple 3; It_Key_01").
func (a *app) parseItemsField() (*[]g3.StackItem, bool) {
	txt := strings.TrimSpace(getText(a.npc.items))
	items := []g3.StackItem{}
	if txt == "" {
		return &items, true
	}
	byName, _ := a.itemTemplates()
	for _, part := range strings.FieldsFunc(txt, func(r rune) bool { return r == ';' || r == ',' }) {
		f := strings.Fields(part)
		if len(f) == 0 {
			continue
		}
		cnt := 1
		if len(f) > 1 {
			v, err := strconv.Atoi(f[1])
			if err != nil || v < 1 {
				messageBox(a.hwnd, fmt.Sprintf(T("„%s“ ist keine gültige Zahl."), f[1]), T(appTitleDE), MB_ICONWARNING)
				return nil, false
			}
			cnt = v
		}
		t := byName[strings.ToLower(f[0])]
		if t == nil {
			messageBox(a.hwnd, fmt.Sprintf(T("Den Gegenstand „%s“ kenne ich nicht (Vorlagenname wie It_Gold oder It_Apple?)."), f[0]), T(appTitleDE), MB_ICONWARNING)
			return nil, false
		}
		var g [16]byte
		copy(g[:], t.RefGUID[:16])
		items = append(items, g3.StackItem{Template: g, Amount: cnt})
	}
	return &items, true
}

// fillNPCItems zeigt die Gegenstaende eines NPCs im Feld "Gegenstaende".
func (a *app) fillNPCItems(i int32) {
	if a.world == nil || i < 0 || int(i) >= len(a.world.Inst) || a.world.Inst[i].Ent.Kind != g3.KindNPC {
		return
	}
	in := &a.world.Inst[i]
	wf, err := a.editFile(in)
	if err != nil {
		return
	}
	a.fillItemsFrom(wf, in.Ent.Index)
}

// fillItemsFrom zeigt die Gegenstaende eines NPCs (Datei + Index) im Feld.
func (a *app) fillItemsFrom(wf *g3.WorldFile, idx int) {
	items, err := wf.InventoryItems(idx)
	if err != nil {
		return
	}
	_, byGUID := a.itemTemplates()
	var parts []string
	for _, it := range items {
		name := "?"
		if t := byGUID[it.Template]; t != nil {
			name = t.Name
		}
		parts = append(parts, fmt.Sprintf("%s %d", name, it.Amount))
	}
	setText(a.npc.items, strings.Join(parts, "; "))
}

// inventoryText beschreibt Schatzsets und Gegenstaende eines NPCs.
func (a *app) inventoryText(i int32) string {
	in := &a.world.Inst[i]
	wf, err := a.editFile(in)
	if err != nil {
		return ""
	}
	ts, err := wf.TreasureSets(in.Ent.Index)
	if err != nil {
		return ""
	}
	var b strings.Builder
	if enc, al, err := wf.Faction(in.Ent.Index); err == nil {
		an := "?"
		if al >= 0 && al < len(alignNames) {
			an = T(alignNames[al])
		}
		en := T("(keine Enklave)")
		if enc != ([16]byte{}) {
			en = "?"
			for _, e := range a.npc.enclaves {
				if e.guid == enc {
					en = e.name
				}
			}
		}
		fmt.Fprintf(&b, T("Gruppe: %s, Enklave: %s")+"\r\n", an, en)
	}
	b.WriteString(T("Inventar (Schatzsets):") + "\r\n")
	for _, s := range ts {
		if s != "" {
			b.WriteString("  " + s + "\r\n")
		}
	}
	return b.String()
}

// applyEquip uebernimmt Haendler, Waffen, Beute, Taschen und Gegenstaende
// auf den gewaehlten NPC.
func (a *app) applyEquip() {
	if a.world == nil || a.selected < 0 || int(a.selected) >= len(a.world.Inst) || a.world.Inst[a.selected].Ent.Kind != g3.KindNPC {
		a.flash(T("Bitte zuerst einen NPC in der Welt anklicken (oder oben in der Liste wählen)."))
		return
	}
	in := &a.world.Inst[a.selected]
	wf, err := a.editFile(in)
	if err != nil {
		a.flash(fmt.Sprintf(T("Dieses Objekt kann nicht bearbeitet werden: %s"), T(err.Error())))
		return
	}
	items, ok := a.parseItemsField()
	if !ok {
		return
	}
	idx := in.Ent.Index
	old := wf.RawCopy(idx)
	if ch := a.treasureChange(); len(ch) > 0 {
		if err := wf.SetTreasureSets(idx, ch); err != nil {
			wf.RestoreRaw(idx, old)
			a.flash(fmt.Sprintf(T("Das Inventar konnte nicht geändert werden: %s"), err.Error()))
			return
		}
	}
	if e, al := a.factionChange(); e != nil || al != nil {
		if err := wf.SetFaction(idx, e, al); err != nil {
			wf.RestoreRaw(idx, old)
			a.flash(fmt.Sprintf(T("Das Inventar konnte nicht geändert werden: %s"), err.Error()))
			return
		}
	}
	if err := wf.SetInventoryItems(idx, *items); err != nil {
		wf.RestoreRaw(idx, old)
		a.flash(fmt.Sprintf(T("Das Inventar konnte nicht geändert werden: %s"), err.Error()))
		return
	}
	a.syncFile(wf)
	a.ed.undo = append(a.ed.undo, func() {
		wf.RestoreRaw(idx, old)
		a.syncFile(wf)
	})
	// Anzeigename (z. B. wenn er verloren ging oder geaendert werden soll)
	if disp := strings.TrimSpace(getText(a.npc.name)); disp != "" {
		if strings.EqualFold(in.Ent.Name, "PC_Hero") {
			// der Held behaelt seinen Namen (er steht auch im Tagebuch)
			messageBox(a.hwnd, T("Der Name des Helden wird nicht geändert – das Feld „Name“ gilt nur für andere NPCs."), T(appTitleDE), MB_ICONINFORMATION)
		} else {
			a.setText("FO_"+in.Ent.Name, disp)
		}
		setText(a.npc.name, "")
	}
	a.flash(fmt.Sprintf(T("Inventar von %s geändert. Strg+Z macht es rückgängig."), in.Ent.Name))
	a.updateNPCInfo()
}

// Politische Ausrichtungen (gEPoliticalAlignment), Reihenfolge = Wert
var alignNames = []string{"Keine (Tiere, Monster)", "Orks", "Nordmarer", "Rebellen", "Waldläufer", "Assassinen", "Nomaden", "Orks (Nordmar)", "Sklaven"}

type enclaveRef struct {
	name string
	guid [16]byte
}

// fillEnclaves listet die Enklaven (Staedte, Lager) der Welt.
func (a *app) fillEnclaves() {
	n := &a.npc
	if a.world == nil {
		return
	}
	if len(n.enclaves) == 0 {
		for i := range a.world.Inst {
			e := a.world.Inst[i].Ent
			for _, c := range e.Classes {
				if c == "gCEnclave_PS" {
					n.enclaves = append(n.enclaves, enclaveRef{e.Name, e.GUID})
					break
				}
			}
		}
		sort.Slice(n.enclaves, func(i, j int) bool { return n.enclaves[i].name < n.enclaves[j].name })
	}
	it := []string{T("(wie Vorbild)"), T("(keine Enklave)")}
	for _, e := range n.enclaves {
		it = append(it, e.name)
	}
	comboFill(n.encl, it, comboSel(n.encl))
}

// factionChange liefert die gewaehlte Enklave und Ausrichtung (nil = wie Vorbild).
func (a *app) factionChange() (*[16]byte, *int) {
	n := &a.npc
	var enc *[16]byte
	switch i := comboSel(n.encl); {
	case i == 1:
		enc = &[16]byte{}
	case i > 1 && i-2 < len(n.enclaves):
		g := n.enclaves[i-2].guid
		enc = &g
	}
	var al *int
	if i := comboSel(n.align); i > 0 && i-1 < len(alignNames) {
		v := i - 1
		al = &v
	}
	return enc, al
}
