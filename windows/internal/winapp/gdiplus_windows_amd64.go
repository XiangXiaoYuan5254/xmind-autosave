package winapp

import (
	"fmt"
	"math"
	"syscall"
	"unsafe"
)

// GDI+ flat API, used to draw the anti-aliased toggle panel into a
// premultiplied-alpha bitmap. Several functions take REAL (float32) arguments;
// Go's windows/amd64 syscall trampoline mirrors the first four arguments into
// XMM0-3 and passes the rest on the stack, which is exactly what the x64
// calling convention expects. That is why this file is amd64-only.

var (
	gdiplus = syscall.NewLazyDLL("gdiplus.dll")

	procGdiplusStartup               = gdiplus.NewProc("GdiplusStartup")
	procGdipAddPathArc               = gdiplus.NewProc("GdipAddPathArc")
	procGdipClosePathFigure          = gdiplus.NewProc("GdipClosePathFigure")
	procGdipCreateBitmapFromScan0    = gdiplus.NewProc("GdipCreateBitmapFromScan0")
	procGdipCreateFont               = gdiplus.NewProc("GdipCreateFont")
	procGdipCreateFontFamilyFromName = gdiplus.NewProc("GdipCreateFontFamilyFromName")
	procGdipCreatePath               = gdiplus.NewProc("GdipCreatePath")
	procGdipCreatePen1               = gdiplus.NewProc("GdipCreatePen1")
	procGdipCreateSolidFill          = gdiplus.NewProc("GdipCreateSolidFill")
	procGdipCreateStringFormat       = gdiplus.NewProc("GdipCreateStringFormat")
	procGdipDeleteBrush              = gdiplus.NewProc("GdipDeleteBrush")
	procGdipDeleteFont               = gdiplus.NewProc("GdipDeleteFont")
	procGdipDeleteFontFamily         = gdiplus.NewProc("GdipDeleteFontFamily")
	procGdipDeleteGraphics           = gdiplus.NewProc("GdipDeleteGraphics")
	procGdipDeletePath               = gdiplus.NewProc("GdipDeletePath")
	procGdipDeletePen                = gdiplus.NewProc("GdipDeletePen")
	procGdipDeleteStringFormat       = gdiplus.NewProc("GdipDeleteStringFormat")
	procGdipDisposeImage             = gdiplus.NewProc("GdipDisposeImage")
	procGdipDrawPath                 = gdiplus.NewProc("GdipDrawPath")
	procGdipDrawString               = gdiplus.NewProc("GdipDrawString")
	procGdipFillEllipse              = gdiplus.NewProc("GdipFillEllipse")
	procGdipFillPath                 = gdiplus.NewProc("GdipFillPath")
	procGdipGetImageGraphicsContext  = gdiplus.NewProc("GdipGetImageGraphicsContext")
	procGdipGraphicsClear            = gdiplus.NewProc("GdipGraphicsClear")
	procGdipSetPixelOffsetMode       = gdiplus.NewProc("GdipSetPixelOffsetMode")
	procGdipSetSmoothingMode         = gdiplus.NewProc("GdipSetSmoothingMode")
	procGdipSetStringFormatAlign     = gdiplus.NewProc("GdipSetStringFormatAlign")
	procGdipSetStringFormatFlags     = gdiplus.NewProc("GdipSetStringFormatFlags")
	procGdipSetStringFormatLineAlign = gdiplus.NewProc("GdipSetStringFormatLineAlign")
	procGdipSetTextRenderingHint     = gdiplus.NewProc("GdipSetTextRenderingHint")
)

func float32Arg(value float32) uintptr {
	return uintptr(math.Float32bits(value))
}

func gdipStatus(name string, status uintptr) error {
	if status == 0 {
		return nil
	}
	return fmt.Errorf("%s: GDI+ status %d", name, status)
}

type gdiplusStartupInput struct {
	Version                  uint32
	DebugEventCallback       uintptr
	SuppressBackgroundThread int32
	SuppressExternalCodecs   int32
}

var gdiplusToken uintptr

func startGDIPlus() error {
	if gdiplusToken != 0 {
		return nil
	}
	input := gdiplusStartupInput{Version: 1}
	return gdipStatus("GdiplusStartup", call(procGdiplusStartup,
		uintptr(unsafe.Pointer(&gdiplusToken)), uintptr(unsafe.Pointer(&input)), 0))
}

type argb uint32

type rectF struct {
	X, Y, Width, Height float32
}

// canvas draws into caller-owned 32-bit premultiplied ARGB pixels.
type canvas struct {
	bitmap   uintptr
	graphics uintptr
}

