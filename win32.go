//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	gdi32    = syscall.NewLazyDLL("gdi32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
	comctl32 = syscall.NewLazyDLL("comctl32.dll")
	shell32  = syscall.NewLazyDLL("shell32.dll")
	ole32    = syscall.NewLazyDLL("ole32.dll")

	pRegisterClassExW     = user32.NewProc("RegisterClassExW")
	pCreateWindowExW      = user32.NewProc("CreateWindowExW")
	pDefWindowProcW       = user32.NewProc("DefWindowProcW")
	pShowWindow           = user32.NewProc("ShowWindow")
	pUpdateWindow         = user32.NewProc("UpdateWindow")
	pPeekMessageW         = user32.NewProc("PeekMessageW")
	pGetMessageW          = user32.NewProc("GetMessageW")
	pTranslateMessage     = user32.NewProc("TranslateMessage")
	pDispatchMessageW     = user32.NewProc("DispatchMessageW")
	pPostQuitMessage      = user32.NewProc("PostQuitMessage")
	pGetClientRect        = user32.NewProc("GetClientRect")
	pMoveWindow           = user32.NewProc("MoveWindow")
	pSendMessageW         = user32.NewProc("SendMessageW")
	pSetWindowTextW       = user32.NewProc("SetWindowTextW")
	pGetWindowTextW       = user32.NewProc("GetWindowTextW")
	pGetWindowTextLengthW = user32.NewProc("GetWindowTextLengthW")
	pLoadCursorW          = user32.NewProc("LoadCursorW")
	pLoadIconW            = user32.NewProc("LoadIconW")
	pSetCapture           = user32.NewProc("SetCapture")
	pReleaseCapture       = user32.NewProc("ReleaseCapture")
	pGetCursorPos         = user32.NewProc("GetCursorPos")
	pScreenToClient       = user32.NewProc("ScreenToClient")
	pSetCursor            = user32.NewProc("SetCursor")
	pRedrawWindow         = user32.NewProc("RedrawWindow")
	pFindWindowW          = user32.NewProc("FindWindowW")
	pSetForegroundWindow  = user32.NewProc("SetForegroundWindow")
	pCreateMutexW         = kernel32.NewProc("CreateMutexW")
	pCloseHandle          = kernel32.NewProc("CloseHandle")
	pSetCursorPos         = user32.NewProc("SetCursorPos")
	pClientToScreen       = user32.NewProc("ClientToScreen")
	pShowCursor           = user32.NewProc("ShowCursor")
	pGetAsyncKeyState     = user32.NewProc("GetAsyncKeyState")
	pGetFocus             = user32.NewProc("GetFocus")
	pSetFocus             = user32.NewProc("SetFocus")
	pMessageBoxW          = user32.NewProc("MessageBoxW")
	pDestroyWindow        = user32.NewProc("DestroyWindow")
	pGetDC                = user32.NewProc("GetDC")
	pSetProcessDPIAware   = user32.NewProc("SetProcessDPIAware")
	pGetDpiForWindow      = user32.NewProc("GetDpiForWindow")
	pGetForegroundWindow  = user32.NewProc("GetForegroundWindow")
	pIsIconic             = user32.NewProc("IsIconic")
	pEnableWindow         = user32.NewProc("EnableWindow")
	pInvalidateRect       = user32.NewProc("InvalidateRect")
	pGetParent            = user32.NewProc("GetParent")

	pChoosePixelFormat = gdi32.NewProc("ChoosePixelFormat")
	pSetPixelFormat    = gdi32.NewProc("SetPixelFormat")
	pSwapBuffers       = gdi32.NewProc("SwapBuffers")
	pGetStockObject    = gdi32.NewProc("GetStockObject")
	pCreateFontW       = gdi32.NewProc("CreateFontW")

	pGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
	pCreateActCtxW    = kernel32.NewProc("CreateActCtxW")
	pActivateActCtx   = kernel32.NewProc("ActivateActCtx")

	pInitCommonControlsEx = comctl32.NewProc("InitCommonControlsEx")

	pSHBrowseForFolderW   = shell32.NewProc("SHBrowseForFolderW")
	pSHGetPathFromIDListW = shell32.NewProc("SHGetPathFromIDListW")
	pCoInitialize         = ole32.NewProc("CoInitialize")
	pCoTaskMemFree        = ole32.NewProc("CoTaskMemFree")
)

