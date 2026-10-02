# Home-AI — карта проекта

> Рабочая карта фактического состояния проекта.  
> Каноническая конечная цель: [PRODUCT_VISION.md](PRODUCT_VISION.md)  
> Аудит уже сделанного: [CURRENT_STATE_AUDIT.md](CURRENT_STATE_AUDIT.md)

**Последняя версия, на которой пользователь подтвердил работу сетевого UI:** `0.1.59-dev`  
**Последняя версия, на которой пользователь подтвердил storage mount + создание файлового хранилища:** `0.1.102-dev`  
**Последняя версия, на которой пользователь подтвердил полный `update → rollback → re-update` и сеть после reboot:** `0.1.109-dev`  
**Последний опубликованный релиз:** `0.1.119-dev` — **AI Local Conversation v1**: persistent per-user chat, optional local Ollama provider и Web AI Agent\
**Текущий срез:** `0.1.120-dev` — управление **AI Agent** из раздела Модули: enable / disable / restart, persistent state, runtime cancellation, permission + audit\
**Следующий engineering milestone:** завершить `0.1.120-dev` и live-проверить управление AI Agent из Модулей; затем подключить локальную модель и перейти к **streaming + controlled model tool-loop**. Windows UI acceptance и release-hardening gates продолжаются параллельно\
**Состояние:** **CORE FOUNDATION COMPLETE** с 2026-10-01. AI Agent развивается как first-party module: contracts `0.1.115`, read tools/API `0.1.117`, local conversation `0.1.119`, runtime module control `0.1.120`. Automatic AI tool-loop пока намеренно выключен.\
**Обновлено:** 2026-10-01

### Кандидат 0.1.120-dev — AI Agent module runtime control

- ✅ В карточке **AI Agent** раздела **Модули** добавлены **Включить / Выключить / Перезапустить**.
- ✅ Добавлено отдельное право `modules.manage`; read-only `modules.read` не позволяет управлять runtime.
- ✅ Migration 019 выдаёт `modules.manage` administrator/owner profile и сохраняет совместимость существующих установок.
- ✅ Статус модуля хранится в SQLite; `disabled` переживает reboot/restart Core.
- ✅ Первый `registered` AI Agent нормализуется в `enabled`; незавершённый `restarting` восстанавливается при старте.
- ✅ Disable отменяет активные AI requests, скрывает AI capabilities и блокирует chat/tools, не останавливая Home-AI-Core.
- ✅ Restart отменяет активные AI requests и пересоздаёт только runtime AI Agent.
- ✅ API: `POST /api/v1/modules/{moduleID}/control` + CSRF + `modules.manage`.
- ✅ Audit: `module.runtime.control`; realtime: `module.runtime.changed`.
- ✅ Disabled AI возвращает explicit `ai_agent_disabled`.
- ✅ Tests покрывают persistence, capability removal, disable/enable/restart/cancellation и permission denial.
- 🧪 Требуется PR/main CI, release workflow и live acceptance на сервере.

### Выпуск 0.1.119-dev — AI Local Conversation v1

- ✅ Migration 018 добавляет persistent AI conversations/messages, привязанные к конкретному Home-AI user.
- ✅ State store проверяет ownership на чтении истории и добавлении сообщений; чужой conversation ID fail-closed.
- ✅ Добавлен optional local Ollama provider через HTTP без обязательной runtime/vendor зависимости.
- ✅ Provider конфигурируется через `HOME_AI_AI_PROVIDER / ENDPOINT / MODEL`; без provider Core продолжает работать, чат показывает disabled state.
- ✅ Conversation API: list/create + messages/history + message generation.
- ✅ Prompt/message/context limits и server-side provider timeout ограничивают ресурсы.
- ✅ Audit `ai.chat.generate` хранит provider/model/duration/размеры, но не текст prompt/response.
- ✅ Web получил отдельный **ИИ Агент / AI Agent**: список диалогов, история, новый чат, provider/model status, RU/EN.
- ✅ Module capability расширен до `ai.chat`.
- ✅ Automatic model-driven tool execution в этом срезе **не включён**; existing tools остаются под explicit API permission boundary.
- ✅ Добавлены tests state/provider/service/API isolation.
- ✅ Сохранён опубликованный Windows visual fix `0.1.118-dev`.
- ✅ PR #113 прошёл полный Core/Web, Windows и Debian CI; main CI также зелёный.
- ✅ Release workflow опубликовал `v0.1.119-dev` с Core amd64/arm64, Windows `.exe`, Debian `.deb` amd64/arm64 и SHA-256.
- 🧪 Требуется live acceptance на реальном HOME AI сервере с настроенным локальным provider/model.
- ⏭ Следом: streaming response + controlled model tool-loop, где read tools идут через permissions, а change/sensitive — через approval.

### Выпуск 0.1.118-dev — Windows button rendering fix после live screenshot

- ✅ Live screenshot `0.1.116-dev` использован как фактическая visual acceptance-проверка; выявлены белые углы/линии у sidebar owner-draw buttons, двойная рамка primary action и обрезанный `HOME AI` wordmark.
- ✅ Причина white-corner artifacts: прямоугольное Win32 child-window `BUTTON` оставляло native button-face background вне нашего `RoundRect`.
- ✅ Все owner-draw buttons получают rounded window region с Figma radius `10 px` и перед рисованием очищают client area цветом реального parent surface.
- ✅ Overview hero actions получили отдельные hero primary/secondary roles, поэтому углы смешиваются с hero surface, а не с белым card background.
- ✅ Primary buttons больше не получают лишний light border.
- ✅ XOR `DrawFocusRect` заменён на стабильный rounded cyan keyboard-focus ring; Overview больше не ставит focus на primary action автоматически при первом показе.
- ✅ Sidebar selected row использует более близкий к Figma alpha-composited fill/border; nav text теперь left-aligned с Figma inset `30 px`.
- ✅ `HOME AI` wordmark больше не должен обрезаться: отдельный brand font + расширенный text slot.
- ✅ Сохранены Tab/Shift+Tab/Ctrl+Tab, arrow navigation, sync/Credential Manager/background agent/tray/updater и AI Agent functionality.
- ✅ После VERSION/docs bump PR и `main` прошли native Windows tests, Windows cross-build, Debian installer checks и полный `core-ci`.
- ✅ Release workflow опубликовал `v0.1.118-dev` с Windows `.exe`, Core update bundles amd64/arm64 и Debian `.deb` amd64/arm64 + SHA-256.
- 🧪 После публикации нужен новый live screenshot на реальном Windows для подтверждения: sidebar corners, selected row, primary/secondary hero buttons, focus state и `HOME AI` wordmark.
### Выпуск 0.1.117-dev — AI Agent read tools + API

- ✅ Добавлены реальные read-only tools: `core.system.status`, `core.jobs.list`, `core.modules.list`.
- ✅ Tool discovery фильтруется по effective permissions текущего пользователя.
- ✅ Перед каждым execute права пользователя проверяются повторно через Tool Registry.
- ✅ `core.jobs.list` не отдаёт AI raw job `input/result` payloads; только status/progress/error summary.
- ✅ Добавлены `/api/v1/ai/status`, `/api/v1/ai/tools`, `/api/v1/ai/tools/<tool>/execute`.
- ✅ Cookie POST сохраняет CSRF boundary Core; bearer/token path использует обычную authentication boundary.
- ✅ Каждое выполнение ограничено server-side timeout.
- ✅ Audit пишет `ai.tool.execute` для success/denied/failed без raw AI input.
- ✅ Regression tests покрывают permission-filtered discovery, success, denial, audit и payload redaction.
- ✅ PR #110 прошёл полный `core-ci`; main CI также завершился успешно.
- ✅ Release workflow опубликовал `v0.1.117-dev` с Core update bundles amd64/arm64, Windows `.exe`, Debian `.deb` amd64/arm64 и SHA-256.
- ⏭ Следующий срез — local model provider + persistent sessions + streaming Web chat.

### Выпуск 0.1.116-dev — compact Figma Windows client

- ✅ Источник визуальной спецификации: Figma `HOME AI Windows Client — Compact Main Window`, file `14TuUh70V7yoRTBg06QyeH`, node `2:2`.
- ✅ Native окно уменьшено с `1180×760` до `976×635`; рабочая client-area рассчитана под Figma content `960×596` с сохранением стандартного Windows title bar.
- ✅ Sidebar приведён к Figma: ширина `196 px`, фон `#070B14`, HOME AI icon, blue/cyan logo accents, compact navigation и status dots.
- ✅ Overview hero перенесён в logo-style: navy→blue GDI gradient, cyan outline, зелёный status circle + white check, compact primary/secondary actions.
- ✅ Геометрия четырёх status cards, **Последние действия**, **Резервные копии** и **Место на HOME AI** перенесена в компактную сетку Figma.
- ✅ Connection / Synchronization / Backups / Settings уплотнены под новое окно; controls не требуют старого размера `1180×760`.
- ✅ Overview connection/folder данные разделены на value + hint, чтобы host и folder count не ломали compact cards.
- ✅ Сохранены RU/EN, Tab/Shift+Tab/Ctrl+Tab, arrow navigation, Credential Manager, profiles, queue/copy, background agent, tray, updater и Debian release path.
- ✅ Используется существующий embedded HOME AI brand icon; временные Figma asset URLs в код не добавлялись.
- ✅ Regression checks фиксируют compact geometry `976×635`, sidebar `196` и RU folder-count formatting.
- ✅ После rebase на опубликованный `0.1.115-dev` PR и `main` прошли native Windows tests, Windows cross-build, Debian installer checks и полный `core-ci`.
- ✅ Release workflow опубликовал `v0.1.116-dev` с Windows `.exe`, Core update bundles amd64/arm64 и Debian `.deb` amd64/arm64 + SHA-256.
- 🧪 Требуется live screenshot/acceptance на реальном Windows: размер окна, DPI/scaling, sidebar/logo palette, hero gradient, cards и все вкладки.
### Выпуск 0.1.115-dev — AI Agent Foundation

