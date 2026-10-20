; Home AI Windows Client — per-user wizard/uninstaller (NSIS Modern UI 2).
; The Go client installer atomically replaces the exe and handles the tray.
; Never delete %APPDATA%\HomeAI, local NAS folders or Windows credentials.

Unicode true
!include "MUI2.nsh"
!include "LogicLib.nsh"
!include "WinMessages.nsh"

!ifndef VERSION
  !error "Set -DVERSION=<version>"
!endif
!ifndef CLIENT_EXE
  !error "Set -DCLIENT_EXE=<absolute-path-to-versioned-exe>"
!endif
!ifndef OUTFILE
  !error "Set -DOUTFILE=<absolute-path-to-setup.exe>"
!endif

!define APP_NAME "HOME AI"
!define APP_EXE "home-ai-windows-client.exe"
!define APP_UNINSTALL "Uninstall HOME AI.exe"
!define APP_REG "Software\HomeAI\WindowsClient"
!define UNINSTALL_REG "Software\Microsoft\Windows\CurrentVersion\Uninstall\HomeAIWindowsClient"
!define START_MENU "$SMPROGRAMS\HOME AI"
!define DESKTOP_SHORTCUT "$DESKTOP\HOME AI.lnk"

Name "${APP_NAME} Windows Client ${VERSION}"
OutFile "${OUTFILE}"
InstallDir "$LOCALAPPDATA\HomeAI\bin"
RequestExecutionLevel user
SetCompressor /SOLID lzma
BrandingText "HOME AI"
ShowInstDetails show
ShowUninstDetails show

!define MUI_ABORTWARNING
!define MUI_LANGDLL_REGISTRY_ROOT "HKCU"
!define MUI_LANGDLL_REGISTRY_KEY "${APP_REG}"
!define MUI_LANGDLL_REGISTRY_VALUENAME "InstallerLanguage"
!define MUI_FINISHPAGE_RUN "$INSTDIR${APP_EXE}"
!define MUI_FINISHPAGE_RUN_PARAMETERS "settings"
!define MUI_FINISHPAGE_RUN_TEXT "$(LaunchAfterSetup)"

!insertmacro MUI_PAGE_WELCOME
!insertmacro MUI_PAGE_COMPONENTS
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH

!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES

!insertmacro MUI_LANGUAGE "Russian"
!insertmacro MUI_LANGUAGE "English"

LangString AppComponent ${LANG_RUSSIAN} "Клиент HOME AI"
LangString AppComponent ${LANG_ENGLISH} "HOME AI client"
LangString DesktopComponent ${LANG_RUSSIAN} "Ярлык на рабочем столе"
LangString DesktopComponent ${LANG_ENGLISH} "Desktop shortcut"
LangString LaunchAfterSetup ${LANG_RUSSIAN} "Запустить HOME AI"
LangString LaunchAfterSetup ${LANG_ENGLISH} "Launch HOME AI"
LangString CloseApp ${LANG_RUSSIAN} "Закройте окно HOME AI, затем нажмите «Повторить». Для завершения работы фонового агента выберите «Выход» в трее.$\r$\n$\r$\nНастройки синхронизации и файлы не будут удалены."
LangString CloseApp ${LANG_ENGLISH} "Close the HOME AI settings window and click Retry. To exit the background agent, use Exit in its tray menu.$\r$\n$\r$\nSync profiles and files will not be deleted."
LangString InstallFailed ${LANG_RUSSIAN} "Не удалось установить HOME AI. Предыдущая версия и её данные сохранены. Код:"
LangString InstallFailed ${LANG_ENGLISH} "Could not install HOME AI. The previous app and data were preserved. Exit code:"
LangString AgentStopText ${LANG_RUSSIAN} "Завершение работы фонового агента HOME AI..."
LangString AgentStopText ${LANG_ENGLISH} "Stopping the HOME AI background agent..."
LangString KeepUserData ${LANG_RUSSIAN} "Настройки, пароли и синхронизируемые файлы сохраняются."
LangString KeepUserData ${LANG_ENGLISH} "Settings, credentials and synchronized files are preserved."

Function .onInit
  !insertmacro MUI_LANGDLL_DISPLAY
FunctionEnd

Function un.onInit
  !insertmacro MUI_UNGETLANGUAGE
FunctionEnd

; An open or tray-hidden Settings window would hold the executable and could
; have unsaved edits. Check it before either replacing or uninstalling.
!macro DEFINE_WAIT_FOR_SETTINGS PREFIX
Function ${PREFIX}WaitForSettingsExit
check:
  FindWindow $0 "HomeAIWindowsSettingsWindow" ""
  StrCmp $0 0 done
  IfSilent silent
  MessageBox MB_RETRYCANCEL|MB_ICONEXCLAMATION "$(CloseApp)" IDRETRY check
