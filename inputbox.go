//go:build windows

package main

// Einfaches Eingabefenster (Text abfragen) ohne Ressourcen-Dialog.

import (
	"syscall"
	"unsafe"
)

const (
	WS_POPUP       = 0x80000000
	WS_CAPTION     = 0x00C00000
	WS_SYSMENU     = 0x00080000
	WS_EX_DLGMODAL = 0x00000001
	BS_DEFPUSH     = 0x0001
	VK_RETURN_     = 0x0D
	idInOK         = 1
	idInCancel     = 2
	idInEdit       = 3
	idInList       = 4
	LBN_DBLCLK_    = 2
)

var inputState struct {
	hwnd, edit uintptr
	list       uintptr
	sel        int
	done       bool
	ok         bool
	text       string
}

var inputClassDone bool

func inputWndProc(hwnd uintptr, msg uint32, w, l uintptr) uintptr {
	switch msg {
	case WM_COMMAND:
		switch loword(w) {
		case idInList:
			if hiword(w) == LBN_DBLCLK_ {
				inputState.sel = int(sendMessage(inputState.list, LB_GETCURSEL, 0, 0))
				inputState.ok = inputState.sel >= 0
				inputState.done = true
			}
			return 0
		case idInOK:
			if inputState.list != 0 {
				inputState.sel = int(sendMessage(inputState.list, LB_GETCURSEL, 0, 0))
				inputState.ok = inputState.sel >= 0
				inputState.done = true
				return 0
			}
			inputState.text = getText(inputState.edit)
			inputState.ok = true
			inputState.done = true
			return 0
		case idInCancel:
			inputState.done = true
			return 0
		}
	case WM_CLOSE:
		inputState.done = true
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), w, l)
	return r
}