- ✅ Добавлен first-party module `ai.agent`, регистрируемый Core при старте.
- ✅ Module Registry теперь объявляет capabilities `ai.agent` и `ai.tools`.
- ✅ Добавлен typed Tool Registry со stable tool/module IDs и JSON input metadata.
- ✅ Каждый tool объявляет требуемые Core permissions и optional exact resource scope.
- ✅ Перед исполнением permission/scope проверяются заново через effective user access.
- ✅ Side-effect classes: `read`, `change`, `sensitive`; change/sensitive требуют явного approval.
- ✅ Добавлен model-provider interface без привязки к конкретному vendor/cloud.
- ✅ Добавлен deterministic provider для contract tests.
- ✅ Tests покрывают manifest validation, allow/deny, scoped grant, approval и cancellation.
- ✅ ADR-0036 фиксирует запрет generic root/shell bypass и ownership tools за domain modules.
- 🚧 Следующий кусок этого же foundation: read-only Core tools (system/jobs/modules), Audit и AI API.
- 🚧 После API — local provider adapter + streaming Web chat.
- 📄 План: [AI_AGENT_FOUNDATION.md](AI_AGENT_FOUNDATION.md).
- ✅ PR #108 прошёл полный `core-ci`; main CI и release workflow также зелёные.
- ✅ GitHub Release `v0.1.115-dev` опубликован с Core amd64/arm64, Windows `.exe`, Debian `.deb` amd64/arm64 и SHA-256.

### Выпуск 0.1.114-dev — standalone Storage Preflight

- ✅ В карточку физического диска добавлена отдельная кнопка **Проверить безопасность / Safety check**.
- ✅ Web вызывает новую API operation `preflight`; пользователь не вводит destructive confirmation и не запускает format/delete.
- ✅ Сервер **принудительно** преобразует `preflight` в существующий destructive planner с `dry_run=true`, даже если клиент не передал dry-run.
- ✅ У standalone preflight нет пути к реальному форматированию или удалению разделов.
- ✅ Повторно используются существующие защиты: system disk, Home-AI file-pool usage, mounts, active swap и cross-disk LVM.
- ✅ Успешная проверка возвращает read-only план; ошибки безопасности показываются через существующие RU/EN friendly messages.
- ✅ Audit получает отдельное событие `storage.preflight.plan`.
- ✅ Добавлен regression test, доказывающий, что API preflight всегда принудительно остаётся dry-run.
- ✅ PR #105 прошёл полный `core-ci`; `main` CI и release workflow завершились успешно.
- ✅ GitHub Release `v0.1.114-dev` опубликован с Core update bundles amd64/arm64, Windows `.exe`, Debian `.deb` amd64/arm64 и SHA-256.
- ✅ Live acceptance standalone Storage Preflight подтверждён пользователем 2026-10-01; проверка работает на установленном сервере без destructive execution.

### ✅ CORE FOUNDATION COMPLETE — 2026-10-01

- ✅ Core API, identity, RBAC/scoped permissions, audit, jobs, events, WebSocket, modules, hardware inventory и privileged-helper boundaries завершены.
- ✅ Update safety подтверждён automated tests и live циклом `update → rollback → re-update`.
- ✅ Active Home-AI network configuration переживает reboot и возвращает connectivity.
- ✅ WireGuard tunnel/peer workflow, reboot persistence, autostart и peer/handshake visibility подтверждены пользователем на живом сервере.
- ✅ Stable/RC Core update verification fail-closed через detached Ed25519; trust-root contract реализован и протестирован.
- ✅ Initial Debian installer acceptance подтверждён пользователем.
- 🔐 Реальный production signing key + установка pinned public trust root остаются **stable-release operational gate**, а не незавершённой функцией Core.
- ✅ Live storage dry-run/preflight acceptance закрыт на `0.1.114-dev`; long-duration updater interruption/recovery tests остаются release-hardening check.
- 🧪 Windows visual/DPI/tray acceptance относится к Windows client product work и не переоткрывает Core Foundation.

### Выпуск 0.1.113-dev — Windows dashboard polish после live acceptance

- ✅ Live-скриншот `0.1.111-dev` использован как фактическая acceptance-проверка: layout запускается, но визуально не принят из-за локализации/типографики/controls.
- ✅ Восстановлен полный русский словарь dashboard: **Последние действия / Резервные копии / Место на HOME AI / Объём недоступен** и остальные новые строки больше не fallback на English.
- ✅ Добавлен regression test равенства RU/EN translation keys.
- ✅ Overview action buttons и sidebar переведены на owner-draw rounded controls; сохранён keyboard focus и navigation.
- ✅ Зелёный status circle + белая галочка рисуются напрямую GDI и больше не перекрываются STATIC control.
- ✅ Уменьшены dashboard fonts и card copy; server URL показывается компактным host label, **Последняя синхронизация** использует короткое **Ещё не было**.
- ✅ Пустой **Последние действия** показывается одной строкой вместо трёх повторов; техническая нижняя строка **Состояние / Готово** скрыта на Overview.
- ✅ При старте клиент тихо проверяет сохранённые server/user credentials из Windows Credential Manager; успешный login обновляет Connection и storage usage без ручного нажатия **Подключить**.
- ✅ Состояния storage разделены: нет подтверждённого подключения vs сервер подключён, но usage не предоставлен.
- ✅ Сохранено `© 2026 TexNik` из `0.1.112-dev`; Core/Windows sync engine/Debian release path не откатывались.
- ✅ После rebase на актуальный `main` native Windows tests, Windows cross-build, Debian installer checks и полный `core-ci` зелёные; release workflow опубликовал `v0.1.113-dev` с Windows `.exe`, Core bundles и Debian `.deb` amd64/arm64 + SHA-256.
- 🧪 Требуется повторный скриншот/проверка `0.1.113-dev` на реальном Windows: RU UI, DPI/scaling, rounded buttons, sidebar, check mark, connection/storage cards и tray.
### Выпуск 0.1.112-dev — copyright / author attribution TexNik

- ✅ Добавлен корневой `NOTICE` с `Copyright 2026 TexNik`.
- ✅ Apache-2.0 copyright notice в `LICENSE` приведён к `TexNik`; условия лицензии не меняются.
- ✅ README и Web package metadata содержат явное авторство `TexNik` и Apache-2.0.
- ✅ Web UI показывает `© 2026 TexNik` после входа и на экранах login/first-run.
- ✅ Native Windows client показывает тот же copyright в постоянной боковой панели.
- ✅ Debian initial installer содержит `/usr/share/doc/home-ai-core/copyright` с attribution `TexNik`.
- ✅ PR #100 прошёл полный `core-ci`; release workflow опубликовал `v0.1.112-dev` с Core amd64/arm64, Windows `.exe` и Debian `.deb` amd64/arm64 + SHA-256.
- ✅ PR #102 синхронизировал карту проекта с фактом публикации `0.1.112-dev`.

### Сводка завершённых работ 0.1.109–0.1.112

- ✅ **Core update safety:** negative bundle tests, полный live `update → rollback → re-update`, сохранение login/System/state и повторный нормальный запуск.
- ✅ **Network persistence:** после reboot сеть подтверждена рабочей на живом сервере; WireGuard reboot/autostart/handshake acceptance также подтверждён.
- ✅ **Security/update signing:** detached Ed25519 verification и fail-closed для stable/RC реализованы; private signing key не хранится в репозитории/на node.
- ✅ **Storage safety:** destructive storage Web actions получили read-only dry-run/preflight до подтверждённого изменения.
- ✅ **Windows client:** утверждённый HOME AI dashboard, sidebar navigation, cards, tray popup, сохранённый sync/agent/Credential Manager engine.
- ✅ **Initial Debian installer:** amd64/arm64 `.deb` теперь собираются/инспектируются в CI и публикуются versioned вместе с релизом.
- ✅ **Project attribution:** `TexNik` закреплён в NOTICE/LICENSE/README, Web UI, Windows UI и Debian package metadata.
- ✅ **Release pipeline:** `0.1.109-dev` → `0.1.112-dev` опубликованы с требуемыми Core/Windows/Debian artifacts и SHA-256.
- 🧪 После закрытия Core остаются отдельные product/release checks: Windows visual/DPI/tray acceptance, signing-key/trust-root provisioning и long-duration updater failure testing. Storage preflight live acceptance закрыт на `0.1.114-dev`.

### Выпуск 0.1.111-dev — Windows visual UI + initial Debian installer

