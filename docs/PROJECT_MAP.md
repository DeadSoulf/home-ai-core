# Home-AI — карта проекта

> Рабочая карта фактического состояния проекта.  
> Каноническая конечная цель: [PRODUCT_VISION.md](PRODUCT_VISION.md)  
> Аудит уже сделанного: [CURRENT_STATE_AUDIT.md](CURRENT_STATE_AUDIT.md)

**Текущая подтверждённая версия:** `0.1.58-dev`  
**Опубликован для проверки:** `0.1.59-dev`  
**Состояние:** фундамент Core работает; `0.1.59-dev` содержит Network Management v1 и WireGuard foundation  
**Обновлено:** 2026-09-29

## Конечный продукт

Home-AI должен стать полностью автономной локальной платформой дома:

```text
                         HOME-AI
                            │
                      AI AGENT
                 memory / tools / policy
                            │
     ┌───────────────┬──────┼──────┬────────────────┐
     │               │             │                │
 SMART HOME        NVR / AI       NAS             VOICE
 devices          cameras        files           terminals
     │               │             │                │
     └───────────────┴──────┬──────┴────────────────┘
                            │
                       HOME-AI-CORE
                 identity / API / events /
                 jobs / modules / audit
                            │
                    CLUSTER CONTROL
                            │
        ┌───────────────────┼───────────────────┐
        │                   │                   │
     Node 1              Node 2              Node 3
   Core / NVR           GPU / AI           Storage / AI
```

Интернет не должен быть обязательным для локальной работы дома.

## Легенда

| Статус | Значение |
|---|---|
| ✅ | готово и уже является частью фундамента |
| 🧪 | реализовано, но нужна отдельная практическая проверка |
| 🚧 | текущая работа |
| ⏭ | следующий продуктовый этап |
| 🕓 | запланировано позже |
| ⏸ | сознательно отложено, не является критическим маршрутом |
| ⚠️ | техдолг / архитектурный риск |

## 1. Фундамент Core

| Подсистема | Статус | Текущее состояние | Роль в конечном Home-AI |
|---|---|---|---|
| Core API | ✅ | Go REST API и health | единый API для Web, AI, voice, clients и cluster |
| State / migrations | ✅ | SQLite + forward migrations | локальное control-plane состояние |
| Node identity | ✅ | стабильный ID узла | основа будущего cluster enrollment |
| Security / Auth | ✅ | owner/member accounts, sessions, RBAC, CSRF, resource-scoped grants | пользователи, комнаты, устройства, камеры, папки и AI policy |
| Audit | ✅ | аудит действий | критично для AI и опасных операций |
| Jobs | ✅ | persistent job engine | долгие операции, модели, storage, NVR, cluster |
| Events | ✅ | durable events | Smart Home/NVR/cluster domain events |
| Realtime | ✅ | WebSocket | live-состояние UI и устройств |
| Module SDK / Registry | ✅ | manifest, dependencies, capabilities | модульная архитектура продукта |
| Signed Module Repository | ✅ foundation | подписи и проверка metadata/package | будущая доставка Home-AI модулей |
| System Info | ✅ | CPU/RAM/GPU/network/storage inventory | локальная диагностика + cluster resources |
| Network Management | ✅ v1 | up/down, MTU, runtime IP/default-route management, audit | локальное администрирование сети |
| WireGuard | ✅ foundation | install, tunnel lifecycle, peers, persistent configs | база собственного удалённого доступа |
| Hardware discovery | ✅ | PCI/GPU/storage данные | ускорители, камеры, adapters |
| Low-level Storage | ✅ | partitions, format, mount, labels, SMART/LVM | фундамент NAS и NVR storage |
| Privileged helper | ✅ | root-граница работает; routing/update/storage физически разделены | host/storage/network privileged actions |
| Update System v2 | ✅ | Web check/download/install/restart | обновление продукта без обычного .deb |
| Rollback | 🧪 | реализован | нужен живой acceptance test |
| Web UI | ✅ foundation | permission-aware navigation, System/Modules/Jobs/Audit/Users, auth shell | основной интерфейс сейчас |
| CI / Release | ✅ | tests/build/smoke/amd64/arm64 | безопасная разработка и релизы |

## 2. Что уже доказано на практике

### ✅ Web Update

Проверен реальный цикл до `0.1.58-dev`:

```text
Check
 -> Download
 -> Verify
 -> Install
 -> Restart
 -> New version running
```

Обычное обновление идёт update-bundle, а `.deb` остаётся bootstrap/recovery форматом.

### ✅ Cleanup

