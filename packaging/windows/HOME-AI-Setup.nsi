; HOME AI Windows client — per-user installer, no administrator rights required.
; Input: /DVERSION=... /DCLIENT_EXE=... /DOUTPUT_DIR=...
Unicode true
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "FileFunc.nsh"

!ifndef VERSION
  !error "VERSION is required"
!endif
!ifndef CLIENT_EXE
  !error "CLIENT_EXE is required"
!endif
!ifndef OUTPUT_DIR
  !error "OUTPUT_DIR is required"
!endif

!define PRODUCT_NAME "HOME AI"
!define UNINSTALL_KEY "Software\Microsoft\Windows\CurrentVersion\Uninstall\HomeAIWindowsClient"
!define CLIENT_NAME "home-ai-windows-client.exe"
!define MUI_ABORTWARNING
!define MUI_ICON "$%WINDIR%\system32\shell32.dll"

Name "${PRODUCT_NAME}"
OutFile "${OUTPUT_DIR}\HOME-AI-Setup_${VERSION}_x64.exe"
InstallDir "$LOCALAPPDATA\HomeAI\bin"
RequestExecutionLevel user
SetCompressor /SOLID lzma
ShowInstDetails show
ShowUninstDetails show
BrandingText "HOME AI | ${VERSION}"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_LICENSE "${NSISDIR}\Docs\Modern UI 2\Readme.txt"
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_RUN "$INSTDIR\${CLIENT_NAME}"
!define MUI_FINISHPAGE_RUN_PARAMETERS "settings"
!define MUI_FINISHPAGE_RUN_TEXT "Open HOME AI"
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "Russian"
!insertmacro MUI_LANGUAGE "English"

Function .onInit
  SetShellVarContext current
  StrCpy $INSTDIR "$LOCALAPPDATA\HomeAI\bin"
FunctionEnd

Section "HOME AI" SecCore
  SectionIn RO
  SetOutPath "$PLUGINSDIR"
  InitPluginsDir
  File "/oname=home-ai-windows-client.exe" "${CLIENT_EXE}"

  ; Use the existing verified, atomic per-user Go installer rather than
  ; overwriting a running client EXE directly from NSIS.
  DetailPrint "Installing HOME AI (preserving synchronization profiles)..."
  ExecWait '"$PLUGINSDIR\${CLIENT_NAME}" client install --reopen-settings=false' $0
  ${If} $0 != 0
    MessageBox MB_ICONSTOP|MB_OK "Could not install HOME AI (exit code $0). Close the client and its tray agent, then retry. No sync profiles were deleted."
    Abort
  ${EndIf}

  SetOutPath "$INSTDIR"
  WriteUninstaller "$INSTDIR\Uninstall HOME AI.exe"

  CreateDirectory "$SMPROGRAMS\HOME AI"
  CreateShortcut "$SMPROGRAMS\HOME AI\HOME AI.lnk" "$INSTDIR\${CLIENT_NAME}" "settings"
  CreateShortcut "$SMPROGRAMS\HOME AI\Uninstall HOME AI.lnk" "$INSTDIR\Uninstall HOME AI.exe"

  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayName" "HOME AI Windows Client"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "Publisher" "HOME AI"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "DisplayIcon" "$INSTDIR\${CLIENT_NAME}"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTALL_KEY}" "UninstallString" '"$INSTDIR\Uninstall HOME AI.exe"'
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINSTALL_KEY}" "NoRepair" 1
SectionEnd

Section /o "Desktop shortcut" SecDesktop
  CreateShortcut "$DESKTOP\HOME AI.lnk" "$INSTDIR\${CLIENT_NAME}" "settings"
SectionEnd

Function un.onInit
  SetShellVarContext current
  MessageBox MB_ICONEXCLAMATION|MB_YESNO "Uninstall HOME AI Windows Client? Saved synchronization profiles and credentials are kept." IDYES +2
  Abort
FunctionEnd

Section "Uninstall"
  ; Do not remove profile JSON, Windows Credential Manager entries, transfer
  ; history or other user data under %APPDATA%\HomeAI.
  ; Remove autostart only for this client's own registration.
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "HomeAIWindowsSyncAgent"

  Delete "$SMPROGRAMS\HOME AI\HOME AI.lnk"
  Delete "$SMPROGRAMS\HOME AI\Uninstall HOME AI.lnk"
  RMDir "$SMPROGRAMS\HOME AI"
  Delete "$DESKTOP\HOME AI.lnk"

  Delete "$INSTDIR\${CLIENT_NAME}"
  ${If} ${FileExists} "$INSTDIR\${CLIENT_NAME}"
    MessageBox MB_ICONEXCLAMATION|MB_OK "HOME AI is still running. Exit the tray application and run uninstall again to remove its executable."
    Abort
  ${EndIf}
  Delete "$INSTDIR\Uninstall HOME AI.exe"
  RMDir "$INSTDIR"
  DeleteRegKey HKCU "${UNINSTALL_KEY}"
SectionEnd
