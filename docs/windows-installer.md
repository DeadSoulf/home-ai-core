# Windows HOME AI installer

Windows users can install Home AI Core's file-synchronization client from a
regular, per-user installer:

`HOME-AI-Setup_<VERSION>_x64.exe`

Download it from the matching official GitHub release. The installation
wizard does **not** require administrator privileges. It installs the client
to `%LOCALAPPDATA%\HomeAI\bin\home-ai-windows-client.exe`, adds a HOME AI
Start Menu shortcut and an uninstall entry to Windows **Installed apps**.
An optional component creates a desktop shortcut.

The wizard uses the same verified internal `client install` handoff as the
existing portable Windows client. It replaces the installed executable
without changing the user's sync profiles, certificate pins, queue, file
history or Windows Credential Manager entries. If another client process
prevents replacement, close the running HOME AI window and tray agent and
retry setup; do not delete the configuration directory.

Uninstall from Windows **Settings → Apps → Installed apps → HOME AI Windows
Client**, or from the HOME AI entry in the Start Menu. The uninstaller removes
program files, shortcuts and the associated agent autostart registration,
but intentionally **keeps all user data** under `%APPDATA%\HomeAI`,
`%LOCALAPPDATA%\HomeAI` (except installed program files), and Windows
Credential Manager. To delete those data permanently, do it separately only
after confirming that they are no longer needed.

The installer is packaged together with the portable client EXE, signed
SHA-256 checksums and detached Ed25519 release metadata. Ed25519 metadata
authenticates official release artifacts when the public key has been
independently verified. The installer is not currently Authenticode signed,
so Windows may show an unknown publisher warning. Never ignore warnings for
an installer whose origin or release checksum you have not verified.

The in-app updater continues to use the portable EXE for updates. For a
fully updated Windows **Installed apps** version label, run the newest setup
wizard from the release as well.
