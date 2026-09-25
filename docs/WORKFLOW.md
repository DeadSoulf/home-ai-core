# Home AI Core — Development Workflow

## Branches

### main

Stable, tested versions only.

Do not develop directly on `main`.

### develop

Current integration branch.

New features and fixes are prepared here first, then tested on the Debian development server.

Future larger features may use temporary branches such as:

```text
feature/security-core
feature/video-core
feature/hypervisor-core
feature/device-core
```

They should merge into `develop` before reaching `main`.

## Server update workflow

The Debian development server should normally track `develop`.

```bash
cd /srv/home-ai-core

git fetch origin
git switch develop
git pull --ff-only origin develop

rm -rf build
cmake -S . -B build -G Ninja
cmake --build build
ctest --test-dir build --output-on-failure
```

If all tests pass, run the core manually for functional verification:

```bash
./build/home-ai-core
```

## Stable release workflow

After a version has been tested successfully on the development server:

1. Merge `develop` into `main`.
2. Rebuild and run tests from `main`.
3. Tag the stable version later when release tagging is introduced.

## Important repository rules

Never commit:

- passwords
- private keys
- API tokens
- session secrets
- personal documents
- video archives
- AI model files
- runtime databases containing personal information
- backup archives

Runtime/private data belongs under `/var/lib/home-ai`, `/var/log/home-ai`, or another configured data volume, not in Git.

## Roles in our workflow

### GitHub

Source of truth for project code and documentation.

### ChatGPT

Prepares and reviews repository changes on the development branch.

### Debian development server

Pulls from GitHub, builds, runs tests and performs real runtime verification.

### Physical server

Future production target after the platform is mature enough to leave the Proxmox development VM.
