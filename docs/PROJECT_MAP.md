# Home-AI — карта проекта

> Рабочая карта фактического состояния проекта.  
> Каноническая конечная цель: [PRODUCT_VISION.md](PRODUCT_VISION.md)  
> Аудит уже сделанного: [CURRENT_STATE_AUDIT.md](CURRENT_STATE_AUDIT.md)

**Последняя версия, на которой пользователь подтвердил работу сетевого UI:** `0.1.59-dev`  
**Опубликован для проверки:** `0.1.67-dev`  
**Состояние:** F2 NAS начат; `0.1.67-dev` добавляет logical pools, private/shared folders, scoped file permissions и Web-раздел **Файлы**  
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
| Network Management | ✅ v3 | runtime controls + persistent DHCP/static/DNS for NetworkManager, systemd-networkd and safe ifupdown-owned profiles | локальное и постоянное администрирование сети |
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

### ✅ Network Management v3

Реализовано:

- управление link up/down;
- изменение MTU;
- runtime добавление/удаление IP-адреса;
- runtime установка/удаление default route;
- определение активного network backend;
- persistent DHCP/static IPv4 profiles;
- persistent gateway и DNS;
- NetworkManager backend;
- systemd-networkd backend;
- ifupdown: чтение существующих stanza, показ source/method/address/gateway/DNS;
- внешние ifupdown-профили отображаются read-only и не перезаписываются;
- Home-AI-owned ifupdown-профили хранятся отдельно в `/etc/network/interfaces.d/50-home-ai-<iface>`;
- при отсутствии include создаётся backup `/etc/network/interfaces` и добавляется только маркированный include-block;
- конфликт нескольких IPv4 stanza блокирует запись до ручного исправления;
- сетевые `ip`/`wg` команды выполняются через transient systemd units и не ослабляют постоянный root-helper;
- отдельные permissions `network.read` / `network.manage`;
- Audit для сетевых действий;
- Web-управление в **System**;
- WireGuard tools install по явной команде владельца;
- создание/запуск/остановка/удаление WireGuard-туннелей;
- добавление/удаление WireGuard peers;
- сохранение конфигурации в `/etc/wireguard`;
- отображение endpoint, allowed IPs, handshake и RX/TX.

Следующий сетевой долг: explicit import/takeover внешнего ifupdown-профиля (отдельным подтверждаемым действием), затем VLAN/bridges/bonds.

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
- 🧪 DHCP/static/WireGuard reboot acceptance из [CORE_ACCEPTANCE.md](CORE_ACCEPTANCE.md);
- ⏭ targeted tests для privileged routing/validation при следующем package-level refactor.

### 🚧 Этап F2 — File Storage / NAS

Первый вертикальный срез готов:

1. ✅ модель logical storage pools;
2. ✅ private/shared folder resources;
3. ✅ stable folder IDs и внутренние relative paths;
4. ✅ `files.read/files.write/files.manage`;
5. ✅ scoped grants на конкретную папку;
6. ✅ private folder → выбранный пользователь;
7. ✅ shared folder → роль `member`;
8. ✅ API list/create pools и folders;
9. ✅ Web-раздел **Файлы** с фильтрацией по effective permissions;
10. ✅ ADR-0016 и тесты.

Второй вертикальный срез готов:

1. ✅ pool root разрешён только на реально смонтированной data filesystem под `/mnt/home-ai-core/...`;
2. ✅ privileged helper проверяет exact mountpoint, writable state и запрет symlink;
3. ✅ Home-AI создаёт только собственную область `.home-ai/`;
4. ✅ physical private/shared directories создаются по stable resource IDs;
5. ✅ директории принадлежат UID/GID Core, сетевой Core остаётся unprivileged;
6. ✅ при неудаче physical folder provisioning metadata/scoped grants откатываются;
7. ✅ Web выбирает pool только из подходящих mounted non-system filesystems;
8. ✅ ADR-0017 фиксирует physical storage boundary.

Третий вертикальный срез:

