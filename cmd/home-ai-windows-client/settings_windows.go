//go:build windows

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/DeadSoulf/home-ai-core/internal/windowsclient"
	"golang.org/x/sys/windows"
)

const (
	settingsWindowClass = "HomeAIWindowsSettingsWindow"
	settingsWindowTitle = "Home-AI Windows Client"

	settingsWMCommand = 0x0111
	settingsWMDestroy = 0x0002
	settingsWMClose   = 0x0010
	settingsWMSetFont = 0x0030
	settingsWMApp     = 0x8000

	settingsWMConnected = settingsWMApp + 201
	settingsWMSyncDone  = settingsWMApp + 202
	settingsWMAgentDone = settingsWMApp + 203

	settingsWSOverlappedWindow = 0x00CF0000
	settingsWSVisible          = 0x10000000
	settingsWSChild            = 0x40000000
	settingsWSTabStop          = 0x00010000
	settingsWSBorder           = 0x00800000
	settingsWSVScroll          = 0x00200000

	settingsESAutoHScroll = 0x0080
	settingsESPassword    = 0x0020

	settingsCBSUppercase    = 0x2000
	settingsCBSDropDownList = 0x0003

	settingsLBSNotify = 0x0001

	settingsSWShow    = 5
	settingsSWRestore = 9

	settingsBNClicked   = 0
	settingsLBNSelect   = 1
	settingsCBNSelect   = 1
	settingsCBErr       = ^uintptr(0)
	settingsLBErr       = ^uintptr(0)
	settingsCBReset     = 0x014B
	settingsCBAddString = 0x0143
	settingsCBGetCurSel = 0x0147
	settingsCBSetCurSel = 0x014E
	settingsLBReset     = 0x0184
	settingsLBAddString = 0x0180
	settingsLBGetCurSel = 0x0188
	settingsLBSetCurSel = 0x0186

	settingsMBOK          = 0x00000000
	settingsMBYesNo       = 0x00000004
	settingsMBIconError   = 0x00000010
	settingsMBIconWarning = 0x00000030
	settingsIDYes         = 6

	settingsIDServerEdit     = 2001
	settingsIDUsernameEdit   = 2002
	settingsIDPasswordEdit   = 2003
	settingsIDConnect        = 2004
	settingsIDFolderCombo    = 2005
	settingsIDSourceEdit     = 2006
	settingsIDBrowse         = 2007
	settingsIDDestination    = 2008
	settingsIDInterval       = 2009
	settingsIDConflict       = 2010
	settingsIDSaveProfile    = 2011
	settingsIDNewProfile     = 2012
	settingsIDProfileList    = 2013
	settingsIDEnableProfile  = 2014
	settingsIDDisableProfile = 2015
	settingsIDDeleteProfile  = 2016
	settingsIDSyncNow        = 2017
	settingsIDAgentEnable    = 2018
	settingsIDAgentDisable   = 2019
	settingsIDRefresh        = 2020

	settingsDefaultGUIFont = 17
	settingsIDIApplication = 32512
	settingsIDCArrow       = 32512

	settingsCreateNoWindow = 0x08000000
)

