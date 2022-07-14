package main

import (
	"errors"
	"fmt"
	"image"
	"syscall"
	"time"

	"github.com/lxn/win"
)

type EmulatorConfig struct {
	MainWindowName    string
	InputWindowClass  string
	ScreenWindowClass string

	Executable string

	Home, AppSwitcher, Back, Escape uintptr
}

var Bluestacks = EmulatorConfig{
	MainWindowName:    "Bluestacks",
	InputWindowClass:  "plrNativeInputWindowClass",
	ScreenWindowClass: "BlueStacksApp",

	// Executable: "Bluestacks.exe",

	Home:        win.VK_HOME,
	AppSwitcher: win.VK_END,
	Back:        win.VK_PRIOR,
	Escape:      win.VK_ESCAPE,
}

var LDPlayer9 = EmulatorConfig{
	MainWindowName:    "LDPlayer",
	InputWindowClass:  "RenderWindow",
	ScreenWindowClass: "subWin",
	Executable:        "C:\\Program Files\\LDPlayer\\LDPlayer.exe",

	Home:        win.VK_F1,
	AppSwitcher: win.VK_F2,
	Back:        win.VK_BACK,
	Escape:      win.VK_ESCAPE,
}

type emulator struct {
	Config EmulatorConfig

	mainwnd, inputwnd, screenwnd win.HWND
}

func (e *emulator) Open(ec EmulatorConfig) error {
	e.Config = ec

	handle, err := findWindow(e.Config.MainWindowName)
	if err != nil {
		return fmt.Errorf("Could not find emulator: %v", err)
	}
	e.mainwnd = win.HWND(handle)

	handle = findWindowEx(handle, 0, syscall.StringToUTF16Ptr(e.Config.InputWindowClass), nil)
	if handle == 0 {
		return errors.New("First child window not found")
	}
	e.inputwnd = win.HWND(handle)

	handle = findWindowEx(handle, 0, syscall.StringToUTF16Ptr(e.Config.ScreenWindowClass), nil)
	if handle == 0 {
		return errors.New("Second child window not found")
	}

	e.screenwnd = win.HWND(handle)

	return nil
}

func (e *emulator) IsForeground() bool {
	return win.GetForegroundWindow() == e.mainwnd
}

func (e *emulator) Rect() (image.Rectangle, error) {
	return windowRect(syscall.Handle(e.screenwnd))
}

func (e *emulator) Capture() (image.Image, error) {
	r, _ := e.Rect()
	return captureWindow(syscall.Handle(e.screenwnd), r)
}

func (e *emulator) pos(p image.Point) uintptr {
	return uintptr(p.Y<<16 | (p.X & 0xFFFF))
}

func (e *emulator) Click(p image.Point, repeat int) {
	for i := 0; i < repeat; i++ {
		win.SendMessage(win.HWND(e.inputwnd), win.WM_LBUTTONDOWN, win.VK_LBUTTON, e.pos(p))
		win.SendMessage(win.HWND(e.inputwnd), win.WM_LBUTTONUP, 0, e.pos(p))
	}
}

func (e *emulator) MouseDown(p image.Point) {
	win.SendMessage(win.HWND(e.inputwnd), win.WM_LBUTTONDOWN, win.VK_LBUTTON, e.pos(p))
}

func (e *emulator) MouseDrag(p image.Point) {
	win.SendMessage(win.HWND(e.inputwnd), win.WM_MOUSEMOVE, win.VK_LBUTTON, e.pos(p))
}

func (e *emulator) MouseUp(p image.Point) {
	win.SendMessage(win.HWND(e.inputwnd), win.WM_LBUTTONUP, 0, e.pos(p))
}

func (e *emulator) SendKey(key uintptr, repeat int) {
	for i := 0; i < repeat; i++ {
		win.PostMessage(win.HWND(e.inputwnd), win.WM_KEYDOWN, key, 0)
		win.PostMessage(win.HWND(e.inputwnd), win.WM_KEYUP, key, 0)
	}
}

func (e *emulator) Activate() {
	win.SendMessage(e.mainwnd, win.WM_ACTIVATE, win.WA_CLICKACTIVE, 0)
	win.SendMessage(e.mainwnd, win.WM_ACTIVATE, win.WA_ACTIVE, 0)
	time.Sleep(time.Millisecond * 5)
}