1. ✅ безопасный browse/list API внутри разрешённой logical folder;
2. ✅ create-directory;
3. ✅ upload с atomic temp+rename и лимитом 512 MiB;
4. ✅ download через стандартный HTTP content path;
5. ✅ защита от absolute/`..`/symlink traversal;
6. ✅ scoped `files.read/files.write` на каждый `file_folder`;
7. ✅ базовый Web file browser;
8. ✅ ADR-0018 и тесты.

Четвёртый вертикальный срез:

1. ✅ delete file / empty directory;
2. ✅ move/rename внутри одной logical folder;
3. ✅ overwrite при move запрещён;
4. ✅ move каталога внутрь самого себя запрещён;
5. ✅ scoped `files.write`, CSRF и Audit;
6. ✅ Web controls для delete и move/rename;
7. ✅ API/Web tests.

Пятый вертикальный срез:

1. ✅ обычный Delete перемещает объект в per-folder recycle bin;
2. ✅ корзина скрыта от обычного file browser;
3. ✅ можно удалить в корзину целое дерево каталога;
4. ✅ список корзины показывает original path/type/size/deleted time;
5. ✅ Restore не перезаписывает существующий target;
6. ✅ отдельное явное Delete permanently;
7. ✅ scoped `files.read/files.write`, CSRF и Audit;
8. ✅ Web recycle-bin UI + API/Web tests.

Шестой вертикальный срез:

1. ✅ disk-backed resumable upload sessions;
2. ✅ upload state переживает restart Core;
3. ✅ последовательные chunks до 8 MiB;
4. ✅ точный server-reported offset для resume;
5. ✅ SHA-256 проверка каждого chunk;
6. ✅ SHA-256 всего файла перед atomic commit;
7. ✅ Web progress + resume по file fingerprint;
8. ✅ cancel/restart несовпадающей незавершённой загрузки;
9. ✅ ADR-0019 и API/Web/filedata tests.

Следующий подэтап:

1. SMB;
2. затем Windows file-copy client;
3. NFS — при необходимости.

## 4. Следующие продуктовые этапы

### 🚧 F2 — File Storage / NAS

Цель: пользовательское файловое хранилище поверх уже готового low-level storage.

Нужно:

- ✅ logical storage pools/volumes metadata;
- ✅ private folders каждого пользователя;
- ✅ shared/common folders;
- ✅ physical directory provisioning + filesystem validation;
- ✅ file CRUD API foundation (browse/create/upload/download/delete/move/rename);
- ✅ Web file manager (browser + mutations + recycle bin + resumable upload);
- 🚧 SMB;
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
| внешние ifupdown profiles нельзя takeover из UI | позже добавить явный import/takeover с backup/diff/confirmation |
| stable signing/channel | сделать до stable/commercial release |
| AI self-development может менять систему | только versioned/audited/rollback + approval policy |

## 9. Контрольные версии

| Версия | Значение |
|---|---|
| `0.1.57-dev` | последняя версия до cleanup |
| `0.1.58-dev` | ✅ первая подтверждённая версия после cleanup, установлена через Web |
| `0.1.59-dev` | ✅ Network Management v1 установлен; на живом сервере обнаружена sandbox-проблема установки WireGuard |
| `0.1.60-dev` | hotfix | APT для WireGuard вынесен в transient systemd service |
| `0.1.61-dev` | UI | branding + обновлённый экран авторизации |
| `0.1.62-dev` | UI | упрощён интерфейс статуса обновлений |
| `0.1.63-dev` | ✅ установлена для проверки | Network Management v2; на сервере определён backend ifupdown |
| `0.1.64-dev` | Network Management v3 | safe ifupdown profiles/ownership |
| `0.1.65-dev` | Network UI | показывает фактический DHCP IPv4 |
| `0.1.66-dev` | Network hotfix | network inventory без AF_NETLINK; исправлена пустая таблица интерфейсов |
| `0.1.67-dev` | 🧪 NAS foundation | logical pools, private/shared folders, scoped permissions, Files Web UI |

## 10. Правило ведения карты

После каждого крупного этапа:

1. обновляем фактический статус;
2. переносим завершённое в доказанные возможности;
3. держим только один основной текущий engineering milestone;
4. не добавляем общий server-platform scope без связи с PRODUCT_VISION;
5. любое архитектурное решение, влияющее на долгосрочный контракт, фиксируем ADR.