type settingsWndClassEx struct {
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

type settingsPoint struct {
	X int32
	Y int32
}

type settingsMessage struct {
	HWnd     windows.Handle
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       settingsPoint
	LPrivate uint32
}

type settingsBrowseInfo struct {
	Owner       windows.Handle
	Root        uintptr
	DisplayName *uint16
	Title       *uint16
	Flags       uint32
	Callback    uintptr
	LParam      uintptr
	Image       int32
}

type settingsConnectResult struct {
	server  string
	user    string
	folders []windowsclient.Folder
	err     error
}

type settingsAsyncResult struct {
	message string
	err     error
}

type windowsSettingsUI struct {
	hwnd         windows.Handle
	configPath   string
	settingsPath string

	serverEdit     windows.Handle
	usernameEdit   windows.Handle
	passwordEdit   windows.Handle
	folderCombo    windows.Handle
	sourceEdit     windows.Handle
	destination    windows.Handle
	interval       windows.Handle
	conflict       windows.Handle
	profileList    windows.Handle
	statusLabel    windows.Handle
	agentLabel     windows.Handle
	saveProfileBtn windows.Handle

	folders           []windowsclient.Folder
	folderIDs         []string
	profiles          []windowsclient.SyncProfile
	selectedProfileID string

	asyncMu       sync.Mutex
	connectResult *settingsConnectResult
	syncResult    *settingsAsyncResult
	agentResult   *settingsAsyncResult
}

var (
	settingsUser32            = windows.NewLazySystemDLL("user32.dll")
	settingsKernel32          = windows.NewLazySystemDLL("kernel32.dll")
	settingsGDI32             = windows.NewLazySystemDLL("gdi32.dll")
	settingsShell32           = windows.NewLazySystemDLL("shell32.dll")
	settingsOle32             = windows.NewLazySystemDLL("ole32.dll")
	procSettingsRegisterClass = settingsUser32.NewProc("RegisterClassExW")
	procSettingsCreateWindow  = settingsUser32.NewProc("CreateWindowExW")
	procSettingsDefWindowProc = settingsUser32.NewProc("DefWindowProcW")
	procSettingsDestroyWindow = settingsUser32.NewProc("DestroyWindow")
	procSettingsPostQuit      = settingsUser32.NewProc("PostQuitMessage")
	procSettingsGetMessage    = settingsUser32.NewProc("GetMessageW")
	procSettingsTranslate     = settingsUser32.NewProc("TranslateMessage")
	procSettingsDispatch      = settingsUser32.NewProc("DispatchMessageW")
	procSettingsShowWindow    = settingsUser32.NewProc("ShowWindow")
	procSettingsUpdateWindow  = settingsUser32.NewProc("UpdateWindow")
	procSettingsSetForeground = settingsUser32.NewProc("SetForegroundWindow")
	procSettingsFindWindow    = settingsUser32.NewProc("FindWindowW")
	procSettingsPostMessage   = settingsUser32.NewProc("PostMessageW")
	procSettingsSendMessage   = settingsUser32.NewProc("SendMessageW")
	procSettingsSetWindowText = settingsUser32.NewProc("SetWindowTextW")
	procSettingsGetTextLength = settingsUser32.NewProc("GetWindowTextLengthW")
	procSettingsGetWindowText = settingsUser32.NewProc("GetWindowTextW")
	procSettingsEnableWindow  = settingsUser32.NewProc("EnableWindow")
	procSettingsMessageBox    = settingsUser32.NewProc("MessageBoxW")
	procSettingsLoadIcon      = settingsUser32.NewProc("LoadIconW")
	procSettingsLoadCursor    = settingsUser32.NewProc("LoadCursorW")
	procSettingsGetModule     = settingsKernel32.NewProc("GetModuleHandleW")
	procSettingsGetConsole    = settingsKernel32.NewProc("GetConsoleWindow")
	procSettingsConsolePIDs   = settingsKernel32.NewProc("GetConsoleProcessList")
	procSettingsGetStock      = settingsGDI32.NewProc("GetStockObject")
	procSettingsBrowseFolder  = settingsShell32.NewProc("SHBrowseForFolderW")
	procSettingsGetPath       = settingsShell32.NewProc("SHGetPathFromIDListW")
	procSettingsTaskMemFree   = settingsOle32.NewProc("CoTaskMemFree")
)

var activeSettingsUI struct {
	sync.Mutex
	value *windowsSettingsUI
}

func runDefaultCommand() error {
	hidePrivateConsoleWindow()
	return runSettingsUI(nil)
}

func hidePrivateConsoleWindow() {
	hwnd, _, _ := procSettingsGetConsole.Call()
	if hwnd == 0 {
		return
	}
	var processes [8]uint32
	count, _, _ := procSettingsConsolePIDs.Call(
		uintptr(unsafe.Pointer(&processes[0])),
		uintptr(len(processes)),
	)
	if count == 1 {
		procSettingsShowWindow.Call(hwnd, 0)
	}
}

func runSettingsUI(args []string) error {
	fs := flag.NewFlagSet("settings", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var configPath, settingsPath string
	fs.StringVar(&configPath, "config", "", "sync profile file")
	fs.StringVar(&settingsPath, "settings", "", "Windows client settings file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("settings does not accept positional arguments")
	}
	if configPath == "" {
		var err error
		configPath, err = defaultSyncConfigPath()
		if err != nil {
			return err
		}
	}
	if settingsPath == "" {
		var err error
		settingsPath, err = defaultClientSettingsPath()
		if err != nil {
			return err
		}
	}
	var err error
	configPath, err = filepath.Abs(configPath)
	if err != nil {
		return fmt.Errorf("resolve sync config path: %w", err)
	}
	settingsPath, err = filepath.Abs(settingsPath)
	if err != nil {
		return fmt.Errorf("resolve client settings path: %w", err)
	}

	if focusExistingSettingsWindow() {
		return nil
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	className, _ := windows.UTF16PtrFromString(settingsWindowClass)
	windowName, _ := windows.UTF16PtrFromString(settingsWindowTitle)
	module, _, moduleErr := procSettingsGetModule.Call(0)
	if module == 0 {
		return fmt.Errorf("get settings module handle: %w", moduleErr)
	}
	icon, _, _ := procSettingsLoadIcon.Call(0, settingsIDIApplication)
	cursor, _, _ := procSettingsLoadCursor.Call(0, settingsIDCArrow)
	class := settingsWndClassEx{
		CbSize:        uint32(unsafe.Sizeof(settingsWndClassEx{})),
		LpfnWndProc:   syscall.NewCallback(settingsWindowProc),
		HInstance:     windows.Handle(module),
		HIcon:         windows.Handle(icon),
		HCursor:       windows.Handle(cursor),
		HbrBackground: windows.Handle(6),
		LpszClassName: className,
		HIconSm:       windows.Handle(icon),
	}
	atom, _, registerErr := procSettingsRegisterClass.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		return fmt.Errorf("register settings window class: %w", registerErr)
	}

	state := &windowsSettingsUI{
		configPath:   configPath,
		settingsPath: settingsPath,
	}
	activeSettingsUI.Lock()
	activeSettingsUI.value = state
	activeSettingsUI.Unlock()
	defer func() {
		activeSettingsUI.Lock()
		if activeSettingsUI.value == state {
			activeSettingsUI.value = nil
		}
		activeSettingsUI.Unlock()
	}()

	hwnd, _, createErr := procSettingsCreateWindow.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
		settingsWSOverlappedWindow,
		0x80000000,
		0x80000000,
		860,
		720,
		0,
		0,
		module,
		0,
	)
	if hwnd == 0 {
		return fmt.Errorf("create settings window: %w", createErr)
	}
	state.hwnd = windows.Handle(hwnd)
	if err := state.createControls(windows.Handle(module)); err != nil {
		procSettingsDestroyWindow.Call(hwnd)
		return err
	}
	state.loadInitial()
	procSettingsShowWindow.Call(hwnd, settingsSWShow)
	procSettingsUpdateWindow.Call(hwnd)

	var msg settingsMessage
	for {
		result, _, callErr := procSettingsGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) == -1 {
			return fmt.Errorf("settings message loop: %w", callErr)
		}
		if result == 0 {
			return nil
		}
		procSettingsTranslate.Call(uintptr(unsafe.Pointer(&msg)))
		procSettingsDispatch.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func startSettingsProcess(configPath string) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate Windows client executable: %w", err)
	}
	args := []string{"settings"}
	if strings.TrimSpace(configPath) != "" {
		args = append(args, "--config", configPath)
	}
	command := exec.Command(executable, args...)
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: settingsCreateNoWindow}
	if err := command.Start(); err != nil {
		return fmt.Errorf("start Home-AI settings: %w", err)
	}
	if err := command.Process.Release(); err != nil {
		return fmt.Errorf("release Home-AI settings process: %w", err)
	}
	return nil
}