func newCanvas(pixels unsafe.Pointer, width, height int32) (*canvas, error) {
	const (
		pixelFormat32bppPARGB             = 0x000E200B
		smoothingModeAntiAlias            = 4
		pixelOffsetModeHalf               = 4
		textRenderingHintAntiAliasGridFit = 3
	)
	c := &canvas{}
	if err := gdipStatus("GdipCreateBitmapFromScan0", call(procGdipCreateBitmapFromScan0,
		uintptr(width), uintptr(height), uintptr(width*4), pixelFormat32bppPARGB,
		uintptr(pixels), uintptr(unsafe.Pointer(&c.bitmap)))); err != nil {
		return nil, err
	}
	if err := gdipStatus("GdipGetImageGraphicsContext", call(procGdipGetImageGraphicsContext,
		c.bitmap, uintptr(unsafe.Pointer(&c.graphics)))); err != nil {
		call(procGdipDisposeImage, c.bitmap)
		return nil, err
	}
	call(procGdipSetSmoothingMode, c.graphics, smoothingModeAntiAlias)
	call(procGdipSetPixelOffsetMode, c.graphics, pixelOffsetModeHalf)
	call(procGdipSetTextRenderingHint, c.graphics, textRenderingHintAntiAliasGridFit)
	call(procGdipGraphicsClear, c.graphics, 0)
	return c, nil
}

func (c *canvas) close() {
	call(procGdipDeleteGraphics, c.graphics)
	call(procGdipDisposeImage, c.bitmap)
}

func (c *canvas) withBrush(color argb, draw func(brush uintptr)) {
	var brush uintptr
	if call(procGdipCreateSolidFill, uintptr(color), uintptr(unsafe.Pointer(&brush))) != 0 {
		return
	}
	defer call(procGdipDeleteBrush, brush)
	draw(brush)
}

func (c *canvas) withRoundedRect(bounds rectF, radius float32, draw func(path uintptr)) {
	var path uintptr
	if call(procGdipCreatePath, 0, uintptr(unsafe.Pointer(&path))) != 0 {
		return
	}
	defer call(procGdipDeletePath, path)

	radius = min(radius, bounds.Width/2, bounds.Height/2)
	diameter := radius * 2
	left, top := bounds.X, bounds.Y
	right, bottom := bounds.X+bounds.Width-diameter, bounds.Y+bounds.Height-diameter
	for _, arc := range [4][3]float32{
		{left, top, 180},
		{right, top, 270},
		{right, bottom, 0},
		{left, bottom, 90},
	} {
		call(procGdipAddPathArc, path, float32Arg(arc[0]), float32Arg(arc[1]), float32Arg(diameter), float32Arg(diameter), float32Arg(arc[2]), float32Arg(90))
	}
	call(procGdipClosePathFigure, path)
	draw(path)
}

func (c *canvas) fillRoundedRect(bounds rectF, radius float32, color argb) {
	c.withRoundedRect(bounds, radius, func(path uintptr) {
		c.withBrush(color, func(brush uintptr) {
			call(procGdipFillPath, c.graphics, brush, path)
		})
	})
}

func (c *canvas) strokeRoundedRect(bounds rectF, radius, width float32, color argb) {
	const unitPixel = 2
	c.withRoundedRect(bounds, radius, func(path uintptr) {
		var pen uintptr
		if call(procGdipCreatePen1, uintptr(color), float32Arg(width), unitPixel, uintptr(unsafe.Pointer(&pen))) != 0 {
			return
		}
		defer call(procGdipDeletePen, pen)
		call(procGdipDrawPath, c.graphics, pen, path)
	})
}

func (c *canvas) fillEllipse(bounds rectF, color argb) {
	c.withBrush(color, func(brush uintptr) {
		call(procGdipFillEllipse, c.graphics, brush,
			float32Arg(bounds.X), float32Arg(bounds.Y), float32Arg(bounds.Width), float32Arg(bounds.Height))
	})
}

// drawText draws one line, left-aligned and vertically centred in bounds.
func (c *canvas) drawText(text string, bounds rectF, pixelSize float32, color argb, families ...string) {
	const (
		fontStyleRegular        = 0
		unitPixel               = 2
		stringAlignmentNear     = 0
		stringAlignmentCenter   = 1
		stringFormatFlagsNoWrap = 0x1000
	)
	var family uintptr
	for _, name := range families {
		if call(procGdipCreateFontFamilyFromName, uintptr(unsafe.Pointer(utf16Ptr(name))), 0, uintptr(unsafe.Pointer(&family))) == 0 {
			break
		}
		family = 0
	}
	if family == 0 {
		return
	}
	defer call(procGdipDeleteFontFamily, family)

	var font uintptr
	if call(procGdipCreateFont, family, float32Arg(pixelSize), fontStyleRegular, unitPixel, uintptr(unsafe.Pointer(&font))) != 0 {
		return
	}
	defer call(procGdipDeleteFont, font)

	var format uintptr
	if call(procGdipCreateStringFormat, 0, 0, uintptr(unsafe.Pointer(&format))) != 0 {
		return
	}
	defer call(procGdipDeleteStringFormat, format)
	call(procGdipSetStringFormatAlign, format, stringAlignmentNear)
	call(procGdipSetStringFormatLineAlign, format, stringAlignmentCenter)
	call(procGdipSetStringFormatFlags, format, stringFormatFlagsNoWrap)

	c.withBrush(color, func(brush uintptr) {
		call(procGdipDrawString, c.graphics, uintptr(unsafe.Pointer(utf16Ptr(text))), ^uintptr(0),
			font, uintptr(unsafe.Pointer(&bounds)), format, brush)
	})
}