- ✅ Windows-клиент перенесён на утверждённый HOME AI dashboard: светлый Windows 11-style фон, боковое меню, крупный статус, rounded cards, блоки **Последние действия / Резервные копии / Место на HOME AI**.
- ✅ Карточки используют реальные данные существующего sync engine: состояние профилей, последний/следующий запуск, локальную HOME AI папку, agent/autostart и доступные server-side usage/quota значения.
- ✅ Обычный левый клик по tray открывает компактный HOME AI status popup с текущим состоянием, последними sync-операциями и быстрыми действиями; правый клик сохраняет context menu.
- ✅ Сохранены Tab/Shift+Tab/Ctrl+Tab navigation, Credential Manager, sync profiles, queue/copy, single-instance background agent, Explorer-open и client update path.
- ✅ Добавлены native Windows tests visual resources/dashboard/tray snapshot; PR и `main` прошли native Windows tests, Windows cross-build и полный `core-ci`.
- ✅ Initial Debian installer больше не является только ручным workflow: обычный `core-ci` собирает и инспектирует amd64/arm64 `.deb`.
- ✅ Каждый push нового `VERSION` теперь собирает amd64/arm64 initial Debian `.deb`, создаёт SHA-256 и передаёт их в общий GitHub Release вместе с Core update bundles и Windows `.exe`.
- ✅ Сохранён release signing из `0.1.110-dev`: Debian job добавлен поверх актуального workflow без удаления **Sign Core update checksums** / Ed25519 trust path.
- ✅ Release workflow опубликовал `v0.1.111-dev`: Core update bundles amd64/arm64, Windows `.exe`, `home-ai-core_0.1.111.dev_amd64.deb`, `home-ai-core_0.1.111.dev_arm64.deb` и соответствующие SHA-256.
- 🧪 Требуется визуальная проверка на реальном Windows: геометрия карточек, DPI/scaling, RU/EN тексты, Tab navigation и tray popup.
- ✅ Первоначальная установка опубликованного Debian `.deb` подтверждена пользователем на живой системе; installer acceptance закрыт.
- 🚧 Точное расписание по дням/часам и отдельный Explorer virtual-drive/provider этим срезом не заявляются; текущий scheduler остаётся interval-based.
### Выпуск 0.1.110-dev — security / repository hardening

- ✅ Добавлена лицензия Apache-2.0 для публичного репозитория.
- ✅ Core updater получил detached Ed25519 verification checksum metadata; stable/RC release без подписи больше не принимается.
- ✅ Release workflow умеет подписывать Core checksum-файлы через GitHub Actions secret `HOME_AI_UPDATE_SIGNING_KEY`; приватный ключ не хранится в репозитории и на Home-AI node.
- ✅ Зафиксирован trust root `/etc/home-ai-core/update-trusted.pub` и отдельный operational document по генерации/ротации ключа.
- ✅ Исправлен stale `helper_protocol: 2` в генераторе update bundle; новый helper contract — protocol v5.
- ✅ Добавлен read-only storage dry-run/preflight для mount/unmount/partition/format/label paths; destructive preflight отдельно проверяет system disk, mounts, active swap, LVM и доступность filesystem tools.
- ✅ Web перед format/create/delete/delete-all сначала выполняет dry-run и только после успешного preflight запускает подтверждённую операцию.
- ✅ Старые PR #34, #55 и #67 проверены против актуального `main` и закрыты как superseded; PR #91 уже слит.
- ✅ PR #95 прошёл полный `core-ci`; release workflow опубликовал `v0.1.110-dev` с Core amd64/arm64 bundles и Windows `.exe`.
- 🧪 Требуется live acceptance storage preflight на реальном сервере.
- 🔐 Проверено фактически: `HOME_AI_UPDATE_SIGNING_KEY` сейчас не настроен, поэтому `0.1.110-dev` опубликован unsigned без `.sig` assets. Для включения подписей нужно один раз создать dedicated Ed25519 key pair, загрузить private PEM в GitHub Actions secret и установить public key на узлы. Stable/RC без этого fail-closed.

### Выпуск 0.1.109-dev — Core 1.0 readiness

- ✅ Добавлен единый [Core 1.0 readiness gate](CORE_1_0_READINESS.md), который отделяет автоматические engineering checks от живых disruptive acceptance-проверок.
- ✅ Update hardening получил негативные regression tests: tampered payload, unexpected archive file и path traversal должны fail-closed; valid bundle остаётся положительным control case.
- ✅ Критерий завершения Core теперь формальный: live rollback/re-update, DHCP/static persistence, WireGuard persistence и stable signing должны быть закрыты до статуса **CORE FOUNDATION COMPLETE**.
- ✅ Live update → rollback на `0.1.109-dev` подтверждён пользователем 2026-10-01: после отката Core/Web/login/state сохранили работоспособность.
- ✅ Re-update до `0.1.109-dev` после rollback подтверждён пользователем; нормальная работа сохранена.
- ✅ Сеть после reboot подтверждена рабочей; конкретный активный режим DHCP/static в этом прогоне не зафиксирован и не выводится предположением.
- ✅ WireGuard tunnel/peer, reboot persistence, autostart и handshake visibility подтверждены пользователем; live gate закрыт.
- ✅ PR #92 прошёл полный `core-ci`; release workflow опубликовал `v0.1.109-dev` с Core amd64/arm64 bundles и Windows `.exe` + SHA-256.
- ✅ PR #96 и #99 зафиксировали live acceptance: update/rollback/re-update и network-after-reboot результаты внесены в canonical Core acceptance docs.
- ✅ Stable-channel Ed25519 verification/fail-closed код реализован в `0.1.110-dev`; до production/stable остаётся operational provisioning dedicated signing key + trust root.

### Выпуск 0.1.108-dev — отображение занятого места папок

- ✅ Для **Занято / лимит** сохраняется текущий точный `UsageOf()` внутри Core как основной путь.
- ✅ Если Core не может прочитать managed folder из service sandbox, privileged helper считает apparent-size конкретной папки через host `du -B1`.
- ✅ Temporary upload parts `.home-ai-upload-*` исключаются из host-`du` fallback.
- ✅ Fallback используется не только в таблице папок, но и для folder quota checks и private-user quota totals.
- ✅ Новая read-only helper operation защищена повышением storage-helper protocol version.
- 🧪 Требуется live-проверка папки **Фото**: ожидается реальное значение, например `0 Б / Без лимита` для пустой папки, вместо **Объём недоступен**.

### Выпуск 0.1.107-dev — лёгкий Windows-клиент HOME AI

- ✅ Старое перегруженное Win32-окно разделено на **Обзор / Подключение / Синхронизация / Резервные копии / Настройки** без изменения существующего sync engine и формата профилей.
- ✅ **Tab / Shift+Tab** переведены на штатную Win32 dialog navigation через `IsDialogMessageW`; **Ctrl+Tab / Ctrl+Shift+Tab** переключают разделы, а боковая radio-navigation поддерживает клавиши со стрелками.
- ✅ В главное окно и системный tray встроен существующий фирменный icon HOME AI; generic Windows application icon больше не является основным.
- ✅ Tray упрощён: обычный левый клик открывает меню, доступны **Синхронизировать**, **Открыть локальную папку**, **Настройки**, **Журнал**, **Выход**.
- ✅ На **Обзоре** показываются только основные пользовательские состояния: подключение, число активных папок, фоновая синхронизация, следующий запуск и последний успешный результат.
- ✅ Сохранены Credential Manager, background agent, sync profiles, manual sync, queue/copy engine, stable install/update path и существующая conflict policy.
- ✅ PR #88 после rebase на актуальный `main` прошёл native Windows tests, Windows cross-build и полный `core-ci`; release workflow опубликовал `v0.1.107-dev` с Core amd64/arm64 bundles и Windows `.exe` + SHA-256.
- 🧪 Реальная Windows acceptance нового вида, клавиатурной навигации, фирменного icon/tray и Explorer-open остаётся пользовательской проверкой.
- 🚧 Scheduler в этом срезе остаётся interval-based; точное расписание **дни + время** — следующий Windows-срез.
- 🚧 HOME AI пока открывает существующую локальную синхронизируемую папку в Проводнике; отдельный виртуальный диск/provider в Explorer этим релизом не заявляется.

### Выпуски 0.1.92–0.1.106-dev — стабилизация Files / Storage на живом сервере

