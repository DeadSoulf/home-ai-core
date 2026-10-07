# HOME AI Core — отчёт по механизму установки и обновления модулей

**Дата:** 7 октября 2026 г.  
**Статус документа:** рабочая фиксация выполненных работ и утверждённой архитектуры.

---

## 1. Цель

Сформировать единый механизм распространения модулей HOME AI Core, при котором пользователь не вставляет Docker manifest вручную и не работает с Docker CLI.

Целевой сценарий:

```text
GitHub → официальный каталог HOME AI Core → вкладка «Модули» → Установить / Обновить
```

---

## 2. Что уже реализовано в HOME AI Core

К моменту принятия новой схемы в Core уже существуют:

- Module Registry;
- Manifest v2;
- dependency/conflict planner;
- capability discovery;
- Docker Runtime;
- privileged helper;
- Job Engine;
- lifecycle install/start/stop/restart/remove;
- отдельная Docker-сеть `home-ai-modules`;
- persistent data для каждого модуля;
- запуск модулей от непривилегированного UID/GID;
- read-only root filesystem;
- dropped Linux capabilities;
- `no-new-privileges`;
- запрет прямого доступа Core и модулей к `docker.sock`;
- обязательное закрепление Docker image через immutable `@sha256`;
- проверка Core compatibility;
- проверка архитектуры;
- проверка capabilities;
- основа signed module repository;
- Ed25519 verification foundation;
- persistent records для module repositories и signing keys.

Эта база остаётся и используется дальше.

---

## 3. Почему ручной Manifest больше не является целевым UX

Текущий интерфейс умеет принимать Manifest v2 вручную.

Для разработчика это полезно, но для конечного пользователя такой сценарий неудобен и требует знания:

- имени Docker image;
- точного immutable digest;
- версии;
- архитектуры;
- capabilities;
- структуры Manifest v2.

Принято решение убрать ручную вставку manifest из обычной вкладки «Модули».

Установка должна происходить только из официального каталога HOME AI Core.

---

## 4. Официальный каталог

Принято создать отдельный GitHub-репозиторий:

`DeadSoulf/home-ai-core_modules`

Назначение репозитория:

- список официальных модулей;
- версии;
- описание;
- metadata;
- Manifest v2;
- immutable image digest;
- информация, необходимая Core для установки и обновления.

Исходный код модулей в этом репозитории храниться не должен.

---

## 5. Репозитории модулей

Каждый модуль остаётся самостоятельным проектом.

Примеры:

```text
DeadSoulf/home-ai-core_dvr
DeadSoulf/home-ai-core_ai
DeadSoulf/home-ai-core_camera
...
```

Каждый модуль имеет:

- собственный GitHub repository;
- собственную версию;
- собственный Docker image;
- собственный CI;
- собственные тесты;
- собственный persistent state;
- собственную продуктовую логику.

Core не должен содержать исходный код продуктовых модулей.

---

## 6. Private distribution

Принято, что официальный каталог и модули являются приватными.

Для Docker images используется приватный container registry, связанный с GitHub. Текущая реализация NVR уже ориентирована на GHCR.

Core должен получить безопасный механизм авторизации для:

- чтения private module catalog;
- pull private Docker images.

GitHub credential не должен попадать внутрь контейнеров модулей.

---

## 7. Публикация новой версии модуля

Принята автоматическая цепочка:

```text
разработка модуля
  ↓
Git tag / release
  ↓
GitHub Actions
  ↓
build
  ↓
tests
  ↓
Docker image
  ↓
immutable sha256 digest
  ↓
обновление официального каталога
```

Digest не должен вручную переноситься человеком из Docker/GitHub в manifest.

Каталог должен обновляться только после успешной сборки и тестирования версии.

---

## 8. Версионирование

Используется обычный SemVer:

```text
1.0.0
1.1.0
1.1.1
2.0.0
```

Отдельные release channels:

- stable;
- beta;
- dev;

не вводятся.

Каталог содержит обычную последовательность опубликованных версий.

---

## 9. Установка модуля

Целевой процесс:

```text
пользователь открывает «Модули»
  ↓
Core загружает официальный каталог
  ↓
показывает доступные модули
  ↓
пользователь нажимает «Установить»
  ↓
Core проверяет:
  - подпись каталога
  - Core compatibility
  - architecture
  - capabilities
  - dependencies
  - conflicts
  - immutable image digest
  ↓
Job Engine
  ↓
privileged helper
  ↓
docker pull @sha256
  ↓
создание контейнера
  ↓
health-check
  ↓
модуль зарегистрирован
```

Docker CLI пользователю не требуется.

---

## 10. Обновления

Обновления устанавливаются только вручную.

Core при этом автоматически проверяет каталог на наличие новых версий.

Также должна быть кнопка:

**«Проверить обновления»**

Пример интерфейса:

```text
NVR

Установлено: 1.1.0
Доступно:   1.2.0

[ Обновить ]
```

Автоматическая установка обновлений не выполняется.

---

## 11. Rollback

Rollback обязателен.

Перед обновлением Core сохраняет:

- установленную версию;
- предыдущий Manifest;
- предыдущий immutable image digest.

Обновление:

```text
старый image
  ↓
pull нового digest
  ↓
замена контейнера
  ↓
start
  ↓
health-check
```

Если health-check успешен:

```text
update complete
```

Если health-check неуспешен:

```text
stop/remove failed container
  ↓
restore previous digest
  ↓
start
  ↓
health-check
  ↓
rollback complete
```

Update и rollback должны фиксироваться в audit.

---

## 12. Удаление

Приняты два разных действия.

### Удалить модуль

Удаляются:

