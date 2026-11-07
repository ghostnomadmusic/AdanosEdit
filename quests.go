//go:build windows

package main

// Reiter "Quests": eigene Quests anlegen (Gegenstaende bringen, jemandem
// etwas ausrichten, jemanden toeten, jemanden zu einer Person bringen,
// jemandem folgen) und die passenden Dialoge erzeugen.
// Gespeichert wird als Quests.m0x/.p0x in der Mod, der Tagebuchtitel kommt in die
// stringtable.ini.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unsafe"

	"g3viewer/g3"
)

const (
	idTabQst = 360 + iota
	idQstList
	idQstNew
	idQstDel
	idQstTitle
	idQstType
	idQstGiver
	idQstTarget
	idQstAmount
	idQstXP
	idQstGold
	idQstApply
	idQstGen
	idQstPoint
	idQstPlace
	idQstZone
	idQstPol
	idQstPolAmt
	idQstEnc
	idQstEncAmt
	idQstAttr
	idQstAttrAmt
	idQstPrereq
	idQstTime
	idQstSpell
)

// questTypes: Eintraege der Auswahl "Aufgabe" -> Quest-Typ im Spiel.
var questTypes = []int{g3.QuestBring, g3.QuestReport, g3.QuestKill, g3.QuestDefeat, g3.QuestDriveAway, g3.QuestArena,
	g3.QuestBringNPC, g3.QuestFollow, g3.QuestReach, g3.QuestRansack, g3.QuestSpell}

// "Ort befreien" (g3.QuestFree) ist vorbereitet, wird aber nicht angeboten:
// Fast jede Enklave hat im Originalspiel schon eine Befreiungsquest, die dann
// ebenfalls erfuellt wuerde. Sinnvoll erst zusammen mit eigenen Enklaven.

// usesPlace: Quest-Art mit eigenem Ort (Arena, Zone, Truhe).
func usesPlace(t int) bool {
	return t == g3.QuestArena || t == g3.QuestReach || t == g3.QuestRansack
}

func questTypeIndex(t int) int {
	for i, x := range questTypes {
		if x == t {
			return i
		}
	}
	return 0
}

// selQuestType liefert den gewaehlten Quest-Typ.
func (a *app) selQuestType() int { return questTypes[max(0, comboSel(a.qst.typ))] }

type qstUI struct {
	listLbl, list, btnNew, del         uintptr
	titleLbl, title, typeLbl, typ      uintptr
	giverLbl, giver, targetLbl, target uintptr
	amountLbl, amount, xpLbl, xp       uintptr
	goldLbl, gold, apply, gen, help    uintptr
	point                              uintptr  // Zielpunkt setzen (Folgen) bzw. gewaehlten Ort uebernehmen
	placeLbl, place                    uintptr  // Ort: Arena, Zone oder Truhe
	zoneBtn                            uintptr  // vorhandene Zone in der Naehe waehlen
	polLbl, pol, polAmt                uintptr  // Ansehen bei einer Gruppe (Orks, Rebellen ...)
	encLbl, enc, encAmt                uintptr  // Ansehen in einer Stadt / einem Lager
	attrLbl, attr, attrAmt             uintptr  // Attribut-Belohnung
	prereqLbl, prereq                  uintptr  // erst nach diesen Quests (FinishedQuests)
	timeLbl, time, timeUnit            uintptr  // Zeitlimit in Tagen
	spell                              uintptr  // Zauber (statt "Ort" bei "Zauber wirken")
	pointX, pointY, pointW             int      // Lage der Knopfzeile
	names                              []string // eigene Quests der Liste
	cur                                int
	changed, loading                   bool
	note                               string

	quests map[string]*g3.Quest // Pfad (klein) -> Quest
	paths  map[string]string    // Pfad (klein) -> Pfad wie im Archiv
}

func (a *app) createQstControls(mk func(ex uint32, class, text string, style uint32, id int) uintptr) {
	q := &a.qst
	q.cur = -1
	a.hTabQst = mk(0, "BUTTON", T("Quests"), BS_AUTORADIOBUTTON|BS_PUSHLIKE, idTabQst)
	q.listLbl = mk(0, "STATIC", "", SS_LEFT, 0)
	q.list = mk(WS_EX_CLIENTEDGE, "LISTBOX", "", WS_VSCROLL|LBS_NOTIFY|LBS_NOINTEGRALHEIGHT|WS_TABSTOP, idQstList)
	q.btnNew = mk(0, "BUTTON", "", BS_PUSHBUTTON|WS_TABSTOP, idQstNew)
	q.del = mk(0, "BUTTON", "", BS_PUSHBUTTON|WS_TABSTOP, idQstDel)
	lbl := func() uintptr { return mk(0, "STATIC", "", SS_LEFT, 0) }
	edit := func(id int) uintptr { return mk(WS_EX_CLIENTEDGE, "EDIT", "", ES_AUTOHSCROLL|WS_TABSTOP, id) }
	q.titleLbl, q.title = lbl(), edit(idQstTitle)
	q.typeLbl = lbl()
	q.typ = mk(0, "COMBOBOX", "", CBS_DROPDOWNLIST|WS_VSCROLL|WS_TABSTOP, idQstType)
	q.giverLbl, q.giver = lbl(), edit(idQstGiver)
	q.targetLbl, q.target = lbl(), edit(idQstTarget)
	q.placeLbl, q.place = lbl(), edit(idQstPlace)
	q.point = mk(0, "BUTTON", "", BS_PUSHBUTTON|WS_TABSTOP, idQstPoint)
	q.zoneBtn = mk(0, "BUTTON", "", BS_PUSHBUTTON|WS_TABSTOP, idQstZone)
	q.amountLbl, q.amount = lbl(), edit(idQstAmount)
	q.xpLbl, q.xp = lbl(), edit(idQstXP)
	q.goldLbl, q.gold = lbl(), edit(idQstGold)
	combo := func(id int) uintptr { return mk(0, "COMBOBOX", "", CBS_DROPDOWNLIST|WS_VSCROLL|WS_TABSTOP, id) }
	q.polLbl, q.pol, q.polAmt = lbl(), combo(idQstPol), edit(idQstPolAmt)
	q.encLbl, q.enc, q.encAmt = lbl(), combo(idQstEnc), edit(idQstEncAmt)
	q.attrLbl, q.attr, q.attrAmt = lbl(), combo(idQstAttr), edit(idQstAttrAmt)
	q.prereqLbl, q.prereq = lbl(), edit(idQstPrereq)
	q.timeLbl, q.time, q.timeUnit = lbl(), edit(idQstTime), lbl()
	q.spell = combo(idQstSpell)
	q.apply = mk(0, "BUTTON", "", BS_PUSHBUTTON|WS_TABSTOP, idQstApply)
	q.gen = mk(0, "BUTTON", "", BS_PUSHBUTTON|WS_TABSTOP, idQstGen)
	q.help = mk(WS_EX_CLIENTEDGE, "EDIT", "", ES_MULTILINE|ES_READONLY|ES_AUTOVSCROLL|WS_VSCROLL, 0)
	a.applyQstTexts()
}

func (a *app) applyQstTexts() {
	q := &a.qst
	setText(a.hTabQst, T("Quests"))
	setText(q.listLbl, T("Eigene Quests:"))
	setText(q.btnNew, T("Neue Quest"))
	setText(q.del, T("Quest löschen"))
	setText(q.titleLbl, T("Titel:"))
	setText(q.typeLbl, T("Aufgabe:"))
	setText(q.giverLbl, T("Auftraggeber:"))
	setText(q.targetLbl, T("Ziel:"))
	setText(q.amountLbl, T("Anzahl:"))
	setText(q.xpLbl, T("Erfahrung:"))
	setText(q.goldLbl, T("Gold:"))
	setText(q.polLbl, T("Ansehen Gruppe:"))
	setText(q.encLbl, T("Ansehen Ort:"))
	setText(q.attrLbl, T("Attribut:"))
	setText(q.prereqLbl, T("Erst nach Quest:"))
	setText(q.timeLbl, T("Zeitlimit:"))
	setText(q.timeUnit, T("Tage (leer = keins)"))
	setText(q.apply, T("Übernehmen"))
	setText(q.gen, T("Passende Dialoge erzeugen"))
	q.loading = true
	comboFill(q.typ, []string{T("Gegenstände bringen"), T("Jemandem etwas ausrichten"), T("Jemanden töten"), T("Jemanden besiegen (nur niederschlagen)"),
		T("Jemanden vertreiben"), T("Arenakampf"),
		T("Jemanden zu einer Person bringen (er folgt dem Helden)"), T("Jemandem folgen (er führt den Helden)"),
		T("Ort erreichen"), T("Truhe ausrauben"), T("Zauber auf jemanden wirken")}, comboSel(q.typ))
	a.fillRewardCombos()
	q.loading = false
	if q.quests != nil {
		a.fillQstList()
	}
	a.updateQstButtons()
	a.updateQstHelp()
}

