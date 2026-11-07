//go:build windows

package main

// Neue Objekte aus den Vorlagen des Spiels (Templates.pak) anlegen.

import (
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"time"
	"unsafe"

	"g3viewer/g3"
	"g3viewer/scene"
)

const (
	BS_AUTORADIOBUTTON = 0x0009
	WS_GROUP           = 0x00020000
)

// Ziel fuer neue Objekte: die System-Dynamik-Datei der Hauptwelt. Sie wird
// immer vollstaendig geladen (anders als Story- oder Quest-Sektoren).
const newObjectFile = "G3_World_01/SysDyn_{9A103CC2-4190-4DB3-9618-0419E5445AAD}/SysDyn_{9A103CC2-4190-4DB3-9618-0419E5445AAD}.lrentdat"

type tplEntry struct {
	t     *g3.Template
	group string // Gegenstand, Objekt, Figur, Effekt
	lower string
}

// Suchbegriffe der Gruppen in allen Sprachen
var groupWords = map[string]string{
	"Gegenstand": "item przedmiot предмет",
	"Objekt":     "object obiekt объект",
	"Figur":      "npc character creature postać существо персонаж",
	"Effekt":     "effect efekt эффект",
}

// tplGroup ordnet eine Vorlage einer Gruppe zu ("" = nicht anbieten).
func tplGroup(t *g3.Template) string {
	p := strings.ToLower(t.Path)
	switch {
	case strings.HasPrefix(p, "items/"):
		if t.Mesh == "" {
			return "" // Faehigkeiten, Rezepte usw. ohne Modell
		}
		return "Gegenstand"
	case strings.HasPrefix(p, "objects/"):
		return "Objekt"
	case strings.HasPrefix(p, "npc/"):
		if t.Kind != g3.KindNPC || strings.Contains(p, "ztest") {
			return ""
		}
		return "Figur"
	case strings.HasPrefix(p, "vfx/"), strings.HasPrefix(p, "sfx/"), strings.HasPrefix(p, "lights/"):
		return "Effekt"
	}
	return ""
}

// loadTemplates liest alle Vorlagen (im Hintergrund).
func (a *app) loadTemplates() {
	go func() {
		t0 := time.Now()
		var list []tplEntry
		var parts npcPartLists
		bad := 0
		for _, e := range a.vfs.List("Templates") {
			if !strings.HasSuffix(strings.ToLower(e.Name), ".tple") {
				continue
			}
			d, err := e.Read()
			if err != nil {
				continue
			}
			t, err := g3.ParseTemplate(d, e.Name)
			if err != nil {
				bad++
				continue
			}
			collectNPCPart(&parts, t)
			g := tplGroup(t)
			if g == "" {
				continue
			}
			if t.Problem != "" {
				bad++
				continue
			}
			list = append(list, tplEntry{t: t, group: g, lower: strings.ToLower(t.Name + " " + t.Path + " " + t.Mesh + " " + g + " " + groupWords[g])})
		}
		sort.Slice(list, func(i, j int) bool { return strings.ToLower(list[i].t.Name) < strings.ToLower(list[j].t.Name) })
		log.Printf("Vorlagen: %d platzierbar, %d nicht unterstuetzt (%v)", len(list), bad, time.Since(t0))
		sortParts(&parts)
		npcParts = parts
		a.tplCh <- list
	}()
}

func (a *app) setSearchMode(tpl bool) {
	a.tplMode = tpl
	setChecked(a.hModeWorld, !tpl)
	setChecked(a.hModeTpl, tpl)
	if tpl {
		setText(a.hSearchLbl, T("Vorlage suchen (z. B. Trank, Truhe, Wolf):"))
	} else {
		setText(a.hSearchLbl, T("Objekt suchen (Name oder Modell):"))
	}
	en := uintptr(0)
	if tpl {
		en = 1
	}
	pEnableWindow.Call(a.hPlace, en)
	a.onSearch()
}

