//go:build windows

package main

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	trayCallbackMessage = 0x8000 + 73
	trayStatusMessage   = 0x8000 + 74

	wmCommand       = 0x0111
	wmDestroy       = 0x0002
	wmClose         = 0x0010
	wmRButtonUp     = 0x0205
	wmLButtonDblClk = 0x0203

	nimAdd    = 0x00000000
	nimModify = 0x00000001
	nimDelete = 0x00000002

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004
	nifInfo    = 0x00000010

	mfString    = 0x00000000
	mfGrayed    = 0x00000001
	mfSeparator = 0x00000800

	tpmRightButton = 0x0002

	traySyncNow    = 1001
	trayOpenLog    = 1002
	trayOpenConfig = 1003
	trayExit       = 1004

	niifInfo  = 0x00000001
	niifError = 0x00000003

	idiApplication = 32512
	idcArrow       = 32512
)

type trayPoint struct {
	X int32
	Y int32
}

type trayMessage struct {
	HWnd     windows.Handle
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       trayPoint
	LPrivate uint32
}

type trayWndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     windows.Handle
	HIcon         windows.Handle
	HCursor       windows.Handle
	HbrBackground windows.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       windows.Handle
}

type trayNotifyIconData struct {
	CbSize           uint32
	HWnd             windows.Handle
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            windows.Handle
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	Version          uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     windows.Handle
}

type windowsAgentTray struct {
	hwnd       windows.Handle
	logPath    string
	configPath string
	runNow     chan struct{}
	exit       chan struct{}
	exitOnce   sync.Once
	closeOnce  sync.Once
	done       chan struct{}

	statusMu    sync.Mutex
	statusText  string
	notifyTitle string
	notifyText  string
	notifyFlags uint32
}

var (
	trayUser32              = windows.NewLazySystemDLL("user32.dll")
	trayShell32             = windows.NewLazySystemDLL("shell32.dll")
	trayKernel32            = windows.NewLazySystemDLL("kernel32.dll")
	procRegisterClassExW    = trayUser32.NewProc("RegisterClassExW")
	procCreateWindowExW     = trayUser32.NewProc("CreateWindowExW")
	procDefWindowProcW      = trayUser32.NewProc("DefWindowProcW")
	procDestroyWindow       = trayUser32.NewProc("DestroyWindow")
	procPostMessageW        = trayUser32.NewProc("PostMessageW")
	procPostQuitMessage     = trayUser32.NewProc("PostQuitMessage")
	procGetMessageW         = trayUser32.NewProc("GetMessageW")
	procTranslateMessage    = trayUser32.NewProc("TranslateMessage")
	procDispatchMessageW    = trayUser32.NewProc("DispatchMessageW")
	procLoadIconW           = trayUser32.NewProc("LoadIconW")
	procLoadCursorW         = trayUser32.NewProc("LoadCursorW")
	procCreatePopupMenu     = trayUser32.NewProc("CreatePopupMenu")
	procAppendMenuW         = trayUser32.NewProc("AppendMenuW")
	procDestroyMenu         = trayUser32.NewProc("DestroyMenu")
	procGetCursorPos        = trayUser32.NewProc("GetCursorPos")
	procSetForegroundWindow = trayUser32.NewProc("SetForegroundWindow")
	procTrackPopupMenu      = trayUser32.NewProc("TrackPopupMenu")
	procShellNotifyIconW    = trayShell32.NewProc("Shell_NotifyIconW")
	procShellExecuteW       = trayShell32.NewProc("ShellExecuteW")
	procGetModuleHandleW    = trayKernel32.NewProc("GetModuleHandleW")
)

var activeWindowsTray struct {
	sync.Mutex
	value *windowsAgentTray
}

