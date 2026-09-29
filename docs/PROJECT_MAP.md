# Home-AI-Core — карта проекта

> Живой документ состояния проекта. Обновлять после завершения заметных этапов, изменения архитектуры или подтверждённого релиза.

**Текущая подтверждённая версия:** `0.1.58-dev`  
**Состояние:** рабочая dev-версия, Web-обновление проверено end-to-end  
**Последнее обновление карты:** 2026-09-29

## Цель проекта

Home-AI-Core — локальный control plane для управления домашним AI-сервером через Web-интерфейс.

Основные направления:

- системная информация и мониторинг;
- управление накопителями и файловыми системами;
- безопасные привилегированные операции через отдельный root-helper;
- управление модулями;
- авторизация, роли и аудит;
- Web-интерфейс;
- обновление Core/Web/helper без переустановки Debian-пакета;
- воспроизводимая сборка, CI и выпуск update-bundle для amd64/arm64.

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
| System info | ✅ Готово | CPU, RAM, GPU, kernel, architecture, network, canonical block tree | Расширять метрики только при практической необходимости |
| Storage | ✅ Готово | Единое дерево устройств, inspect, mount/unmount, format, partition operations, labels, SMART/LVM integration | Усилить тесты защитных ограничений и UX опасных операций |
| Security / Auth | ✅ Готово | Bootstrap owner, cookie session, CSRF, permissions, audit, logout/session revoke | Продолжать security regression tests |
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

1. ⏭ Разделить большой updater helper на update и storage части.
2. ⏭ Сохранить текущий Unix socket и protocol compatibility.
3. ⏭ Добавить/усилить unit tests для routing и privileged operation validation.
4. ⏭ Прогнать полный `core-ci`.
5. 🧪 Провести реальный rollback с `0.1.58-dev` на предыдущую версию.
6. 🧪 После rollback снова обновиться вперёд и подтвердить полный recovery cycle.
7. ⏭ Проверить Storage UI после cleanup:
   - диски и разделы;
   - mount/unmount;
   - format;
   - создание/удаление разделов;
   - labels;
   - SMART;
   - LVM;
   - защита от опасных операций.
8. ⏭ Улучшить Web UX для длительных и destructive операций.
9. 🕓 Подготовить правила stable/dev channel, когда dev-цикл станет достаточно устойчивым.

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
