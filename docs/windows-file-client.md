# Windows file-copy client

The Home-AI Windows client is a native Windows sync application using the same resumable NAS upload API as the Web file manager. A Win32 settings window is the normal setup surface; the CLI remains available for diagnostics and automation.

Supported now:

- token-mode Home-AI login;
- listing accessible NAS folders;
- copying one local file to a selected Home-AI folder;
- recursive directory copying, including empty directories;
- a persistent local queue with explicit add, list, run and retry commands;
- resume after interruption;
- per-chunk SHA-256 and final whole-file SHA-256;
- retry for transient transfer failures.
- persistent scheduled push-sync profiles;
- explicit sync conflict policies: stop, skip or replace-to-trash.

## Download

Each Home-AI development release publishes two separate Windows client assets:

home-ai-windows-client_<version>_amd64.exe
home-ai-windows-client_<version>_amd64.exe.sha256

## Windows client language

The native Windows UI supports **Русский** and **English** in the same executable. On the first GUI launch, Russian Windows locales default to Russian; other Windows locales default to English. The language selector in the settings window applies immediately and the selection is saved for later GUI and tray sessions.

The saved language is non-secret metadata in `%APPDATA%\\HomeAI\\windows-client.json`. Passwords remain in Windows Credential Manager. The diagnostic command-line interface remains English.

## Windows settings UI

On Windows, launch the client without arguments to open the native settings window:

```powershell
.\home-ai-windows-client_<version>_amd64.exe
```

The settings window can connect to Home-AI, discover writable NAS folders, choose a local folder, create or edit sync profiles, select the interval and conflict policy, run sync manually, and enable or disable the background agent.

The last server URL and username are stored in `%APPDATA%\HomeAI\windows-client.json`. Passwords are never stored there: after a successful connection they are kept in Windows Credential Manager. Sync profiles remain in `%APPDATA%\HomeAI\sync-profiles.json` and contain no password or bearer token.

Once the background agent is enabled, the tray menu includes **Settings** and **Sync now**. Double-clicking the tray icon opens the settings window. If the agent is already running, GUI **Sync now** signals that existing scheduler instead of starting another one.

## PowerShell authentication

Set the password only for the current PowerShell process:

$env:HOME_AI_PASSWORD = 'your Home-AI password'

After use:

Remove-Item Env:HOME_AI_PASSWORD

The password is not accepted as a command-line argument and is not stored by the client.

On Windows, the client can instead store the password in Windows Credential Manager. `HOME_AI_PASSWORD` still takes precedence when it is present.

```powershell
$env:HOME_AI_PASSWORD = 'your Home-AI password'
.\home-ai-windows-client_<version>_amd64.exe credentials save --server http://HOME_AI_SERVER:8080 --username alice
Remove-Item Env:HOME_AI_PASSWORD

.\home-ai-windows-client_<version>_amd64.exe credentials status --server http://HOME_AI_SERVER:8080 --username alice
```

After that, `folders`, `copy`, `queue run`, `sync run` and `sync watch` can authenticate without an environment password. Remove the saved password with `credentials delete --server ... --username ...`. The secret is not copied into queue or sync-profile JSON.

## List folders

.\home-ai-windows-client_<version>_amd64.exe folders --server http://HOME_AI_SERVER:8080 --username alice

## Upload a file

.\home-ai-windows-client_<version>_amd64.exe upload --server http://HOME_AI_SERVER:8080 --username alice --folder nsf_xxxxxxxxxxxxxxxx --source C:\Users\Alice\Documents\archive.zip --dest Backups/archive.zip

Running the same command again after interruption resumes a matching unfinished upload. Use --restart-stale only when you explicitly want to cancel a different unfinished upload for the same destination.

## Copy a directory

```powershell
.\home-ai-windows-client_<version>_amd64.exe copy --server http://HOME_AI_SERVER:8080 --username alice --folder nsf_xxxxxxxxxxxxxxxx --source C:\Users\Alice\Documents --dest Backups/Documents
```

`copy` plans the whole source tree before transfer. It creates missing destination directories and preserves empty directories. Without `--dest`, the source file/directory name is used. Relative paths stay inside the selected Home-AI folder. Symlinks and special files are rejected; an existing different file is a conflict, not an overwrite.

This immediate command uses server resumable uploads. Use the queue below to retain the complete multi-file plan across client restarts.

## Persistent queue

Add work without logging in or transferring files:

```powershell
.\home-ai-windows-client_<version>_amd64.exe queue add --server http://HOME_AI_SERVER:8080 --username alice --folder nsf_xxxxxxxxxxxxxxxx --source C:\Users\Alice\Documents --dest Backups/Documents
.\home-ai-windows-client_<version>_amd64.exe queue list
```

Then set `HOME_AI_PASSWORD` as above and run pending transfers:

```powershell
.\home-ai-windows-client_<version>_amd64.exe queue run
Remove-Item Env:HOME_AI_PASSWORD
```

The default queue is `%APPDATA%\HomeAI\transfer-queue.json` on Windows. Every queue command accepts `--queue C:\Path\queue.json` for a different queue. A queue belongs to one server and username; use a separate file for another account or server. Passwords and bearer tokens are never stored in the queue. A new process authenticates again.

