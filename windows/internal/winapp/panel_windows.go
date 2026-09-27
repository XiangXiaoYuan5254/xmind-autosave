package winapp

import (
	"math"
	"syscall"
	"unsafe"
)

// togglePanel is the small "自动保存" switch floating next to XMind's title
// bar. It is a layered, non-activating tool window: clicking it never takes
// the keyboard focus away from XMind.
type togglePanel struct {
	hwnd     uintptr
	onToggle func()

	visible bool
	origin  point
	size    point

	renderedState panelState
	hasRendered   bool
}

type panelState struct {
	enabled bool
	dark    bool
	dpi     uint32
}

// Layout in 96-DPI units, matching the macOS panel.
const (
	panelWidth      = 132
	panelHeight     = 30
	panelMargin     = 6 // room for the shadow
	panelRadius     = 9
	panelTopOffset  = 7
	switchWidth     = 42
	switchHeight    = 22
	switchTrailing  = 8
	labelLeading    = 10
	labelPixelSize  = 12
	panelClassName  = "XMindAutoSave.TogglePanel"
	panelWindowText = "XMind 自动保存开关"
)

var activePanel *togglePanel

var panelWindowProc = syscall.NewCallback(func(hwnd, msg, wParam, lParam uintptr) uintptr {
	switch uint32(msg) {
	case wmMouseActivate:
		return maNoActivate
	case wmSetCursor:
		call(procSetCursor, call(procLoadCursorW, 0, idcHand))
		return 1
	case wmLButtonUp:
		if activePanel != nil && activePanel.onToggle != nil {
			activePanel.onToggle()
		}
		return 0
	}
	return defWindowProc(hwnd, uint32(msg), wParam, lParam)
})

func newTogglePanel(onToggle func()) (*togglePanel, error) {
	if err := startGDIPlus(); err != nil {
		return nil, err
	}
	if err := registerWindowClass(panelClassName, panelWindowProc, call(procLoadCursorW, 0, idcHand)); err != nil {
		return nil, err
	}
	hwnd, err := createWindow(wsExLayered|wsExToolWindow|wsExTopmost|wsExNoActivate, panelClassName, panelWindowText, wsPopup)
	if err != nil {
		return nil, err
	}
	panel := &togglePanel{hwnd: hwnd, onToggle: onToggle}
	activePanel = panel
	return panel, nil
}

// show places the panel on XMind's title bar area, like the macOS version:
// about a fifth of the way across, 7 px below the top edge.
func (p *togglePanel) show(xmindWindow uintptr, bounds rect, enabled, dark bool) {
	state := panelState{enabled: enabled, dark: dark, dpi: dpiForWindow(xmindWindow)}
	scale := float64(state.dpi) / 96
	if !p.hasRendered || state != p.renderedState {
		if p.render(state) != nil {
			return
		}
	}

	width := float64(bounds.width())
	panel := panelWidth * scale
	maximumOffset := math.Max(12*scale, width-panel-12*scale)
	offset := math.Min(maximumOffset, math.Max(170*scale, math.Min(250*scale, width*0.21)))
	origin := point{
		X: bounds.Left + int32(math.Round(offset-panelMargin*scale)),
		Y: bounds.Top + int32(math.Round((panelTopOffset-panelMargin)*scale)),
	}

	if !p.visible || origin != p.origin {
		call(procSetWindowPos, p.hwnd, hwndTopmost, uintptr(origin.X), uintptr(origin.Y), 0, 0,
			swpNoSize|swpNoActivate|swpShowWindow)
		p.origin = origin
		p.visible = true
	}
}

func (p *togglePanel) hide() {
	if p.visible {
		call(procShowWindow, p.hwnd, swHide)
		p.visible = false
	}
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]uint32
}

type blendFunction struct {
	BlendOp             byte
	BlendFlags          byte
	SourceConstantAlpha byte
	AlphaFormat         byte
}