Перед `0.1.58-dev` удалены старые/дублирующие storage модели, неиспользуемые helpers и устаревшие Web/schema элементы.

### ✅ Storage foundation

Используется единое каноническое дерево block devices. Низкоуровневые безопасные storage операции уже дают основу для будущего NAS.

### ✅ Network Management v1

Реализовано:

- управление link up/down;
- изменение MTU;
- добавление/удаление IP-адреса;
- установка/удаление default route;
- отдельные permissions `network.read` / `network.manage`;
- Audit для сетевых действий;
- Web-управление в **System**;
- WireGuard tools install по явной команде владельца;
- создание/запуск/остановка/удаление WireGuard-туннелей;
- добавление/удаление WireGuard peers;
- сохранение конфигурации в `/etc/wireguard`;
- отображение endpoint, allowed IPs, handshake и RX/TX.

Ограничение v1: настройки физических интерфейсов пока runtime-only. Постоянные DHCP/static/DNS-профили будут отдельным сетевым этапом.

### ✅ Multi-user foundation

Реализовано:

- роли `owner` и `member`;
- API создания и списка пользователей;
- Web-раздел **Пользователи**;
- новые `member` получают только права на собственную учетную запись/сессию;
- административные страницы скрываются по permissions;
- ограниченный пользователь не опрашивает недоступные System/Updates/Realtime;
- resource-scoped permissions готовы для будущего доступа к папкам, комнатам и камерам.

## 3. Текущая работа

### ✅ Этап F1 — фундамент готов для продуктовых модулей

Готово:

1. ✅ Разделён большой `cmd/home-ai-core-updater/main.go`:
   - `main.go` — socket/router;
   - `update_ops.go` — update/install/rollback;
   - `storage_ops.go` — storage operations;
   - `unix_helpers.go` — Unix/socket helpers.
2. ✅ Внешний protocol/socket не изменён.
3. ✅ Полный `core-ci` после extraction refactor прошёл успешно.

Готово дополнительно:

4. ✅ Добавлен фундамент resource-scoped permissions:
   - глобальные permissions сохранены;
   - scoped grants для ролей;
   - scoped grants для отдельных пользователей;
   - `Actor.Allows(permission, resource_type, resource_id)`;
   - миграция `009_resource_permissions.sql`;
   - ADR-0014;
   - тесты и полный CI.
5. ✅ Добавлена household multi-user foundation:
   - роль `member`;
   - API list/create users;
   - Web UI управления пользователями;
   - permission-aware Web navigation;
   - миграция `010_household_users.sql`;
   - ADR-0015;
   - полный CI.

Отдельно проверить в эксплуатации:

- 🧪 rollback и повторное обновление на установленном сервере;
- ⏭ targeted tests для privileged routing/validation при следующем package-level refactor.

### 🚧 Этап F2 — File Storage / NAS

Текущий следующий подэтап:

1. определить модель storage spaces/pools;
2. определить private home каждого пользователя;
3. определить shared/common folders;
4. связать доступ с user IDs и resource-scoped permissions;
5. затем добавить файловое API/Web file manager и сетевые протоколы.

## 4. Следующие продуктовые этапы

### 🚧 F2 — File Storage / NAS

Цель: пользовательское файловое хранилище поверх уже готового low-level storage.

Нужно:

- storage pools/volumes;
- private folders каждого пользователя;
- shared/common folders;
- Web file manager;
- SMB;
- NFS при необходимости;
- quotas/policies;
- snapshots/backup where supported;
- capacity/health monitoring.

### ⏭ F3 — Native Smart Home

Home Assistant не является основой.

Нужно:

- собственная модель device/entity/state;
- rooms/zones;
- Zigbee;
- Matter/Thread;
- MQTT;
- Wi-Fi/LAN;
- Bluetooth;
- Modbus;
- discovery/pairing;
- scenes;
- schedules;
- automation engine;
- permission-scoped actions;
- events/audit.

### ⏭ F4 — Cameras / NVR

Нужно:

- camera entities;
- RTSP;
- ONVIF where useful;
- live view;
- continuous/event recording;
- archive/timeline;
- выделенная ёмкость архива;
- ring overwrite самых старых незакреплённых записей;
- motion/object events;
- AI vision hooks;
- локальная база известных лиц;
- face recognition при наличии подходящего hardware.

### ⏭ F5 — Local AI Agent

AI становится центральным управляющим слоем.

Нужно:

- local LLM adapters;
- vision model adapters;
- tool registry;
- context/memory;
- permissions inherited from user;
- approval workflow;
- Smart Home tools;
- NVR search/tools;
- file search/tools;
- server/cluster diagnostics;
- automation proposals;
- self-improvement proposals;
- version/rollback/audit for AI-driven changes;
- optional external AI adapters.