const (
	WS_OVERLAPPEDWINDOW = 0x00CF0000
	WS_CHILD            = 0x40000000
	WS_VISIBLE          = 0x10000000
	WS_CLIPCHILDREN     = 0x02000000
	WS_CLIPSIBLINGS     = 0x04000000
	WS_VSCROLL          = 0x00200000
	WS_HSCROLL          = 0x00100000
	WS_BORDER           = 0x00800000
	WS_TABSTOP          = 0x00010000
	WS_EX_CLIENTEDGE    = 0x00000200

	CS_OWNDC   = 0x0020
	CS_HREDRAW = 0x0002
	CS_VREDRAW = 0x0001
	CS_DBLCLKS = 0x0008

	CW_USEDEFAULT = 0x80000000

	SW_SHOW          = 5
	SW_SHOWMAXIMIZED = 3

	WM_DESTROY        = 0x0002
	WM_SIZE           = 0x0005
	WM_SETFOCUS       = 0x0007
	WM_KILLFOCUS      = 0x0008
	WM_CLOSE          = 0x0010
	WM_QUIT           = 0x0012
	WM_ERASEBKGND     = 0x0014
	WM_SETFONT        = 0x0030
	WM_GETMINMAXINFO  = 0x0024
	WM_KEYDOWN        = 0x0100
	WM_KEYUP          = 0x0101
	WM_CHAR           = 0x0102
	WM_COMMAND        = 0x0111
	WM_TIMER          = 0x0113
	WM_MOUSEMOVE      = 0x0200
	WM_LBUTTONDOWN    = 0x0201
	WM_LBUTTONUP      = 0x0202
	WM_RBUTTONDOWN    = 0x0204
	WM_RBUTTONUP      = 0x0205
	WM_MBUTTONDOWN    = 0x0207
	WM_MBUTTONUP      = 0x0208
	WM_MOUSEWHEEL     = 0x020A
	WM_CAPTURECHANGED = 0x0215
	WM_USER           = 0x0400

	PM_REMOVE = 0x0001

	BS_PUSHBUTTON   = 0x0000
	BS_AUTOCHECKBOX = 0x0003
	BM_GETCHECK     = 0x00F0
	BM_SETCHECK     = 0x00F1
	BST_CHECKED     = 1

	ES_LEFT        = 0x0000
	ES_MULTILINE   = 0x0004
	ES_AUTOVSCROLL = 0x0040
	ES_AUTOHSCROLL = 0x0080
	ES_READONLY    = 0x0800
	EN_CHANGE      = 0x0300

	LBS_NOTIFY           = 0x0001
	LBS_NOINTEGRALHEIGHT = 0x0100
	LB_ADDSTRING         = 0x0180
	LB_RESETCONTENT      = 0x0184
	LB_GETCURSEL         = 0x0188
	LB_SETCURSEL         = 0x0186
	LB_GETCOUNT          = 0x018B
	LBN_SELCHANGE        = 1
	LBN_DBLCLK           = 2

	SS_LEFT           = 0x0000
	SS_LEFTNOWORDWRAP = 0x000C

	IDC_ARROW       = 32512
	IDC_SIZEWE      = 32644
	WM_SETCURSOR    = 0x0020
	IDI_APPLICATION = 32512

	MB_OK              = 0x0000
	MB_ICONERROR       = 0x0010
	MB_ICONINFORMATION = 0x0040

	DEFAULT_GUI_FONT = 17

	VK_SHIFT    = 0x10
	VK_CONTROL  = 0x11
	VK_MENU     = 0x12
	VK_ESCAPE   = 0x1B
	VK_SPACE    = 0x20
	VK_PRIOR    = 0x21
	VK_NEXT     = 0x22
	VK_LEFT     = 0x25
	VK_UP       = 0x26
	VK_RIGHT    = 0x27
	VK_DOWN     = 0x28
	VK_DELETE   = 0x2E
	VK_ADD      = 0x6B
	VK_SUBTRACT = 0x6D
	VK_F1       = 0x70

	ICC_STANDARD_CLASSES = 0x00004000
	ICC_WIN95_CLASSES    = 0x000000FF

	PFD_DRAW_TO_WINDOW = 0x00000004
	PFD_SUPPORT_OPENGL = 0x00000020
	PFD_DOUBLEBUFFER   = 0x00000001
	PFD_TYPE_RGBA      = 0
	PFD_MAIN_PLANE     = 0

	BIF_RETURNONLYFSDIRS = 0x00000001
	BIF_NEWDIALOGSTYLE   = 0x00000040
	BIF_EDITBOX          = 0x00000010
)

type POINT struct{ X, Y int32 }
type RECT struct{ Left, Top, Right, Bottom int32 }

type MSG struct {
	Hwnd     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       POINT
	LPrivate uint32
}

type WNDCLASSEXW struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type PIXELFORMATDESCRIPTOR struct {
	NSize           uint16
	NVersion        uint16
	DwFlags         uint32
	IPixelType      byte
	CColorBits      byte
	CRedBits        byte
	CRedShift       byte
	CGreenBits      byte
	CGreenShift     byte
	CBlueBits       byte
	CBlueShift      byte
	CAlphaBits      byte
	CAlphaShift     byte
	CAccumBits      byte
	CAccumRedBits   byte
	CAccumGreenBits byte
	CAccumBlueBits  byte
	CAccumAlphaBits byte
	CDepthBits      byte
	CStencilBits    byte
	CAuxBuffers     byte
	ILayerType      byte
	BReserved       byte
	DwLayerMask     uint32
	DwVisibleMask   uint32
	DwDamageMask    uint32
}