silent:
  SetErrorLevel 2
  Abort
done:
FunctionEnd
!macroend

!insertmacro DEFINE_WAIT_FOR_SETTINGS ""
!insertmacro DEFINE_WAIT_FOR_SETTINGS "un."

Section "$(AppComponent)" SecMain
  SectionIn RO
  Call WaitForSettingsExit

  ; The stable installed file is managed by the existing Go installer,
  ; which verifies its copy and waits for a running agent to exit.
  InitPluginsDir
  SetOutPath "$PLUGINSDIR"
  File "/oname=home-ai-client-payload.exe" "${CLIENT_EXE}"
  DetailPrint "Installing HOME AI ${VERSION}..."
  nsExec::ExecToStack '"$PLUGINSDIR\home-ai-client-payload.exe" client install --reopen-settings=false'
  Pop $0
  Pop $1
  StrCmp $0 "0" installed
  DetailPrint "$1"
  MessageBox MB_OK|MB_ICONSTOP "$(InstallFailed) $0$\r$\n$1"
  SetErrorLevel 3
  Abort

installed:
  SetOutPath "$INSTDIR"
  WriteUninstaller "$INSTDIR${APP_UNINSTALL}"
  CreateDirectory "${START_MENU}"
  CreateShortcut "${START_MENU}\HOME AI.lnk" "$INSTDIR${APP_EXE}" "settings" "$INSTDIR${APP_EXE}" 0
  CreateShortcut "${START_MENU}\Uninstall HOME AI.lnk" "$INSTDIR${APP_UNINSTALL}"

  WriteRegStr HKCU "${APP_REG}" "InstallDir" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTALL_REG}" "DisplayName" "HOME AI Windows Client"
  WriteRegStr HKCU "${UNINSTALL_REG}" "DisplayVersion" "${VERSION}"
  WriteRegStr HKCU "${UNINSTALL_REG}" "Publisher" "Home AI Core"
  WriteRegStr HKCU "${UNINSTALL_REG}" "InstallLocation" "$INSTDIR"
  WriteRegStr HKCU "${UNINSTALL_REG}" "DisplayIcon" "$INSTDIR${APP_EXE},0"
  WriteRegStr HKCU "${UNINSTALL_REG}" "UninstallString" '"$INSTDIR${APP_UNINSTALL}"'
  WriteRegStr HKCU "${UNINSTALL_REG}" "QuietUninstallString" '"$INSTDIR${APP_UNINSTALL}" /S'
  WriteRegStr HKCU "${UNINSTALL_REG}" "URLInfoAbout" "https://github.com/DeadSoulf/home-ai-core"
  WriteRegDWORD HKCU "${UNINSTALL_REG}" "NoModify" 1
  WriteRegDWORD HKCU "${UNINSTALL_REG}" "NoRepair" 1
SectionEnd

Section /o "$(DesktopComponent)" SecDesktop
  CreateShortcut "${DESKTOP_SHORTCUT}" "$INSTDIR${APP_EXE}" "settings" "$INSTDIR${APP_EXE}" 0
SectionEnd

Section "Uninstall"
  Call un.WaitForSettingsExit
  DetailPrint "$(AgentStopText)"

  ; Unregister only HOME AI's Run entry. Ask its tray to exit gracefully.
  FindWindow $0 "HomeAIWindowsSyncTray" "Home-AI Sync Agent"
  StrCmp $0 0 +2
  SendMessage $0 ${WM_CLOSE} 0 0
  Sleep 600
  DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "HomeAIWindowsSyncAgent"

  Delete "${START_MENU}\HOME AI.lnk"
  Delete "${START_MENU}\Uninstall HOME AI.lnk"
  RMDir "${START_MENU}"
  Delete "${DESKTOP_SHORTCUT}"

  ; Existing sync profiles, client settings, credentials, TLS pins, local and
  ; remote files are retained by design. Only binaries and shortcuts removed.
  Delete /REBOOTOK "$INSTDIR${APP_EXE}"
  Delete /REBOOTOK "$INSTDIR${APP_UNINSTALL}"
  RMDir /REBOOTOK "$INSTDIR"
  DeleteRegKey HKCU "${UNINSTALL_REG}"
  DeleteRegValue HKCU "${APP_REG}" "InstallDir"
  DetailPrint "$(KeepUserData)"
SectionEnd
