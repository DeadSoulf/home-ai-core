# Home-AI — карта проекта

**Текущая версия Core:** `0.1.167-dev`  
**Состояние:** базовый Home-AI-Core и Docker Module Runtime v2 уже работают.  
**Текущий этап:** переход от ручной установки Manifest v2 к официальному каталогу модулей HOME AI Core.  
**Следующий крупный этап:** private GitHub/GHCR catalog, обнаружение обновлений, ручное обновление с health-check и автоматическим rollback.

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
- ✅ Windows file client;
- ✅ Web-консоль сервера в `Система → Консоль` для администратора;
- ✅ отдельное permission `system.console`;
- ✅ одноразовый CSRF-защищённый ticket перед WebSocket-сессией;
- ✅ shell работает от непривилегированного пользователя `home-ai-core` внутри systemd sandbox.

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
- ✅ Windows file client;
- ✅ Web-консоль сервера (`Система → Консоль`);
- ✅ permission `system.console` только для администратора;
- ✅ audit открытия/закрытия консоли без журналирования введённых команд.

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

### Live-проверка на реальном Home-AI node

- ✅ установка модуля через Web UI Core;
- ✅ start;
- ✅ stop;
- ✅ restart;
- ✅ remove;
- ✅ lifecycle-кнопки в Web UI отрабатывают через Core;
- ⏭ проверить сохранение `/data` после remove;
- ⏭ проверить повторную установку через Core;
- ⏭ подтвердить persistent state после reinstall;
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

**Статус: 🔄 активный этап**

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

Принята модель обновлений:

- ✅ обновление запускается пользователем вручную;
- ✅ Core должен автоматически проверять наличие новых версий;
- ✅ используется обычная SemVer-нумерация `major.minor.patch`;
- ✅ release channels (`stable/beta/dev`) не используются;
- ⏭ обнаружение новой версии по официальному каталогу;
- ⏭ сравнение установленной версии с последней доступной;
- ⏭ проверка нового immutable image digest;
- ⏭ отдельный update job;
- ⏭ health-check новой версии после запуска;
- ⏭ сохранение предыдущего manifest/image digest;
- ⏭ автоматический rollback при неуспешном health-check;
- ⏭ audit update/rollback;
- ⏭ очистка устаревших Docker images после успешного обновления.

### Data lifecycle

Принято разделение удаления:

- ⏭ «Удалить модуль» — контейнер и регистрация удаляются, данные сохраняются;
- ⏭ «Удалить модуль и данные» — отдельное подтверждаемое действие;
- ⏭ отдельная операция `purge data`;
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

## M6 — Official Module Catalog / Store

**Статус: 🔄 архитектура согласована, реализация следующая**

Цель — полностью убрать ручную вставку Manifest из обычного пользовательского сценария.

### Принятая архитектура

- ✅ официальный каталог размещается в отдельном GitHub-репозитории `DeadSoulf/home-ai-core_modules`;
- ✅ каждый продуктовый модуль остаётся в собственном GitHub-репозитории;
- ✅ Docker images публикуются отдельно от Core и закрепляются immutable `@sha256`;
- ✅ каталог и образы приватные;
- ✅ Core доверяет только нашим официальным модулям;
- ✅ сторонние module repositories пока не поддерживаются;
- ✅ release channels не используются;
- ✅ версии модулей используют обычный SemVer;
- ✅ новая версия модуля появляется в каталоге только после CI build/test/publish;
- ✅ digest должен попадать в каталог автоматически, без ручного копирования;
- ✅ установка — только из каталога одной кнопкой;
- ✅ обновление — только вручную пользователем;
- ✅ проверка наличия обновлений — автоматически + кнопка ручной проверки;
- ✅ неуспешное обновление должно автоматически откатываться на предыдущий digest;
- ✅ ручное поле «Вставьте manifest доверенного модуля» убрать из обычного UI.

### Каталог

Целевая цепочка публикации:

```text
module repository
  ↓
Git tag / release
  ↓
GitHub Actions
  ↓
build + tests
  ↓
private container image
  ↓
immutable sha256 digest
  ↓
official home-ai-core_modules catalog
  ↓
HOME AI Core refresh
```

### UI «Модули»

Раздел должен иметь два логических блока:

```text
Доступные
  ├─ карточка модуля
  ├─ версия
  ├─ описание
  └─ Установить

Установленные
  ├─ текущая версия
  ├─ состояние
  ├─ доступное обновление
  ├─ Запустить / Остановить / Перезапустить
  ├─ Обновить
  ├─ Удалить
  └─ Удалить модуль и данные
```

Если в каталоге опубликована версия новее установленной:

```text
NVR
Установлено: 1.1.0
Доступно: 1.2.0
[ Обновить ]
```

### Нужно реализовать

- ⏭ новый Docker-image-oriented формат официального каталога;
- ⏭ private repository authentication для Core;
- ⏭ private container registry authentication;
- ⏭ repository/catalog refresh job;
- ⏭ периодическую автоматическую проверку обновлений;
- ⏭ кнопку «Проверить обновления»;
- ⏭ last-known-good snapshot каталога;
- ⏭ catalog API;
- ⏭ список доступных модулей;
- ⏭ проверку совместимости Core/архитектуры/capabilities;
- ⏭ dependency/conflict validation непосредственно перед mutation;
- ⏭ update-plan preview;
- ⏭ update job;
- ⏭ post-update health-check;
- ⏭ automatic rollback;
- ⏭ audit update/rollback;
- ⏭ Web UI каталога;
- ⏭ убрать ручной Manifest install из обычного UI.

### Пользовательский сценарий

```text
Модули
  ↓
Core обновляет официальный private catalog
  ↓
Доступные
  ↓
Установить
  ↓
Core проверяет compatibility + digest
  ↓
Job Engine
  ↓
Docker Runtime
  ↓
health-check
  ↓
Установлено
```

Обновление:

```text
Core обнаружил новую версию
  ↓
«Доступно обновление»
  ↓
пользователь нажал «Обновить»
  ↓
Core сохраняет предыдущий digest
  ↓
pull нового immutable image
  ↓
замена контейнера
  ↓
health-check
  ├─ OK → новая версия активна
  └─ ERROR → автоматический rollback
```

---

## M7 — Первые продуктовые модули

**Статус: ⏭ после стабилизации runtime**

Планируемые семейства модулей:

- 🔄 NVR / камеры — первый реальный продуктовый модуль;
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

На версии `0.1.167-dev` базовый Docker Module Runtime уже существует. Следующая задача — превратить техническую установку по Manifest в полноценный официальный каталог модулей.

Первый продуктовый модуль — NVR — развивается отдельно в `DeadSoulf/home-ai-core_dvr`.

Принято, что NVR после установки не будет открываться как отдельный пользовательский Web-сервис. Он должен появляться в HOME AI Core как новый раздел интерфейса, а Core будет выступать control plane и точкой доступа к модулю.

Главная ближайшая задача:

> реализовать private official catalog `home-ai-core_modules`, автоматическое обнаружение доступных версий, установку одной кнопкой и ручное обновление с обязательным health-check и автоматическим rollback.

Продуктовая логика NVR при этом остаётся внутри отдельного Docker-модуля и не переносится обратно в Core.
