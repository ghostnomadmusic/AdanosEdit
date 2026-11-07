//go:build windows

package main

import (
	"g3viewer/lang"
)

const (
	CBS_DROPDOWNLIST = 0x0003
	CB_ADDSTRING     = 0x0143
	CB_GETCURSEL     = 0x0147
	CB_SETCURSEL     = 0x014E
	CB_RESETCONTENT  = 0x014B
	CBN_SELCHANGE    = 1
)

// T uebersetzt einen (deutschen) Text in die eingestellte Sprache.
func T(s string) string { return lang.T(s) }

// setLanguage stellt die Sprache um und beschriftet die Oberflaeche neu.
func (a *app) setLanguage(i int) {
	lang.Set(i)
	cfg := loadConfig()
	cfg["Language"] = lang.Code()
	saveConfig(cfg)
	a.applyTexts()
	a.updateFlag()
	pSetFocus.Call(a.glHwnd)
}

// applyTexts setzt alle festen Beschriftungen in der aktuellen Sprache.
func (a *app) applyTexts() {
	setText(a.hModeWorld, T("In der Welt suchen"))
	setText(a.hModeTpl, T("Neues Objekt aus Vorlage anlegen"))
	if a.tplMode {
		setText(a.hSearchLbl, T("Vorlage suchen (z. B. Trank, Truhe, Wolf):"))
	} else {
		setText(a.hSearchLbl, T("Objekt suchen (Name oder Modell):"))
	}
	setText(a.hPlace, T("Neu platzieren (in der Bildmitte)"))
	for id, h := range a.hChk {
		setText(h, T(chkLabels[id]))
	}
	setText(a.hInfoLbl, T("Ausgewähltes Objekt:"))
	setText(a.hHelp, T("Hilfe (F1)"))
	setText(a.hFolder, T("Gothic-Ordner …"))
	setText(a.hEditLbl, T("Bearbeiten (Position in cm, Drehung in Grad):"))
	setText(a.hLblRot, T("Drehung"))
	setText(a.hApply, T("Übernehmen"))
	setText(a.hCopy, T("Kopieren"))
	setText(a.hDelete, T("Löschen"))
	setText(a.hUndo, T("Rückgängig"))
	setText(a.hClearNPC, T("NPCs entfernen …"))
	setText(a.hChest, T("Truhe / Schloss …"))
	setText(a.hSave, T("Änderungen als Mod speichern"))
	setText(a.hCheck, T("Mod prüfen …"))
	setText(a.hLangLbl, T("Sprache:"))
	setText(a.hModLbl, T("Mod:"))
	a.applyNPCTexts()
	a.applyDlgTexts()
	a.applyQstTexts()
	a.applyPlaceTexts()
	if a.vfs != nil {
		a.fillModList()
	}
	a.setViewDist(a.viewDist)
	a.updateTitle()
	a.lastStatusText = ""
	a.updateStatus()
	a.onSearch()
	if a.selected >= 0 {
		a.select_(a.selected, false)
	} else if a.tplMode {
		if te := a.selectedTemplate(); te != nil {
			a.showTemplateInfo(te)
		}
	}
	a.layout()
}