// Deutsche Suchbegriffe auf die (englischen) Namen der Spieldaten abbilden.
var tplSynonyms = map[string]string{
	"trank": "potion", "truhe": "chest", "kiste": "box", "schwert": "sword", "axt": "axe", "bogen": "bow",
	"pfeil": "arrow", "fackel": "torch", "stuhl": "chair", "tisch": "table", "bett": "bed", "fass": "barrel",
	"schrank": "cupboard", "regal": "shelf", "lampe": "lamp", "kerze": "candle", "pflanze": "plant",
	"ring": "ring", "amulett": "amulet", "gold": "gold", "rüstung": "armor", "helm": "helmet", "schild": "shield",
	"wolf": "wolf", "ork": "orc", "baum": "tree", "stein": "stone", "fels": "rock", "zaun": "fence",
	"feuer": "fire", "buch": "book", "brief": "letter", "schlüssel": "key", "essen": "food", "fleisch": "meat",
	"brot": "bread", "bier": "beer", "wein": "wine", "spruchrolle": "scroll", "rune": "rune", "kraut": "herb",
	"pilz": "mushroom", "erz": "ore", "werkzeug": "tool", "hammer": "hammer", "stab": "staff", "keule": "club",
	"lanze": "lance", "speer": "spear", "armbrust": "crossbow", "bolzen": "bolt", "leiche": "corpse",
	"knochen": "bone", "schädel": "skull", "zelt": "tent", "wagen": "cart", "boot": "boat", "tür": "door",
	"leiter": "ladder", "heil": "health", "heiltrank": "health", "manatrank": "mana", "pfeile": "arrow", "münze": "gold", "goldmünze": "gold", "kessel": "cauldron", "amboss": "anvil", "bank": "bench", "sack": "sack", "korb": "basket", "eimer": "bucket", "fahne": "banner", "tier": "animal", "schaf": "sheep", "huhn": "chicken", "schwein": "pig", "bär": "bear", "keiler": "boar", "skelett": "skeleton", "zombie": "zombie", "goblin": "goblin", "troll": "troll",
	// Russisch
	"зелье": "potion", "сундук": "chest", "ящик": "box", "меч": "sword", "топор": "axe", "лук": "bow", "стрела": "arrow", "факел": "torch", "стул": "chair", "стол": "table", "кровать": "bed", "бочка": "barrel", "шкаф": "cupboard", "полка": "shelf", "лампа": "lamp", "свеча": "candle", "растение": "plant", "кольцо": "ring", "амулет": "amulet", "золото": "gold", "доспех": "armor", "шлем": "helmet", "щит": "shield", "волк": "wolf", "орк": "orc", "дерево": "tree", "камень": "stone", "скала": "rock", "забор": "fence", "огонь": "fire", "книга": "book", "письмо": "letter", "ключ": "key", "еда": "food", "мясо": "meat", "хлеб": "bread", "пиво": "beer", "вино": "wine", "свиток": "scroll", "руна": "rune", "трава": "herb", "гриб": "mushroom", "руда": "ore", "молот": "hammer", "посох": "staff", "дубина": "club", "копьё": "spear", "копье": "spear", "арбалет": "crossbow", "болт": "bolt", "кость": "bone", "череп": "skull", "палатка": "tent", "телега": "cart", "лодка": "boat", "дверь": "door", "лестница": "ladder", "лечение": "health", "мана": "mana", "медведь": "bear", "кабан": "boar", "скелет": "skeleton", "зомби": "zombie", "гоблин": "goblin", "тролль": "troll", "овца": "sheep", "курица": "chicken", "свинья": "pig", "колодец": "well", "статуя": "statue", "алтарь": "altar", "трон": "throne", "котёл": "cauldron", "котел": "cauldron", "наковальня": "anvil", "скамья": "bench", "мешок": "sack", "корзина": "basket", "ведро": "bucket", "знамя": "banner",
	// Polnisch
	"mikstura": "potion", "eliksir": "potion", "skrzynia": "chest", "kufer": "chest", "pudło": "box", "miecz": "sword", "topór": "axe", "łuk": "bow", "strzała": "arrow", "pochodnia": "torch", "krzesło": "chair", "stół": "table", "łóżko": "bed", "beczka": "barrel", "szafa": "cupboard", "półka": "shelf", "lampa": "lamp", "świeca": "candle", "roślina": "plant", "pierścień": "ring", "amulet": "amulet", "złoto": "gold", "zbroja": "armor", "hełm": "helmet", "tarcza": "shield", "wilk": "wolf", "drzewo": "tree", "kamień": "stone", "skała": "rock", "płot": "fence", "ogień": "fire", "książka": "book", "list": "letter", "klucz": "key", "jedzenie": "food", "mięso": "meat", "chleb": "bread", "piwo": "beer", "wino": "wine", "zwój": "scroll", "runa": "rune", "zioło": "herb", "grzyb": "mushroom", "ruda": "ore", "młot": "hammer", "kostur": "staff", "maczuga": "club", "włócznia": "spear", "kusza": "crossbow", "bełt": "bolt", "kość": "bone", "czaszka": "skull", "namiot": "tent", "wóz": "cart", "łódź": "boat", "drzwi": "door", "drabina": "ladder", "leczenie": "health", "niedźwiedź": "bear", "dzik": "boar", "szkielet": "skeleton", "owca": "sheep", "kura": "chicken", "świnia": "pig", "studnia": "well", "posąg": "statue", "ołtarz": "altar", "tron": "throne", "kocioł": "cauldron", "kowadło": "anvil", "ławka": "bench", "worek": "sack", "kosz": "basket", "wiadro": "bucket", "sztandar": "banner", "brunnen": "well", "statue": "statue", "altar": "altar", "thron": "throne",
}