func focusExistingSettingsWindow() bool {
	className, _ := windows.UTF16PtrFromString(settingsWindowClass)
	windowName, _ := windows.UTF16PtrFromString(settingsWindowTitle)
	hwnd, _, _ := procSettingsFindWindow.Call(
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(windowName)),
	)
	if hwnd == 0 {
		return false
	}
	procSettingsShowWindow.Call(hwnd, settingsSWRestore)
	procSettingsSetForeground.Call(hwnd)
	return true
}

func settingsWindowProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	activeSettingsUI.Lock()
	state := activeSettingsUI.value
	activeSettingsUI.Unlock()

	switch message {
	case settingsWMCommand:
		if state != nil {
			id := uint16(wParam & 0xffff)
			notify := uint16((wParam >> 16) & 0xffff)
			state.handleCommand(id, notify)
			return 0
		}
	case settingsWMConnected:
		if state != nil {
			state.finishConnect()
			return 0
		}
	case settingsWMSyncDone:
		if state != nil {
			state.finishSync()
			return 0
		}
	case settingsWMAgentDone:
		if state != nil {
			state.finishAgent()
			return 0
		}
	case settingsWMClose:
		procSettingsDestroyWindow.Call(hwnd)
		return 0
	case settingsWMDestroy:
		procSettingsPostQuit.Call(0)
		return 0
	}
	result, _, _ := procSettingsDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return result
}