- контейнер;
- registry record.

Сохраняются:

- `/var/lib/home-ai-core/modules/<module-id>`;
- persistent data.

### Удалить модуль и данные

Удаляются:

- контейнер;
- registry record;
- persistent data.

Это действие требует отдельного подтверждения.

---

## 13. Вкладка «Модули»

Новый интерфейс должен иметь два основных блока.

### Доступные

Карточка содержит:

- название;
- описание;
- последнюю версию;
- совместимость;
- состояние;
- кнопку «Установить».

### Установленные

Карточка содержит:

- установленную версию;
- состояние;
- health;
- наличие обновления;
- Запустить;
- Остановить;
- Перезапустить;
- Обновить;
- Удалить;
- Удалить модуль и данные.

Ручное поле вставки Manifest v2 из обычного UI удаляется.

---

## 14. Доверие

HOME AI Core устанавливает только наши официальные модули.

На текущем этапе:

- сторонние catalogs не разрешаются;
- сторонние signing keys не добавляются пользователем;
- официальный catalog является единственным источником модулей;
- Core должен проверять подпись и immutable digest.

Архитектура подписей в Core уже существует и должна быть адаптирована к Docker image-oriented catalog.

---

## 15. Проверка обновлений

Core должен:

- периодически обновлять catalog metadata;
- сравнивать установленные версии с опубликованными;
- показывать наличие обновления в UI;
- поддерживать ручную кнопку refresh;
- хранить last-known-good catalog snapshot;
- не удалять установленный модуль при временной недоступности GitHub.

---

## 16. Первый продуктовый модуль — NVR

Первым реальным модулем является:

`DeadSoulf/home-ai-core_dvr`

NVR остаётся отдельным Docker workload.

Продуктовая логика NVR не переносится в Core.

После установки NVR должен появляться в HOME AI Core как новый раздел интерфейса.

Целевая модель:

```text
HOME AI Core UI
  ├─ Главная
  ├─ Система
  ├─ Модули
  └─ NVR
       ↓
     h.a.c._nvr
```

То есть пользователь не должен открывать отдельный `:8090` как основной способ использования модуля.

Нужно реализовать безопасный Core-to-module routing/proxy и декларативное подключение раздела модуля к основному интерфейсу.

---

## 17. Выполненные работы по NVR для Module Runtime

Для `home-ai-core_dvr` уже выполнена адаптация к модели модулей Core:

- данные перенесены с `/app/output` на стандартный `/data`;
- добавлен `NVR_DATA_ROOT=/data`;
- entrypoint работает с writable `/data`;
- root filesystem может быть read-only;
- HOME перенесён в `/tmp`;
- отключена запись Python bytecode;
- image поднят до версии `1.1.0`;
- добавлен GitHub Actions workflow публикации module image;
- добавлен тест запуска с:
  - непривилегированным UID/GID;
  - `--read-only`;
  - `--cap-drop ALL`;
  - `no-new-privileges`;
  - tmpfs `/tmp`;
  - единственным persistent mount `/data`;
- тест HOME AI Core runtime profile успешно прошёл.

---

## 18. Что осталось реализовать в Core

Следующая очередь работ:

1. Создать `DeadSoulf/home-ai-core_modules`.
2. Определить Docker-oriented schema официального каталога.
3. Реализовать безопасную GitHub authentication для private catalog.
4. Реализовать private registry authentication.
5. Реализовать catalog refresh job.
6. Реализовать last-known-good catalog snapshot.
7. Реализовать API списка доступных модулей.
8. Реализовать UI «Доступные / Установленные».
9. Убрать ручной manifest из обычного UI.
10. Реализовать update discovery.
11. Реализовать update job.
12. Реализовать post-update health verification.
13. Реализовать automatic rollback.
14. Реализовать audit update/rollback.
15. Реализовать purge data.
16. Реализовать Core-to-module API/UI routing.
17. Подключить NVR как новый раздел HOME AI Core.

---

## 19. Итоговая архитектура

```text
┌─────────────────────────────┐
│ GitHub module repositories  │
│ NVR / AI / Camera / ...     │
└──────────────┬──────────────┘
               │ release
               ▼
┌─────────────────────────────┐
│ GitHub Actions              │
│ build + test                │
└──────────────┬──────────────┘
               │
               ▼
┌─────────────────────────────┐
│ Private Container Registry  │
│ immutable @sha256           │
└──────────────┬──────────────┘
               │ publish metadata
               ▼
┌─────────────────────────────┐
│ home-ai-core_modules        │
│ official signed catalog     │
└──────────────┬──────────────┘
               │ refresh
               ▼
┌─────────────────────────────┐
│ HOME AI Core               │
│ Catalog / Jobs / Audit      │
└──────────────┬──────────────┘
               │ install/update
               ▼
┌─────────────────────────────┐
│ Privileged helper           │
└──────────────┬──────────────┘
               │
               ▼
┌─────────────────────────────┐
│ Docker module               │
│ /data + immutable image     │
└─────────────────────────────┘
```

---

## 20. Зафиксированные решения

- официальный каталог: отдельный GitHub repository;
- каждый модуль: отдельный GitHub repository;
- distribution: private;
- только официальные модули HOME AI Core;
- публикация metadata после CI автоматически;
- обновления устанавливаются вручную;
- Core автоматически обнаруживает новые версии;
- rollback обязателен;
- данные при обычном remove сохраняются;
- полное удаление данных — отдельное действие;
- release channels не используются;
- SemVer используется;
- UI делится на «Доступные» и «Установленные»;
- ручной Manifest install удаляется из обычного UI;
- NVR интегрируется как новый раздел основного интерфейса HOME AI Core.