func (a *app) searchTemplates(txt string) {
	if a.templates == nil {
		sendMessage(a.hResults, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(T("Vorlagen werden noch eingelesen …")))))
		return
	}
	words := strings.Fields(strings.ToLower(txt))
	for i, w := range words {
		if s, ok := tplSynonyms[w]; ok {
			words[i] = s
		}
	}
	for i := range a.templates {
		te := &a.templates[i]
		ok := true
		for _, w := range words {
			if !strings.Contains(te.lower, w) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		a.tplResults = append(a.tplResults, i)
		sendMessage(a.hResults, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(te.t.Name+"   ("+T(te.group)+")"))))
		if len(a.tplResults) >= 2000 {
			break
		}
	}
	if len(a.tplResults) == 0 {
		sendMessage(a.hResults, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(T("(nichts gefunden)")))))
	}
}

func (a *app) selectedTemplate() *tplEntry {
	i := int(sendMessage(a.hResults, LB_GETCURSEL, 0, 0))
	if i < 0 || i >= len(a.tplResults) {
		return nil
	}
	return &a.templates[a.tplResults[i]]
}

func (a *app) showTemplateInfo(te *tplEntry) {
	t := te.t
	var b strings.Builder
	fmt.Fprintf(&b, T("Vorlage: %s\r\n(%s)\r\n\r\n"), t.Name, T(te.group))
	if t.Mesh != "" {
		fmt.Fprintf(&b, T("Modell: %s\r\n"), t.Mesh)
	}
	sz := [3]float32{t.LocalMax[0] - t.LocalMin[0], t.LocalMax[1] - t.LocalMin[1], t.LocalMax[2] - t.LocalMin[2]}
	if g3.BoxValid(t.LocalMin, t.LocalMax) {
		fmt.Fprintf(&b, T("Größe: %.0f × %.0f × %.0f cm\r\n"), sz[0], sz[1], sz[2])
	}
	fmt.Fprintf(&b, T("Datei: %s\r\n\r\n"), t.Path)
	b.WriteString(T("„Neu platzieren“ (oder Doppelklick) setzt das Objekt in die Bildmitte.\r\n"))
	if te.group == "Figur" {
		b.WriteString(T("\r\nHinweis: Neue Figuren sind experimentell. Ohne Tagesablauf stehen sie nur herum; manche reagieren im Spiel nicht wie gewohnt.\r\n"))
	}
	b.WriteString("\r\nProperty-Sets:\r\n")
	for _, c := range t.Classes {
		fmt.Fprintf(&b, "  %s\r\n", c.Name)
	}
	setText(a.hInfo, b.String())
}

