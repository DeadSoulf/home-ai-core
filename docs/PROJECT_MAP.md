# Home-AI — карта проекта

**Текущая версия Core:** `0.1.166-dev`  
**Состояние:** базовый Home-AI-Core и Docker Module Runtime v2 уже работают.  
**Текущий этап:** проверка полного жизненного цикла первого независимого Docker-модуля.  
**Следующий крупный этап:** health/logs/update/rollback для модулей и подготовка каталога модулей.

---

## 1. Что уже работает в Core

### Базовая платформа

- ✅ первичная установка на чистый Debian 13;
- ✅ amd64 и arm64;
- ✅ доступ к Web UI по LAN сразу после установки;
- ✅ создание первого владельца без bootstrap-токена;
- ✅ users, sessions, RBAC и resource permissions;
- ✅ CSRF и audit;
- ✅ persistent jobs;
- ✅ durable events;
- ✅ WebSocket transport;
- ✅ SQLite state;
- ✅ system and hardware inventory;
- ✅ network management;
- ✅ storage foundation;
- ✅ Core updater;
- ✅ privileged helper;
- ✅ обновление Core через Web UI;
- ✅ Windows file client.

### Архитектура модулей

- ✅ Module Registry;
- ✅ dependency/conflict planner;
- ✅ capability discovery;
- ✅ Docker Engine runtime;
- ✅ bridge-сеть `home-ai-modules`;
- ✅ отдельный каталог данных модулей;
- ✅ Manifest v2;
- ✅ Docker image только через immutable `@sha256`;
- ✅ install/start/stop/restart/remove через Job Engine;
- ✅ lifecycle проходит через privileged helper;
- ✅ Web UI для установки Manifest v2 и управления модулем;
- ✅ persistent data сохраняются после удаления контейнера;
- ✅ Core не получает `docker.sock`;
- ✅ модуль не получает `docker.sock`;
- ✅ модуль запускается от непривилегированного UID/GID;
- ✅ read-only root filesystem;
- ✅ dropped Linux capabilities;
- ✅ `no-new-privileges`.

---

## 2. Что Core принципиально не содержит

Home-AI-Core — это control plane, а не место для продуктовой бизнес-логики.

Новые функции не должны добавляться как:

```text
internal/nvr
internal/nas
internal/ai
internal/smarthome
...
```

Функциональные возможности должны поставляться как отдельные Docker-модули.

Каждый модуль:

1. имеет собственный репозиторий;
2. имеет собственный Docker image;
3. имеет отдельную версию;
4. объявляет совместимость с Core;
5. объявляет capabilities и permissions;
6. имеет собственный persistent state;
7. устанавливается независимо от Core;
8. обновляется независимо от Core;
9. запускается только через Module Runtime;
10. взаимодействует с Core только через документированные контракты.

---

# Engineering milestones

## M0 — Core foundation

**Статус: ✅ готово**

- ✅ security / users / sessions / RBAC;
- ✅ jobs / events / audit;
- ✅ Web UI;
- ✅ hardware/system inventory;
- ✅ network foundation;
- ✅ storage foundation;
- ✅ privileged helper;
- ✅ Core updater;
- ✅ Debian installer;
- ✅ Windows file client.

---

## M1 — Docker runtime foundation

**Статус: ✅ готово**

- ✅ Docker устанавливается на поддерживаемом Debian;
- ✅ capability `host.docker`;
- ✅ helper восстанавливает Docker runtime;
- ✅ bridge-сеть `home-ai-modules`;
- ✅ каталог `/var/lib/home-ai-core/modules`;
- ✅ Core не имеет доступа к Docker socket;
- ✅ модуль не имеет доступа к Docker socket;
- ✅ lifecycle разрешён только через helper.

---

## M2 — Module Manifest v2

**Статус: ✅ готово**

- ✅ Docker runtime обязателен;
- ✅ immutable image `@sha256`;
- ✅ Core version constraint;
- ✅ amd64/arm64 requirements;
- ✅ dependencies;
- ✅ conflicts;
- ✅ capabilities;
- ✅ permissions;
- ✅ health metadata;
- ✅ API namespace;
- ✅ declarative UI navigation;
- ✅ lifecycle declaration;
- ✅ запрет произвольных shell-команд;
- ✅ запрет module-specific host packages.

---

## M3 — Module lifecycle v1

**Статус: ✅ готово**

- ✅ install;
- ✅ start / enable;
- ✅ stop / disable;
- ✅ restart;
- ✅ remove;
- ✅ persistent Job Engine;
- ✅ audit постановки lifecycle jobs;
- ✅ Web UI controls;
- ✅ persistent data сохраняются при remove;
- ✅ unmanaged Docker containers не затрагиваются helper-ом.

---

## M4 — Первый независимый Docker-модуль

**Статус: 🔄 текущий этап**

Reference implementation уже собран в отдельной staging-ветке и не входит в `main` Core. После live-проверки он будет вынесен в отдельный репозиторий.

### Уже сделано