func startAgentTray(logPath, configPath string) (*agentTrayRuntime, error) {
	logPath, err := filepath.Abs(logPath)
	if err != nil {
		return nil, fmt.Errorf("resolve tray log path: %w", err)
	}
	configPath, err = filepath.Abs(configPath)
	if err != nil {
		return nil, fmt.Errorf("resolve tray sync config path: %w", err)
	}
	state := &windowsAgentTray{
		logPath:    logPath,
		configPath: configPath,
		runNow:     make(chan struct{}, 1),
		exit:       make(chan struct{}),
		done:       make(chan struct{}),
		statusText: agentTraySummary(configPath, time.Now()),
	}
	ready := make(chan error, 1)
	go state.loop(ready)
	if err := <-ready; err != nil {
		return nil, err
	}
	return &agentTrayRuntime{
		RunNow:      state.runNow,
		Exit:        state.exit,
		ReportCycle: state.reportCycle,
		close: func() error {
			state.closeOnce.Do(func() {
				if state.hwnd != 0 {
					procPostMessageW.Call(uintptr(state.hwnd), wmClose, 0, 0)
				}
			})
			<-state.done
			return nil
		},
	}, nil
}

func (state *windowsAgentTray) loop(ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	defer close(state.done)

	className, _ := windows.UTF16PtrFromString("HomeAIWindowsSyncTray")
	windowName, _ := windows.UTF16PtrFromString("Home-AI Sync Agent")
	module, _, moduleErr := procGetModuleHandleW.Call(0)
	if module == 0 {
		ready <- fmt.Errorf("get tray module handle: %w", moduleErr)
		return
	}
	icon, _, _ := procLoadIconW.Call(0, idiApplication)
	cursor, _, _ := procLoadCursorW.Call(0, idcArrow)
	class := trayWndClassEx{
		CbSize:        uint32(unsafe.Sizeof(trayWndClassEx{})),
		LpfnWndProc:   syscall.NewCallback(trayWindowProc),
		HInstance:     windows.Handle(module),
		HIcon:         windows.Handle(icon),
		HCursor:       windows.Handle(cursor),
		LpszClassName: className,
		HIconSm:       windows.Handle(icon),
	}
	atom, _, registerErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		ready <- fmt.Errorf("register tray window class: %w", registerErr)
		return
	}
	hwnd, _, createErr := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		0,
		0, 0, 0, 0,
		0, 0,
		module,
		0,
	)
	if hwnd == 0 {
		ready <- fmt.Errorf("create tray window: %w", createErr)
		return
	}
	state.hwnd = windows.Handle(hwnd)
	activeWindowsTray.Lock()
	activeWindowsTray.value = state
	activeWindowsTray.Unlock()
	defer func() {
		activeWindowsTray.Lock()
		if activeWindowsTray.value == state {
			activeWindowsTray.value = nil
		}
		activeWindowsTray.Unlock()
	}()

	var notify trayNotifyIconData
	notify.CbSize = uint32(unsafe.Sizeof(notify))
	notify.HWnd = state.hwnd
	notify.UID = 1
	notify.UFlags = nifMessage | nifIcon | nifTip
	notify.UCallbackMessage = trayCallbackMessage
	notify.HIcon = windows.Handle(icon)
	copy(notify.SzTip[:], windows.StringToUTF16(state.tooltip()))
	added, _, addErr := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&notify)))
	if added == 0 {
		procDestroyWindow.Call(uintptr(state.hwnd))
		ready <- fmt.Errorf("add tray icon: %w", addErr)
		return
	}
	defer procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&notify)))
	ready <- nil

	var msg trayMessage
	for {
		result, _, callErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) == -1 {
			state.signalExit()
			_ = callErr
			return
		}
		if result == 0 {
			state.signalExit()
			return
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func trayWindowProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	activeWindowsTray.Lock()
	state := activeWindowsTray.value
	activeWindowsTray.Unlock()
	switch message {
	case trayCallbackMessage:
		if state == nil {
			break
		}
		switch uint32(lParam) {
		case wmRButtonUp:
			state.showMenu()
			return 0
		case wmLButtonDblClk:
			state.openPath(state.logPath)
			return 0
		}
	case trayStatusMessage:
		if state != nil {
			state.applyStatus()
		}
		return 0
	case wmCommand:
		if state == nil {
			break
		}
		switch uint16(wParam & 0xffff) {
		case traySyncNow:
			select {
			case state.runNow <- struct{}{}:
			default:
			}
		case trayOpenLog:
			state.openPath(state.logPath)
		case trayOpenConfig:
			state.openPath(state.configPath)
		case trayExit:
			state.signalExit()
			procDestroyWindow.Call(hwnd)
		}
		return 0
	case wmDestroy:
		if state != nil {
			state.signalExit()
		}
		procPostQuitMessage.Call(0)
		return 0
	}
	result, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
	return result
}