// editFilePath oeffnet eine Weltdatei per Archivpfad zum Bearbeiten.
func (a *app) editFilePath(path string) (*g3.WorldFile, error) {
	for i := range a.world.Inst {
		if a.world.Inst[i].Ent.Source == path {
			return a.editFile(&a.world.Inst[i])
		}
	}
	// Datei ohne sichtbare Instanzen
	e := a.vfs.Find("Projects_compiled", path)
	if e == nil {
		return nil, fmt.Errorf(T("Datei %s nicht gefunden"), path)
	}
	d, err := e.Read()
	if err != nil {
		return nil, err
	}
	wf, err := g3.OpenWorldFile(d, path)
	if err != nil {
		return nil, err
	}
	a.ed.files[path] = wf
	a.ed.inst[path] = map[int]int32{}
	return wf, nil
}

// placeTemplate legt ein neues Objekt aus der gewaehlten Vorlage an.
func (a *app) placeTemplate() {
	te := a.selectedTemplate()
	if te == nil || a.world == nil {
		a.flash(T("Bitte zuerst links eine Vorlage auswählen."))
		return
	}
	t := te.t
	path := newObjectFile
	if a.vfs.Find("Projects_compiled", path) == nil {
		a.flash(T("Die Zieldatei für neue Objekte wurde nicht gefunden."))
		return
	}
	if a.ed.files[path] == nil {
		setText(a.hStatus, T("Zieldatei wird geöffnet …"))
	}
	wf, err := a.editFilePath(path)
	if err != nil {
		a.flash(fmt.Sprintf(T("Neue Objekte können nicht angelegt werden: %s"), T(err.Error())))
		return
	}
	// Position: Bildmitte auf dem Boden, sonst 3 m vor der Kamera
	var p scene.Vec3
	if c, ok := a.centerPoint(); ok {
		p = c
		if g3.BoxValid(t.LocalMin, t.LocalMax) {
			p[1] -= t.LocalMin[1] // Unterkante auf den Boden
		}
	} else {
		f := a.cam.Forward()
		p = a.cam.Pos.Add(f.Scale(300))
	}
	// Ausrichtung: zur Kamera gedreht (nur um die Hochachse)
	f := a.cam.Forward()
	yaw := float32(0)
	if f[0] != 0 || f[2] != 0 {
		yaw = float32(math.Atan2(float64(-f[0]), float64(-f[2])))
	}
	s, c := float32(math.Sin(float64(yaw))), float32(math.Cos(float64(yaw)))
	world := [16]float32{c, 0, -s, 0, 0, 1, 0, 0, s, 0, c, 0, p[0], p[1], p[2], 1}
	ci, err := wf.AddFromTemplate(t, world)
	if err != nil {
		a.flash(fmt.Sprintf(T("Anlegen nicht möglich: %s"), T(err.Error())))
		return
	}
	ne := wf.Ents[ci]
	sector := path
	if j := strings.LastIndex(sector, "/"); j >= 0 {
		sector = sector[:j]
	}
	sector = strings.TrimPrefix(sector, "G3_World_01/")
	ii := a.world.Add(ne, sector)
	if ii < 0 {
		wf.Delete(ci)
		a.syncFile(wf)
		a.flash(T("Das Objekt konnte nicht angezeigt werden (keine gültige Größe)."))
		return
	}
	a.ed.inst[wf.Path][ci] = ii
	wf.Changed = map[int]bool{ci: true}
	a.syncFile(wf)
	a.ed.undo = append(a.ed.undo, func() {
		wf.Delete(ci)
		a.syncFile(wf)
		if a.selected == ii {
			a.select_(-1, false)
		}
	})
	a.select_(ii, false)
	pSetFocus.Call(a.glHwnd)
	a.flash(fmt.Sprintf(T("„%s“ angelegt. Mit Alt + linker Maustaste verschieben, Umschalt+F setzt es auf den Boden, Strg+Z entfernt es wieder."), t.Name))
}