- ✅ создан минимальный `reference.module`;
- ✅ HTTP health endpoint `/health`;
- ✅ persistent state в `/data/state.json`;
- ✅ повторный запуск увеличивает `boot_count`;
- ✅ Docker image собирается без root-зависимостей;
- ✅ локальный lifecycle smoke-test проходит;
- ✅ persistent `/data` переживает пересоздание контейнера;
- ✅ multi-arch image собран для amd64/arm64;
- ✅ image опубликован в GHCR;
- ✅ anonymous pull из GHCR проверен без авторизации;
- ✅ Manifest v2 генерируется автоматически с immutable image digest;
- ✅ текущий reference image закреплён digest `sha256:60177fb078509ea63e74c9776f92a1dd987e88be2cd0b146cc553ec89f987336`.

### Осталось проверить на реальном Home-AI node

- ⏭ установить модуль через Web UI Core;
- ⏭ проверить start;
- ⏭ проверить stop;
- ⏭ проверить restart;
- ⏭ проверить remove;
- ⏭ проверить сохранение `/data` после remove;
- ⏭ проверить повторную установку через Core;
- ⏭ подтвердить работу на реальном amd64 node;
- ⏭ подтвердить работу на реальном arm64 node;
- ⏭ вынести готовый reference implementation в отдельный репозиторий;
- ⏭ зафиксировать его как шаблон для будущих модулей.

### Критерий завершения M4

Полный цикл должен работать без ручного использования Docker CLI:

```text
install
  ↓
running
  ↓
stop
  ↓
start
  ↓
restart
  ↓
remove
  ↓
reinstall
```

---

## M5 — Module Runtime v2 hardening

**Статус: ⏭ следующий этап**

### Health

- ⏭ health reconciliation;
- ⏭ состояние starting / healthy / unhealthy;
- ⏭ автоматическое обнаружение падения контейнера;
- ⏭ отображение health в Web UI.

### Logs

- ⏭ безопасный просмотр stdout/stderr;
- ⏭ API логов;
- ⏭ Web UI логов;
- ⏭ ограничение размера и времени чтения.

### Update

- ⏭ обнаружение новой версии;
- ⏭ проверка нового digest;
- ⏭ update job;
- ⏭ проверка health после обновления;
- ⏭ сохранение предыдущего digest;
- ⏭ rollback при неуспешном обновлении.

### Data lifecycle

- ⏭ отдельная операция `purge data`;
- ⏭ подтверждение удаления persistent state;
- ⏭ backup/restore contract.

### Дополнительные runtime permissions

Пока v2 специально не разрешает дополнительные host-возможности.

В будущем только декларативно и через policy:

- ⏭ ports;
- ⏭ devices;
- ⏭ additional mounts;
- ⏭ GPU access;
- ⏭ USB access;
- ⏭ host networking только для отдельных доверенных сценариев.

---

## M6 — Module Catalog / Store

**Статус: ⏭ запланировано**

Цель — чтобы пользователь не вставлял Manifest вручную.

### Нужно сделать

- ⏭ catalog API;
- ⏭ список доступных модулей;
- ⏭ signed catalog metadata;
- ⏭ trusted signing keys;
- ⏭ repository refresh job;
- ⏭ проверка совместимости до установки;
- ⏭ install plan preview;
- ⏭ update plan preview;
- ⏭ dependencies/conflicts перед mutation;
- ⏭ Web UI каталога;
- ⏭ кнопка «Установить»;
- ⏭ кнопка «Обновить».

Целевой пользовательский сценарий:

```text
Modules
  ↓
Catalog
  ↓
выбрать модуль
  ↓
Install
  ↓
Core проверяет compatibility / permissions / digest
  ↓
Job Engine
  ↓
Docker Runtime
```

---

## M7 — Первые продуктовые модули

**Статус: ⏭ после стабилизации runtime**

Планируемые семейства модулей:

- ⏭ NVR / камеры;
- ⏭ NAS / расширенное хранилище;
- ⏭ backup;
- ⏭ Smart Home;
- ⏭ Local AI;
- ⏭ voice;
- ⏭ VPN / remote access;
- ⏭ containers / services;
- ⏭ virtualization — если будет подтверждена необходимость.

Порядок реализации определяется отдельно. Ни один из этих модулей не должен возвращать продуктовую логику обратно в Core.

---

## M8 — Multi-node Home-AI

**Статус: 🔭 долгосрочный этап**

Архитектура Core должна позволять в будущем объединять несколько Home-AI nodes.

Направления:

- ⏭ node discovery;
- ⏭ trusted node pairing;
- ⏭ cluster identity;
- ⏭ распределение модулей по nodes;
- ⏭ hardware-aware scheduling;
- ⏭ единый Web UI;
- ⏭ общий event/control plane;
- ⏭ безопасное удалённое выполнение jobs.

Multi-node не должен требовать изменения базовой модели безопасности модулей.

---

# Архитектурное правило проекта

```text
Core   = trusted control plane
Module = isolated product workload
Docker = module runtime
Helper = privileged execution boundary
Jobs   = mutation execution model
```

Core владеет:

- identity;
- security;
- permissions;
- audit;
- jobs;
- events;
- orchestration;
- registry;
- system foundations.

Модуль владеет:

- своей бизнес-логикой;
- своим Docker image;
- своей версией;
- своим persistent state;
- своим API namespace;
- своим health contract.

---

# Текущая точка проекта

На версии `0.1.166-dev` инфраструктура внешних Docker-модулей уже существует.

Главная ближайшая задача:

> прогнать уже опубликованный `reference.module` через полный lifecycle на реальном Home-AI node, после чего добавить health, logs, update/rollback и каталог модулей.

До завершения M4 новые продуктовые функции в Core не добавляем.
