# Home-AI — карта проекта

> Рабочая карта фактического состояния проекта.  
> Каноническая конечная цель: [PRODUCT_VISION.md](PRODUCT_VISION.md)  
> Аудит уже сделанного: [CURRENT_STATE_AUDIT.md](CURRENT_STATE_AUDIT.md)

**Текущая подтверждённая версия:** `0.1.58-dev`  
**Состояние:** фундамент Core работает; Web-обновление проверено на реальной установке  
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
| Security / Auth | ✅ | owner bootstrap, sessions, RBAC, CSRF | пользователи и политика доступа |
| Audit | ✅ | аудит действий | критично для AI и опасных операций |
| Jobs | ✅ | persistent job engine | долгие операции, модели, storage, NVR, cluster |
| Events | ✅ | durable events | Smart Home/NVR/cluster domain events |
| Realtime | ✅ | WebSocket | live-состояние UI и устройств |
| Module SDK / Registry | ✅ | manifest, dependencies, capabilities | модульная архитектура продукта |
| Signed Module Repository | ✅ foundation | подписи и проверка metadata/package | будущая доставка Home-AI модулей |
| System Info | ✅ | CPU/RAM/GPU/network/storage inventory | локальная диагностика + cluster resources |
| Hardware discovery | ✅ | PCI/GPU/storage данные | ускорители, камеры, adapters |
| Low-level Storage | ✅ | partitions, format, mount, labels, SMART/LVM | фундамент NAS и NVR storage |
| Privileged helper | ✅ / ⚠️ | безопасная root-граница работает | host/storage/network privileged actions |
| Update System v2 | ✅ | Web check/download/install/restart | обновление продукта без обычного .deb |
| Rollback | 🧪 | реализован | нужен живой acceptance test |
| Web UI | ✅ foundation | System/Modules/Jobs/Audit и auth shell | основной интерфейс сейчас |
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

## 3. Текущая работа

### 🚧 Этап F1 — закончить фундамент

1. Разделить большой `cmd/home-ai-core-updater/main.go`:
   - socket/router;
   - update/install/rollback;
   - storage operations.
2. Не менять внешний protocol/socket при первом extraction refactor.
3. Добавить targeted tests для privileged routing/validation.
4. Прогнать полный CI.
5. Реально проверить rollback и повторное обновление.
6. Расширить permission model в сторону будущих resource scopes.

После этого фундамент считаем достаточно устойчивым для продуктовых модулей.

## 4. Следующие продуктовые этапы

### ⏭ F2 — File Storage / NAS

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

- собственный secure remote channel;
- device enrollment;
- revocation;
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
| updater helper слишком большой | extraction refactor |
| rollback не проверен живым циклом | acceptance test |
| RBAC пока недостаточно resource-aware | добавить scopes до Smart Home/NAS |
| durable events не подходят для media/high-rate telemetry | оставить media отдельным data plane |
| SQLite single-node | не превращать локальную схему в неявный cluster contract |
| cluster leadership не определён | сохранить abstraction, решить позже |
| stable signing/channel | сделать до stable/commercial release |
| AI self-development может менять систему | только versioned/audited/rollback + approval policy |

## 9. Контрольные версии

| Версия | Значение |
|---|---|
| `0.1.57-dev` | последняя версия до cleanup |
| `0.1.58-dev` | ✅ первая подтверждённая версия после cleanup, установлена через Web |

## 10. Правило ведения карты

После каждого крупного этапа:

1. обновляем фактический статус;
2. переносим завершённое в доказанные возможности;
3. держим только один основной текущий engineering milestone;
4. не добавляем общий server-platform scope без связи с PRODUCT_VISION;
5. любое архитектурное решение, влияющее на долгосрочный контракт, фиксируем ADR.
