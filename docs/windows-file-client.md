# Windows file-copy client

The first Home-AI Windows client is a command-line file-copy tool using the same resumable NAS upload API as the Web file manager.

Supported now:

- token-mode Home-AI login;
- listing accessible NAS folders;
- copying one local file to a selected Home-AI folder;
- resume after interruption;
- per-chunk SHA-256 and final whole-file SHA-256;
- retry for transient transfer failures.

## Download

Each Home-AI development release publishes two separate Windows client assets:

home-ai-windows-client_<version>_amd64.exe
home-ai-windows-client_<version>_amd64.exe.sha256

## PowerShell authentication

Set the password only for the current PowerShell process:

$env:HOME_AI_PASSWORD = 'your Home-AI password'

After use:

Remove-Item Env:HOME_AI_PASSWORD

The password is not accepted as a command-line argument and is not stored by the client.

## List folders

.\home-ai-windows-client_<version>_amd64.exe folders --server http://HOME_AI_SERVER:8080 --username alice

## Upload a file

.\home-ai-windows-client_<version>_amd64.exe upload --server http://HOME_AI_SERVER:8080 --username alice --folder nsf_xxxxxxxxxxxxxxxx --source C:\Users\Alice\Documents\archive.zip --dest Backups/archive.zip

Running the same command again after interruption resumes a matching unfinished upload. Use --restart-stale only when you explicitly want to cancel a different unfinished upload for the same destination.

## Current limitations

- no recursive folder copy;
- no persistent transfer queue;
- no scheduled/automatic sync;
- no GUI/tray client;
- no Windows Credential Manager yet.
