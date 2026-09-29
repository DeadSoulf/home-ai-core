# Home-AI-Core — карта проекта

> Живой документ состояния проекта. Обновлять после завершения заметных этапов, изменения архитектуры или подтверждённого релиза.

**Текущая подтверждённая версия:** `0.1.58-dev`  
**Состояние:** рабочая dev-версия, Web-обновление проверено end-to-end  
**Последнее обновление карты:** 2026-09-29

## Главная цель проекта

Home-AI-Core — основа **автономного локального сервера умного дома**, который должен продолжать работать без обязательной зависимости от внешнего облака.

Конечная система должна объединить в одной платформе:

- **умный дом** — устройства, датчики, реле, освещение, климат, сценарии и автоматизации;
- **камеры и видеонаблюдение** — RTSP/IP-камеры, live view, запись, архив, события и AI-анализ видео;
- **файловое хранилище** — локальный NAS, общие папки, дисковые пулы, контроль состояния, резервирование и управление данными;
- **собственного локального AI-агента** — единый интеллектуальный слой, который понимает состояние дома, камер, файлов и серверов и может выполнять разрешённые действия;
- **кластер Home-AI-Core** — несколько серверов в локальной сети, объединённых в одну систему для распределения вычислений, хранения, камер и AI-нагрузки;
- **единый Web control plane** — управление всей системой из одного интерфейса.

Текущие Core/API, System, Storage, Security, Updater, CI и Web UI — это **фундамент**, на котором строятся эти конечные функции.

## Целевая архитектура

```text
                         Home-AI
                            │
                   ┌────────┴────────┐
                   │    AI Agent     │
                   │ orchestration   │
                   └────────┬────────┘
                            │
          ┌─────────────────┼─────────────────┐
          │                 │                 │
      Smart Home         Cameras        File Storage
      devices             RTSP/NVR          NAS/Data
          │                 │                 │
          └─────────────────┼─────────────────┘
                            │
                     Home-AI-Core
                            │
                   Local cluster fabric
                            │
          ┌─────────────────┼─────────────────┐
          │                 │                 │
       Server 1          Server 2          Server 3
      Core / NVR        GPU / AI         Storage / AI
```

Главный архитектурный принцип: **одна логическая система может состоять из нескольких физических серверов**, а пользователь должен видеть и управлять ею как единым Home-AI.

## Основные направления

1. **Core platform** — API, Web UI, security, modules, updater, system information.
2. **Smart Home** — устройства, протоколы, сущности, состояния, автоматизации и сценарии.
3. **Cameras / NVR** — RTSP, запись, архив, события, детекция и AI vision.
4. **File Storage / NAS** — диски, файловые системы, пулы, shares, резервирование и доступ.
5. **Local AI Agent** — LLM/vision/tools, память, контекст дома и безопасное выполнение действий.
6. **Cluster** — обнаружение узлов, доверие, ресурсы, health, scheduler и распределение задач.
7. **Operations** — обновления, rollback, monitoring, logs, backups, recovery и CI/release.

## Легенда статусов

| Статус | Значение |
|---|---|
| ✅ Готово | Реализовано и используется |
| 🧪 Проверить | Реализовано, но нужен отдельный практический сценарий проверки |
| 🚧 В работе | Текущий активный этап |
| ⏭ Дальше | Ближайшая задача |
| 🕓 Позже | Не блокирует текущую разработку |
| ⚠️ Техдолг | Работает, но требует архитектурного улучшения |

## Карта подсистем

