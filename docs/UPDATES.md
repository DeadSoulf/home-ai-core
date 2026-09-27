# Home AI Core — Server Updates

## Goal

The development server can detect changes in the configured GitHub branch and offer an update from the Web panel.

Default development settings:

```text
update.repository=/srv/home-ai-core
update.remote=origin
update.branch=develop
update.check_interval_seconds=60
```

The future production server can track `main` instead.

## Web flow

Open:

```text
Система -> Обновление сервера
```

The user-facing panel is intentionally minimal. It shows:

- the version currently running on the server
- the version available on the configured GitHub branch
- Check / Update / Restart controls when allowed

Git SHA values, branch internals, build percentages, individual stages and compiler/test logs
remain backend diagnostics and are not shown in the normal Web UI.

The top bar shows the concrete available version, for example `Доступна версия <version>`.

## Update sequence

After an administrator confirms the update, Home AI Core performs:

```text
check clean working tree
        ↓
git pull --ff-only
        ↓
configure immutable build-release-<commit> with CMake/Ninja
        ↓
compile
        ↓
run CTest in that exact directory
        ↓
move current build entry to build-prev
        ↓
atomically point build -> tested build-release-<commit>
        ↓
offer restart
```

If configuration, compilation, or tests fail, the source tree is reset to the previous commit and the running build is kept.

Build directories are never renamed after CMake configuration. CMake and CTest embed absolute
paths in generated metadata, so moving `build-next` to `build` can leave the active tree
pointing at executables that no longer exist. Starting with 0.0.50, each tested build remains in
an immutable `build-release-<commit>` directory and the stable `build` path is a directory
symlink switched only after the full test suite succeeds. `build-prev` keeps the previous build
entry for rollback.

## Restart

After a successful update the Web panel enables:

```text
Перезапустить сервер
```

The current process shuts down Web/Core services and replaces itself with:

```text
/srv/home-ai-core/build/home-ai-core
```

The recommended deployment runs Home AI Core as the `home-ai-core.service` systemd service.
The in-process restart replaces the running binary while remaining under systemd supervision;
if the process later fails, `Restart=on-failure` still applies.

## Security

An authenticated user with `system.manage` can request:

- manual update check
- update installation
- restart

Users with `system.view` can read update status without receiving update-control permission.

The update source and branch are taken from local configuration; the Web API does not accept arbitrary repository URLs or arbitrary shell commands.

Git operations are executed as the same Unix user that runs Home AI Core. The updater does not require root privileges.

The update is refused when:

- the current Git branch differs from the configured update branch
- tracked local source changes are present
- Git cannot perform a fast-forward pull
- build configuration fails
- compilation fails
- tests fail

## Runtime configuration

Mutable settings are stored in:

```text
runtime/home-ai.conf
```

instead of the tracked repository template. This prevents Web configuration changes from making the Git working tree dirty and blocking updates.

On first startup after this change, the runtime configuration is copied from:

```text
config/home-ai.conf
```

The `runtime/` directory is ignored by Git.

## APIs

Authenticated status:

```text
GET /api/update/status
```

Admin actions:

```text
POST /api/update/check
POST /api/update/apply
POST /api/update/restart
```


## Internal progress model

The Web UI polls update status while the server is running, but the detailed progress model is kept for backend diagnostics rather than rendered in the normal update panel. The internal stages are:

```text
5%   GitHub check
15%  fetch changes
25%  CMake configuration
30-70%  Ninja build
70-90%  CTest
92-95%  build activation
100% restart request
```

Ninja `[current/total]` output and CTest `current/total Test` output are parsed so the
build and test percentages are based on actual command progress rather than a timer.
The updater keeps reading child-process output even after the visible log reaches its 64 KiB
rolling limit, preventing a verbose build from being terminated by a closed pipe.