func (a *app) qstControls() []uintptr {
	q := &a.qst
	return []uintptr{q.listLbl, q.list, q.btnNew, q.del, q.titleLbl, q.title, q.typeLbl, q.typ, q.giverLbl, q.giver,
		q.targetLbl, q.target, q.placeLbl, q.place, q.point, q.zoneBtn, q.amountLbl, q.amount, q.xpLbl, q.xp, q.goldLbl, q.gold, q.apply, q.gen, q.help,
		q.polLbl, q.pol, q.polAmt, q.encLbl, q.enc, q.encAmt, q.attrLbl, q.attr, q.attrAmt, q.prereqLbl, q.prereq, q.timeLbl, q.time, q.timeUnit, q.spell}
}

func (a *app) layoutQst(x, y, w, bottom int) {
	q := &a.qst
	lh := a.scale(20)
	ch := a.scale(22)
	total := bottom - y
	moveWindow(q.listLbl, x, y, w, lh)
	y += lh
	listH := total * 16 / 100
	moveWindow(q.list, x, y, w, listH)
	y += listH + a.scale(4)
	bw := (w - a.scale(6)) / 2
	moveWindow(q.btnNew, x, y, bw, a.scale(26))
	moveWindow(q.del, x+bw+a.scale(6), y, w-bw-a.scale(6), a.scale(26))
	y += a.scale(34)
	lw := a.scale(100)
	row := func(l, c uintptr, combo bool) {
		moveWindow(l, x, y+a.scale(4), lw, lh)
		h := ch + a.scale(2)
		if combo {
			h = a.scale(200)
		}
		moveWindow(c, x+lw, y, w-lw, h)
		y += ch + a.scale(6)
	}
	row(q.titleLbl, q.title, false)
	row(q.typeLbl, q.typ, true)
	row(q.giverLbl, q.giver, false)
	row(q.targetLbl, q.target, false)
	moveWindow(q.spell, x+lw, y, w-lw, a.scale(300))
	row(q.placeLbl, q.place, false)
	q.pointX, q.pointY, q.pointW = x+lw, y, w-lw
	a.layoutQstPointRow()
	y += a.scale(32)
	row(q.amountLbl, q.amount, false)
	// Erfahrung und Gold in einer Zeile
	half := (w - lw - a.scale(8)) / 2
	gl := a.scale(40)
	moveWindow(q.xpLbl, x, y+a.scale(4), lw, lh)
	moveWindow(q.xp, x+lw, y, half, ch+a.scale(2))
	moveWindow(q.goldLbl, x+lw+half+a.scale(8), y+a.scale(4), gl, lh)
	moveWindow(q.gold, x+lw+half+a.scale(8)+gl, y, w-lw-half-a.scale(8)-gl, ch+a.scale(2))
	y += ch + a.scale(6)
	// Belohnung: Auswahl + Betrag
	aw := a.scale(56)
	pair := func(l, c, e uintptr) {
		moveWindow(l, x, y+a.scale(4), lw, lh)
		moveWindow(c, x+lw, y, w-lw-aw-a.scale(4), a.scale(300))
		moveWindow(e, x+w-aw, y, aw, ch+a.scale(2))
		y += ch + a.scale(6)
	}
	pair(q.polLbl, q.pol, q.polAmt)
	pair(q.encLbl, q.enc, q.encAmt)
	pair(q.attrLbl, q.attr, q.attrAmt)
	row(q.prereqLbl, q.prereq, false)
	tw := a.scale(56)
	moveWindow(q.timeLbl, x, y+a.scale(4), lw, lh)
	moveWindow(q.time, x+lw, y, tw, ch+a.scale(2))
	moveWindow(q.timeUnit, x+lw+tw+a.scale(6), y+a.scale(4), w-lw-tw-a.scale(6), lh)
	y += ch + a.scale(6)
	y += a.scale(4)
	moveWindow(q.apply, x, y, w, a.scale(28))
	y += a.scale(32)
	moveWindow(q.gen, x, y, w, a.scale(28))
	y += a.scale(34)
	moveWindow(q.help, x, y, w, max(bottom-y, a.scale(40)))
}

// --- Daten -------------------------------------------------------------------

// loadQuests liest alle Quests (einmalig).
func (a *app) loadQuests() {
	q := &a.qst
	if q.quests != nil || a.vfs == nil {
		return
	}
	a.loadDialogs() // Texte
	q.quests = map[string]*g3.Quest{}
	q.paths = map[string]string{}
	for _, e := range a.vfs.List("Quests") {
		if !strings.HasSuffix(strings.ToLower(e.Name), ".quest") {
			continue
		}
		if b, err := e.Read(); err == nil {
			q.quests[strings.ToLower(e.Name)] = g3.ParseQuest(b)
			q.paths[strings.ToLower(e.Name)] = e.Name
		}
	}
	for p, b := range a.ed.extra["Quests"] {
		if b == nil {
			delete(q.quests, strings.ToLower(p))
		} else {
			q.quests[strings.ToLower(p)] = g3.ParseQuest(b)
			q.paths[strings.ToLower(p)] = p
		}
	}
}

func isOwnQuest(name string) bool { return strings.HasPrefix(name, ownInfoPrefix) }

// questNames liefert die eigenen und die uebrigen Quests (sortiert).
func (a *app) questNames() (own, other []string) {
	a.loadQuests()
	for _, qq := range a.qst.quests {
		if n := qq.Name(); isOwnQuest(n) {
			own = append(own, n)
		} else if n != "" {
			other = append(other, n)
		}
	}
	sort.Strings(own)
	sort.Slice(other, func(i, j int) bool { return strings.ToLower(other[i]) < strings.ToLower(other[j]) })
	return
}

func (a *app) questByName(name string) *g3.Quest {
	return a.qst.quests[strings.ToLower(g3.QuestPath(name))]
}

// questTitle liefert den Tagebuchtitel einer Quest.
func (a *app) questTitle(name string) string {
	if qq := a.questByName(name); qq != nil {
		if t, ok := a.infoText(qq.Get("LogTopic")); ok && t != "" {
			return t
		}
	}
	return name
}

// displayName liefert den Anzeigenamen eines NPCs oder Gegenstands (FO_...).
func (a *app) displayName(name string) string {
	if t, ok := a.infoText("FO_" + name); ok && t != "" && t != "-" {
		return t
	}
	return name
}

// worldHasName prueft, ob es ein Objekt dieses Namens in der Welt gibt.
func (a *app) worldHasName(name string) bool {
	if a.world == nil {
		return true
	}
	for i := range a.world.Inst {
		if strings.EqualFold(a.world.Inst[i].Ent.Name, name) && !a.world.Inst[i].Hidden {
			return true
		}
	}
	return false
}

// --- Oberflaeche ---------------------------------------------------------------

func (a *app) fillQstList() {
	q := &a.qst
	a.loadQuests()
	own, _ := a.questNames()
	q.names = own
	sendMessage(q.list, LB_RESETCONTENT, 0, 0)
	for _, n := range own {
		sendMessage(q.list, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(a.questTitle(n)))))
	}
	if q.cur >= len(q.names) {
		q.cur = -1
	}
	if q.cur >= 0 {
		sendMessage(q.list, LB_SETCURSEL, uintptr(q.cur), 0)
	}
	a.updateQstButtons()
}

func (a *app) curQuest() *g3.Quest {
	q := &a.qst
	if q.cur >= 0 && q.cur < len(q.names) {
		return a.questByName(q.names[q.cur])
	}
	return nil
}

