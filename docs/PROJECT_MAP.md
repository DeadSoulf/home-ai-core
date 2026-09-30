# Home-AI — карта проекта

> Рабочая карта фактического состояния проекта.  
> Каноническая конечная цель: [PRODUCT_VISION.md](PRODUCT_VISION.md)  
> Аудит уже сделанного: [CURRENT_STATE_AUDIT.md](CURRENT_STATE_AUDIT.md)

**Последняя версия, на которой пользователь подтвердил работу сетевого UI:** `0.1.59-dev`  
**Последний опубликованный релиз:** `0.1.81-dev` — native Windows settings GUI + sync/agent controls\
**Текущий срез:** Windows client RU/EN localization — PR #52; candidate for `0.1.82-dev`\
**Следующий engineering milestone:** release RU/EN client, then automatic Windows release discovery/download + SHA-256 verified install handoff\
**Состояние:** F2 NAS продолжается; logical pools, private/shared folders, scoped file permissions, Web-раздел **Файлы** и Windows-клиент уже есть в репозитории. Их наличие не заменяет проверку на сервере.  
**Обновлено:** 2026-09-30

### Кандидат 0.1.82-dev — русский и английский Windows-клиент

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

Седьмой вертикальный срез:

1. ✅ explicit Samba installation;
2. ✅ managed SMB shares только для Home-AI logical folders;
3. ✅ private folder → владелец + owner;
4. ✅ shared folder → активные household users;
5. ✅ owner получает RW ко всем managed shares;
6. ✅ guest access запрещён, SMB2.10 minimum;
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

Основной следующий engineering milestone: automatic Windows release discovery/download + SHA-256 verified install handoff. Self-update handoff и tray health/notifications входят в `0.1.80-dev`; live Windows/NAS acceptance, logout/reboot autostart acceptance, SMB и rollback остаются отдельными практическими проверками перед отметкой F2 как завершённого.

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
- ✅ SMB foundation (managed Windows shares; live acceptance pending);
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
| `0.1.73-dev` | 🧪 Windows CLI foundation | одиночные файлы, resume, SHA-256, отдельный Windows release asset |
| `0.1.74-dev` | 🧪 Web navigation | опубликована; сгруппированное меню и отдельные разделы Системы; пользовательская приёмка не подтверждена |
| `0.1.75-dev` | 🧪 Windows copy queue | опубликован; очередь и перенос дерева папок, пользовательская приёмка остаётся |
| `0.1.76-dev` | 🧪 Windows scheduled push sync | опубликован; persistent profiles, due-run/watch, explicit conflict policy; пользовательская приёмка остаётся |
| `0.1.77-dev` | 🧪 Windows Credential Manager | опубликован; secure same-user password storage + auth fallback; пользовательская приёмка остаётся |
| `0.1.78-dev` | 🧪 Windows background sync agent | HKCU Run autostart, lock/logging; reboot/logon acceptance остаётся |
| `0.1.79-dev` | 🧪 Windows stable install + tray | `%LOCALAPPDATA%` install, native tray controls; real Windows acceptance остаётся |

## 10. Правило ведения карты

После каждого крупного этапа:

1. обновляем фактический статус;
2. переносим завершённое в доказанные возможности;
3. держим только один основной текущий engineering milestone;
4. не добавляем общий server-platform scope без связи с PRODUCT_VISION;
5. любое архитектурное решение, влияющее на долгосрочный контракт, фиксируем ADR.
