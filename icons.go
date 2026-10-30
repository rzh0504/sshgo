package main

import (
	"github.com/egoist/mygo/ui"
)

// icon draws an icon of sshgo's own, on 16, as a status glyph.
func icon(shapes string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.35" stroke-linecap="round" stroke-linejoin="round">` + shapes + `</svg>`))
}

func glyph(path string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 56 56"><path fill="currentColor" d="` + path + `"/></svg>`))
}

var (
	sidebarIcon = glyph("M7.715 49.574h40.57c4.899 0 7.36-2.437 7.36-7.265V13.69c0-4.828-2.461-7.265-7.36-7.265H7.715C2.84 6.426.355 8.84.355 13.69v28.62c0 4.851 2.485 7.265 7.36 7.265m.07-3.773c-2.344 0-3.656-1.242-3.656-3.68V13.88c0-2.438 1.312-3.68 3.656-3.68h10.43v35.602ZM48.215 10.2c2.32 0 3.656 1.242 3.656 3.68v28.24c0 2.438-1.336 3.68-3.656 3.68h-26.32V10.199Zm-34.5 8.696c.703 0 1.336-.633 1.336-1.313c0-.703-.633-1.312-1.336-1.312h-5.04c-.702 0-1.312.609-1.312 1.312c0 .68.61 1.313 1.313 1.313Zm0 6.07c.703 0 1.336-.633 1.336-1.336s-.633-1.29-1.336-1.29h-5.04c-.702 0-1.312.587-1.312 1.29s.61 1.336 1.313 1.336Zm0 6.047c.703 0 1.336-.586 1.336-1.29c0-.702-.633-1.312-1.336-1.312h-5.04c-.702 0-1.312.61-1.312 1.313s.61 1.289 1.313 1.289Z")
	plusIcon    = glyph("M27.988 47.734c1.149 0 2.11-.914 2.11-2.039V30.11h15.14c1.125 0 2.11-.96 2.11-2.109c0-1.148-.985-2.086-2.11-2.086h-15.14v-15.61c0-1.124-.961-2.038-2.11-2.038c-1.148 0-2.086.914-2.086 2.039v15.61h-15.14c-1.125 0-2.11.937-2.11 2.085s.985 2.11 2.11 2.11h15.14v15.585c0 1.125.938 2.04 2.086 2.04")
)

var (
	terminalIcon = icon(`<rect x="1.8" y="2.8" width="12.4" height="10.4" rx="2"/><path d="M5 6.6l2.2 2.2L5 11M9 11.4h2.6"/>`)
	serverIcon   = icon(`<rect x="1.8" y="2.6" width="12.4" height="8.6" rx="1.6"/><path d="M5.4 7h.01M7.6 7h.01"/><path d="M5.5 14.2h5M8 11.2v3"/>`)
	closeIcon    = icon(`<path d="M4.2 4.2l7.6 7.6M11.8 4.2l-7.6 7.6"/>`)
	minimizeIcon = icon(`<path d="M3.5 8h9"/>`)
	maximizeIcon = icon(`<rect x="3.5" y="3.5" width="9" height="9" rx="1.5"/>`)
	restoreIcon  = icon(`<rect x="3.5" y="5.5" width="7" height="7" rx="1.2"/><path d="M5.5 3.5h5.5a1.5 1.5 0 0 1 1.5 1.5v5.5"/>`)
	lockIcon     = icon(`<rect x="3.2" y="7" width="9.6" height="6.6" rx="1.6"/><path d="M5.6 7V5a2.4 2.4 0 0 1 4.8 0v2"/><path d="M8 9.8v1.6"/>`)
	shieldIcon   = icon(`<path d="M8 1.9l5 2v4.2c0 2.9-2.1 5.2-5 6.2-2.9-1-5-3.3-5-6.2V3.9z"/><path d="M5.8 8.1l1.6 1.6 3-3.2"/>`)
	copyIcon     = icon(`<rect x="5.4" y="5.4" width="8" height="8" rx="1.6"/><path d="M10.6 5.4V4a1.4 1.4 0 0 0-1.4-1.4H4A1.4 1.4 0 0 0 2.6 4v5.2A1.4 1.4 0 0 0 4 10.6h1.4"/>`)
	trashIcon    = icon(`<path d="M2.8 4.5h10.4M6.4 4.5V3.2a.9.9 0 0 1 .9-.9h1.4a.9.9 0 0 1 .9.9v1.3M4.2 4.5l.6 8.1a1.3 1.3 0 0 0 1.3 1.2h3.8a1.3 1.3 0 0 0 1.3-1.2l.6-8.1"/>`)
	pencilIcon   = icon(`<path d="M3 13l.4-2.7L10.9 2.8a1.3 1.3 0 0 1 1.9 0l.4.4a1.3 1.3 0 0 1 0 1.9L5.7 12.6z"/>`)
	retryIcon    = icon(`<path d="M13 8a5 5 0 1 1-1.5-3.6"/><path d="M13 2.8v3.4h-3.4"/>`)
)