func (a *app) updateQstButtons() {
	q := &a.qst
	has := a.curQuest() != nil
	for _, h := range []uintptr{q.del, q.title, q.typ, q.giver, q.target, q.amount, q.xp, q.gold, q.apply, q.gen,
		q.pol, q.polAmt, q.enc, q.encAmt, q.attr, q.attrAmt, q.prereq, q.time} {
		v := uintptr(0)
		if has {
			v = 1
		}
		pEnableWindow.Call(h, v)
	}
	t := a.selQuestType()
	en := func(h uintptr, on bool) {
		v := uintptr(0)
		if on {
			v = 1
		}
		pEnableWindow.Call(h, v)
	}
	en(q.point, has && (t == g3.QuestFollow || usesPlace(t)))
	en(q.place, has && usesPlace(t))
	en(q.amount, has && (t == g3.QuestBring || t == g3.QuestRansack))
	// Beschriftungen je nach Aufgabe
	giver, target, place, point := T("Auftraggeber:"), T("Ziel:"), T("Ort:"), T("Gewählten Ort übernehmen")
	switch t {
	case g3.QuestFollow:
		giver, target, point = T("Führer:"), T("Zielpunkt:"), T("Zielpunkt in der Bildmitte setzen")
	case g3.QuestBringNPC:
		target = T("Wen bringen:")
	case g3.QuestDriveAway:
		target = T("Wen vertreiben:")
	case g3.QuestArena:
		target, place, point = T("Gegner:"), T("Arena:"), T("Gewählte Arena übernehmen")
	case g3.QuestReach:
		target, place, point = T("Wer (leer = Held):"), T("Ort (Zone):"), T("Gewählte Zone übernehmen")
	case g3.QuestRansack:
		target, place, point = T("Gegenstand:"), T("Truhe:"), T("Gewählte Truhe übernehmen")
	case g3.QuestFree:
		target = T("Ort (Enklave):")
	case g3.QuestSpell:
		target, place = T("Auf wen:"), T("Zauber:")
	}
	// bei "Zauber wirken" steht statt des Ortes die Zauberauswahl
	if t == g3.QuestSpell && a.tab == 3 {
		pShowWindow.Call(q.place, SW_HIDE)
		pShowWindow.Call(q.spell, SW_SHOW)
	} else {
		pShowWindow.Call(q.spell, SW_HIDE)
		if a.tab == 3 {
			pShowWindow.Call(q.place, SW_SHOW)
		}
	}
	en(q.spell, has && t == g3.QuestSpell)
	setText(q.giverLbl, giver)
	setText(q.targetLbl, target)
	setText(q.placeLbl, place)
	if t == g3.QuestReach {
		point = T("Gewählte übernehmen")
	}
	setText(q.point, point)
	if t == g3.QuestReach {
		setText(q.zoneBtn, T("Zone wählen / anlegen …"))
	} else {
		setText(q.zoneBtn, T("Zone in der Nähe …"))
	}
	en(q.zoneBtn, has && (t == g3.QuestReach || t == g3.QuestArena))
	a.layoutQstPointRow()
}

// questGiver liefert den Auftraggeber (bei "ausrichten" aus dem Start-Dialog,
// bei "folgen" den Fuehrer).
func (a *app) questGiver(qq *g3.Quest) string {
	if qq.GetInt("Type") == g3.QuestFollow {
		return qq.Get("DeliveryEntities")
	}
	if g := qq.Get("DestinationEntity"); g != "" && qq.GetInt("Type") != g3.QuestReport && !usesPlace(qq.GetInt("Type")) {
		return g
	}
	a.loadDialogs()
	for _, in := range a.dlg.infos {
		if strings.EqualFold(in.Quest, qq.Name()) && in.ConditionType == g3.CondActivator {
			return in.Owner
		}
	}
	if usesPlace(qq.GetInt("Type")) {
		return ""
	}
	return qq.Get("DestinationEntity")
}

// showQuest zeigt die gewaehlte Quest in den Feldern.
func (a *app) showQuest() {
	q := &a.qst
	q.loading = true
	defer func() { q.loading = false }()
	q.changed = false
	qq := a.curQuest()
	a.updateQstButtons()
	if qq == nil {
		for _, h := range []uintptr{q.title, q.giver, q.target, q.place, q.amount, q.xp, q.gold, q.polAmt, q.encAmt, q.attrAmt, q.prereq, q.time} {
			setText(h, "")
		}
		for _, h := range []uintptr{q.pol, q.enc, q.attr} {
			sendMessage(h, CB_SETCURSEL, 0, 0)
		}
		a.updateQstHelp()
		return
	}
	setText(q.title, a.questTitle(qq.Name()))
	t := qq.GetInt("Type")
	sendMessage(q.typ, CB_SETCURSEL, uintptr(questTypeIndex(t)), 0)
	a.updateQstButtons()
	setText(q.giver, a.questGiver(qq))
	setText(q.place, "")
	switch {
	case t == g3.QuestFollow:
		setText(q.target, qq.Get("DestinationEntity"))
	case usesPlace(t):
		tg := qq.Get("DeliveryEntities")
		if t == g3.QuestReach && strings.EqualFold(tg, "PC_Hero") {
			tg = ""
		}
		setText(q.target, tg)
		setText(q.place, qq.Get("DestinationEntity"))
	default:
		setText(q.target, qq.Get("DeliveryEntities"))
	}
	setText(q.amount, qq.Get("DeliveryAmounts"))
	setText(q.xp, qq.Get("ExperiencePoints"))
	setText(q.gold, a.questGold(qq.Name()))
	a.showQuestRewards(qq)
	si := 0
	for i, sp := range questSpells {
		if strings.EqualFold(sp.id, qq.Get("SpellName")) {
			si = i
		}
	}
	sendMessage(q.spell, CB_SETCURSEL, uintptr(si), 0)
	a.updateQstHelp()
}

// questGold liest die Gold-Belohnung aus dem erzeugten Abschluss-Dialog.
func (a *app) questGold(name string) string {
	a.loadDialogs()
	for _, in := range a.dlg.infos {
		if !isOwnInfo(in) || !strings.EqualFold(in.Quest, name) || (in.ConditionType != g3.CondDelivery && in.ConditionType != g3.CondSuccess) {
			continue
		}
		for _, c := range in.Script {
			if c.Cmd == "Give" && c.Ent2 == "player" && strings.EqualFold(c.ID1, "It_Gold") {
				return c.ID2
			}
		}
	}
	return ""
}

