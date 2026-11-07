//go:build windows

package main

// Hilfefenster: langer Text mit Bildlaufleiste (statt Meldungsfenster).

import (
	"strings"
	"syscall"
	"unsafe"
)

var helpDlg struct{ done bool }

var helpClassDone bool

func helpWndProc(hwnd uintptr, msg uint32, w, l uintptr) uintptr {
	switch msg {
	case WM_COMMAND:
		if loword(w) == idInOK {
			helpDlg.done = true
			return 0
		}
	case WM_CLOSE:
		helpDlg.done = true
		return 0
	}
	r, _, _ := pDefWindowProcW.Call(hwnd, uintptr(msg), w, l)
	return r
}

// showHelp zeigt die Hilfe in einem eigenen Fenster.
func (a *app) showHelp() {
	if !helpClassDone {
		if err := registerClass("AdanosEditHelp", syscall.NewCallback(helpWndProc), 0, 16); err != nil {
			messageBox(a.hwnd, T(helpText), T(appTitleDE), MB_ICONINFORMATION)
			return
		}
		helpClassDone = true
	}
	var rc RECT
	pGetClientRect.Call(a.hwnd, uintptr(unsafe.Pointer(&rc)))
	W, H := a.scale(720), int(rc.Bottom)-a.scale(80)
	if H > a.scale(860) {
		H = a.scale(860)
	}
	if W > int(rc.Right)-a.scale(40) {
		W = int(rc.Right) - a.scale(40)
	}
	pt := POINT{int32((int(rc.Right) - W) / 2), int32((int(rc.Bottom) - H) / 2)}
	pClientToScreen.Call(a.hwnd, uintptr(unsafe.Pointer(&pt)))
	h := createWindow(WS_EX_DLGMODAL, "AdanosEditHelp", T(appTitleDE)+" "+version+" – "+T("Hilfe"),
		WS_POPUP|WS_CAPTION|WS_SYSMENU|WS_VISIBLE, int(pt.X), int(pt.Y), W, H, a.hwnd, 0)
	if h == 0 {
		return
	}
	var cr RECT
	pGetClientRect.Call(h, uintptr(unsafe.Pointer(&cr)))
	cw, ch := int(cr.Right), int(cr.Bottom)
	m := a.scale(10)
	bh := a.scale(30)
	text := strings.ReplaceAll(strings.ReplaceAll(T(helpText), "\r\n", "\n"), "\n", "\r\n")
	ed := createWindow(WS_EX_CLIENTEDGE, "EDIT", "", WS_CHILD|WS_VISIBLE|ES_MULTILINE|ES_READONLY|ES_AUTOVSCROLL|WS_VSCROLL,
		m, m, cw-2*m, ch-3*m-bh, h, 0)
	sendMessage(ed, WM_SETFONT, a.font, 1)
	sendMessage(ed, 0x00C5, 0, 0) // EM_LIMITTEXT: kein Limit
	setText(ed, text)
	ok := createWindow(0, "BUTTON", "OK", WS_CHILD|WS_VISIBLE|BS_DEFPUSH|WS_TABSTOP, cw-m-a.scale(110), ch-m-bh, a.scale(110), bh, h, idInOK)
	sendMessage(ok, WM_SETFONT, a.font, 1)
	helpDlg.done = false
	pEnableWindow.Call(a.hwnd, 0)
	pSetFocus.Call(ed)
	var msg MSG
	for !helpDlg.done {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		if msg.Message == WM_KEYDOWN && (msg.WParam == VK_ESCAPE || msg.WParam == VK_RETURN_) {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
	pEnableWindow.Call(a.hwnd, 1)
	pDestroyWindow.Call(h)
	pSetFocus.Call(a.glHwnd)
}