| Подсистема | Статус | Что уже есть | Что дальше |
|---|---|---|---|
| Core / API | ✅ Готово | Go backend, системное API, modules API, security API, update API, health endpoint | Продолжать покрытие тестами при новых функциях |
| Web UI | ✅ Готово | React/TypeScript UI, System page, update flow, system/storage отображение | Улучшать UX ошибок, прогресса и destructive actions |
| Update System v2 | ✅ Готово | Check → Download → Verify → Install → Restart через Web; bundle по архитектуре; checksum; manifest verification | Добавить production-hardening и стабильный канал |
| Rollback | 🧪 Проверить | Backup предыдущей версии, API rollback, Web-кнопка и reconnect | Провести отдельный реальный rollback-тест и повторное обновление вперёд |
| Updater Helper | ⚠️ Техдолг | Root-helper, Unix socket, peer UID check, versioned protocol, update и storage allowlist | Разделить update и storage handlers без изменения протокола |
| System info | ✅ Готово | CPU, RAM, GPU, kernel, architecture, network, canonical block tree | Позже использовать эти данные для cluster resource inventory |
| Storage — низкий уровень | ✅ Готово | Единое дерево устройств, inspect, mount/unmount, format, partition operations, labels, SMART/LVM integration | Усилить тесты защитных ограничений и UX опасных операций |
| File Storage / NAS | 🕓 Позже | Низкоуровневый storage-фундамент уже есть | Спроектировать пулы, shares, права доступа, сетевую публикацию, квоты, snapshots/backup |
| Smart Home | 🕓 Позже | Пока отдельного runtime для устройств и автоматизаций нет | Определить device/entity model, протоколы подключения, события, состояния и automation engine |
| Cameras / NVR | 🕓 Позже | Отдельный camera/NVR слой ещё не реализован | RTSP ingestion, live view, recording, archive, retention, events, AI vision hooks |
| Local AI Agent | 🕓 Позже | Core уже даёт будущую базу API/security/modules | Спроектировать agent runtime, tools, permissions, memory/context, local model adapters |
| Cluster / Multi-node | 🕓 Позже | Есть системная информация узла; межузлового control plane пока нет | Node discovery, trust, heartbeats, resource inventory, scheduler, placement и failover |
| Security / Auth | ✅ Готово | Bootstrap owner, cookie session, CSRF, permissions, audit, logout/session revoke | Расширить модель доверия на devices, agents и cluster nodes |
| CI | ✅ Готово | gofmt, go mod tidy lock check, JSON/shell validation, Web typecheck/tests/build, Go test/vet/build, integration smoke, cross-build | Добавить targeted tests после рефакторинга helper |
| Release | ✅ Готово | GitHub Actions публикует update bundles для amd64/arm64 | Стабилизировать release/channel policy |
| Initial install | ✅ Готово | Debian package устанавливает Core, Web, helper, systemd units и зависимости | Оставить .deb только bootstrap/recovery |
| Documentation | 🚧 В работе | Update System v2, ADR и технические документы | Поддерживать эту карту и связать новые решения с ADR |

## Сделано и проверено

### Update System v2

Нормальное обновление больше не зависит от нового Debian-пакета.

Рабочий поток:

```text
Web UI
  ↓
fresh update check
  ↓
GitHub Releases
  ↓
architecture-specific tar.gz
  ↓
SHA-256 + manifest verification
  ↓
prepared bundle
  ↓
root updater helper
  ↓
backup
  ↓
Core + Web + helper replacement
  ↓
service restart
  ↓
new version
```

Подтверждено на реальном обновлении до `0.1.58-dev`:

- Web увидел новую версию;
- bundle скачался;
- bundle прошёл проверку;
- установка была запущена из Web;
- Core перезапустился;
- новая версия успешно работает после обновления.

Для обычных обновлений используются:

```text
home-ai-core-update_<version>_<arch>.tar.gz
home-ai-core-update_<version>_<arch>.tar.gz.sha256
```

`.deb` остаётся только для:

- первоначальной установки;
- аварийного восстановления;
- package-level изменений, если они когда-либо понадобятся.

### Очистка кода

Перед `0.1.58-dev` выполнен отдельный cleanup-проход.

Убрано, в частности:

- старое дублирующее block-device inventory;
- устаревшие `block_devices` модели в Go и Web;
- старые тесты удалённого storage-кода;
- неиспользуемые systeminfo helpers;
- лишние updater helpers;
- неиспользуемый manifest byte-decoder;
- устаревшие CSS и Web references;
- старые переводы и schema references.

После чистки проект снова прошёл CI.

### Storage inventory

Вместо двух конкурирующих представлений накопителей используется одно каноническое дерево:

```text
block_tree
```

Это уменьшает расхождение между backend, API и Web.

### CI

Основной CI проверяет:

- `go mod tidy` без изменений lock-файлов;
- `gofmt`;
- JSON schemas;
- shell syntax;
- Web dependencies;
- TypeScript typecheck;
- Web tests;
- Web production build;
- `go test ./...`;
- `go vet ./...`;
- Core build;
- integration smoke test;
- amd64 build;
- arm64 build.

## Сейчас в работе

### 🚧 Рефакторинг `cmd/home-ai-core-updater/main.go`

Root-helper сейчас выполняет две большие группы задач:

1. update / install / rollback;
2. privileged storage operations.

Файл стал слишком большим и смешивает разные ответственности.

Цель следующего этапа — **разделить код без изменения поведения, API, socket path и protocol contract**.

Предлагаемая структура:

```text
cmd/home-ai-core-updater/
  main.go                  # запуск, socket, connection lifecycle, routing

internal/
  updaterhelper/
    protocol.go            # текущий wire contract

  privileged/
    update/
      install.go
      rollback.go
      backup.go

    storage/
      inspect.go
      operations.go
      validation.go
      smart.go
      lvm.go
```

Точное имя пакетов можно скорректировать во время рефакторинга, но внешний протокол на первом этапе менять не нужно.

## Ближайший план

### Этап 1 — укрепить фундамент

1. ⏭ Разделить большой updater helper на update и storage части.
2. ⏭ Сохранить текущий Unix socket и protocol compatibility.
3. ⏭ Добавить/усилить unit tests для routing и privileged operation validation.
4. ⏭ Прогнать полный `core-ci`.
5. 🧪 Провести реальный rollback с `0.1.58-dev` на предыдущую версию и снова обновиться вперёд.
6. ⏭ Проверить Storage UI после cleanup и защиту destructive операций.

