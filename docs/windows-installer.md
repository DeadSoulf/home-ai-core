# HOME AI Windows installer

HOME AI now ships a normal Windows setup wizard in addition to the portable
Windows-client executable. The setup is for **Windows 10/11 x64** and installs
for the currently signed-in Windows user, with no administrator rights.

## Installation and upgrade

Download HOMEAI-Setup_<version>_amd64.exe from the official
[GitHub releases](https://github.com/DeadSoulf/home-ai-core/releases).
Verify the SHA-256 file against authenticated release metadata before running.
The Windows executable is currently **not Authenticode-signed**; Windows
SmartScreen may warn. Do not disable security software system-wide.

The NSIS Modern UI wizard supports Russian and English, creates a Start Menu
HOME AI shortcut and an entry under Windows Settings > Installed apps.
The wizard also offers an optional desktop shortcut and a final Launch HOME AI
checkbox. The install directory is deliberately fixed to the existing
per-user location:

    %LOCALAPPDATA%\HomeAI\bin\home-ai-windows-client.exe

This is the same directory used by the built-in client update mechanism,
so launching Setup over an existing installation preserves the user's data
and configured agent. A running Settings window must be closed before the
installer can replace its executable; Setup prompts instead of silently
killing it. A running tray agent is stopped gracefully by the existing
Go install handoff and restarted when its autostart registration is enabled.

Setup does **not** turn on synchronization or create credentials on its own.
Once the user has configured a profile, enabling the background agent is
handled through HOME AI Settings as before.

## Uninstall and data

To uninstall, use Windows Settings > Installed apps > HOME AI Windows Client
> Uninstall, or the Uninstall HOME AI shortcut in Start.

The uninstaller removes application binaries, Start Menu/desktop shortcuts
and only HOME AI's own sync-agent autostart registry value. It does not
delete any of the following:

- Sync profiles, preferences and pinned TLS identities under
  %APPDATA%\HomeAI
- Windows Credential Manager secrets
- Source folders and files synchronized from the Windows computer
- Files, accounts or NAS storage on the Home AI Core server

The app may be reinstalled later with the same settings and profile data.

## Developer build

On an Ubuntu runner with NSIS installed, use the existing release build
to generate the embedded Windows manifest and version-stamped EXE, then run:

    sudo apt-get install nsis
    sh scripts/build-windows-setup.sh \
      dist/home-ai-windows-client_0.1.209-dev_amd64.exe \
      dist/HOMEAI-Setup_0.1.209-dev_amd64.exe

The VERSION file determines the actual current version; the above name
is only an example. The build script refuses a missing/unversioned input
or a missing makensis executable.

The release workflow publishes both the portable client and normal Setup
and signs their SHA-256 metadata through the existing Ed25519 release
signing process. Windows CI smoke tests execute the setup silently,
check shortcuts and uninstall metadata, reinstall over the existing copy,
and verify that uninstall leaves the user's test sync profile untouched.

The in-app updater deliberately downloads the versioned standalone EXE;
once installed via Setup, later updates use that same stable install
directory and refresh Installed apps version metadata.
