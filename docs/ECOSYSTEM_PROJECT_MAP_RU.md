# HOME AI Core — общая карта экосистемы

**Дата фиксации:** 2026-10-12  
**Статус:** базовая карта и план работ; не заменяет проверку работающего сервера.  
**Область:** только `home-ai-core`, `home-ai-core_modules`, `home-ai-core_dvr` и официальный сайт `homeaicore.ru`.

## 1. Границы
Не входят: AI Remote Commander, home-ai-bridge, home-ai-bridge-installer, rtsp_viever, PicoKVM, Local AI/NPU, iDRAC, DJI, FIKSIK MIKSIK, LES-NET. Их возможные будущие интеграции не означают включение в состав.

## 2. Роли и источники истины
| Компонент | Источник | Назначение |
|---|---|---|
| Core | https://github.com/DeadSoulf/home-ai-core | Go/React control plane: identity, RBAC, jobs/events, storage/network, Web UI, updater, Docker Module Runtime |
| Modules | https://github.com/DeadSoulf/home-ai-core_modules | Официальный `catalog.json`, Manifest v2, метаданные версий и digest |
| DVR/NVR | https://github.com/DeadSoulf/home-ai-core_dvr | Самостоятельный Docker-модуль камер, HCNetSDK, RTSP, FFmpeg, записи и архива |
| Сайт | https://homeaicore.ru | Публичный портал, документация, информация о проекте, будущая публикация обновлений; репозиторий WordPress-темы не подтверждён |

## 3. Целевая архитектура
```text
homeaicore.ru (публичный портал / документация / дистрибуция)
           |
           v
HOME AI Core (control plane, API, RBAC, jobs, updater)
           |
           +--> Official catalog (home-ai-core_modules/catalog.json)
           |       +--> nvr.module -> immutable GHCR digest
           |       +--> reference.module -> immutable GHCR digest
           |
           +--> constrained privileged helper -> Docker Module Runtime
                     |
                     +--> NVR container (HCNetSDK / RTSP / FFmpeg)
                              +--> persistent data, recordings, SQLite
```
Сайт не должен быть обязательным для локальной работы Core. Core не содержит продуктовые NVR-функции и не имеет прямого доступа к docker.sock. Контейнеры устанавливаются через проверяемые манифесты и ограниченный helper.

## 4. Подтверждено по репозиториям
- `home-ai-core/README.md`: Go backend, React/TypeScript, SQLite, RBAC, CSRF, audit, jobs/events, WebSocket, Module Registry, updater, Debian 13 installer (amd64/arm64).
- `home-ai-core/docs/PROJECT_MAP.md`: задокументирована версия `0.1.167-dev`; M0–M3 обозначены выполненными, M4–M5 в работе. Это статус документа, не live-верификация.
- `home-ai-core/docs/sdk/module-sdk-v2.md`: immutable `@sha256`, отдельная сеть, отсутствие host ports/docker.sock, read-only rootfs, /data и непривилегированный UID/GID.
- `home-ai-core_modules/catalog.json` от 2026-10-07: `nvr.module` версии `1.1.1`, Core `>=0.1.167 <1.0.0`, amd64, образ `ghcr.io/deadsoulf/home-ai-core-nvr@sha256:9f89c074832380b9e7cf9cdd41454ceffa5c2bdb364cdb6be45f02de6f9910d1`; `reference.module` 0.1.0.
- `home-ai-core_dvr/docs/PROJECT_CONTEXT_RU.md`: recorder на FFmpeg, Hikvision/RTSP, SQLite, сегменты 300 с, retention 7 дней/50 ГБ, программный предел 32 камер, Live View 9 окон. API запускался локально, но нагрузочные и recovery-тесты не подтверждены.
- `home-ai-core_dvr/README.md`: HCNetSDK Linux amd64, порт 8090, самостоятельная веб-консоль без собственной HTTP-аутентификации. Нельзя публиковать её напрямую в Интернет.

## 5. Риски и несоответствия
1. **P0 GHCR:** ранее установка NVR возвращала `unauthorized`. Проверить разрешения registry и pull с реального Core node.
2. **P0 DVR integration:** standalone compose использует `h.a.c._nvr`, порт `8090:8090`, `/app/output`; Module SDK v2 требует управляемое имя, отсутствие host ports и `/data`. Нужна проверка адаптера и реальной установки.
3. **P0 security:** DVR HTTP без отдельной авторизации; доступ только через авторизованный Core/proxy, ограниченная сеть; безопасное хранение секретов камер.
4. **P1 updates:** в PROJECT_MAP автоматическая проверка обновлений, health reconciliation, rollback и purge/backup отмечены незавершёнными.
5. **P1 catalog:** README каталога называет его приватным, но GitHub 2026-10-12 показал visibility public. Уточнить требуемую модель доступа.
6. **P1 version drift:** сверить GitHub, опубликованные digest, установленную версию Core и контейнеры.
7. **P1 architecture:** NVR только amd64; Core поддерживает также arm64. UI обязан блокировать несовместимую установку.
8. **P2 website:** актуальная публикация WordPress и каталог обновлений не подтверждены; проверять отдельно, не считать готовыми.
9. **P2 reliability:** длительная запись, RTSP reconnect, сбой FFmpeg, restart, reboot, сохранность архива, лимит 32 камер не подтверждены полноценными тестами.
10. **P2 licensing:** проверить права распространения сторонних бинарных библиотек Hikvision.

## 6. План работ и критерии готовности
| Приоритет | Работа | Критерий |
|---|---|---|
| P0 | Инвентаризация Core node | зафиксированы installed version, git SHA, сервисы, Docker images и конфигурация |
| P0 | GHCR authentication | NVR pull по digest проходит с целевого сервера |
| P0 | DVR security/isolation | нет прямого публичного доступа к 8090, секреты защищены |
| P1 | Catalog install | NVR устанавливается одной кнопкой из официального каталога |
| P1 | Lifecycle/data | install/start/stop/restart/remove/reinstall сохраняют записи и SQLite |
| P1 | Update/rollback | новая версия проходит health-check; неудачная автоматически откатывается |
| P1 | Reconciliation/logs | Web UI отражает healthy/unhealthy, падения и ограниченные логи |
| P2 | Website/release distribution | сайт доступен, версии и ссылки проверяемы, WordPress отделён от update API |
| P2 | NVR soak/recovery | документированы результаты длительных тестов и восстановления |

## 7. Правила ведения
- Эта карта — **общая**; технические детали остаются в документах соответствующих репозиториев.
- Факты отделять от планов: «в документации», «проверено CI», «проверено на устройстве».
- После каждого изменения API, совместимости, публикации или инфраструктуры обновлять карту и дату проверки.
- Никаких изменений работающего сервера или образов без отдельного тестирования.