func (state *windowsSettingsUI) createControls(module windows.Handle) error {
	font, _, _ := procSettingsGetStock.Call(settingsDefaultGUIFont)
	label := func(text string, x, y, w, h int32) windows.Handle {
		return state.createControl(module, "STATIC", text, settingsWSChild|settingsWSVisible, x, y, w, h, 0, windows.Handle(font))
	}
	edit := func(id uint16, text string, x, y, w, h int32, extra uint32) windows.Handle {
		return state.createControl(module, "EDIT", text, settingsWSChild|settingsWSVisible|settingsWSTabStop|settingsWSBorder|settingsESAutoHScroll|extra, x, y, w, h, id, windows.Handle(font))
	}
	button := func(id uint16, text string, x, y, w, h int32) windows.Handle {
		return state.createControl(module, "BUTTON", text, settingsWSChild|settingsWSVisible|settingsWSTabStop, x, y, w, h, id, windows.Handle(font))
	}

	label("Server", 20, 20, 90, 22)
	state.serverEdit = edit(settingsIDServerEdit, "", 120, 16, 510, 26, 0)
	button(settingsIDConnect, "Connect & save", 645, 16, 175, 28)

	label("User", 20, 56, 90, 22)
	state.usernameEdit = edit(settingsIDUsernameEdit, "", 120, 52, 230, 26, 0)
	label("Password", 370, 56, 90, 22)
	state.passwordEdit = edit(settingsIDPasswordEdit, "", 460, 52, 190, 26, settingsESPassword)
	label("blank = stored credential", 660, 56, 160, 22)

	label("Remote folder", 20, 94, 95, 22)
	state.folderCombo = state.createControl(module, "COMBOBOX", "", settingsWSChild|settingsWSVisible|settingsWSTabStop|settingsWSVScroll|settingsCBSDropDownList, 120, 90, 700, 240, settingsIDFolderCombo, windows.Handle(font))

	label("Local source", 20, 132, 95, 22)
	state.sourceEdit = edit(settingsIDSourceEdit, "", 120, 128, 610, 26, 0)
	button(settingsIDBrowse, "Browse...", 740, 128, 80, 28)

	label("Destination", 20, 168, 95, 22)
	state.destination = edit(settingsIDDestination, "", 120, 164, 300, 26, 0)
	label("Interval", 440, 168, 70, 22)
	state.interval = edit(settingsIDInterval, "15m", 510, 164, 100, 26, 0)
	label("Conflict", 625, 168, 65, 22)
	state.conflict = state.createControl(module, "COMBOBOX", "", settingsWSChild|settingsWSVisible|settingsWSTabStop|settingsWSVScroll|settingsCBSDropDownList, 690, 164, 130, 160, settingsIDConflict, windows.Handle(font))
	state.comboAdd(state.conflict, "Stop")
	state.comboAdd(state.conflict, "Skip")
	state.comboAdd(state.conflict, "Trash + replace")
	procSettingsSendMessage.Call(uintptr(state.conflict), settingsCBSetCurSel, 0, 0)

	state.saveProfileBtn = button(settingsIDSaveProfile, "Add profile", 120, 204, 125, 30)
	button(settingsIDNewProfile, "New / clear", 255, 204, 115, 30)
	button(settingsIDEnableProfile, "Enable", 380, 204, 95, 30)
	button(settingsIDDisableProfile, "Disable", 485, 204, 95, 30)
	button(settingsIDDeleteProfile, "Delete", 590, 204, 95, 30)
	button(settingsIDRefresh, "Refresh", 695, 204, 125, 30)

	label("Sync profiles", 20, 252, 100, 22)
	state.profileList = state.createControl(module, "LISTBOX", "", settingsWSChild|settingsWSVisible|settingsWSTabStop|settingsWSBorder|settingsWSVScroll|settingsLBSNotify, 20, 276, 800, 245, settingsIDProfileList, windows.Handle(font))

	state.agentLabel = label("Autostart: checking...", 20, 542, 430, 24)
	button(settingsIDSyncNow, "Sync now", 20, 576, 120, 32)
	button(settingsIDAgentEnable, "Enable agent", 150, 576, 135, 32)
	button(settingsIDAgentDisable, "Disable agent", 295, 576, 135, 32)

	label("Status", 20, 630, 60, 22)
	state.statusLabel = label("Ready", 80, 630, 740, 42)

	if state.serverEdit == 0 || state.usernameEdit == 0 || state.passwordEdit == 0 ||
		state.folderCombo == 0 || state.sourceEdit == 0 || state.destination == 0 ||
		state.interval == 0 || state.conflict == 0 || state.profileList == 0 ||
		state.statusLabel == 0 || state.agentLabel == 0 {
		return errors.New("create Windows settings controls")
	}
	return nil
}

func (state *windowsSettingsUI) createControl(module windows.Handle, class, text string, style uint32, x, y, w, h int32, id uint16, font windows.Handle) windows.Handle {
	classPtr, _ := windows.UTF16PtrFromString(class)
	textPtr, _ := windows.UTF16PtrFromString(text)
	hwnd, _, _ := procSettingsCreateWindow.Call(
		0,
		uintptr(unsafe.Pointer(classPtr)),
		uintptr(unsafe.Pointer(textPtr)),
		uintptr(style),
		uintptr(x),
		uintptr(y),
		uintptr(w),
		uintptr(h),
		uintptr(state.hwnd),
		uintptr(id),
		uintptr(module),
		0,
	)
	if hwnd != 0 && font != 0 {
		procSettingsSendMessage.Call(hwnd, settingsWMSetFont, uintptr(font), 1)
	}
	return windows.Handle(hwnd)
}

func (state *windowsSettingsUI) loadInitial() {
	settings, settingsErr := windowsclient.LoadClientSettings(state.settingsPath)
	if settingsErr == nil {
		state.setText(state.serverEdit, settings.ServerURL)
		state.setText(state.usernameEdit, settings.Username)
	}
	if err := state.reloadProfiles(""); err != nil {
		state.setStatus(err.Error())
	} else if settingsErr != nil {
		state.setStatus(settingsErr.Error())
	}
	if settings.ServerURL == "" && len(state.profiles) > 0 {
		state.setText(state.serverEdit, state.profiles[0].ServerURL)
		state.setText(state.usernameEdit, state.profiles[0].Username)
	}
	state.refreshAgentStatus()
}

