package main

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>
void mqStartHover(void);
*/
import "C"

import (
	"unsafe"
)

// hoverApp is the app whose frames the menu-bar hover preview shows.
var hoverApp *App

// startHoverPreview shows a live mini view of the panel under the menu-bar icon while the
// pointer rests on it.
func startHoverPreview(a *App) {
	hoverApp = a
	C.mqStartHover()
}

//export mqPreviewFrame
func mqPreviewFrame(n *C.int) unsafe.Pointer {
	if hoverApp == nil {
		return nil
	}
	b := hoverApp.E.Preview()
	if len(b) == 0 {
		return nil
	}
	*n = C.int(len(b))
	return C.CBytes(b)
}

//export mqPreviewTitle
func mqPreviewTitle() *C.char {
	if hoverApp == nil {
		return nil
	}
	st := hoverApp.E.Status()
	s := st.Program
	if st.Overlay != "" {
		s += " · " + st.Overlay
	}
	switch st.Panel {
	case "connected":
		s += "   ·   on the panel"
	case "disabled":
		s += "   ·   panel off"
	default:
		s += "   ·   no panel, preview only"
	}
	return C.CString(s)
}
