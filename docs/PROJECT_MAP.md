# Home-AI — карта проекта

**Состояние:** Core foundation готова и переводится на архитектуру внешних Docker-модулей.  
**Текущий этап:** очистка Core от продуктовой логики и подготовка контейнерного runtime.  
**Следующий этап:** Docker Module Runtime и установка первого независимого модуля.

## Core — сохраняем

- identity и node state;
- users, sessions, RBAC и resource permissions;
- CSRF и audit;
- jobs и durable events;
- WebSocket transport;
- Module Registry / Module SDK;
- system and hardware inventory;
- storage foundation;
- network management;
- Core updater и privileged helper;
- Web UI;
- Debian installer;
- Windows file client.

## Core — не содержит

Core больше не является местом для реализации функциональных продуктовых сервисов. Новый код расширений не должен попадать в `internal/<product>` и не должен компилироваться в `home-ai-core`.

Модули будут:

1. поставляться отдельно от Core;
2. иметь собственный репозиторий и Docker image;
3. иметь отдельную версию;
4. объявлять совместимость с Core;
5. устанавливаться и обновляться независимо;
6. запускаться и останавливаться через Module Runtime;
7. взаимодействовать с Core только через документированные контракты.

## Ближайшие engineering milestones

### M1 — Docker runtime foundation

- ✅ Docker устанавливается на поддерживаемом Debian;
- ✅ capability `host.docker` отражает активный Docker Engine;
- ✅ root-helper восстанавливает Docker runtime на обновлённых установках;
- ✅ создана отдельная bridge-сеть `home-ai-modules`;
- ✅ создан каталог данных `/var/lib/home-ai-core/modules`;
- ✅ Core не получает Docker socket;
- ⏭ следующий срез: безопасный container lifecycle и manifest v2.

### M2 — Container module manifest

Расширить manifest данными контейнера:

- image/repository;
- version/tag/digest;
- supported architectures;
- health endpoint;
- API namespace;
- mounts/volumes;
- required host capabilities;
- declared ports;
- dependencies;
- upgrade/rollback metadata.

### M3 — Module lifecycle

Поддержать из Web UI и API:

- install;
- start/enable;
- stop/disable;
- restart;
- update;
- remove;
- health/status;
- logs.

### M4 — первый внешний модуль

Первый функциональный модуль создаётся только после завершения общей Docker-инфраструктуры и не добавляет продуктовый код обратно в Core.

## Правило архитектуры

```text
Core = control plane
Module = isolated workload
Docker = module runtime
```

Core владеет безопасностью, идентификацией, разрешениями, аудитом и orchestration. Модуль владеет собственной бизнес-логикой и собственным persistent state.