type ACTCTXW struct {
	CbSize                 uint32
	DwFlags                uint32
	LpSource               *uint16
	WProcessorArchitecture uint16
	WLangId                uint16
	LpAssemblyDirectory    *uint16
	LpResourceName         *uint16
	LpApplicationName      *uint16
	HModule                uintptr
}

type INITCOMMONCONTROLSEX struct {
	DwSize uint32
	DwICC  uint32
}

type BROWSEINFOW struct {
	HwndOwner      uintptr
	PidlRoot       uintptr
	PszDisplayName *uint16
	LpszTitle      *uint16
	UlFlags        uint32
	Lpfn           uintptr
	LParam         uintptr
	IImage         int32
}

type MINMAXINFO struct {
	PtReserved     POINT
	PtMaxSize      POINT
	PtMaxPosition  POINT
	PtMinTrackSize POINT
	PtMaxTrackSize POINT
}

func u16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func loword(v uintptr) int { return int(int16(v & 0xffff)) }
func hiword(v uintptr) int { return int(int16((v >> 16) & 0xffff)) }

func sendMessage(h uintptr, msg uint32, w, l uintptr) uintptr {
	r, _, _ := pSendMessageW.Call(h, uintptr(msg), w, l)
	return r
}

func setText(h uintptr, s string) {
	pSetWindowTextW.Call(h, uintptr(unsafe.Pointer(u16(s))))
}

func getText(h uintptr) string {
	n, _, _ := pGetWindowTextLengthW.Call(h)
	buf := make([]uint16, n+1)
	pGetWindowTextW.Call(h, uintptr(unsafe.Pointer(&buf[0])), uintptr(n+1))
	return syscall.UTF16ToString(buf)
}

func messageBox(owner uintptr, text, title string, flags uintptr) {
	pMessageBoxW.Call(owner, uintptr(unsafe.Pointer(u16(text))), uintptr(unsafe.Pointer(u16(title))), flags)
}

func createWindow(exStyle uint32, class, title string, style uint32, x, y, w, h int, parent, id uintptr) uintptr {
	hinst, _, _ := pGetModuleHandleW.Call(0)
	r, _, _ := pCreateWindowExW.Call(uintptr(exStyle), uintptr(unsafe.Pointer(u16(class))), uintptr(unsafe.Pointer(u16(title))),
		uintptr(style), uintptr(x), uintptr(y), uintptr(w), uintptr(h), parent, id, hinst, 0)
	return r
}

func moveWindow(h uintptr, x, y, w, hh int) {
	if w < 0 {
		w = 0
	}
	if hh < 0 {
		hh = 0
	}
	pMoveWindow.Call(h, uintptr(x), uintptr(y), uintptr(w), uintptr(hh), 1)
}

func isChecked(h uintptr) bool { return sendMessage(h, BM_GETCHECK, 0, 0) == BST_CHECKED }
func setChecked(h uintptr, v bool) {
	var c uintptr
	if v {
		c = BST_CHECKED
	}
	sendMessage(h, BM_SETCHECK, c, 0)
}

func keyDown(vk int) bool {
	r, _, _ := pGetAsyncKeyState.Call(uintptr(vk))
	return r&0x8000 != 0
}

const manifestXML = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
<dependency><dependentAssembly>
<assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls" version="6.0.0.0" processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/>
</dependentAssembly></dependency>
</assembly>`

// browseFolder zeigt einen Ordnerauswahldialog.
func browseFolder(owner uintptr, title string) string {
	pCoInitialize.Call(0)
	buf := make([]uint16, 520)
	bi := BROWSEINFOW{HwndOwner: owner, PszDisplayName: &buf[0], LpszTitle: u16(title),
		UlFlags: BIF_RETURNONLYFSDIRS | BIF_NEWDIALOGSTYLE | BIF_EDITBOX}
	pidl, _, _ := pSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return ""
	}
	defer pCoTaskMemFree.Call(pidl)
	path := make([]uint16, 1040)
	r, _, _ := pSHGetPathFromIDListW.Call(pidl, uintptr(unsafe.Pointer(&path[0])))
	if r == 0 {
		return ""
	}
	return syscall.UTF16ToString(path)
}

const (
	MB_YESNO       = 0x0004
	MB_YESNOCANCEL = 0x0003
	MB_ICONWARNING = 0x0030
	IDYES          = 6
	IDNO           = 7
	IDCANCEL       = 2
)

func messageBoxRet(owner uintptr, text, title string, flags uintptr) int {
	r, _, _ := pMessageBoxW.Call(owner, uintptr(unsafe.Pointer(u16(text))), uintptr(unsafe.Pointer(u16(title))), flags)
	return int(r)
}