### Этап 2 — файловое хранилище

1. Спроектировать File Storage / NAS поверх существующего storage-фундамента.
2. Добавить storage pools и управление томами.
3. Добавить сетевые shares и права доступа.
4. Добавить snapshots/backup policy и мониторинг заполнения/состояния.

### Этап 3 — камеры и NVR

1. Добавить сущность Camera и управление источниками RTSP.
2. Реализовать live view.
3. Реализовать запись и архив.
4. Добавить retention policy и события.
5. Подготовить hooks для motion/object/AI vision анализа.

### Этап 4 — Smart Home

1. Определить единую модель device/entity/state.
2. Добавить локальные интеграции и протоколы устройств.
3. Реализовать event bus.
4. Реализовать сценарии и automation engine.
5. Связать устройства с security permissions и audit.

### Этап 5 — собственный AI Agent

1. Создать agent runtime внутри платформы.
2. Добавить адаптеры локальных LLM/vision моделей.
3. Дать агенту строго разрешённые tools для Home-AI-Core.
4. Добавить память/контекст дома, камер, файлов и серверов.
5. Добавить human confirmation для опасных действий.

### Этап 6 — кластер Home-AI-Core

1. Обнаружение узлов в локальной сети.
2. Безопасное взаимное доверие и идентификация узлов.
3. Heartbeat и общий inventory CPU/RAM/GPU/storage.
4. Scheduler и placement задач.
5. Распределение AI inference, video processing и storage workloads.
6. Health-aware migration/failover там, где это допустимо.
7. Единое отображение нескольких серверов как одной системы в Web UI.

### Этап 7 — автономность и production hardening

1. Stable/dev release channels.
2. Подписанные release manifests/artifacts.
3. Disaster recovery и backup конфигурации.
4. Failure/integration tests для обновлений, storage, cameras, cluster и agent.
5. Проверка работы ключевых функций при отсутствии Internet.


## Долгосрочные критерии готовности

Home-AI можно считать достигшим основной цели, когда выполняются все условия:

- система устанавливается на один сервер и работает полностью локально;
- умный дом продолжает выполнять базовые автоматизации без Internet;
- камеры доступны локально, запись и архив не требуют облачного сервиса;
- файлы хранятся и управляются локально через Home-AI;
- собственный AI-агент может работать на локальной модели и управлять разрешёнными функциями;
- второй и последующие серверы автоматически или управляемо присоединяются к локальному кластеру;
- задачи могут размещаться на подходящих узлах в зависимости от CPU/GPU/storage ресурсов;
- отказ одного дополнительного узла не разрушает весь control plane;
- Web UI показывает весь дом, камеры, хранилище, AI и серверы как одну систему;
- внешние cloud-сервисы могут использоваться как опция, но не являются обязательной основой работы.

## Техдолг и риски

### ⚠️ Root-helper слишком большой

Главный текущий архитектурный долг. Изменения storage-кода могут случайно затронуть update-код и наоборот.

**Решение:** extraction refactor без изменения поведения.

### ⚠️ Update и storage делят один privileged protocol

Это не ошибка само по себе, но package naming и границы ответственности становятся менее очевидными.

**Решение:** сначала разделить внутреннюю реализацию, затем отдельно решить, нужно ли переименовывать protocol package.

### 🧪 Rollback реализован, но требует отдельного живого acceptance test

Код, backup metadata, helper operation и Web flow существуют.

Нужно подтвердить полный сценарий:

```text
0.1.58-dev
  ↓ rollback
previous version
  ↓ update
0.1.58-dev or newer
```

### ⚠️ Production signing ещё не завершён

Bundle проверяется через SHA-256 и manifest, но для будущего stable-канала нужен отдельный механизм подписи release manifest/artifacts.

### ⚠️ Нужны failure tests

Стоит отдельно тестировать:

- потерю сети во время download;
- повреждённый archive/checksum;
- неполный bundle;
- отказ systemd restart;
- несовместимый helper protocol;
- прерывание установки;
- rollback после неуспешного старта новой версии.

## Контрольные точки релизов

| Версия | Состояние | Значение |
|---|---|---|
| `0.1.57-dev` | Историческая | Последний release до cleanup-прохода |
| `0.1.58-dev` | ✅ Подтверждена | Первая версия после cleanup, успешно обнаружена и установлена через Web |

## Правило ведения карты

После каждого заметного этапа:

1. обновить текущую версию;
2. перенести завершённые задачи в **Сделано и проверено**;
3. оставить только один основной блок **Сейчас в работе**;
4. обновить **Ближайший план**;
5. добавить новые риски в **Техдолг и риски**;
6. для архитектурных решений при необходимости создавать отдельный ADR.

Эта карта описывает **фактическое состояние проекта**, а не список пожеланий. Новая функция считается готовой только после реализации и проверки.