func (state *windowsSettingsUI) handleCommand(id, notify uint16) {
	switch id {
	case settingsIDConnect:
		if notify == settingsBNClicked {
			state.beginConnect()
		}
	case settingsIDBrowse:
		if notify == settingsBNClicked {
			state.browseSource()
		}
	case settingsIDSaveProfile:
		if notify == settingsBNClicked {
			state.saveProfile()
		}
	case settingsIDNewProfile:
		if notify == settingsBNClicked {
			state.clearProfile()
		}
	case settingsIDEnableProfile:
		if notify == settingsBNClicked {
			state.setSelectedProfileEnabled(true)
		}
	case settingsIDDisableProfile:
		if notify == settingsBNClicked {
			state.setSelectedProfileEnabled(false)
		}
	case settingsIDDeleteProfile:
		if notify == settingsBNClicked {
			state.deleteSelectedProfile()
		}
	case settingsIDRefresh:
		if notify == settingsBNClicked {
			_ = state.reloadProfiles(state.selectedProfileID)
			state.refreshAgentStatus()
			state.setStatus("Refreshed.")
		}
	case settingsIDProfileList:
		if notify == settingsLBNSelect {
			state.selectProfileFromList()
		}
	case settingsIDSyncNow:
		if notify == settingsBNClicked {
			state.syncNow()
		}
	case settingsIDAgentEnable:
		if notify == settingsBNClicked {
			state.enableAgent()
		}
	case settingsIDAgentDisable:
		if notify == settingsBNClicked {
			state.disableAgent()
		}
	case settingsIDFolderCombo:
		_ = notify == settingsCBNSelect
	}
}