func (a *app) updateQstHelp() {
	q := &a.qst
	var b strings.Builder
	if q.note != "" {
		b.WriteString(q.note + "\r\n\r\n")
	}
	switch a.selQuestType() {
	case g3.QuestDriveAway:
		b.WriteString(T("Jemanden vertreiben: Ziel = der NPC, der verschwinden soll. Der Held stellt ihn zur Rede, es kommt zum Zweikampf; nach seiner Niederlage gibt er nach und zieht weg, danach holt sich der Held beim Auftraggeber die Belohnung. Wohin er geht, bestimmt ein Tagesablauf namens „Vertrieben“: im Reiter NPCs für ihn anlegen und die Plätze setzen (sonst bleibt er, wo er ist)."))
	case g3.QuestArena:
		b.WriteString(T("Arenakampf: Gegner = der NPC, gegen den der Held in der Arena antritt. Arena = eine vorhandene Arena-Zone (z. B. Montera_Arena, Faring_Arena_City, Arena_Silden, Arena_Ishtar) – eintippen oder in der Welt anklicken (Häkchen „Hilfsobjekte“) und „Gewählte Arena übernehmen“. Nach dem Gespräch gehen beide in die Arena; besiegt der Held den Gegner dort, ist die Quest geschafft. Verliert er, kann er erneut antreten."))
	case g3.QuestReach:
		b.WriteString(T("Ort erreichen: Ort = eine vorhandene Zone in der Welt (z. B. Varant_Passarea, MoraSul_Zone) – eintippen (mit Vorschlagsliste) oder in der Welt anklicken (Häkchen „Hilfsobjekte“) und „Gewählte übernehmen“ – oder mit „Zone wählen / anlegen …“ eine Zone um die Bildmitte wählen bzw. eine neue runde Zone in der Bildmitte anlegen. Neue Zonen trägt AdanosEdit beim Speichern in die Navigationskarte der Mod ein (nur so erkennt Gothic 3 sie). Am besten klein halten und nicht mitten auf Wege von NPCs legen; es sollte nur eine Mod mit eigener Navigationskarte gleichzeitig integriert sein. Wer = wer dort ankommen muss; leer = der Held. Sobald er die Zone betritt, ist die Quest geschafft; die Belohnung gibt es beim Auftraggeber."))
	case g3.QuestFree:
		b.WriteString(T("Ort befreien: Ziel = eine Enklave, also eine Stadt, ein Lager oder Ort, der von einer Gruppe gehalten wird (z. B. Nrd_OrcCamp_5, ein Orklager in Nordmar mit 17 Orks; mit Vorschlagsliste, mehrere mit ; trennen). Die Quest ist geschafft, wenn der Held die Besatzer besiegt hat. Ohne Besatzer klappt das nicht – wer vorher NPCs entfernt hat, legt eigene mit dieser Enklave an. Achtung: Städte wie Montera oder Geldern lösen beim Befreien auch die Revolution des Originalspiels aus."))
	case g3.QuestSpell:
		b.WriteString(T("Zauber auf jemanden wirken: Auf wen = der NPC, Zauber = welcher Zauber auf ihn gesprochen werden soll (z. B. Heilung anderer, Schlaf, Furcht). Der Auftraggeber gibt dem Helden die passende Schriftrolle. Im Spiel zählt nur genau dieser Zauber, und er muss genau diesen NPC treffen – andere NPCs in der Nähe können ihn abfangen, also das Ziel anvisieren, bis sein Name erscheint. Danach holt sich der Held beim Auftraggeber die Belohnung. Angriffszauber können das Ziel töten."))
	case g3.QuestRansack:
		b.WriteString(T("Truhe ausrauben: Truhe in der Welt anklicken und „Gewählte Truhe übernehmen“ (sie bekommt dabei einen eigenen Namen). Gegenstand und Anzahl = was der Held herausholen muss (z. B. It_Gold und 300). Liegt nicht genug davon fest in der Truhe, legt AdanosEdit es hinein. Das Schloss stellst du im Reiter Welt mit „Truhe / Schloss …“ ein."))
	case g3.QuestBringNPC:
		b.WriteString(T("Jemanden zu einer Person bringen: Wen bringen = der NPC, der dem Helden folgen soll. Auftraggeber = die Person, zu der er gebracht wird (sie vergibt die Quest). Der Held spricht den NPC an, der schließt sich ihm an; kommt der Held mit ihm beim Auftraggeber an, kann er die Quest abgeben."))
		b.WriteString(" " + T("Danach geht der NPC zurück zu seinen gewohnten Plätzen. Soll er bleiben: im Reiter NPCs für ihn einen neuen Tagesablauf mit Plätzen beim Auftraggeber anlegen und im Abgabe-Dialog die Zeile „> Tagesablauf NPC-Name Ablauf-Name“ ergänzen."))
	case g3.QuestFollow:
		b.WriteString(T("Jemandem folgen: Führer = der NPC, der den Helden führt (er vergibt die Quest). Zielpunkt = wohin er geht: Kamera dorthin bewegen und „Zielpunkt in der Bildmitte setzen“ klicken (oder den Namen eines vorhandenen Punkts eintragen). Nach dem Gespräch läuft der Führer los; ist er angekommen, spricht er den Helden von selbst an und die Quest ist geschafft."))
	case g3.QuestReport:
		b.WriteString(T("Jemandem etwas ausrichten: Der Held soll mit einer Person sprechen. Ziel = Name dieser Person (wie im Dialog-Reiter). Die Quest ist erfüllt, wenn der Held mit ihr spricht."))
	case g3.QuestDefeat:
		b.WriteString(T("Jemanden besiegen: Ziel = der NPC, den der Held im Zweikampf niederschlagen soll (nicht töten). Ist der Auftraggeber derselbe NPC, fordert er den Helden selbst heraus. Sonst schickt der Auftraggeber den Helden zu ihm; der Held fordert ihn heraus und holt sich nach dem Sieg beim Auftraggeber die Belohnung."))
	case g3.QuestKill:
		b.WriteString(T("Jemanden töten: Ziel = Name des NPCs oder Monsters in der Welt (mehrere mit ; trennen). Ist das Ziel tot, gilt die Quest als erfolgreich; danach holt sich der Held beim Auftraggeber die Belohnung."))
	default:
		b.WriteString(T("Gegenstände bringen: Ziel = Vorlagenname des Gegenstands (z. B. It_Apple), Anzahl = wie viele. Mehrere Gegenstände mit ; trennen (z. B. It_Apple;It_Bread und 3;2). Hat der Held alles dabei, kann er die Quest beim Auftraggeber abgeben."))
	}
	b.WriteString("\r\n\r\n" + T("Weitere Belohnungen: Ansehen bei einer Gruppe (Orks, Rebellen …) oder in einem Ort (Stadt, Lager, Clan) und ein Attribut (z. B. Stärke +1) – das Spiel vergibt sie beim Erfolg. Erst nach Quest: die Quest lässt sich erst annehmen, wenn diese Quests geschafft sind (Namen aus der Vorschlagsliste, mehrere mit ; trennen) – so entstehen Questketten. Zeitlimit: nach so vielen Spieltagen ist der Auftrag verfallen; der Auftraggeber sagt es dem Helden und beendet die Quest."))
	b.WriteString("\r\n\r\n" + T("Erfahrung bekommt der Held automatisch beim Erfolg, das Gold im Abschluss-Dialog. „Passende Dialoge erzeugen“ legt die Gespräche zum Vergeben, Nachfragen und Abgeben an; im Reiter Dialoge kannst du sie danach umschreiben. Neue Quests brauchen ein neues Archiv: nach dem Speichern die Mod im Mod Starter einmal neu integrieren."))
	setText(q.help, b.String())
}

// commitQuest uebernimmt Aenderungen (falls vorhanden); false = Fehler, bleiben.
func (a *app) commitQuest() bool {
	q := &a.qst
	if !q.changed {
		return true
	}
	msg := a.applyQuest()
	if msg == "" {
		return true
	}
	if messageBoxRet(a.hwnd, msg+"\n\n"+T("Änderungen an dieser Quest verwerfen?"), T(appTitleDE), MB_YESNO|MB_ICONWARNING) != IDYES {
		return false
	}
	q.changed = false
	q.note = ""
	return true
}

func (a *app) onApplyQuest() {
	if msg := a.applyQuest(); msg != "" {
		messageBox(a.hwnd, msg, T(appTitleDE), MB_ICONWARNING)
	}
}