// inputBox fragt einen Text ab. ok=false bei Abbruch.
func (a *app) inputBox(title, prompt, def string) (string, bool) {
	if !inputClassDone {
		if err := registerClass("AdanosEditInput", syscall.NewCallback(inputWndProc), 0, 16); err != nil {
			return "", false
		}
		inputClassDone = true
	}
	W, H := a.scale(460), a.scale(170)
	var rc RECT
	pGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
	pt := POINT{int32((int(rc.Right) - W) / 2), int32((int(rc.Bottom) - H) / 2)}
	pClientToScreen.Call(a.hwnd, uintptr(unsafe.Pointer(&pt)))
	h := createWindow(WS_EX_DLGMODAL, "AdanosEditInput", title, WS_POPUP|WS_CAPTION|WS_SYSMENU|WS_VISIBLE,
		int(pt.X), int(pt.Y), W, H, a.hwnd, 0)
	if h == 0 {
		return "", false
	}
	child := uint32(WS_CHILD | WS_VISIBLE)
	mk := func(ex uint32, class, text string, style uint32, id int, x, y, w, hh int) uintptr {
		c := createWindow(ex, class, text, child|style, a.scale(x), a.scale(y), a.scale(w), a.scale(hh), h, uintptr(id))
		sendMessage(c, WM_SETFONT, a.font, 1)
		return c
	}
	mk(0, "STATIC", prompt, SS_LEFT, 0, 12, 10, 430, 40)
	ed := mk(WS_EX_CLIENTEDGE, "EDIT", def, ES_AUTOHSCROLL|WS_TABSTOP, idInEdit, 12, 52, 430, 26)
	mk(0, "BUTTON", "OK", BS_DEFPUSH|WS_TABSTOP, idInOK, 236, 92, 100, 30)
	mk(0, "BUTTON", T("Abbrechen"), BS_PUSHBUTTON|WS_TABSTOP, idInCancel, 342, 92, 100, 30)
	inputState.hwnd, inputState.edit = h, ed
	inputState.done, inputState.ok, inputState.text = false, false, ""
	pEnableWindow.Call(a.hwnd, 0)
	pSetFocus.Call(ed)
	sendMessage(ed, 0x00B1, 0, ^uintptr(0)) // EM_SETSEL: alles markieren
	var msg MSG
	for !inputState.done {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		if msg.Message == WM_KEYDOWN && msg.WParam == VK_RETURN_ {
			inputState.text = getText(ed)
			inputState.ok = true
			inputState.done = true
			break
		}
		if msg.Message == WM_KEYDOWN && msg.WParam == VK_ESCAPE {
			inputState.done = true
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	pEnableWindow.Call(a.hwnd, 1)
	pDestroyWindow.Call(h)
	pSetFocus.Call(a.glHwnd)
	return inputState.text, inputState.ok
}

// listBox laesst einen Eintrag aus einer Liste waehlen. ok=false bei Abbruch.
func (a *app) listBox(title, prompt string, items []string, sel int) (int, bool) {
	if !inputClassDone {
		if err := registerClass("AdanosEditInput", syscall.NewCallback(inputWndProc), 0, 16); err != nil {
			return 0, false
		}
		inputClassDone = true
	}
	W, H := a.scale(640), a.scale(300)
	var rc RECT
	pGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
	pt := POINT{int32((int(rc.Right) - W) / 2), int32((int(rc.Bottom) - H) / 2)}
	pClientToScreen.Call(a.hwnd, uintptr(unsafe.Pointer(&pt)))
	h := createWindow(WS_EX_DLGMODAL, "AdanosEditInput", title, WS_POPUP|WS_CAPTION|WS_SYSMENU|WS_VISIBLE,
		int(pt.X), int(pt.Y), W, H, a.hwnd, 0)
	if h == 0 {
		return 0, false
	}
	child := uint32(WS_CHILD | WS_VISIBLE)
	mk := func(ex uint32, class, text string, style uint32, id int, x, y, w, hh int) uintptr {
		c := createWindow(ex, class, text, child|style, a.scale(x), a.scale(y), a.scale(w), a.scale(hh), h, uintptr(id))
		sendMessage(c, WM_SETFONT, a.font, 1)
		return c
	}
	mk(0, "STATIC", prompt, SS_LEFT, 0, 12, 10, 610, 54)
	lb := mk(WS_EX_CLIENTEDGE, "LISTBOX", "", WS_VSCROLL|WS_TABSTOP|LBS_NOTIFY|LBS_NOINTEGRALHEIGHT, idInList, 12, 68, 610, 150)
	for _, it := range items {
		sendMessage(lb, LB_ADDSTRING, 0, uintptr(unsafe.Pointer(u16(it))))
	}
	sendMessage(lb, LB_SETCURSEL, uintptr(sel), 0)
	mk(0, "BUTTON", "OK", BS_DEFPUSH|WS_TABSTOP, idInOK, 416, 228, 100, 30)
	mk(0, "BUTTON", T("Abbrechen"), BS_PUSHBUTTON|WS_TABSTOP, idInCancel, 522, 228, 100, 30)
	inputState.hwnd, inputState.edit, inputState.list = h, 0, lb
	inputState.done, inputState.ok, inputState.sel = false, false, -1
	pEnableWindow.Call(a.hwnd, 0)
	pSetFocus.Call(lb)
	var msg MSG
	for !inputState.done {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		if msg.Message == WM_KEYDOWN && msg.WParam == VK_RETURN_ {
			inputState.sel = int(sendMessage(lb, LB_GETCURSEL, 0, 0))
			inputState.ok = inputState.sel >= 0
			inputState.done = true
			break
		}
		if msg.Message == WM_KEYDOWN && msg.WParam == VK_ESCAPE {
			inputState.done = true
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	inputState.list = 0
	pEnableWindow.Call(a.hwnd, 1)
	pDestroyWindow.Call(h)
	pSetFocus.Call(a.glHwnd)
	return inputState.sel, inputState.ok
}