func (state *windowsSettingsUI) beginConnect() {
	server := strings.TrimSpace(state.text(state.serverEdit))
	user := strings.TrimSpace(state.text(state.usernameEdit))
	password := state.text(state.passwordEdit)
	if server == "" || user == "" {
		state.showError("Server URL and username are required.")
		return
	}
	state.setStatus("Connecting to Home-AI...")
	go func() {
		result := &settingsConnectResult{}
		canonicalServer, err := windowsclient.NormalizeServerURL(server)
		if err != nil {
			result.err = err
			state.postConnectResult(result)
			return
		}
		client, err := windowsclient.New(canonicalServer)
		if err != nil {
			result.err = err
			state.postConnectResult(result)
			return
		}
		result.server = canonicalServer
		result.user = user
		resolvedPassword := password
		if resolvedPassword == "" {
			resolvedPassword, err = windowsclient.LoadPassword(result.server, result.user)
			if err != nil {
				result.err = fmt.Errorf("load stored credential: %w", err)
				state.postConnectResult(result)
				return
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := client.Login(ctx, result.user, resolvedPassword); err != nil {
			result.err = fmt.Errorf("login: %w", err)
			state.postConnectResult(result)
			return
		}
		folders, err := client.Folders(ctx)
		if err != nil {
			result.err = fmt.Errorf("list Home-AI folders: %w", err)
			state.postConnectResult(result)
			return
		}
		if password != "" {
			if err := windowsclient.SavePassword(result.server, result.user, password); err != nil {
				result.err = fmt.Errorf("save Windows credential: %w", err)
				state.postConnectResult(result)
				return
			}
		}
		if err := windowsclient.SaveClientSettings(state.settingsPath, windowsclient.ClientSettings{
			Version:   1,
			ServerURL: result.server,
			Username:  result.user,
		}); err != nil {
			result.err = err
			state.postConnectResult(result)
			return
		}
		result.folders = folders
		state.postConnectResult(result)
	}()
}

func (state *windowsSettingsUI) postConnectResult(result *settingsConnectResult) {
	state.asyncMu.Lock()
	state.connectResult = result
	state.asyncMu.Unlock()
	procSettingsPostMessage.Call(uintptr(state.hwnd), settingsWMConnected, 0, 0)
}

func (state *windowsSettingsUI) finishConnect() {
	state.asyncMu.Lock()
	result := state.connectResult
	state.connectResult = nil
	state.asyncMu.Unlock()
	if result == nil {
		return
	}
	if result.err != nil {
		state.setStatus("Connection failed: " + result.err.Error())
		state.showError(result.err.Error())
		return
	}
	state.setText(state.serverEdit, result.server)
	state.setText(state.usernameEdit, result.user)
	state.setText(state.passwordEdit, "")
	state.folders = append([]windowsclient.Folder(nil), result.folders...)
	preferred := state.currentFolderID()
	state.populateFolders(preferred)
	writable := 0
	for _, folder := range result.folders {
		if folder.CanWrite {
			writable++
		}
	}
	state.setStatus(fmt.Sprintf("Connected. %d writable folder(s) available.", writable))
}

func (state *windowsSettingsUI) saveProfile() {
	server := strings.TrimSpace(state.text(state.serverEdit))
	user := strings.TrimSpace(state.text(state.usernameEdit))
	source := strings.TrimSpace(state.text(state.sourceEdit))
	destination := strings.TrimSpace(state.text(state.destination))
	folderID := state.currentFolderID()
	every, err := time.ParseDuration(strings.TrimSpace(state.text(state.interval)))
	if err != nil {
		state.showError("Invalid interval. Use values such as 15m, 1h or 24h.")
		return
	}
	if server == "" || user == "" || folderID == "" || source == "" {
		state.showError("Server, user, remote folder and local source are required.")
		return
	}
	input := windowsclient.SyncProfileInput{
		ServerURL:      server,
		Username:       user,
		FolderID:       folderID,
		Source:         source,
		Destination:    destination,
		Every:          every,
		ConflictPolicy: state.currentConflictPolicy(),
	}
	var profile windowsclient.SyncProfile
	if state.selectedProfileID == "" {
		profile, err = windowsclient.AddSyncProfile(state.configPath, input)
	} else {
		profile, err = windowsclient.UpdateSyncProfile(state.configPath, state.selectedProfileID, input)
	}
	if err != nil {
		state.showError(err.Error())
		return
	}
	_ = windowsclient.SaveClientSettings(state.settingsPath, windowsclient.ClientSettings{
		Version:   1,
		ServerURL: profile.ServerURL,
		Username:  profile.Username,
	})
	if err := state.reloadProfiles(profile.ID); err != nil {
		state.showError(err.Error())
		return
	}
	state.setStatus("Sync profile saved.")
}

func (state *windowsSettingsUI) reloadProfiles(selectID string) error {
	profiles, err := windowsclient.LoadSyncProfiles(state.configPath)
	if err != nil {
		return err
	}
	state.profiles = profiles
	procSettingsSendMessage.Call(uintptr(state.profileList), settingsLBReset, 0, 0)
	selectedIndex := -1
	for i, profile := range profiles {
		enabled := "OFF"
		if profile.Enabled {
			enabled = "ON"
		}
		last := "never"
		if profile.LastAttemptAt != nil {
			if profile.LastError == "" {
				last = "OK"
			} else {
				last = "FAILED"
			}
		}
		item := fmt.Sprintf("[%s] %s -> %s | %s | %s", enabled, filepath.Base(profile.Source), profile.Destination, profile.Interval(), last)
		state.listAdd(state.profileList, item)
		if profile.ID == selectID {
			selectedIndex = i
		}
	}
	if selectedIndex >= 0 {
		procSettingsSendMessage.Call(uintptr(state.profileList), settingsLBSetCurSel, uintptr(selectedIndex), 0)
		state.applyProfile(profiles[selectedIndex])
	} else if selectID == "" {
		state.selectedProfileID = ""
		state.setText(state.saveProfileBtn, "Add profile")
	}
	return nil
}

func (state *windowsSettingsUI) selectProfileFromList() {
	index := state.listSelection(state.profileList)
	if index < 0 || index >= len(state.profiles) {
		return
	}
	state.applyProfile(state.profiles[index])
}

func (state *windowsSettingsUI) applyProfile(profile windowsclient.SyncProfile) {
	state.selectedProfileID = profile.ID
	state.setText(state.serverEdit, profile.ServerURL)
	state.setText(state.usernameEdit, profile.Username)
	state.setText(state.sourceEdit, profile.Source)
	state.setText(state.destination, profile.Destination)
	state.setText(state.interval, profile.Interval().String())
	state.setConflictPolicy(profile.ConflictPolicy)
	state.populateFolders(profile.FolderID)
	state.setText(state.saveProfileBtn, "Update profile")
	state.setStatus("Editing profile " + profile.ID)
}

func (state *windowsSettingsUI) clearProfile() {
	state.selectedProfileID = ""
	procSettingsSendMessage.Call(uintptr(state.profileList), settingsLBSetCurSel, settingsLBErr, 0)
	state.setText(state.sourceEdit, "")
	state.setText(state.destination, "")
	state.setText(state.interval, "15m")
	state.setConflictPolicy(windowsclient.SyncConflictStop)
	state.setText(state.saveProfileBtn, "Add profile")
	state.setStatus("New profile.")
}

func (state *windowsSettingsUI) setSelectedProfileEnabled(enabled bool) {
	if state.selectedProfileID == "" {
		state.showError("Select a sync profile first.")
		return
	}
	if err := windowsclient.SetSyncProfileEnabled(state.configPath, state.selectedProfileID, enabled); err != nil {
		state.showError(err.Error())
		return
	}
	if err := state.reloadProfiles(state.selectedProfileID); err != nil {
		state.showError(err.Error())
		return
	}
	if enabled {
		state.setStatus("Profile enabled.")
	} else {
		state.setStatus("Profile disabled.")
	}
	state.refreshAgentStatus()
}

func (state *windowsSettingsUI) deleteSelectedProfile() {
	if state.selectedProfileID == "" {
		state.showError("Select a sync profile first.")
		return
	}
	if !state.confirm("Delete the selected sync profile?") {
		return
	}
	if err := windowsclient.RemoveSyncProfile(state.configPath, state.selectedProfileID); err != nil {
		state.showError(err.Error())
		return
	}
	state.selectedProfileID = ""
	state.clearProfile()
	if err := state.reloadProfiles(""); err != nil {
		state.showError(err.Error())
		return
	}
	state.setStatus("Profile deleted.")
	state.refreshAgentStatus()
}

func (state *windowsSettingsUI) populateFolders(preferred string) {
	procSettingsSendMessage.Call(uintptr(state.folderCombo), settingsCBReset, 0, 0)
	state.folderIDs = state.folderIDs[:0]
	selected := -1
	for _, folder := range state.folders {
		if !folder.CanWrite {
			continue
		}
		label := folder.Name
		if folder.PoolName != "" {
			label += " (" + folder.PoolName + ")"
		}
		label += " — " + folder.ID
		state.comboAdd(state.folderCombo, label)
		state.folderIDs = append(state.folderIDs, folder.ID)
		if folder.ID == preferred {
			selected = len(state.folderIDs) - 1
		}
	}
	if preferred != "" && selected < 0 {
		state.comboAdd(state.folderCombo, "Saved folder — "+preferred)
		state.folderIDs = append(state.folderIDs, preferred)
		selected = len(state.folderIDs) - 1
	}
	if selected < 0 && len(state.folderIDs) > 0 {
		selected = 0
	}
	if selected >= 0 {
		procSettingsSendMessage.Call(uintptr(state.folderCombo), settingsCBSetCurSel, uintptr(selected), 0)
	}
}

func (state *windowsSettingsUI) currentFolderID() string {
	index := state.comboSelection(state.folderCombo)
	if index >= 0 && index < len(state.folderIDs) {
		return state.folderIDs[index]
	}
	if state.selectedProfileID != "" {
		for _, profile := range state.profiles {
			if profile.ID == state.selectedProfileID {
				return profile.FolderID
			}
		}
	}
	return ""
}

func (state *windowsSettingsUI) currentConflictPolicy() string {
	index := state.comboSelection(state.conflict)
	switch index {
	case 1:
		return windowsclient.SyncConflictSkip
	case 2:
		return windowsclient.SyncConflictReplaceToTrash
	default:
		return windowsclient.SyncConflictStop
	}
}

func (state *windowsSettingsUI) setConflictPolicy(policy string) {
	index := uintptr(0)
	switch policy {
	case windowsclient.SyncConflictSkip:
		index = 1
	case windowsclient.SyncConflictReplaceToTrash:
		index = 2
	}
	procSettingsSendMessage.Call(uintptr(state.conflict), settingsCBSetCurSel, index, 0)
}

func (state *windowsSettingsUI) syncNow() {
	sent, err := windowsclient.SignalUserAgentSyncNow()
	if err != nil {
		state.showError(err.Error())
		return
	}
	if sent {
		state.setStatus("Sync requested through the running Home-AI agent.")
		return
	}
	if err := validateAgentReady(state.configPath); err != nil {
		state.showError(err.Error())
		return
	}
	state.setStatus("Syncing enabled profiles...")
	go func() {
		count, err := executeSyncProfiles(context.Background(), state.configPath, "", false, 3, false)
		result := &settingsAsyncResult{
			message: fmt.Sprintf("Manual sync finished. %d profile(s) processed.", count),
			err:     err,
		}
		state.asyncMu.Lock()
		state.syncResult = result
		state.asyncMu.Unlock()
		procSettingsPostMessage.Call(uintptr(state.hwnd), settingsWMSyncDone, 0, 0)
	}()
}

func (state *windowsSettingsUI) finishSync() {
	state.asyncMu.Lock()
	result := state.syncResult
	state.syncResult = nil
	state.asyncMu.Unlock()
	if result == nil {
		return
	}
	_ = state.reloadProfiles(state.selectedProfileID)
	if result.err != nil {
		state.setStatus("Sync failed: " + result.err.Error())
		state.showError(result.err.Error())
		return
	}
	state.setStatus(result.message)
}

func (state *windowsSettingsUI) enableAgent() {
	state.setStatus("Enabling Home-AI background agent...")
	go func() {
		result := &settingsAsyncResult{}
		if err := validateAgentReady(state.configPath); err != nil {
			result.err = err
			state.postAgentResult(result)
			return
		}
		executable, err := os.Executable()
		if err != nil {
			result.err = err
			state.postAgentResult(result)
			return
		}
		installed, err := windowsclient.InstallUserClientWithHandoff(executable, 15*time.Second)
		if err != nil {
			result.err = err
			state.postAgentResult(result)
			return
		}
		if err := windowsclient.InstallUserAgentAutostart(installed.Path, state.configPath); err != nil {
			result.err = err
			state.postAgentResult(result)
			return
		}
		if !windowsclient.UserAgentRunning() {
			if err := windowsclient.StartUserAgent(installed.Path, state.configPath); err != nil {
				result.err = err
				state.postAgentResult(result)
				return
			}
		}
		result.message = "Background agent enabled and started."
		state.postAgentResult(result)
	}()
}

func (state *windowsSettingsUI) disableAgent() {
	state.setStatus("Disabling Home-AI background agent...")
	go func() {
		result := &settingsAsyncResult{}
		if err := windowsclient.RemoveUserAgentAutostart(); err != nil {
			result.err = err
			state.postAgentResult(result)
			return
		}
		_, err := windowsclient.RequestUserAgentExit()
		if err != nil {
			result.err = err
			state.postAgentResult(result)
			return
		}
		result.message = "Background agent autostart disabled."
		state.postAgentResult(result)
	}()
}

func (state *windowsSettingsUI) postAgentResult(result *settingsAsyncResult) {
	state.asyncMu.Lock()
	state.agentResult = result
	state.asyncMu.Unlock()
	procSettingsPostMessage.Call(uintptr(state.hwnd), settingsWMAgentDone, 0, 0)
}

func (state *windowsSettingsUI) finishAgent() {
	state.asyncMu.Lock()
	result := state.agentResult
	state.agentResult = nil
	state.asyncMu.Unlock()
	state.refreshAgentStatus()
	if result == nil {
		return
	}
	if result.err != nil {
		state.setStatus("Agent error: " + result.err.Error())
		state.showError(result.err.Error())
		return
	}
	state.setStatus(result.message)
}

func (state *windowsSettingsUI) refreshAgentStatus() {
	info, err := windowsclient.UserAgentAutostartStatus()
	if err != nil {
		state.setText(state.agentLabel, "Autostart status error: "+err.Error())
		return
	}
	autostart := "OFF"
	if info.Enabled {
		autostart = "ON"
	}
	running := "stopped"
	if windowsclient.UserAgentRunning() {
		running = "running"
	}
	state.setText(state.agentLabel, "Autostart: "+autostart+"  |  Agent: "+running)
}

func (state *windowsSettingsUI) browseSource() {
	title, _ := windows.UTF16PtrFromString("Select local folder to sync")
	var display [260]uint16
	info := settingsBrowseInfo{
		Owner:       state.hwnd,
		DisplayName: &display[0],
		Title:       title,
		Flags:       0x0001 | 0x0040,
	}
	pidl, _, _ := procSettingsBrowseFolder.Call(uintptr(unsafe.Pointer(&info)))
	if pidl == 0 {
		return
	}
	defer procSettingsTaskMemFree.Call(pidl)
	var path [260]uint16
	ok, _, _ := procSettingsGetPath.Call(pidl, uintptr(unsafe.Pointer(&path[0])))
	if ok == 0 {
		state.showError("Windows did not return a filesystem path for the selected folder.")
		return
	}
	state.setText(state.sourceEdit, windows.UTF16ToString(path[:]))
}

func (state *windowsSettingsUI) setStatus(text string) {
	state.setText(state.statusLabel, text)
}

func (state *windowsSettingsUI) showError(text string) {
	state.messageBox(text, settingsMBOK|settingsMBIconError)
}

func (state *windowsSettingsUI) confirm(text string) bool {
	return state.messageBox(text, settingsMBYesNo|settingsMBIconWarning) == settingsIDYes
}

func (state *windowsSettingsUI) messageBox(text string, flags uint32) uintptr {
	body, _ := windows.UTF16PtrFromString(text)
	title, _ := windows.UTF16PtrFromString(settingsWindowTitle)
	result, _, _ := procSettingsMessageBox.Call(
		uintptr(state.hwnd),
		uintptr(unsafe.Pointer(body)),
		uintptr(unsafe.Pointer(title)),
		uintptr(flags),
	)
	return result
}

func (state *windowsSettingsUI) text(hwnd windows.Handle) string {
	length, _, _ := procSettingsGetTextLength.Call(uintptr(hwnd))
	if length == 0 {
		return ""
	}
	buffer := make([]uint16, int(length)+1)
	procSettingsGetWindowText.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&buffer[0])), length+1)
	return windows.UTF16ToString(buffer)
}