- **0.1.92-dev** — добавлена первая адресация подразделов Files через hash-route для **Хранилище** и **Доступ Windows**.
- **0.1.93-dev** — hash-route заменён на реальные SPA-маршруты `/files/storage` и `/files/windows`; добавлены route tests и integration smoke.
- **0.1.94-dev** — updater получил retry transient HTTP 408/429/502/503/504 и network errors, увеличенный timeout и сохранение SHA-256/size verification.
- **0.1.95-dev** — исправлен blank screen **Файлы → Хранилище** при `mountpoints: null`; API теперь стабильно сериализует пустые mountpoints как `[]`, Web терпит legacy/null.
- **0.1.96-dev** — исправлена битая UTF-8/Windows mojibake-кодировка в Files storage UI (`вЂ—`, `В·` и аналогичные символы).
- **0.1.97-dev** — добавлена кнопка **Монтировать** прямо в **Файлы → Хранилище**, автоподстановка mountpoint в создание pool; mount сначала пробует quota options и при несовместимости повторяет обычный mount.
- **0.1.98-dev** — после провала mount добавлена read-only диагностика filesystem: ext2/3/4 через `e2fsck -n`, XFS через `xfs_repair -n`, FAT через `fsck.fat -n`; automatic repair/reformat запрещён.
- **0.1.99-dev** — mount стал идемпотентным, helper ждёт видимость результата через `findmnt`, API возвращает canonical mountpoint, Web сразу отражает его и локализует типовые storage-сообщения на русский.
- **0.1.100-dev** — усилена проверка backing storage при создании Files pool: root должен быть реальной mount boundary и соответствовать назначенному block device.
- **0.1.101-dev** — проверка mount backing переведена на Linux `/proc/self/mountinfo` с major:minor device ID вместо зависимости только от `lsblk`/stat heuristics.
- **0.1.102-dev** — исправлен корень mount-проблемы: privileged updater helper работал в отдельном systemd mount namespace. `mount`, `umount`, `findmnt` теперь выполняются через PID 1 host mount namespace с `nsenter`. На живом сервере пользователь подтвердил успешное создание Files storage pool.
- **0.1.103-dev** — для Files pool добавлен fallback ёмкости: сначала live `statfs(root_path)`, при недоступности — сохранённый backing device/UUID, block-device size и privileged storage inspection для free space. Тот же fallback используется в reserve enforcement. Пользовательская проверка **«Свободно / Всего»** остаётся открытой.
- **0.1.104-dev** — если capacity всё равно недоступна на живом сервере, Files UI больше не показывает неоднозначный `—`: в pool capacity и назначенном storage выводится явное **«Объём недоступен»**.
- **0.1.105-dev** — найден следующий namespace-разрыв: mount уже выполнялся в host namespace, но filesystem inspection (`lsblk FSAVAIL/MOUNTPOINTS`) всё ещё работал внутри sandbox helper-а. Inspection переведён в PID 1 host mount namespace.
- **0.1.106-dev** — capacity переведена на другой механизм: helper через host `findmnt` определяет реальную точку монтирования и читает `Total/Available` через `df -B1`; `lsblk` и offline inspection остаются fallback. Storage inspection cache теперь сбрасывается сразу после операций с диском. На живом сервере пользователь подтвердил корректное отображение **Свободно / Всего**.
- ✅ Для всей цепочки сохранены CI, Go/Web tests, Linux amd64/arm64 cross-build и versioned release assets.

### Выпуск 0.1.91-dev — завершение пользователей и управления файлами