func (p *togglePanel) render(state panelState) error {
	scale := float32(state.dpi) / 96
	width := int32(math.Round(float64((panelWidth + 2*panelMargin) * scale)))
	height := int32(math.Round(float64((panelHeight + 2*panelMargin) * scale)))

	screen := call(procGetDC, 0)
	defer call(procReleaseDC, 0, screen)
	memory := call(procCreateCompatibleDC, screen)
	defer call(procDeleteDC, memory)

	info := bitmapInfo{Header: bitmapInfoHeader{
		Width:    width,
		Height:   -height, // top-down rows, as GDI+ expects
		Planes:   1,
		BitCount: 32,
	}}
	info.Header.Size = uint32(unsafe.Sizeof(info.Header))
	var pixels unsafe.Pointer
	bitmap, err := callErr(procCreateDIBSection, memory, uintptr(unsafe.Pointer(&info)), 0,
		uintptr(unsafe.Pointer(&pixels)), 0, 0)
	if bitmap == 0 || pixels == nil {
		return err
	}
	defer call(procDeleteObject, bitmap)
	previous := call(procSelectObject, memory, bitmap)
	defer call(procSelectObject, memory, previous)

	c, err := newCanvas(pixels, width, height)
	if err != nil {
		return err
	}
	drawPanel(c, state.enabled, state.dark, scale)
	c.close()

	size := point{X: width, Y: height}
	source := point{}
	blend := blendFunction{SourceConstantAlpha: 255, AlphaFormat: 1} // AC_SRC_OVER, AC_SRC_ALPHA
	const ulwAlpha = 0x2
	if ok, err := callErr(procUpdateLayeredWindow, p.hwnd, screen, 0, uintptr(unsafe.Pointer(&size)),
		memory, uintptr(unsafe.Pointer(&source)), 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha); ok == 0 {
		return err
	}
	p.size = size
	p.renderedState = state
	p.hasRendered = true
	return nil
}

func drawPanel(c *canvas, enabled, dark bool, scale float32) {
	s := func(value float32) float32 { return value * scale }
	margin := s(panelMargin)
	body := rectF{X: margin, Y: margin, Width: s(panelWidth), Height: s(panelHeight)}

	// Soft shadow.
	for spread := float32(4); spread >= 1; spread-- {
		c.fillRoundedRect(rectF{
			X:      body.X - s(spread),
			Y:      body.Y - s(spread) + s(1),
			Width:  body.Width + 2*s(spread),
			Height: body.Height + 2*s(spread),
		}, s(panelRadius+spread), 0x07000000)
	}

	background, border, label := argb(0xF5F8F8F8), argb(0x1F000000), argb(0xE0000000)
	onTrack := argb(0xFF618F7A) // low-saturation green from the macOS switch
	if dark {
		background, border, label = 0xF22C2C2E, 0x33FFFFFF, 0xE6FFFFFF
		onTrack = 0xFF6BA38A
	}
	c.fillRoundedRect(body, s(panelRadius), background)
	c.strokeRoundedRect(rectF{X: body.X + 0.5, Y: body.Y + 0.5, Width: body.Width - 1, Height: body.Height - 1},
		s(panelRadius), 1, border)

	track := rectF{
		X:      body.X + body.Width - s(switchTrailing+switchWidth),
		Y:      body.Y + (body.Height-s(switchHeight))/2 + s(1),
		Width:  s(switchWidth),
		Height: s(switchHeight - 2),
	}
	c.drawText("自动保存", rectF{
		X:      body.X + s(labelLeading),
		Y:      body.Y,
		Width:  track.X - body.X - s(labelLeading),
		Height: body.Height,
	}, s(labelPixelSize), label, "Microsoft YaHei UI", "Microsoft YaHei", "Segoe UI")

	trackColor := argb(0x8C8E8E93) // system gray at 55 %
	if enabled {
		trackColor = onTrack
	}
	c.fillRoundedRect(track, track.Height/2, trackColor)

	knob := track.Height - s(4)
	knobX := track.X + s(2)
	if enabled {
		knobX = track.X + track.Width - knob - s(2)
	}
	knobY := track.Y + s(2)
	c.fillEllipse(rectF{X: knobX - s(0.5), Y: knobY, Width: knob + s(1), Height: knob + s(1)}, 0x30000000)
	c.fillEllipse(rectF{X: knobX, Y: knobY, Width: knob, Height: knob}, 0xFFFFFFFF)
}

// systemUsesDarkTheme reads the "app mode" chosen in Windows settings.
func systemUsesDarkTheme() bool {
	value, ok := registryDWORD(`Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, "AppsUseLightTheme")
	return ok && value == 0
}
