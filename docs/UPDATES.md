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

The panel shows:

- configured branch
- local Git commit
- remote GitHub commit
- update state
- progress/error message
- build/test output when relevant

The top bar also shows `Доступно обновление` when the remote commit differs from the local commit.

## Update sequence

After an administrator confirms the update, Home AI Core performs:

```text
check clean working tree
        ↓
git pull --ff-only
        ↓
configure build-next with CMake/Ninja
        ↓
compile
        ↓
run CTest
        ↓
keep old build as build-prev
        ↓
activate new build
        ↓
offer restart
```

If configuration, compilation, or tests fail, the source tree is reset to the previous commit and the running build is kept.

## Restart

After a successful update the Web panel enables:

```text
Перезапустить сервер
```

The current process shuts down Web/Core services and replaces itself with:

```text
/srv/home-ai-core/build/home-ai-core
```

A separate systemd service is therefore not required for the current development workflow.

## Security

Only an authenticated `admin` can request:

- manual update check
- update installation
- restart

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