Only one process can open a queue at a time. The checkpoint is limited to 64 MiB; adding a plan that exceeds this limit fails without changing the saved queue. Split very large plans between queue files.

Ctrl+C or a canceled/timed-out transfer leaves interrupted work pending. Run `queue run` again to continue. Completed entries are retained and skipped; partially uploaded files resume from the server's verified offset. A process that was killed releases its OS lock; running entries become pending when the queue reopens.

Other failures stop the queue and remain visible in `queue list`. Retry the affected job explicitly:

```powershell
.\home-ai-windows-client_<version>_amd64.exe queue retry --job JOB_ID
.\home-ai-windows-client_<version>_amd64.exe queue run
```

The queued source snapshot includes size, modification time and SHA-256. A changed or missing source fails; retry keeps the original plan, so restore the original source or add a new job for the changed files. Retry preserves already completed entries. The queue does not rescan a directory to add files created after planning.

If the server already committed a file but the client lost the response, the client verifies the existing file's size and SHA-256 before recording success. This reads the remote file back and may take time for large files. A different existing file is never replaced. `--restart-stale` on `queue add` only authorizes replacement of a mismatched unfinished upload; it does not authorize replacing an existing file.

Content verification permits a long transfer while bytes keep arriving. The connection still has an inactivity limit and respects Ctrl+C.

## Scheduled push sync

Create a profile without logging in:

```powershell
.\home-ai-windows-client_<version>_amd64.exe sync add --server http://HOME_AI_SERVER:8080 --username alice --folder nsf_xxxxxxxxxxxxxxxx --source C:\Users\Alice\Documents --dest Backups\Documents --every 15m --conflict stop
.\home-ai-windows-client_<version>_amd64.exe sync list
```

Profiles are stored in `%APPDATA%\HomeAI\sync-profiles.json` by default and contain no password or bearer token.

Conflict policies are explicit:

- `stop` stops on a different existing destination;
- `skip` leaves that remote item unchanged and continues;
- `replace-to-trash` first moves the exact conflicting remote item to the Home-AI recycle bin, then copies the local version.

A conflicting parent/ancestor is never automatically replaced. Local deletions are not propagated to the server in this first sync mode.

Run one profile immediately:

```powershell
$env:HOME_AI_PASSWORD = 'your Home-AI password'
.\home-ai-windows-client_<version>_amd64.exe sync run --profile SYNC_PROFILE_ID
```

Run all enabled profiles that are due:

```powershell
.\home-ai-windows-client_<version>_amd64.exe sync run
```

Keep a foreground scheduler running:

```powershell
.\home-ai-windows-client_<version>_amd64.exe sync watch
```

The watcher reloads profile configuration periodically and respects each persisted interval. It does not persist credentials; the password remains only in the process environment. Use `sync disable`, `sync enable` or `sync remove` with `--profile ID` to manage profiles.

## Background sync agent

After saving Windows credentials and creating at least one enabled sync profile, enable per-user autostart:

```powershell
.\home-ai-windows-client_<version>_amd64.exe agent install
.\home-ai-windows-client_<version>_amd64.exe agent status
```

The agent is registered under the current user's Windows `Run` key and starts after that user logs on. The registry command contains only the executable path, `agent run`, and the sync-profile path; it contains no password or token.

`agent install` verifies that every distinct account used by enabled profiles has a Windows Credential Manager password. The background process reuses `sync watch`, runs only one instance per profile file, hides its console, and writes a rotating per-user log at `%LOCALAPPDATA%\HomeAI\sync-agent.log` on normal Windows installations.

Disable autostart with:

```powershell
.\home-ai-windows-client_<version>_amd64.exe agent remove
```

`agent install` first copies the client to the stable per-user path `%LOCALAPPDATA%\\HomeAI\\bin\\home-ai-windows-client.exe` and registers that installed copy for autostart. The original downloaded release asset can then be moved or deleted.

You can also install or refresh the per-user copy explicitly:

```powershell
.\\home-ai-windows-client_<version>_amd64.exe client install
.\\home-ai-windows-client_<version>_amd64.exe client status
```

When the background agent is running, the Windows notification-area icon provides **Sync now**, **Open log**, **Open sync profiles**, and **Exit**. **Sync now** signals the existing sequential scheduler rather than starting a second copy process.

When updating from a newer downloaded release, `client install` now handles a running tray agent automatically. If the installed executable is locked, the new client requests a clean tray-agent exit, retries the atomic replacement for up to 15 seconds, and restarts the agent when its managed autostart entry is enabled.

The tray status line shows the number of enabled profiles and the latest `OK` / `FAILED` result. The tooltip refreshes after sync cycles. Failed cycles show a Windows notification; successful background cycles stay silent, while manual **Sync now** reports completion.

This handoff installs an executable you already downloaded. Automatic release discovery/download is not part of this slice.

## Current limitations

- the first native settings UI focuses on connection, sync profiles and agent control; richer transfer history/progress is still deferred;
- Windows client release discovery/download remains manual; the install handoff itself is automatic;
- a conventional Windows installer / Start Menu / uninstall entry is still deferred;
- no bidirectional sync or local-delete propagation;
- no sync-history/recycle-bin retention policy;
- queue history pruning is still deferred.
