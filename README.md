# Home-AI-Core

Home-AI-Core — доверенное ядро и управляющий уровень локальной платформы Home-AI.

Core отвечает только за общую инфраструктуру платформы: идентификацию, безопасность, пользователей и права, задания и события, Web UI, системную информацию, хранилище, обновления и реестр внешних модулей.

Продуктовая функциональность не должна встраиваться в бинарник Core. Функциональные расширения поставляются отдельно и будут запускаться как изолированные Docker-модули, управляемые Core.

## Что входит в Core

- Go backend и React/TypeScript Web UI;
- первичная настройка владельца и аутентифицированные сессии;
- SQLite с последовательными миграциями;
- RBAC, ресурсные разрешения, CSRF и аудит;
- persistent jobs и durable events;
- WebSocket transport;
- Module Registry и Module SDK;
- системная и аппаратная инвентаризация;
- базовое управление несистемными дисками и файловыми хранилищами;
- сетевые настройки и защищённые операции через privileged helper;
- обновление самого Core через Web UI;
- Debian installer для amd64/arm64;
- Windows-клиент для файловых операций.

## Архитектура модулей

Core не содержит реализацию продуктовых модулей.

Целевая модель:

```text
Home-AI-Core
  ├─ Security / Users / RBAC
  ├─ Jobs / Events / Audit
  ├─ Storage / Network / System
  ├─ Core Updater
  ├─ Module Registry
  └─ Docker Module Runtime
       ├─ module A
       ├─ module B
       └─ module C
```

Каждый модуль должен иметь собственный жизненный цикл, версию и контейнерный образ и устанавливаться/обновляться независимо от Core.

## Установка

Для чистого **Debian 13** на `amd64` или `arm64` под `root`:

```sh
apt update && apt install -y curl ca-certificates && curl -fsSL https://raw.githubusercontent.com/DeadSoulf/home-ai-core/main/scripts/install.sh | sh
```

После установки откройте Web UI с другого компьютера в той же локальной сети:

```text
http://IP_СЕРВЕРА:8080/
```

При первом открытии создаётся учётная запись владельца.

Обычные обновления Core после первоначальной установки выполняются через **System → Updates**.

## Runtime

По умолчанию Core слушает:

```text
0.0.0.0:8080
```

Сетевой Core работает от непривилегированной учётной записи `home-ai-core`.

Привилегированный helper:

```text
/usr/libexec/home-ai-core/home-ai-core-updater
```

Core не должен получать прямой root-доступ или прямой доступ к привилегированным системным интерфейсам. Такие операции выполняются через ограниченный helper с явными контрактами.

## Репозиторий

Основные документы:

- `docs/PRODUCT_VISION.md` — границы продукта и архитектурная модель;
- `docs/PROJECT_MAP.md` — текущее состояние и следующие этапы;
- `docs/CURRENT_STATE_AUDIT.md` — что остаётся внутри Core;
- `docs/sdk/module-sdk-v2.md` — контракт Docker-модулей.

## Авторство

Copyright © 2026 **TexNik**.

## Лицензия

Home-AI-Core распространяется по лицензии Apache License 2.0. См. `LICENSE`.