Правило: AI может готовить изменения самостоятельно, но изменения конфигурации и чувствительные действия применяются после требуемого согласования.

### 🕓 F6 — Voice

- microphone terminals;
- wake word;
- STT;
- AI Agent;
- permission/approval;
- TTS.

В перспективе голос должен стать одним из основных интерфейсов Home-AI.

### 🕓 F7 — Remote Access and Clients

#### WireGuard

Базовый WireGuard уже реализован.

Осталось для полноценного Home-AI remote access:

- device enrollment;
- выпуск клиентских конфигураций/QR;
- revocation;
- policy маршрутов и доступных сервисов;
- без прямой публикации SMB/NFS в Internet.

#### Windows client

- копирование выбранных файлов на Home-AI;
- private/shared destination;
- resumable transfers;
- integrity checking;
- LAN + remote operation.

Это не Windows system-image backup.

#### Android

Отдельное приложение позже, после стабилизации серверных API.

### 🕓 F8 — Multi-node Cluster

- discovery;
- explicit owner enrollment;
- authenticated/encrypted trust;
- heartbeat;
- CPU/RAM/GPU/storage inventory;
- workload requirements;
- scheduler;
- locality;
- AI/video workload placement;
- degraded operation;
- reallocation after node failure where possible;
- единый Web view.

Модель leader/master пока специально не фиксируется.

## 5. AI и права пользователей

Целевая модель:

```text
User / Voice / Automation
          │
          v
       AI Agent
          │
          v
Effective user permissions
          │
     ┌────┼─────────────┐
     │    │             │
 auto   approval       deny
     │    │
     └────┴────> audited tool/action
```

Права должны поддерживать scope по:

- пользователю/роли;
- комнате/зоне;
- устройству;
- камере;
- папке/share;
- модулю;
- чувствительности операции.

AI не может расширять собственные права.

## 6. Что сохраняем из ранней архитектуры

Точно сохраняем:

- API-first;
- least privilege;
- Core/Module разделение;
- Jobs;
- Events;
- Module SDK;
- signed packages;
- state migrations;
- node identity;
- hardware discovery;
- third-party license policy;
- audit;
- rollback.

Это напрямую поддерживает конечный Home-AI.

## 7. Что убираем из главного маршрута

### ⏸ Generic Docker management

Может понадобиться как технология/опциональный модуль, но не как главный продукт.

### ⏸ KVM / Virtualization

Не удаляем концепцию навсегда, но она не должна задерживать NAS, Smart Home, NVR и AI.

### ⏸ Generic third-party App Store

Приоритет — магазин/репозиторий **Home-AI модулей**, а не каталог любых self-hosted приложений.

### ❌ Home Assistant dependency

Собственный Smart Home runtime заменяет эту идею. Совместимость когда-нибудь может быть отдельным bridge-модулем.

### ⏸ iOS client

Не является текущим приоритетом.

## 8. Техдолг / риски

| Риск | Действие |
|---|---|
| updater helper раньше был монолитным | ✅ первый extraction refactor выполнен; package-level split можно сделать позже |
| rollback не проверен живым циклом | acceptance test |
| resource scopes заложены, но UI/API управления grants ещё нет | добавить управление ролями/scopes вместе с NAS/Smart Home |
| durable events не подходят для media/high-rate telemetry | оставить media отдельным data plane |
| SQLite single-node | не превращать локальную схему в неявный cluster contract |
| cluster leadership не определён | сохранить abstraction, решить позже |
| physical NIC profiles пока runtime-only | добавить backend-aware DHCP/static/DNS persistence |
| stable signing/channel | сделать до stable/commercial release |
| AI self-development может менять систему | только versioned/audited/rollback + approval policy |

## 9. Контрольные версии

| Версия | Значение |
|---|---|
| `0.1.57-dev` | последняя версия до cleanup |
| `0.1.58-dev` | ✅ первая подтверждённая версия после cleanup, установлена через Web |
| `0.1.59-dev` | 🧪 опубликована; Network Management v1 + WireGuard foundation, ожидает живую Web-установку |

## 10. Правило ведения карты

После каждого крупного этапа:

1. обновляем фактический статус;
2. переносим завершённое в доказанные возможности;
3. держим только один основной текущий engineering milestone;
4. не добавляем общий server-platform scope без связи с PRODUCT_VISION;
5. любое архитектурное решение, влияющее на долгосрочный контракт, фиксируем ADR.