// applyQuest uebernimmt die Felder; Rueckgabe = Fehlermeldung.
func (a *app) applyQuest() string {
	q := &a.qst
	old := a.curQuest()
	if old == nil {
		return ""
	}
	fail := func(m string) string {
		q.note = "⚠ " + m
		a.updateQstHelp()
		return m
	}
	if loadConfig()["SaveMode"] == "data" {
		return fail(T("Dialoge brauchen eigene Archive. Das geht nur mit dem Mod Starter.\n\nStelle dafür in AdanosEdit.ini SaveMode=mds ein."))
	}
	title := strings.TrimSpace(getText(q.title))
	if title == "" {
		return fail(T("Bitte einen Titel für die Quest eingeben (er steht im Tagebuch)."))
	}
	typ := a.selQuestType()
	giver := strings.TrimSpace(getText(q.giver))
	if giver == "" {
		return fail(T("Bitte den Auftraggeber eintragen (Name des NPCs)."))
	}
	var targets []string
	for _, t := range strings.Split(getText(q.target), ";") {
		if t = strings.TrimSpace(t); t != "" {
			targets = append(targets, t)
		}
	}
	if len(targets) == 0 && typ == g3.QuestReach {
		targets = []string{"PC_Hero"}
	}
	if len(targets) == 0 {
		return fail(T("Bitte ein Ziel eintragen."))
	}
	place := strings.TrimSpace(getText(q.place))
	if usesPlace(typ) {
		if place == "" {
			switch typ {
			case g3.QuestArena:
				return fail(T("Bitte die Arena eintragen (z. B. Montera_Arena)."))
			case g3.QuestReach:
				return fail(T("Bitte den Ort (eine Zone) eintragen."))
			default:
				return fail(T("Bitte die Truhe in der Welt anklicken und „Gewählte Truhe übernehmen“ drücken."))
			}
		}
		need := map[int]string{g3.QuestArena: "gCArena_PS", g3.QuestReach: "gCNavZone_PS", g3.QuestRansack: "gCInventory_PS"}[typ]
		if a.world != nil && a.findInstWithClass(place, need) < 0 {
			switch typ {
			case g3.QuestArena:
				return fail(fmt.Sprintf(T("„%s“ ist keine Arena. Vorhandene Arenen sind z. B. Montera_Arena, Faring_Arena_City, Arena_Silden, Arena_Ishtar."), place))
			case g3.QuestReach:
				return fail(fmt.Sprintf(T("„%s“ ist keine Zone in der Welt."), place))
			default:
				return fail(fmt.Sprintf(T("„%s“ ist keine Truhe in der Welt."), place))
			}
		}
	}
	var amounts []string
	if typ == g3.QuestRansack {
		v := strings.TrimSpace(getText(q.amount))
		if v == "" {
			v = "1"
		}
		if n, err := strconv.Atoi(v); err != nil || n < 1 {
			return fail(fmt.Sprintf(T("„%s“ ist keine gültige Zahl."), v))
		}
		amounts = []string{v}
	} else if typ == g3.QuestBring {
		as := strings.Split(getText(q.amount), ";")
		for i := range targets {
			v := "1"
			if i < len(as) && strings.TrimSpace(as[i]) != "" {
				v = strings.TrimSpace(as[i])
			}
			if n, err := strconv.Atoi(v); err != nil || n < 1 {
				return fail(fmt.Sprintf(T("„%s“ ist keine gültige Zahl."), v))
			}
			amounts = append(amounts, v)
		}
	} else {
		for range targets {
			amounts = append(amounts, "1")
		}
	}
	xp := 0
	if s := strings.TrimSpace(getText(q.xp)); s != "" {
		v, err := strconv.Atoi(s)
		if err != nil || v < 0 {
			return fail(fmt.Sprintf(T("„%s“ ist keine gültige Zahl."), s))
		}
		xp = v
	}
	if s := strings.TrimSpace(getText(q.gold)); s != "" {
		if v, err := strconv.Atoi(s); err != nil || v < 0 {
			return fail(fmt.Sprintf(T("„%s“ ist keine gültige Zahl."), s))
		}
	}
	// Warnungen
	var warn []string
	if !a.worldHasName(giver) {
		warn = append(warn, fmt.Sprintf(T("Einen NPC „%s“ finde ich in der Welt nicht."), giver))
	}
	items := a.itemNames()
	for _, t := range targets {
		if typ == g3.QuestReach && strings.EqualFold(t, "PC_Hero") {
			continue
		}
		if typ == g3.QuestBring || typ == g3.QuestRansack {
			if items != nil && !items[strings.ToLower(t)] {
				warn = append(warn, fmt.Sprintf(T("Den Gegenstand „%s“ kenne ich nicht (Vorlagenname wie It_Gold oder It_Apple?)."), t))
			}
		} else if typ == g3.QuestFollow {
			if !a.worldHasName(t) {
				warn = append(warn, fmt.Sprintf(T("Einen Zielpunkt „%s“ finde ich in der Welt nicht. Setze ihn mit „Zielpunkt in der Bildmitte setzen“."), t))
			}
		} else if !a.worldHasName(t) {
			warn = append(warn, fmt.Sprintf(T("Einen NPC „%s“ finde ich in der Welt nicht."), t))
		}
	}
	if typ == g3.QuestFree {
		// Ziele sind Enklaven, keine NPCs: Warnungen dazu neu bilden
		warn = nil
		if !a.worldHasName(giver) {
			warn = append(warn, fmt.Sprintf(T("Einen NPC „%s“ finde ich in der Welt nicht."), giver))
		}
		for _, t := range targets {
			n, ok := a.enclaveMembers(t)
			if !ok {
				return fail(fmt.Sprintf(T("„%s“ ist keine Enklave in der Welt. Bitte aus der Vorschlagsliste wählen (z. B. Nrd_OrcCamp_5)."), t))
			}
			if n == 0 {
				warn = append(warn, fmt.Sprintf(T("Zur Enklave „%s“ gehört in deiner Welt kein NPC mehr. Ohne Besatzer kann sie nicht befreit werden: eigene NPCs mit dieser Enklave anlegen oder stattdessen „Jemanden töten“ verwenden."), t))
			}
		}
		for i := range amounts {
			amounts[i] = "1"
		}
	}
	if typ != g3.QuestBring && typ != g3.QuestKill && typ != g3.QuestReport && typ != g3.QuestFree && len(targets) > 1 {
		return fail(T("Bitte nur ein Ziel eintragen."))
	}
	if typ == g3.QuestBringNPC && strings.EqualFold(targets[0], giver) {
		return fail(T("Der NPC kann nicht zu sich selbst gebracht werden. Auftraggeber = die Person, zu der er gebracht wird."))
	}
	nq := old.Clone()
	name := nq.Name()
	key := "QUEST_" + name
	nq.Set("LogTopic", key)
	nq.SetInt("Type", typ)
	nq.SetInt("ExperiencePoints", xp)
	nq.Set("DeliveryEntities", strings.Join(targets, ";"))
	nq.Set("DeliveryAmounts", strings.Join(amounts, ";"))
	if msg := a.applyQuestRewards(nq); msg != "" {
		return fail(msg)
	}
	switch typ {
	case g3.QuestReport:
		nq.Set("DestinationEntity", "")
	case g3.QuestDefeat:
		if strings.EqualFold(giver, targets[0]) {
			nq.Set("DestinationEntity", "")
		} else {
			nq.Set("DestinationEntity", giver)
		}
	case g3.QuestFollow:
		// der Fuehrer geht zum Zielpunkt
		nq.Set("DeliveryEntities", giver)
		nq.Set("DestinationEntity", targets[0])
	case g3.QuestArena, g3.QuestReach, g3.QuestRansack:
		nq.Set("DestinationEntity", place)
	case g3.QuestFree, g3.QuestSpell:
		nq.Set("DestinationEntity", "")
	default:
		nq.Set("DestinationEntity", giver)
	}
	if typ == g3.QuestSpell {
		nq.Set("SpellName", questSpells[max(0, comboSel(q.spell))].id)
	} else if nq.Get("SpellName") != "" {
		nq.Set("SpellName", "")
	}
	a.setText(key, title)
	path := g3.QuestPath(name)
	a.ed.setExtra("Quests", path, nq.Bytes())
	q.quests[strings.ToLower(path)] = nq
	q.paths[strings.ToLower(path)] = path
	// Gold im Abschluss-Dialog anpassen
	a.updateQuestGold(name, strings.TrimSpace(getText(q.gold)))
	// Zeitlimit: Gespraech "zu spaet" ergaenzen bzw. entfernen
	a.updateOvertimeDialog(nq, giver)
	// Truhe ausrauben: genug davon muss fest in der Truhe liegen
	if typ == g3.QuestRansack {
		n, _ := strconv.Atoi(amounts[0])
		if msg := a.ensureChestItem(place, targets[0], n); msg != "" {
			warn = append(warn, msg)
		}
	}
	q.changed = false
	if len(warn) > 0 {
		q.note = "⚠ " + strings.Join(warn, "\r\n⚠ ")
	} else {
		q.note = "✓ " + T("Übernommen. Zum Spielen speichern und die Mod im Mod Starter neu integrieren.")
	}
	cur := q.cur
	a.fillQstList()
	q.cur = cur
	sendMessage(q.list, LB_SETCURSEL, uintptr(cur), 0)
	a.updateQstButtons()
	a.updateQstHelp()
	a.updateTitle()
	return ""
}

// setText merkt sich einen neuen Text fuer die stringtable.ini (alle Sprachen gleich).
func (a *app) setText(key, text string) {
	a.ed.setString(key, []string{text})
	v := make([]string, len(g3.StringLangs))
	for i := range v {
		v[i] = text
	}
	if a.dlg.strs != nil {
		a.dlg.strs[key] = v
	}
}