func (state *windowsSettingsUI) setText(hwnd windows.Handle, value string) {
	ptr, err := windows.UTF16PtrFromString(value)
	if err != nil {
		return
	}
	procSettingsSetWindowText.Call(uintptr(hwnd), uintptr(unsafe.Pointer(ptr)))
}

func (state *windowsSettingsUI) comboAdd(hwnd windows.Handle, value string) {
	ptr, err := windows.UTF16PtrFromString(value)
	if err != nil {
		return
	}
	procSettingsSendMessage.Call(uintptr(hwnd), settingsCBAddString, 0, uintptr(unsafe.Pointer(ptr)))
}

func (state *windowsSettingsUI) listAdd(hwnd windows.Handle, value string) {
	ptr, err := windows.UTF16PtrFromString(value)
	if err != nil {
		return
	}
	procSettingsSendMessage.Call(uintptr(hwnd), settingsLBAddString, 0, uintptr(unsafe.Pointer(ptr)))
}

func (state *windowsSettingsUI) comboSelection(hwnd windows.Handle) int {
	index, _, _ := procSettingsSendMessage.Call(uintptr(hwnd), settingsCBGetCurSel, 0, 0)
	if index == settingsCBErr {
		return -1
	}
	return int(index)
}

func (state *windowsSettingsUI) listSelection(hwnd windows.Handle) int {
	index, _, _ := procSettingsSendMessage.Call(uintptr(hwnd), settingsLBGetCurSel, 0, 0)
	if index == settingsLBErr {
		return -1
	}
	return int(index)
}

func (state *windowsSettingsUI) setControlEnabled(hwnd windows.Handle, enabled bool) {
	value := uintptr(0)
	if enabled {
		value = 1
	}
	procSettingsEnableWindow.Call(uintptr(hwnd), value)
}

func parsePositiveInt(value string) (int, bool) {
	number, err := strconv.Atoi(strings.TrimSpace(value))
	return number, err == nil && number > 0
}