func (state *windowsAgentTray) signalExit() {
	state.exitOnce.Do(func() { close(state.exit) })
}

func (state *windowsAgentTray) reportCycle(manual bool, count int, runErr error) {
	state.statusMu.Lock()
	state.statusText = agentTraySummary(state.configPath, time.Now())
	switch {
	case runErr != nil:
		state.notifyTitle = "Home-AI sync failed"
		state.notifyText = "Open the agent log for details."
		state.notifyFlags = niifError
	case manual:
		state.notifyTitle = "Home-AI sync complete"
		state.notifyText = fmt.Sprintf("%d profile(s) processed.", count)
		state.notifyFlags = niifInfo
	default:
		state.notifyTitle = ""
		state.notifyText = ""
		state.notifyFlags = 0
	}
	state.statusMu.Unlock()
	if state.hwnd != 0 {
		procPostMessageW.Call(uintptr(state.hwnd), trayStatusMessage, 0, 0)
	}
}

func (state *windowsAgentTray) tooltip() string {
	state.statusMu.Lock()
	status := state.statusText
	state.statusMu.Unlock()
	if status == "" {
		return "Home-AI Sync Agent"
	}
	return "Home-AI Sync Agent — " + status
}

func (state *windowsAgentTray) applyStatus() {
	state.statusMu.Lock()
	status := state.statusText
	title := state.notifyTitle
	message := state.notifyText
	flags := state.notifyFlags
	state.notifyTitle = ""
	state.notifyText = ""
	state.notifyFlags = 0
	state.statusMu.Unlock()

	var notify trayNotifyIconData
	notify.CbSize = uint32(unsafe.Sizeof(notify))
	notify.HWnd = state.hwnd
	notify.UID = 1
	notify.UFlags = nifTip
	tip := "Home-AI Sync Agent"
	if status != "" {
		tip += " — " + status
	}
	copy(notify.SzTip[:], windows.StringToUTF16(tip))
	procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&notify)))

	if message == "" {
		return
	}
	notify = trayNotifyIconData{}
	notify.CbSize = uint32(unsafe.Sizeof(notify))
	notify.HWnd = state.hwnd
	notify.UID = 1
	notify.UFlags = nifInfo
	copy(notify.SzInfoTitle[:], windows.StringToUTF16(title))
	copy(notify.SzInfo[:], windows.StringToUTF16(message))
	notify.DwInfoFlags = flags
	procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&notify)))
}

func (state *windowsAgentTray) showMenu() {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)
	summary := agentTraySummary(state.configPath, time.Now())
	state.statusMu.Lock()
	state.statusText = summary
	state.statusMu.Unlock()
	appendTrayMenu(menu, mfString|mfGrayed, 0, summary)
	appendTrayMenu(menu, mfSeparator, 0, "")
	appendTrayMenu(menu, mfString, traySyncNow, "Sync now")
	appendTrayMenu(menu, mfSeparator, 0, "")
	appendTrayMenu(menu, mfString, trayOpenLog, "Open log")
	appendTrayMenu(menu, mfString, trayOpenConfig, "Open sync profiles")
	appendTrayMenu(menu, mfSeparator, 0, "")
	appendTrayMenu(menu, mfString, trayExit, "Exit")
	var point trayPoint
	if ok, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&point))); ok == 0 {
		return
	}
	procSetForegroundWindow.Call(uintptr(state.hwnd))
	procTrackPopupMenu.Call(
		menu,
		tpmRightButton,
		uintptr(point.X),
		uintptr(point.Y),
		0,
		uintptr(state.hwnd),
		0,
	)
}

func appendTrayMenu(menu uintptr, flags uint32, id uint16, label string) {
	var text *uint16
	if label != "" {
		text, _ = windows.UTF16PtrFromString(label)
	}
	procAppendMenuW.Call(menu, uintptr(flags), uintptr(id), uintptr(unsafe.Pointer(text)))
}

func (state *windowsAgentTray) openPath(path string) {
	verb, _ := windows.UTF16PtrFromString("open")
	target, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	result, _, _ := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(target)),
		0,
		0,
		1,
	)
	if result <= 32 {
		_ = errors.New("ShellExecuteW failed")
	}
}