// updateQuestGold passt die Gold-Belohnung im erzeugten Abschluss-Dialog an.
func (a *app) updateQuestGold(quest, gold string) {
	d := &a.dlg
	for lp, in := range d.infos {
		if !isOwnInfo(in) || !strings.EqualFold(in.Quest, quest) || (in.ConditionType != g3.CondDelivery && in.ConditionType != g3.CondSuccess) {
			continue
		}
		var sc []g3.InfoCmd
		found := false
		for _, c := range in.Script {
			if strings.EqualFold(c.ID1, "It_Gold") && (c.Cmd == "Grant" && c.Ent1 == "npc" || c.Cmd == "Give" && c.Ent1 == "npc" && c.Ent2 == "player") {
				if gold == "" || gold == "0" {
					found = true
					continue
				}
				if c.ID2 != gold {
					found = true
				}
				c.ID2 = gold
			}
			sc = append(sc, c)
		}
		if !found {
			continue
		}
		ni := *in
		ni.Script = sc
		p := d.paths[lp]
		a.ed.setExtra("Infos", p, ni.Bytes())
		d.infos[lp] = &ni
	}
}

// newQuest legt eine neue Quest an.
func (a *app) newQuest() {
	q := &a.qst
	if !a.commitQuest() {
		return
	}
	a.loadQuests()
	name := ""
	for i := 1; ; i++ {
		name = fmt.Sprintf("%sQuest_%02d", a.ownPrefix(), i)
		if a.questByName(name) == nil {
			break
		}
	}
	nq := g3.NewQuest(name)
	nq.Set("LogTopic", "QUEST_"+name)
	nq.SetInt("ExperiencePoints", 100)
	path := g3.QuestPath(name)
	q.quests[strings.ToLower(path)] = nq
	q.paths[strings.ToLower(path)] = path
	a.setText("QUEST_"+name, T("Neue Quest"))
	a.fillQstList()
	for i, n := range q.names {
		if n == name {
			q.cur = i
		}
	}
	sendMessage(q.list, LB_SETCURSEL, uintptr(q.cur), 0)
	a.showQuest()
	// Vorgaben: Auftraggeber = gewaehlter NPC
	g := a.dlg.owner
	if a.selected >= 0 && a.world != nil && int(a.selected) < len(a.world.Inst) {
		if e := a.world.Inst[a.selected].Ent; e.Kind == g3.KindNPC && e.Name != "" {
			g = e.Name
		}
	}
	setText(q.giver, g)
	setText(q.amount, "1")
	setText(q.gold, "50")
	q.changed = true
	pSetFocus.Call(q.title)
	sendMessage(q.title, EM_SETSEL, 0, ^uintptr(0))
}

// deleteQuest loescht die gewaehlte Quest (und auf Wunsch ihre Dialoge).
func (a *app) deleteQuest() {
	q := &a.qst
	qq := a.curQuest()
	if qq == nil {
		return
	}
	name := qq.Name()
	if messageBoxRet(a.hwnd, fmt.Sprintf(T("Die Quest „%s“ löschen?"), a.questTitle(name)), T(appTitleDE), MB_YESNO|MB_ICONQUESTION) != IDYES {
		return
	}
	a.loadDialogs()
	var dl []string
	for lp, in := range a.dlg.infos {
		if isOwnInfo(in) && strings.EqualFold(in.Quest, name) {
			dl = append(dl, lp)
		}
	}
	if len(dl) > 0 && messageBoxRet(a.hwnd, fmt.Sprintf(T("Auch die %d Dialoge dieser Quest löschen?"), len(dl)), T(appTitleDE), MB_YESNO|MB_ICONQUESTION) == IDYES {
		for _, lp := range dl {
			p := a.dlg.paths[lp]
			if p == "" {
				p = lp
			}
			a.ed.setExtra("Infos", p, nil)
			delete(a.dlg.infos, lp)
		}
		a.indexDialogs()
	}
	path := g3.QuestPath(name)
	a.ed.setExtra("Quests", path, nil)
	delete(q.quests, strings.ToLower(path))
	q.cur = -1
	q.changed = false
	q.note = ""
	a.fillQstList()
	a.showQuest()
	a.updateTitle()
}