- ✅ Один стабильный user ID, редактирование логина/имени, пароль своей учётной записи и административный сброс пароля.
- ✅ Смена пароля/логина завершает старые входы; отключение пользователя отзывает все сессии. Последний включённый администратор защищён внутри транзакции.
- ✅ Папки: отдельный редактор имени, доступа и лимита; личный владелец сохраняет свои права, остальные grants назначаются явно.
- ✅ Лимит папки и общий лимит личных папок пользователя; корзина и незавершённые загрузки входят в учёт. Одновременные загрузки не могут занять одну квоту дважды.
- ✅ Файловые project quotas ext4/XFS ограничивают SMB-запись в папку. Личный лимит распределяется конечными лимитами всех личных папок; общий kernel reserve из 0.1.87-dev сохранён.
- ✅ Папки без личного/папочного лимита сохраняют прежнюю SMB-запись с защитой резерва. Конечный лимит без подтверждённой файловой квоты делает соответствующую SMB-папку read-only.
- ✅ Отзыв прав останавливает Samba и активные подключения до записи новых прав, затем автоматически перестраивает конфигурацию. При сбое доступ остаётся приостановленным; состояние видно в Web. При старте производится сверка.
- ✅ Web: **Файлы → Папки / Хранилище / Доступ Windows**, **Ваша учётная запись**, компактный редактор пользователей. Новый интерфейс физических дисков 0.1.90-dev сохранён.
- ✅ Приватные события фильтруются по актуальным grants; старые WebSocket-сессии закрываются; число подписок ограничено суммарно. Загруженный HTML/SVG выдаётся безопасным скачиванием.
- ✅ Миграция `017_nas_quotas.sql`; ADR-0035. Изолированный kernel test ext4 подтвердил запрет записи сверх квоты от обычного пользователя и снятие лимита.
- ✅ PR #71 вошёл в `main`; полный CI зелёный. Опубликован `v0.1.91-dev`: Core bundles amd64/arm64 и Windows-клиент с SHA-256. Пройдены race checks, 28 Web-тестов, native Windows tests, проверка манифестов и запуск упакованного Core.
- ✅ Скачанные опубликованные файлы `v0.1.91-dev` проверены отдельно: SHA-256 обоих Core bundles и Windows `.exe`, версия/архитектура манифестов и контрольные суммы всех упакованных файлов совпадают.
- ✅ Codex Security завершил статический аудит `internal/` исходной версии `0.1.86-dev` (`fd295109`): проверено 155 файлов, подтверждены 2 high и 3 medium. Исправления всех пяти вошли в [PR #71](https://github.com/DeadSoulf/home-ai-core/pull/71): безопасное скачивание активных документов, отзыв SMB-доступа, защита приватных событий, отзыв WebSocket-сессий и суммарный лимит подписок. Это аудит исходной версии, а не повторный аудит релиза после исправлений.
- 🧪 Установка на реальный сервер, открытые SMB-подключения, перезагрузка и Windows/NAS acceptance остаются эксплуатационной проверкой. NFS/snapshots остаются опциональными последующими этапами.

### Выпуск 0.1.90-dev — действия вне таблицы

- Действия физического диска перенесены из правой колонки в верхнюю карточку выбранного диска.
- Колонка **Действия** удалена из таблицы разделов/LVM выбранного диска.
- Для раздела/LVM добавлена раскрываемая панель управления непосредственно под его строкой.
- В панели собраны mount/unmount, изменение метки, назначение files/video и destructive actions.
- System-disk protection и usage-lock для занятого Files storage продолжают использовать прежние серверные проверки.
- Storage API не менялся; переработан только Web layout поверх существующей safety-логики.
- PR #70 прошёл полный CI; release workflow опубликовал `v0.1.90-dev` с amd64/arm64 Core bundles и Windows `.exe` + SHA-256.
- Практическая проверка нового расположения действий на установленном сервере остаётся acceptance step.

### Выпуск 0.1.89-dev — компактная таблица диска

- Таблица разделов/LVM внутри выбранного физического диска стала заметно компактнее.
- Суммарная default width уменьшена примерно с 1540 px до 1100 px.
- Колонка **Действия** уменьшена с 330 px до 180 px; остальные колонки также сжаты.
- Заголовки колонок могут переноситься на несколько строк, не растягивая всю таблицу.
- Уменьшены padding, размеры compact-кнопок и selector назначения.
- Ключ сохранённых ширин колонок переведён на v2, чтобы старые широкие значения localStorage не перекрывали новый default.
- PR #69 прошёл полный CI; release workflow опубликовал `v0.1.89-dev` с amd64/arm64 Core bundles и Windows `.exe` + SHA-256.
- Практическая проверка компактной таблицы на установленном сервере остаётся acceptance step.

### Выпуск 0.1.88-dev — новый интерфейс управления дисками

- **Система → Хранилище** больше не смешивает обзор всех устройств с destructive actions в одной широкой таблице.
- Первый экран показывает только физические диски с именем, моделью/transport, размером, partition table, unallocated space, system/SMART status.
- Выбор диска открывает отдельный focused view конкретного physical disk.
- Внутри выбранного диска остаются существующие разделы/LVM и все операции: purpose files/video, mount/unmount, rename, create, format и delete.
- Существующие server-side usage-lock и system-disk protections не меняются и продолжают блокировать опасные операции.
- Новый слой реализован поверх текущего StorageDevices, чтобы не дублировать storage API и проверенную safety-логику.
- PR #68 прошёл полный CI; release workflow опубликовал `v0.1.88-dev` с amd64/arm64 Core bundles и Windows `.exe` + SHA-256.
- Практическая проверка нового storage UI на установленном сервере остаётся acceptance step.

### Выпуск 0.1.87-dev — kernel hard reserve для SMB

- Managed Samba shares уже используют `force user = home-ai-core`; этот UID становится kernel quota boundary для прямых SMB-записей.
- Helper protocol v3 передаёт pool root и reserve policy в privileged helper.
- Hard limit рассчитывается как текущее quota-usage Home-AI + только свободное место выше configured reserve.
- Существующие данные вне `.home-ai` учитываются через live free-space, поэтому Home-AI не предполагает, что владеет всем диском.
- `setquota` применяет block hard limit, `repquota` проверяет фактическое usage/limit после записи.
- `smb.apply` fail-closed для non-zero reserve: Samba config не активируется, если kernel protection не готов.
- Изменение reserve пытается немедленно синхронизировать quota; Web отдельно показывает **Kernel enforced / Not ready** и конкретную причину.
- Samba installation устанавливает также пакет `quota`.
- Новые ext4 создаются с embedded user quota и `-m 0`, ext4 mounts получают `usrquota`, XFS — `uquota`.
- Legacy ext4/XFS автоматически не переформатируются и не live-remount: для них нужен controlled maintenance path.
- ADR-0034 и PR #65 прошли полный CI; release workflow опубликовал `v0.1.87-dev` с amd64/arm64 Core bundles и Windows `.exe` + SHA-256.
- Практическая проверка SMB quota на установленном сервере остаётся acceptance step.

### Выпуск 0.1.86-dev — резерв свободного места NAS

- Миграция `016_nas_pool_capacity_policy.sql` добавляет pool-wide hard reserve и warning threshold.
- Значения по умолчанию: **5% жёсткий резерв / 10% предупреждение**.
- Ёмкость читается live через Linux `statfs`, а раздел **Файлы** показывает free/total и состояние pool.
- Администратор может менять reserve (0–50%) и warning (0–95%; 0 отключает предупреждение).
- Direct uploads не могут временным файлом пересечь reserve.
- Resumable upload проверяется при создании и повторно перед каждым chunk.
- При нарушении резерва Core возвращает HTTP 507 `file_pool_reserve_reached`; Windows-клиент получает ту же серверную защиту.
- Move / корзина / restore остаются разрешены как same-filesystem rename.
- SMB hard reserve пока не заявляется: Samba пишет напрямую, поэтому следующий слой — filesystem/Samba quota enforcement.
- ADR-0033 и PR #63 прошли полный CI; release workflow опубликовал `v0.1.86-dev` с amd64/arm64 Core bundles и Windows `.exe` + SHA-256.
- Практическая проверка на установленном сервере остаётся acceptance step.

### Выпуск 0.1.85-dev — защита дисков и привязка NAS pool к filesystem

- Storage purpose API показывает `in_use` и `used_by` для физических storage.
- Занятый Files storage нельзя через Home-AI переназначить, снять purpose, размонтировать, форматировать или удалить.
- **Система → Хранилище** показывает, что раздел используется файловым хранилищем, и блокирует опасные действия.
- Usage-lock учитывает дочерние LVM volumes и backing storage tree.
- Миграция `015_nas_pool_storage_identity.sql` добавляет в NAS pool backing device path и filesystem UUID.
- Новый NAS pool создаётся только на точном mounted filesystem с purpose=`files`.
- UUID является основной устойчивой identity; device path используется как fallback.
- Защита продолжает узнавать pool после размонтирования или смены `/dev/...` имени.
- Старые pools без сохранённой identity продолжают работать через mount-path fallback.
- PR #60 и #61 прошли полный CI; release workflow опубликовал `v0.1.85-dev` с amd64/arm64 Core bundles и Windows `.exe` + SHA-256.
- Практическая проверка на установленном сервере остаётся acceptance step.

### Выпуск 0.1.84-dev — автоматическое обновление Windows-клиента

- Добавлена команда `client update`, которая сама получает список Home-AI GitHub releases.
- Выбирается только релиз с точной парой versioned Windows amd64 assets: `.exe` и `.sha256`.
- Release metadata, checksum и executable имеют жёсткие size limits; внешние URL требуют HTTPS.
- Скачанный executable сначала проверяется по опубликованному SHA-256, затем повторно проверяется после помещения в per-user update cache.
- Запускается только проверенный новый executable, который передаёт установку существующему `client install` handoff.
- Старый установленный процесс может завершиться после запуска child updater; bounded retry ждёт освобождения executable.
- Если работает tray-agent, сохраняется существующий штатный WM_CLOSE → replace → restart flow.
- Пароли, bearer tokens и Home-AI credentials в update process arguments/state не передаются.
- ADR-0032 и PR #58 прошли полный CI, включая native Windows tests и Windows cross-build.
- Release workflow опубликовал `v0.1.84-dev`: amd64/arm64 Core bundles и Windows `.exe` + `.sha256`.
- Практическая проверка `0.1.83-dev → 0.1.84-dev` на реальном Windows остаётся acceptance step.

### Выпуск 0.1.83-dev — единые пользователи и права доступа

- Один пользователь Home-AI действует во всём Core: Web, NAS/SMB и будущие Smart Home, камеры/NVR и AI-модули.
- Профили: **Administrator / Parent / Child / Guest / Friend**.
- Administrator имеет полный доступ; последнего активного Administrator нельзя отключить или понизить.
- Parent / Child / Guest / Friend — стартовые шаблоны, после выбора которых администратор настраивает права вручную.
- Добавлены direct per-user global permissions поверх существующего RBAC.
- Web **Пользователи** получает общий каталог возможностей Core и редактор прав.
- Для NAS-папок уже доступны точечные grants чтения/записи конкретным пользователям.
- Shared folders больше не выдаются всей роли автоматически; старые grants мигрируют в персональные, новые назначаются вручную.
- SMB использует те же effective permissions и scoped folder grants, что и Web/API.
- Защищены self-escalation и выдача administrator-only security permissions не-администраторам.
- Миграция: `014_unified_user_access.sql`.
- ADR-0031 фиксирует каноническую identity/access модель.
- PR #56 прошёл полный CI; live проверка на сервере остаётся acceptance step.

### Текущий F2 slice — Files / Storage после 0.1.103-dev

- ✅ У физического data-раздела/LVM есть явное назначение `files` или `video`.
- ✅ При создании нового раздела в **Система → Хранилище** пользователь выбирает **Файлы** или **Видео**; существующее назначение можно изменить или снять с учётом safety locks.
- ✅ Назначение хранится отдельно от NAS pool/folder и использует filesystem UUID как устойчивую identity, когда UUID доступен.
- ✅ Форматирование сохраняет purpose и перепривязывает новый filesystem UUID; удаление storage очищает assignment.
- ✅ **Файлы → Хранилище** показывает только storage с purpose=`files` и состояния: отсутствует / форматирование / mount / готов / используется pool.
- ✅ Files storage можно смонтировать прямо из раздела **Файлы**; canonical managed path — `/mnt/home-ai-core/<device>`.
- ✅ Mount идемпотентен, имеет quota-option fallback, read-only filesystem diagnostics и проверку результата.
- ✅ Storage mount выполняется в host mount namespace PID 1, поэтому результат виден privileged helper, Home-AI-Core и системе.
- ✅ Только `files` storage предлагается при создании NAS pool; `video` остаётся зарезервированным для будущего NVR/media data plane.
- ✅ Новый NAS pool сохраняет backing device path + filesystem UUID и создаётся только на точном mounted storage с purpose=`files`.
- ✅ Проверка backing filesystem использует normal inventory fast path и Linux `/proc/self/mountinfo`/major:minor validation.
- ✅ Занятый Files storage получает usage-lock: нельзя безопасно снять/сменить purpose, размонтировать, форматировать или удалить backing storage без явного освобождения.
- ✅ Pool capacity policy: hard reserve 5% и warning 10% по умолчанию; пороги настраиваются в **Файлы**.
- ✅ Ёмкость pool читается через live Linux `statfs`; с 0.1.103-dev при его недоступности используется fallback по backing device/UUID + privileged storage inspection.
- ✅ Direct/resumable uploads и Windows client учитывают pool reserve; при нарушении Core возвращает HTTP 507.
- ✅ Managed SMB shares защищены kernel quota policy; ext4/XFS quota path реализован и per-folder/per-user quotas добавлены в 0.1.91-dev.
- ✅ Web-раздел **Файлы** имеет реальные маршруты `/files`, `/files/storage`, `/files/windows`.
- ✅ Blank storage page из-за `mountpoints:null`, mojibake и race после mount исправлены.
- 🧪 На живом сервере подтверждены mount и создание Files pool в 0.1.102-dev.
- ✅ В 0.1.106 host `findmnt → df -B1` подтверждён на живом сервере: **Свободно / Всего** отображается корректно.
- 🧪 Live acceptance SMB, quota enforcement, reboot persistence и Windows/NAS остаётся отдельным эксплуатационным этапом.
- 🚧 Controlled migration для legacy filesystems без готовых quota features остаётся отдельной maintenance-задачей.

### Выпуск 0.1.82-dev — русский и английский Windows-клиент

- Один Windows `.exe` поддерживает русский и английский интерфейс без отдельной сборки.
- При первом запуске русская Windows locale выбирает русский; остальные locale используют английский.
- В окне настроек есть переключатель **Русский / English**, который применяется сразу и сохраняется.
- Выбранный язык хранится как non-secret поле `language` в `windows-client.json`; пароль остаётся только в Windows Credential Manager.
- Локализованы labels, buttons, conflict policy, profile status, client-side validation, confirmations и status messages.
- Tray использует тот же язык: меню, sync summary, tooltip и notifications.
- Смена языка посылает running tray-agent refresh message без перезапуска sync scheduler.
- Низкоуровневые server/OS error details остаются verbatim, но показываются внутри локализованного error framing.
- CLI пока остаётся английским диагностическим интерфейсом.
- ADR-0029 фиксирует localization boundary.
- PR #52 прошёл полный `core-ci`, включая native Windows tests и Windows cross-build.
- Практическая проверка переключения языка на реальном Windows остаётся acceptance step.

### Выпуск 0.1.81-dev — полноценные настройки Windows-клиента

- Запуск Windows-клиента без аргументов открывает нативное окно настроек; существующий CLI сохраняется.
- В окне доступны сервер, пользователь, пароль, подключение и discovery доступных writable NAS folders.
- Пароль после успешного входа сохраняется только в Windows Credential Manager; JSON настроек хранит только server URL и username.
- Можно выбрать локальную папку стандартным Windows dialog и создать/редактировать sync profile.
- Профили можно включать, выключать и удалять; редактирование сохраняет ID, enabled state и историю последнего результата.
- Настраиваются interval и conflict policy `stop` / `skip` / `replace-to-trash`.
- **Sync now** при запущенном agent сигналит существующему scheduler и не создаёт второй параллельный sync.
- Из GUI можно включить/выключить per-user background agent; отображаются autostart и running state.
- Tray получает **Settings**; double-click также открывает окно настроек.
- ADR-0028 фиксирует credential boundary и переиспользование существующего sync backend.
- PR #50 прошёл полный `core-ci`, включая native Windows tests и Windows cross-build.
- Практическая проверка GUI на реальном Windows остаётся acceptance step.

### Выпуск 0.1.80-dev

- `client install` и `agent install` умеют обновлять stable per-user binary даже при запущенном tray-agent.
- При Windows sharing/access lock новый client находит только Home-AI-owned tray window, отправляет штатный `WM_CLOSE` и до 15 секунд повторяет atomic replacement.
- После успешной замены managed autostart-agent запускается снова под тем же Windows user.
- HKCU Run разбирается только в точном Home-AI canonical формате; произвольная shell command не принимается.
- SHA-256 content comparison не перезапускает agent, если installed binary уже совпадает с source.
- Tray показывает количество enabled profiles и последний `OK` / `FAILED` результат.
- Failed sync cycle даёт Windows notification; manual **Sync now** сообщает о завершении; успешные фоновые циклы не создают notification spam.
- Native Windows CI реально проверяет locked executable → tray `WM_CLOSE` → release handle → atomic replace.
- ADR-0027 и полный `core-ci` прошли до release PR.
- Практическая проверка upgrade `0.1.79-dev → 0.1.80-dev` на реальном Windows остаётся acceptance step.

### Выпуск 0.1.79-dev

- Windows-клиент получает стабильный per-user путь `%LOCALAPPDATA%\\HomeAI\\bin\\home-ai-windows-client.exe`.
- `client install/status` позволяют установить или проверить текущую пользовательскую копию без admin rights.
- Установка использует temp-file, durable write, atomic replace и SHA-256 verification.
- `agent install` теперь сначала устанавливает клиент в стабильный путь и регистрирует HKCU Run именно на эту копию.
- Добавлен native Win32 tray без внешней GUI-зависимости.
- Tray actions: **Sync now**, **Open log**, **Open sync profiles**, **Exit**.
- **Sync now** использует существующий sequential scheduler внутри того же single-instance agent; второй sync-процесс не запускается.
- ADR-0026, native Windows tests и полный `core-ci` прошли до release PR.
- Практическая проверка установки/tray/autostart на реальном Windows остаётся acceptance step.

### Выпуск 0.1.78-dev

- Добавлены `agent install/status/remove/run` для Windows background sync agent.
- Autostart регистрируется в `HKCU\\Software\\Microsoft\\Windows\\CurrentVersion\\Run` текущего пользователя и запускается после logon.
- Run command содержит только путь к `.exe`, `agent run` и путь к sync-config; password/token туда не попадают.
- `agent install` требует хотя бы один enabled sync profile и сохранённые Credential Manager credentials для всех используемых accounts.
- Agent использует single-instance OS lock на profile-set, скрывает console window и пишет rotating local log.
- Scheduler и authentication переиспользуют существующие `sync watch` + Windows Credential Manager.
- ADR-0025 и native Windows HKCU Run round-trip tests прошли до merge PR #45.
- Ограничение: это user-session agent, не LocalSystem service; tray UI и стабильный install path остаются следующим срезом.

### Выпуск 0.1.77-dev

- Добавлен Windows Credential Manager store для Home-AI password.
- `HOME_AI_PASSWORD` остаётся приоритетным one-shot override; при его отсутствии Windows-клиент использует сохранённый credential.
- Добавлены `credentials save/status/delete`; пароль не принимается через command line и не печатается.
- `folders`, copy, queue и scheduled sync могут аутентифицироваться после restart процесса без password в queue/sync JSON.
- Generic credential scoped к тому же Windows user на локальной машине.
- Native Windows CI реально проверяет save/read/replace/delete и login без env password.
- ADR-0024 фиксирует, что будущий background agent должен работать в user logon session, а не как LocalSystem.
- Следующий срез: user-session autostart/background agent и затем tray UI.

### Выпуск 0.1.76-dev

- Реализованы persistent sync profiles Windows-клиента без сохранения пароля/токена.
- `sync run` выполняет выбранный профиль либо все due-профили; `sync watch` запускает foreground scheduler и перечитывает конфигурацию.
- Каждый цикл строит свежий plan, поэтому новые и изменённые локальные файлы попадают в следующий запуск.
- Конфликты обрабатываются только явной политикой: `stop`, `skip`, `replace-to-trash`.
- `replace-to-trash` перемещает точный конфликтующий объект в корзину Home-AI; конфликтующий ancestor автоматически не удаляется.
- Локальные удаления пока не зеркалируются на NAS: это additive push sync, а не destructive mirror.
- ADR-0023, native Windows tests и полный `core-ci` прошли до merge PR #43.
- Следующий клиентский срез: background service/tray и безопасное хранение учётных данных для работы после перезагрузки Windows.

### Выпуск 0.1.74-dev

- Меню сгруппировано по задачам; на телефоне открывается отдельной панелью.
- **Система** разделена на оборудование, хранилище, сеть и обновления.
- Главная показывает состояние сервера и последние операции; технические идентификаторы и метаданные доступны в раскрываемых подробностях.
- Файлы, пользователи, управление дисками и сетью, установка обновлений и откат сохранены. Навигация и управляющие кнопки учитывают права пользователя.
- Следующая практическая проверка: обновить сервер через **Система → Обновления**, проверить меню на компьютере и телефоне, доступность существующих функций и корректность прав. Приёмка NAS и живой откат остаются отдельными незавершёнными шагами.

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
| Security / Auth | 🚧 unified users | единые Core accounts, профили Administrator/Parent/Child/Guest/Friend, direct permissions, sessions, RBAC, CSRF, resource-scoped grants | один пользователь и единая policy для Web, NAS, NVR, Smart Home, камер и AI |
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
| Web UI | ✅ foundation / 🧪 0.1.74 | сгруппированное меню, четыре раздела System, Files/Modules/Jobs/Audit/Users, auth shell; новая компоновка ожидает проверку на сервере | основной интерфейс сейчас |
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

### ✅ Unified multi-user access

Единая система пользователей реализована; практическая проверка всех транспортов на сервере остаётся:

- один Core user ID используется всеми модулями вместо отдельных NAS/NVR/Smart-Home пользователей;
- профили: **Administrator / Parent / Child / Guest / Friend**;
- Administrator имеет полный доступ и защищён от удаления последнего администратора;
- Parent / Child / Guest / Friend — стартовые шаблоны, а не жёсткие наборы прав;
- прямые global permissions назначаются конкретному пользователю;
- Web **Пользователи** показывает общий каталог возможностей Core и позволяет включать/выключать их;
- resource-scoped permissions остаются точными grants на конкретный объект;
- файловые папки уже доступны как ресурсы с отдельными правами чтения/записи;
- существующие shared-folder grants старого `member` мигрируют в персональные grants;
- новые shared folders не привязаны к одной роли: доступ выбирается по пользователям;
- редактирование логина и имени сохраняет стабильный user ID; доступны собственная смена пароля и административный сброс;
- смена пароля/логина и отключение учётной записи отзывают сессии; последний включённый Administrator защищён транзакцией;
- личный владелец сохраняет чтение/запись при редактировании прав; настройки папки и личного лимита пользователя доступны в Web;
- изменение прав автоматически приостанавливает и пересобирает управляемый SMB-доступ; ошибки оставляют доступ приостановленным;
- тот же механизм должен использоваться будущими комнатами, устройствами, камерами/NVR и AI tools;
- ADR-0031 фиксирует каноническую модель identity/access.

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
   - исходные owner/member accounts;
   - API list/create users;
   - permission-aware Web navigation;
   - resource-scoped grants;
   - миграция `010_household_users.sql`;
   - ADR-0015.
6. ✅ Unified user access:
   - профили Administrator / Parent / Child / Guest / Friend;
   - единый permission catalog всего Core;
   - direct per-user global permissions;
   - редактирование effective access существующих пользователей;
   - explicit per-user resource access;
   - защита последнего Administrator;
   - миграция `014_unified_user_access.sql`;
   - ADR-0031;
   - Web access editor + RU/EN UI;
   - следующий шаг после CI: live acceptance на сервере.

Отдельный **Core 1.0 readiness** gate ведётся в [CORE_1_0_READINESS.md](CORE_1_0_READINESS.md):

- ✅ negative update-bundle tests для tamper/unexpected-file/path-traversal;
- ✅ полный `update → rollback → re-update` подтверждён на живом сервере (`0.1.109-dev`);
- ✅ активная Home-AI network configuration переживает reboot и возвращает connectivity; точный режим DHCP/static в этом прогоне не фиксировался;
- ✅ WireGuard reboot/autostart/handshake acceptance подтверждён пользователем и закрыт в [CORE_ACCEPTANCE.md](CORE_ACCEPTANCE.md);
- ✅ stable/RC fail-closed Ed25519 verification реализован в `0.1.110-dev`;
- 🔐 operational provisioning signing key + pinned trust root остаётся до stable/commercial release;
- ⏭ targeted tests для privileged routing/validation при следующем package-level refactor.

### 🚧 Этап F2 — File Storage / NAS

Первый вертикальный срез готов:

1. ✅ модель logical storage pools;
2. ✅ private/shared folder resources;
3. ✅ stable folder IDs и внутренние relative paths;
4. ✅ `files.read/files.write/files.manage`;
5. ✅ scoped grants на конкретную папку;
6. ✅ private folder → выбранный пользователь;
7. ✅ shared folder → explicit per-user access через общий редактор пользователей;
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

Седьмой вертикальный срез:

1. ✅ explicit Samba installation;
2. ✅ managed SMB shares только для Home-AI logical folders;
3. ✅ private folder → владелец + Administrator;
4. ✅ shared folder → только пользователи с explicit files.read/files.write grant;
5. ✅ Administrator получает RW ко всем managed shares через files.manage;
6. ✅ анонимный SMB guest access запрещён, SMB2.10 minimum;
7. ✅ отдельные SMB credentials без хранения пароля в Core;
8. ✅ managed /etc/samba/home-ai.conf + backup/include/testparm/reload;
9. ✅ Web SMB status/users/shares/credentials UI;
10. ✅ ADR-0020 и helper/API/Web tests.

Восьмой вертикальный срез:

1. ✅ Windows file-copy engine;
2. ✅ token-mode login без сохранения пароля/токена;
3. ✅ список разрешённых NAS folders;
4. ✅ resumable upload поверх стандартного Home-AI API;
5. ✅ per-chunk SHA-256 + whole-file SHA-256;
6. ✅ resume verification по server chunk history;
7. ✅ retry transient HTTP/network failures;
8. ✅ explicit stale-upload replacement policy;
9. ✅ Windows amd64 CLI cross-build;
10. ✅ .exe + SHA256 публикуются отдельными release assets;
11. ✅ ADR-0021 + usage guide + end-to-end tests.

Следующий подэтап:

1. live SMB acceptance на установленном сервере;
2. 🧪 Windows persistent copy queue + recursive folder copy — опубликовано в `0.1.75-dev`; автоматические тесты прошли, пользовательская проверка на Windows/NAS остаётся;
3. ✅ scheduled/automatic push sync — persistent profiles, due-run/watch scheduler и explicit conflict policy реализованы в `0.1.76-dev`;
4. ✅ Windows Credential Manager authentication — реализовано в `0.1.77-dev`;
5. ✅ Windows user-session background agent — HKCU Run autostart + single-instance/logging реализованы в `0.1.78-dev`;
6. 🧪 Windows tray UI + стабильный per-user install/update path — реализовано в `0.1.79-dev`; native/full CI пройдены, пользовательская Windows acceptance остаётся;
7. NFS — при необходимости.

Девятый вертикальный срез (`0.1.75-dev`):

1. ✅ persistent local JSON transfer queue без паролей/токенов;
2. ✅ atomic checkpoints и OS process lock, освобождаемый при завершении процесса;
3. ✅ recursive copy с сохранением структуры и пустых каталогов;
4. ✅ frozen source plan с size/mtime/SHA-256 и запретом symlink traversal;
5. ✅ completed items пропускаются; interrupted running items восстанавливаются как pending;
6. ✅ явный retry failed job без потери завершённых элементов;
7. ✅ проверка существующего файла по скачанным байтам и SHA-256 для восстановления потерянного ответа;
8. ✅ atomic server upload commit без перезаписи одновременно созданного target;
9. ✅ ADR-0022, CLI guide, tests против Core API и native Windows tests;
10. 🧪 пользовательская приёмка на установленном NAS/Windows остаётся незавершённой.

Десятый вертикальный срез (`0.1.76-dev`):

1. ✅ persistent non-secret sync profiles;
2. ✅ интервалы и enabled/disabled state;
3. ✅ fresh rescan/plan на каждом запуске;
4. ✅ `sync run` для ручного запуска выбранного или due-профилей;
5. ✅ `sync watch` как foreground scheduler;
6. ✅ conflict policy `stop` / `skip` / `replace-to-trash`;
7. ✅ recoverable replacement через существующую Home-AI recycle bin;
8. ✅ запрет автоматического удаления конфликтующего ancestor;
9. ✅ локальные удаления не распространяются на NAS в этом срезе;
10. ✅ ADR-0023 + native Windows tests + полный core-ci;
11. 🧪 пользовательская приёмка scheduled sync на реальном Windows/NAS остаётся незавершённой.

Одиннадцатый вертикальный срез (`0.1.77-dev`):

1. ✅ Windows Credential Manager generic credential store;
2. ✅ password keying по normalized server URL + username;
3. ✅ local-machine / same-user persistence;
4. ✅ `HOME_AI_PASSWORD` остаётся explicit override;
5. ✅ Credential Manager fallback для обычных copy/queue/sync flows;
6. ✅ `credentials save/status/delete` без password argument;
7. ✅ queue/sync JSON по-прежнему не содержит password/token;
8. ✅ secret buffers очищаются where practical, password не логируется;
9. ✅ native Windows real credential round-trip test;
10. ✅ ADR-0024;
11. 🧪 пользовательская проверка Credential Manager на реальном Windows остаётся незавершённой.

Двенадцатый вертикальный срез (`0.1.78-dev`):

1. ✅ `agent install/status/remove/run`;
2. ✅ current-user HKCU Run autostart после Windows logon;
3. ✅ startup command без password/token;
4. ✅ install readiness: enabled profiles + Credential Manager credentials;
5. ✅ single-instance OS lock per sync config;
6. ✅ hidden Windows console для background run;
7. ✅ rotating per-user local agent log;
8. ✅ переиспользование `sync watch` и существующей conflict policy;
9. ✅ native Windows HKCU Run round-trip test;
10. ✅ ADR-0025;
11. 🧪 пользовательская проверка autostart после logout/reboot остаётся незавершённой.

Тринадцатый вертикальный срез (`0.1.79-dev`):

1. ✅ stable per-user binary path `%LOCALAPPDATA%\\HomeAI\\bin\\home-ai-windows-client.exe`;
2. ✅ `client install/status`;
3. ✅ atomic executable copy/update + SHA-256 verification;
4. ✅ `agent install` регистрирует стабильную установленную копию;
5. ✅ native Win32 notification-area tray без внешней GUI dependency;
6. ✅ tray `Sync now` через существующий scheduler;
7. ✅ tray `Open log` / `Open sync profiles` / `Exit`;
8. ✅ single-instance semantics сохранены, второй sync process не создаётся;
9. ✅ ADR-0026 + native Windows tests + полный core-ci;
10. 🧪 реальная Windows acceptance установки, tray и обновления установленного binary остаётся незавершённой.

Текущий Windows engineering slice: `0.1.118-dev` исправляет live-дефекты `0.1.116-dev`: native white corners у owner-draw buttons, лишние/double borders, startup focus, nav alignment и clipped `HOME AI`. Sync/agent/update engine остаётся прежним; repeat live Windows visual/DPI acceptance открыта.

Четырнадцатый вертикальный срез (`0.1.91-dev`) — пользователи, папки и квоты:

1. ✅ полный жизненный цикл учётной записи: имя/логин, смена/сброс пароля, отключение и отзыв сессий;
2. ✅ редактор имени папки и прямых прав чтения/записи с сохранением личного владельца;
3. ✅ лимит папки и общий лимит личных папок пользователя с учётом корзины и незавершённых загрузок;
4. ✅ сериализация операций и резервирований, исключающая двойное расходование квоты параллельными загрузками;
5. ✅ project hard quotas ext4/XFS для заданных SMB-лимитов; без подтверждённой квоты ограниченная папка остаётся read-only;
6. ✅ общий kernel reserve SMB сохранён; автоматическая сверка прав и приостановка активных подключений перед отзывом;
7. ✅ отдельные разделы Web для папок, хранилища, доступа Windows и своей учётной записи;
8. ✅ безопасное скачивание HTML/SVG, фильтрация приватных событий и актуализация прав WebSocket;
9. ✅ миграция `017_nas_quotas.sql`, [ADR-0035](decisions/0035-users-folder-quotas.md), CI и реальные тесты квоты на временном ext4-образе;
10. 🧪 остаётся приёмка реальных SMB-подключений, XFS-квот, перезагрузки и Windows-клиента на установленном сервере.

## 4. Следующие продуктовые этапы

### 🚧 F2 — File Storage / NAS

Цель: пользовательское файловое хранилище поверх уже готового low-level storage.

Нужно:

- ✅ logical storage pools/volumes metadata;
- ✅ explicit physical storage purpose (`files` / `video`) with Web assignment and Files filtering;
- ✅ private folders каждого пользователя;
- ✅ shared/common folders;
- ✅ physical directory provisioning + filesystem validation;
- ✅ file CRUD API foundation (browse/create/upload/download/delete/move/rename);
- ✅ Web file manager (browser + mutations + recycle bin + resumable upload);
- ✅ SMB foundation (managed Windows shares; live acceptance pending);
- NFS при необходимости;
- ✅ quotas/policies: резерв pool, лимиты папки/личных папок пользователя и kernel enforcement для SMB; эксплуатационная приёмка остаётся;
- snapshots/backup where supported;
- 🚧 capacity/health monitoring: live capacity, резерв и предупреждения реализованы; причина «Объём недоступен» на конкретном сервере и эксплуатационная приёмка остаются.

### 🚧 AI Agent Foundation — стартует до полного F3

AI Agent начинается сейчас как **first-party module**, чтобы развиваться вместе с Core и будущими domain modules.

Первый target: `0.1.115-dev`.

Foundation включает:

- `ai.agent` module identity/capability;
- typed Tool Registry;
- provider adapter interface;
- effective-permission enforcement;
- side-effect classes `read / change / sensitive`;
- approval policy foundation;
- AI session/tool audit;
- первые read-only Core tools;
- deterministic test provider и regression tests.

Архитектура: [AI_AGENT_FOUNDATION.md](AI_AGENT_FOUNDATION.md), [ADR-0036](decisions/0036-ai-agent-first-party-tool-contract.md).

Smart Home, NAS, NVR и Cluster в дальнейшем добавляют собственные tools; AI не получает generic root/shell bypass.

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
- выделенная ёмкость архива через storage purpose `video`;
- ring overwrite самых старых незакреплённых записей;
- motion/object events;
- AI vision hooks;
- локальная база известных лиц;
- face recognition при наличии подходящего hardware.

### ⏭ F5 — Full Local AI Agent

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

Первый рабочий CLI/engine уже реализован:

- ✅ token-mode Home-AI login;
- ✅ список разрешённых private/shared destinations;
- ✅ копирование выбранного файла;
- ✅ resumable transfers;
- ✅ per-chunk + whole-file SHA-256;
- ✅ retry/resume;
- ✅ Windows amd64 release asset.

Следующие срезы:

- 🧪 persistent local transfer queue и recursive folder copy реализованы в `0.1.75-dev`; остаётся пользовательская проверка на Windows и NAS;
- ✅ scheduled/automatic push sync реализован в `0.1.76-dev`: persistent profiles, due-run/watch и explicit conflicts (`stop` / `skip` / `replace-to-trash`);
- ✅ Windows Credential Manager authentication реализован в `0.1.77-dev`;
- ✅ user-session background agent реализован в `0.1.78-dev`: HKCU Run autostart + single-instance/logging;
- 🚧 tray UI + stable per-user install/update path;
- LAN + remote/WireGuard operation.

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
| rollback не проверен живым циклом | ✅ `0.1.109-dev`: update → rollback → re-update подтверждён на живом сервере |
| resource scopes заложены, но UI/API управления grants ещё нет | добавить управление ролями/scopes вместе с NAS/Smart Home |
| durable events не подходят для media/high-rate telemetry | оставить media отдельным data plane |
| SQLite single-node | не превращать локальную схему в неявный cluster contract |
| cluster leadership не определён | сохранить abstraction, решить позже |
| внешние ifupdown profiles нельзя takeover из UI | позже добавить явный import/takeover с backup/diff/confirmation |
| stable signing/channel | ✅ verification/fail-closed реализован в `0.1.110-dev`; 🔐 осталось operational provisioning signing key + trust root до stable/commercial release |
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
| `0.1.73-dev` | 🧪 Windows CLI foundation | одиночные файлы, resume, SHA-256, отдельный Windows release asset |
| `0.1.74-dev` | 🧪 Web navigation | опубликована; сгруппированное меню и отдельные разделы Системы; пользовательская приёмка не подтверждена |
| `0.1.75-dev` | 🧪 Windows copy queue | опубликован; очередь и перенос дерева папок, пользовательская приёмка остаётся |
| `0.1.76-dev` | 🧪 Windows scheduled push sync | опубликован; persistent profiles, due-run/watch, explicit conflict policy; пользовательская приёмка остаётся |
| `0.1.77-dev` | 🧪 Windows Credential Manager | опубликован; secure same-user password storage + auth fallback; пользовательская приёмка остаётся |
| `0.1.78-dev` | 🧪 Windows background sync agent | HKCU Run autostart, lock/logging; reboot/logon acceptance остаётся |
| `0.1.79-dev` | 🧪 Windows stable install + tray | `%LOCALAPPDATA%` install, native tray controls; real Windows acceptance остаётся |
| `0.1.91-dev` | ✅ Unified users + NAS quotas | пользователи, folder access/quotas, project quotas, Files tabs; live acceptance продолжается |
| `0.1.92-dev` | Files hash navigation | первый переход к отдельным storage/windows subsection |
| `0.1.93-dev` | ✅ Real Files routes | `/files/storage`, `/files/windows`, SPA fallback + smoke |
| `0.1.94-dev` | ✅ Updater 504 recovery | retry transient GitHub/CDN download failures |
| `0.1.95-dev` | ✅ Files storage blank-page fix | `mountpoints:null` → стабильный `[]` + Web guard |
| `0.1.96-dev` | ✅ Files UTF-8 fix | исправлены mojibake/битые символы |
| `0.1.97-dev` | ✅ Direct Files mount action | mount из Files, quota fallback, auto-select mountpoint |
| `0.1.98-dev` | ✅ Read-only filesystem diagnostics | `e2fsck -n` / XFS / FAT diagnostics без auto-repair |
| `0.1.99-dev` | ✅ Mounted-state + RU messages | idempotent mount, findmnt wait, immediate UI mountpoint |
| `0.1.100-dev` | Pool backing validation | mount-boundary/device validation |
| `0.1.101-dev` | ✅ mountinfo validation | `/proc/self/mountinfo` + major:minor |
| `0.1.102-dev` | ✅ Host mount namespace | mount/umount/findmnt через PID 1; создание Files pool подтверждено пользователем |
| `0.1.103-dev` | 🧪 Pool free/total fallback | backing device/UUID + privileged inspection; live UI confirmation pending |
| `0.1.104-dev` | ✅ Explicit unavailable capacity state | «Объём недоступен» вместо прочерка при неизвестной capacity |
| `0.1.105-dev` | 🧪 Host-namespace capacity inspection | `lsblk FSAVAIL/MOUNTPOINTS` теперь видит host mount; live confirmation pending |
| `0.1.106-dev` | ✅ Direct host df capacity | `findmnt → df -B1` подтверждён на живом сервере; total/free отображаются |
| `0.1.107-dev` | 🧪 Windows client light UI | Обзор/Подключение/Синхронизация/Резервные копии/Настройки, Tab navigation, HOME AI icon/tray; live Windows acceptance pending |
| `0.1.108-dev` | 🧪 Folder usage host du | fallback для **Занято / лимит** через privileged host `du`; live confirmation pending |
| `0.1.109-dev` | ✅ Core 1.0 readiness base | полный update → rollback → re-update, network reboot и WireGuard live acceptance пройдены; Foundation позднее формально закрыт как COMPLETE |
| `0.1.110-dev` | 🧪 Security/repository hardening | опубликовано: Apache-2.0, Ed25519 Core update verification, storage dry-run/preflight; live acceptance и signing-key provisioning pending |
| `0.1.111-dev` | 🧪 Windows visual UI + Debian installer | dashboard/tray опубликованы; Debian initial installer live acceptance подтверждён; повторная Windows UI acceptance продолжается |
| `0.1.112-dev` | ✅ TexNik copyright attribution | опубликовано: NOTICE/LICENSE/README/Web/Windows/Debian attribution; runtime behavior unchanged; полный CI/release зелёный |
| `0.1.113-dev` | 🧪 Windows UI polish | опубликовано: RU localization, owner-draw buttons, fixed status check, compact cards, silent startup connection refresh; automated CI/release зелёные, repeat live acceptance pending |
| `0.1.114-dev` | ✅ Standalone storage preflight | опубликовано и live-подтверждено: Safety check + forced server-side dry-run + audit/test |
| `0.1.115-dev` | ✅ AI Agent Foundation v1 contracts | опубликовано: first-party `ai.agent`, Tool Registry, provider contract, permission/scope/approval boundaries + tests; API/audit/read tools next |
| `0.1.116-dev` | 🧪 Compact Figma Windows UI | опубликовано: native window `976×635`, dark logo-style sidebar, gradient hero, compact cards/pages; automated CI/release зелёные, live acceptance pending |
| `0.1.117-dev` | ✅ AI read tools + API | опубликовано: system/jobs/modules read tools + permission filtering + timeout + audit + authenticated API; PR/main CI и release зелёные |
| `0.1.118-dev` | 🧪 Windows owner-draw visual fix | опубликовано: rounded child regions, parent-surface corner erase, clean primary border/focus, left-aligned nav, unclipped HOME AI; automated CI/release зелёные, live acceptance pending |
| `0.1.119-dev` | 🧪 AI Local Conversation v1 | опубликовано: persistent per-user chat + optional Ollama provider + Web AI page + audit redaction; automated CI/release зелёные, live local-model acceptance pending |
| `0.1.120-dev` | 🚧 AI Agent runtime control | Modules enable/disable/restart + persistent status + modules.manage + audit/runtime cancellation; CI/release pending |

## 10. Правило ведения карты

После каждого крупного этапа:

1. обновляем фактический статус;
2. переносим завершённое в доказанные возможности;
3. держим только один основной текущий engineering milestone;
4. не добавляем общий server-platform scope без связи с PRODUCT_VISION;
5. любое архитектурное решение, влияющее на долгосрочный контракт, фиксируем ADR.
