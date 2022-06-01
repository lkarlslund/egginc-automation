package main

import (
	"syscall"
	"unsafe"

	"github.com/lxn/win"
)

const (
	WM_KEYDOWN uint32 = 0x0100
	WM_KEYUP   uint32 = 0x0101
	VK_A              = 0x41
)

type mouseInput struct {
	itype                  uint32
	x, y                   int32
	mousedata, flags, time uint32
	extrainfo              uintptr
}

const (
	KEYEVENTF_EXTENDEDKEY = 0x0001 // If specified, the scan code was preceded by a prefix byte that has the value 0xE0 (224).
	KEYEVENTF_KEYUP       = 0x0002 // If specified, the key is being released. If not specified, the key is being pressed.
	KEYEVENTF_SCANCODE    = 0x0008 // If specified, wScan identifies the key and wVk is ignored.
	KEYEVENTF_UNICODE     = 0x0004 // If specified, the system synthesizes a VK_PACKET keystroke. The wVk parameter must be zero. This flag can only be combined with the KEYEVENTF_KEYUP flag. For more information, see the Remarks section.
)

type keyInput struct {
	itype     uint32 // 1 for keys
	vk        uint16
	wscan     uint16
	dwFlags   uint32
	time      uint32
	extrainfo uintptr
}

func sendClick(handle syscall.Handle, x, y int) {
	p := win.POINT{
		X: int32(x),
		Y: int32(y),
	}
	win.ClientToScreen(win.HWND(handle), &p)

	inputs := []mouseInput{
		{
			itype:     0, // Mouse
			x:         p.X,
			y:         p.Y,
			mousedata: 0,
			flags:     0x0002, // left mouse down
		},
		{
			itype: 0, // Mouse
			x:     p.X,
			y:     p.Y,
			flags: 0x04, // left mouse up
		},
	}

	win.SendInput(uint32(len(inputs)), unsafe.Pointer(&inputs[0]), int32(unsafe.Sizeof(mouseInput{}))*int32(len(inputs)))

	// robotgo.MoveClick(int(p.X), int(p.Y))
	// pos := uintptr(x<<16 | y)
	// win.SendMessage(win.HWND(handle), WM_KEYDOWN, VK_A, pos)
	// win.SendMessage(win.HWND(handle), WM_KEYUP, VK_A, pos)
}