// generateQuestDialogs legt die Standard-Gespraeche einer Quest an.
func (a *app) generateQuestDialogs() {
	q := &a.qst
	if !a.commitQuest() {
		return
	}
	qq := a.curQuest()
	if qq == nil {
		return
	}
	name := qq.Name()
	a.loadDialogs()
	n := 0
	for _, in := range a.dlg.infos {
		if isOwnInfo(in) && strings.EqualFold(in.Quest, name) {
			n++
		}
	}
	if n > 0 && messageBoxRet(a.hwnd, fmt.Sprintf(T("Für diese Quest gibt es schon %d Dialoge. Trotzdem neue anlegen?"), n), T(appTitleDE), MB_YESNO|MB_ICONQUESTION) != IDYES {
		return
	}
	giver := strings.TrimSpace(getText(q.giver))
	if giver == "" {
		giver = a.questGiver(qq)
	}
	typ := qq.GetInt("Type")
	targets := strings.Split(qq.Get("DeliveryEntities"), ";")
	if typ == g3.QuestFollow {
		targets = []string{qq.Get("DestinationEntity")}
	}
	amounts := strings.Split(qq.Get("DeliveryAmounts"), ";")
	var tn []string
	for i, t := range targets {
		s := a.displayName(t)
		if qq.GetInt("Type") == g3.QuestBring || qq.GetInt("Type") == g3.QuestRansack {
			am := "1"
			if i < len(amounts) && amounts[i] != "" {
				am = amounts[i]
			}
			s = am + "× " + s
		}
		tn = append(tn, s)
	}
	what := strings.Join(tn, ", ")
	gold := strings.TrimSpace(getText(q.gold))
	reward := ""
	if gold != "" && gold != "0" {
		reward = "\n> " + T("Geben") + " It_Gold " + gold
	}
	gname := a.displayName(giver)
	type gd struct {
		owner string
		ct    int
		perm  bool
		text  string
		imp   bool // NPC spricht den Helden von selbst an
		news  bool // NPC meldet sich nach einem Ereignis (z. B. nach dem Zweikampf)
	}
	// Bedingung an den Zustand eines NPCs je Dialog: "Name=Zustand" (1 unbesiegt, 2 besiegt)
	secs := map[int]string{}
	var ds []gd
	var extraNote string
	place := qq.Get("DestinationEntity")
	switch typ {
	case g3.QuestDriveAway:
		foe := targets[0]
		fname := a.displayName(foe)
		leave := ""
		if r := a.driveAwayRoutine(foe); r != "" {
			leave = "\n> " + T("Tagesablauf") + " npc " + r
		} else {
			extraNote = fmt.Sprintf(T("Hinweis: %s hat noch keinen Tagesablauf „Vertrieben“. Lege ihn im Reiter NPCs an (Plätze, wohin er geht) und ergänze im Dialog „Schon gut … ich verschwinde“ die Zeile „> Tagesablauf npc Vertrieben“."), fname)
		}
		ds = []gd{
			{giver, g3.CondActivator, false, T("Held: Hast du Arbeit für mich?") + "\n" + fmt.Sprintf(T("NPC: %s treibt sich hier herum und macht Ärger. Sorg dafür, dass er verschwindet!"), fname), false, false},
			// wie im Original (Bernado_Pay): Herausforderung nur, solange er
			// unbesiegt ist; "Hast du jetzt genug?" erst, wenn der Held ihn besiegt hat
			{foe, g3.CondRunning, true, T("Held: Verschwinde von hier!") + "\n" + T("NPC: Zwing mich doch!") + "\n> !Attack;npc;;Duel\n> " + T("Ende"), false, false},
			{foe, g3.CondDelivery, false, T("Held: Hast du jetzt genug?") + "\n" + T("NPC: Schon gut, schon gut … ich verschwinde.") + leave + "\n> " + T("Ende"), false, false},
			{giver, g3.CondSuccess, false, fmt.Sprintf(T("Held: %s ist weg."), fname) + "\n" + T("NPC: Gut gemacht! Hier ist deine Belohnung.") + reward, false, false},
		}
		secs = map[int]string{1: foe + "=1", 2: foe + "=2"}
	case g3.QuestArena:
		foe := targets[0]
		fname := a.displayName(foe)
		rerun := T("Held: Ich will noch einmal antreten.") + "\n> !SaySVM;npc;;ThoughtSo\n> !SaySVM;npc;;SendToArena"
		if strings.EqualFold(foe, giver) {
			ds = []gd{
				{foe, g3.CondActivator, false, T("Held: Ich will in der Arena gegen dich antreten.") + "\n" + T("NPC: Gut. Wir sehen uns in der Arena!") + "\n> " + T("Ende"), false, false},
				{foe, g3.CondRerun, true, rerun, false, false},
				{foe, g3.CondSuccess, false, T("Held: Das war ein guter Kampf.") + "\n" + T("NPC: Du hast gewonnen. Hier ist dein Preis.") + reward, false, false},
			}
		} else {
			ds = []gd{
				{giver, g3.CondActivator, false, T("Held: Ich will in der Arena kämpfen.") + "\n" + fmt.Sprintf(T("NPC: Dein Gegner ist %s. Geh in die Arena!"), fname) + "\n> " + T("Ende"), false, false},
				{giver, g3.CondRerun, true, rerun, false, false},
				{giver, g3.CondSuccess, false, fmt.Sprintf(T("Held: Ich habe %s in der Arena besiegt."), fname) + "\n" + T("NPC: Gut gemacht! Hier ist dein Preis.") + reward, false, false},
			}
		}
	case g3.QuestReach:
		pl := a.displayName(place)
		ask := fmt.Sprintf(T("NPC: Geh nach %s und sieh dich dort um."), pl)
		if who := targets[0]; !strings.EqualFold(who, "PC_Hero") {
			ask = fmt.Sprintf(T("NPC: Sorg dafür, dass %s nach %s kommt."), a.displayName(who), pl)
		}
		ds = []gd{
			{giver, g3.CondActivator, false, T("Held: Hast du einen Auftrag für mich?") + "\n" + ask, false, false},
			{giver, g3.CondRunning, true, T("Held: Wegen deines Auftrags ...") + "\n" + fmt.Sprintf(T("NPC: Du warst noch nicht in %s."), pl), false, false},
			{giver, g3.CondSuccess, false, fmt.Sprintf(T("Held: Ich war in %s."), pl) + "\n" + T("NPC: Sehr gut! Hier ist deine Belohnung.") + reward, false, false},
		}
	case g3.QuestFree:
		var pl []string
		for _, t := range targets {
			pl = append(pl, enclaveLabel(t))
		}
		where := strings.Join(pl, ", ")
		ds = []gd{
			{giver, g3.CondActivator, false, T("Held: Hast du Arbeit für mich?") + "\n" + fmt.Sprintf(T("NPC: %s wird von Feinden gehalten. Vertreib die Besatzer und befreie den Ort!"), where), false, false},
			{giver, g3.CondRunning, true, fmt.Sprintf(T("Held: Wegen %s ..."), where) + "\n" + T("NPC: Solange die Besatzer dort sind, ist nichts gewonnen."), false, false},
			{giver, g3.CondSuccess, false, fmt.Sprintf(T("Held: %s ist befreit."), where) + "\n" + T("NPC: Großartig! Hier ist deine Belohnung.") + reward, false, false},
		}
	case g3.QuestSpell:
		who := targets[0]
		wname := a.displayName(who)
		sp := spellByID(qq.Get("SpellName"))
		ds = []gd{
			{giver, g3.CondActivator, false, T("Held: Kann ich dir irgendwie helfen?") + "\n" + fmt.Sprintf(T("NPC: Sprich den Zauber „%s“ auf %s. Hier, nimm diese Schriftrolle."), T(sp.de), wname) + "\n> " + T("Geben") + " " + sp.scroll + " 1", false, false},
			{giver, g3.CondRunning, true, fmt.Sprintf(T("Held: Wegen %s ..."), wname) + "\n" + T("NPC: Hast du den Zauber schon gesprochen? Ziel genau auf ihn, sonst trifft er jemand anderen."), false, false},
			{giver, g3.CondSuccess, false, fmt.Sprintf(T("Held: Ich habe den Zauber auf %s gesprochen."), wname) + "\n" + T("NPC: Danke! Hier ist deine Belohnung.") + reward, false, false},
		}
	case g3.QuestRansack:
		ds = []gd{
			{giver, g3.CondActivator, false, T("Held: Hast du Arbeit für mich?") + "\n" + fmt.Sprintf(T("NPC: In einer Truhe ganz in der Nähe liegen %s. Hol sie dir – aber lass dich nicht erwischen!"), what), false, false},
			{giver, g3.CondRunning, true, T("Held: Wegen der Truhe ...") + "\n" + T("NPC: Worauf wartest du noch?"), false, false},
			{giver, g3.CondSuccess, false, T("Held: Ich habe die Truhe ausgeräumt.") + "\n" + T("NPC: Gut gemacht! Hier ist deine Belohnung.") + reward, false, false},
		}
	case g3.QuestBringNPC:
		who := targets[0]
		wname := a.displayName(who)
		ds = []gd{
			{giver, g3.CondActivator, false, T("Held: Kann ich dir irgendwie helfen?") + "\n" + fmt.Sprintf(T("NPC: Ja. Bring %s zu mir. Ich muss dringend mit ihm reden."), wname), false, false},
			{who, g3.CondRunning, false, fmt.Sprintf(T("Held: %s will dich sprechen. Komm mit mir."), gname) + "\n" + T("NPC: Gut, geh voraus. Ich folge dir.") + "\n> !SetPartyEnabled;npc;;true\n> !JoinPlayer;npc;;true", false, false},
			{giver, g3.CondDelivery, false, fmt.Sprintf(T("Held: Ich habe %s zu dir gebracht."), wname) + "\n" + T("NPC: Sehr gut! Hier ist deine Belohnung.") + "\n> !JoinPlayer;" + who + ";;false\n> !SetPartyEnabled;" + who + ";;false" + reward, false, false},
		}
	case g3.QuestDefeat:
		foe := targets[0]
		fname := a.displayName(foe)
		fight := "\n> !Attack;npc;;Duel\n> " + T("Ende")
		if strings.EqualFold(foe, giver) {
			ds = []gd{
				{foe, g3.CondActivator, false, T("Held: Ich fordere dich zum Zweikampf heraus!") + "\n" + T("NPC: Na schön. Zeig mir, was du kannst!") + fight, false, false},
				{foe, g3.CondSuccess, false, T("NPC: Du hast mich besiegt. Alle Achtung!") + reward, false, true},
			}
		} else {
			ds = []gd{
				{giver, g3.CondActivator, false, T("Held: Hast du Arbeit für mich?") + "\n" + fmt.Sprintf(T("NPC: %s hält sich für unbesiegbar. Zeig ihm im Zweikampf, wo sein Platz ist – aber bring ihn nicht um!"), fname), false, false},
				{foe, g3.CondRunning, false, T("Held: Ich fordere dich zum Zweikampf heraus!") + "\n" + T("NPC: Das wirst du bereuen!") + fight, false, false},
				{giver, g3.CondSuccess, false, fmt.Sprintf(T("Held: Ich habe %s besiegt."), fname) + "\n" + T("NPC: Gut gemacht! Hier ist deine Belohnung.") + reward, false, false},
			}
		}
	case g3.QuestFollow:
		ds = []gd{
			{giver, g3.CondActivator, false, T("Held: Kannst du mir den Weg zeigen?") + "\n" + T("NPC: Sicher. Folge mir, ich bringe dich hin.") + "\n> " + T("Ende"), false, false},
			{giver, g3.CondRunning, true, T("Held: Wie weit ist es noch?") + "\n" + T("NPC: Nicht mehr weit. Bleib dicht bei mir."), false, false},
			{giver, g3.CondDelivery, false, T("NPC: Da sind wir.") + "\n" + T("NPC: Hier trennen sich unsere Wege.") + reward, true, false},
		}
	case g3.QuestReport:
		target := targets[0]
		ds = []gd{
			{giver, g3.CondActivator, false, T("Held: Kann ich dir irgendwie helfen?") + "\n" + fmt.Sprintf(T("NPC: Ja. Geh zu %s und sag ihm, dass ich ihn sprechen will."), what), false, false},
			{giver, g3.CondRunning, true, fmt.Sprintf(T("Held: Wegen deiner Nachricht an %s ..."), what) + "\n" + T("NPC: Warst du schon bei ihm?"), false, false},
			{target, g3.CondDelivery, false, fmt.Sprintf(T("Held: %s schickt mich. Er will dich sprechen."), gname) + "\n" + T("NPC: Danke, dass du mir Bescheid sagst. Hier, nimm das.") + reward, false, false},
		}
	case g3.QuestKill:
		ds = []gd{
			{giver, g3.CondActivator, false, T("Held: Hast du Arbeit für mich?") + "\n" + fmt.Sprintf(T("NPC: %s macht mir Ärger. Erledige das für mich, und es soll dein Schaden nicht sein."), what), false, false},
			{giver, g3.CondRunning, true, fmt.Sprintf(T("Held: Wegen %s ..."), what) + "\n" + T("NPC: Worauf wartest du noch?"), false, false},
			{giver, g3.CondSuccess, false, fmt.Sprintf(T("Held: %s ist erledigt."), what) + "\n" + T("NPC: Gut gemacht! Hier ist deine Belohnung.") + reward, false, false},
		}
	default:
		ds = []gd{
			{giver, g3.CondActivator, false, T("Held: Hast du Arbeit für mich?") + "\n" + fmt.Sprintf(T("NPC: Ja. Bring mir %s, dann wirst du belohnt."), what), false, false},
			{giver, g3.CondRunning, true, T("Held: Wegen deines Auftrags ...") + "\n" + fmt.Sprintf(T("NPC: Ich warte noch auf %s."), what), false, false},
			{giver, g3.CondDelivery, false, T("Held: Ich habe, was du wolltest.") + "\n" + T("NPC: Sehr gut! Hier ist deine Belohnung.") + reward, false, false},
		}
	}
	if qq.GetInt("RunningTimeDays") > 0 {
		// Zeitlimit abgelaufen: der Auftraggeber spricht den Helden an
		ds = append(ds, gd{giver, g3.CondOvertime, false, overtimeScript(name), true, false})
	}
	for gi, g := range ds {
		it := g3.InfoInfo
		if g.imp {
			it = g3.InfoImportant
		} else if g.news {
			it = g3.InfoNews
		}
		var secNPC, secState string
		if sc := secs[gi]; sc != "" {
			secNPC, secState, _ = strings.Cut(sc, "=")
		}
		if msg := a.addOwnInfoSec(g.owner, g.ct, it, name, g.perm, g.text, secNPC, secState); msg != "" {
			messageBox(a.hwnd, msg, T(appTitleDE), MB_ICONWARNING)
			return
		}
	}
	a.indexDialogs()
	var owners []string
	seenOwner := map[string]bool{}
	for _, g := range ds {
		if !seenOwner[strings.ToLower(g.owner)] {
			seenOwner[strings.ToLower(g.owner)] = true
			owners = append(owners, g.owner)
		}
	}
	q.note = "✓ " + fmt.Sprintf(T("%d Dialoge angelegt. Im Reiter Dialoge kannst du sie bei %s ansehen und umschreiben."), len(ds), strings.Join(owners, ", "))
	if extraNote != "" {
		q.note += "\r\n⚠ " + extraNote
	}
	a.updateQstHelp()
	a.updateTitle()
	// Dialoge des Auftraggebers zeigen
	a.dlg.owner = giver
	a.dlg.cur = -1
	a.dlg.keepOwner = true
	if a.dlg.npcs != 0 {
		a.fillDlgList()
	}
}

// addOwnInfo legt ein eigenes Gespraech an (Text in der Form des Dialog-Reiters).
func (a *app) addOwnInfo(owner string, ct, infoType int, quest string, perm bool, text string) string {
	return a.addOwnInfoSec(owner, ct, infoType, quest, perm, text, "", "")
}

// addOwnInfoSec wie addOwnInfo, mit Bedingung an den Zustand eines NPCs
// (CondSecondaryNPC/-states, z. B. "2" = vom Helden besiegt).
func (a *app) addOwnInfoSec(owner string, ct, infoType int, quest string, perm bool, text, secNPC, secState string) string {
	d := &a.dlg
	a.loadDialogs()
	maxSort := 90000
	names := map[string]bool{}
	for _, in := range d.infos {
		names[strings.ToLower(in.Name)] = true
		if isOwnInfo(in) && in.SortID > maxSort {
			maxSort = in.SortID
		}
	}
	base := a.ownPrefix() + npcInternalName(owner)
	name := ""
	for i := 1; ; i++ {
		name = fmt.Sprintf("%s_%02d", base, i)
		if !names[strings.ToLower(name)] {
			break
		}
	}
	in := &g3.Info{Name: name, SortID: maxSort + 1, Owner: owner, ConditionType: ct, InfoType: infoType,
		Folder: "G3Viewer/", Quest: quest, Permanent: perm, CondSecondaryNPC: secNPC, CondSecondaryNPCstates: secState}
	sc, strs, _, err := a.parseScript(text, in)
	if err != nil {
		return err.Error()
	}
	in.Script = placeMarker(sc, name)
	for k, t := range strs {
		a.setText(k, t)
	}
	p := g3.InfoPath(name)
	a.ed.setExtra("Infos", p, in.Bytes())
	d.infos[strings.ToLower(p)] = in
	d.paths[strings.ToLower(p)] = p
	return ""
}

func (a *app) onQstTab() {
	a.loadQuests()
	a.fillQstList()
	a.showQuest()
}

// placeQuestPoint legt fuer "Jemandem folgen" einen benannten Zielpunkt in der
// Bildmitte an (oder versetzt ihn dorthin, falls es ihn schon gibt).
func (a *app) placeQuestPoint() {
	q := &a.qst
	qq := a.curQuest()
	if qq == nil || a.world == nil {
		return
	}
	c, ok := a.centerPoint()
	if !ok {
		a.flash(T("Bitte die Kamera so drehen, dass die Bildmitte auf den Boden zeigt."))
		return
	}
	name := qq.Name() + "_Ziel"
	world := [16]float32{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, c[0], c[1], c[2], 1}
	// vorhandenen Punkt versetzen
	for i := range a.world.Inst {
		in := &a.world.Inst[i]
		if !strings.EqualFold(in.Ent.Name, name) || in.Hidden {
			continue
		}
		wf, err := a.editFile(in)
		if err != nil {
			break
		}
		k := in.Ent.Index
		old := wf.Ents[k].World
		wf.SetWorld(k, world)
		a.syncFile(wf)
		a.ed.undo = append(a.ed.undo, func() {
			wf.SetWorld(k, old)
			a.syncFile(wf)
		})
		setText(q.target, name)
		q.changed = true
		a.select_(int32(i), false)
		a.flash(fmt.Sprintf(T("Zielpunkt „%s“ versetzt. Strg+Z macht es rückgängig."), name))
		return
	}
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
		a.flash(T("Keine Vorlage für Routinepunkte gefunden."))
		return
	}
	wf, err := a.editFilePath(newObjectFile)
	if err != nil {
		a.flash(fmt.Sprintf(T("Neue Objekte können nicht angelegt werden: %s"), T(err.Error())))
		return
	}
	ci, err := wf.AddFromTemplate(tpl, world)
	if err == nil {
		err = wf.SetName(ci, name)
	}
	if err != nil {
		a.flash(fmt.Sprintf(T("Anlegen nicht möglich: %s"), T(err.Error())))
		return
	}
	sector := strings.TrimPrefix(newObjectFile[:strings.LastIndex(newObjectFile, "/")], "G3_World_01/")
	if a.ed.inst[wf.Path] == nil {
		a.ed.inst[wf.Path] = map[int]int32{}
	}
	ii := a.world.Add(wf.Ents[ci], sector)
	if ii >= 0 {
		a.ed.inst[wf.Path][ci] = ii
	}
	wf.Changed = map[int]bool{ci: true}
	a.syncFile(wf)
	a.ed.undo = append(a.ed.undo, func() {
		wf.Delete(ci)
		a.syncFile(wf)
	})
	setText(q.target, name)
	q.changed = true
	if ii >= 0 {
		a.select_(ii, false)
	}
	a.flash(fmt.Sprintf(T("Zielpunkt „%s“ angelegt (sichtbar mit „Hilfsobjekte“). Mit Alt + linker Maustaste verschieben, Strg+Z entfernt ihn wieder."), name))
}
