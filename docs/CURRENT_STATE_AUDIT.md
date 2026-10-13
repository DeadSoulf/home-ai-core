# Home-AI-Core — Current State Audit

## Решение

Home-AI-Core является небольшим доверенным control plane. Продуктовая бизнес-логика выносится из ядра в независимо поставляемые контейнерные модули.

## Оставить в Core

| Область | Решение | Причина |
| --- | --- | --- |
| Identity / node state | KEEP | Базовая идентичность узла |
| Authentication / sessions | KEEP | Общий security boundary |
| Users / RBAC / resource grants | KEEP | Общая модель доступа |
| CSRF / Audit | KEEP | Защита и трассируемость действий |
| Jobs | KEEP | Общая оркестрация длительных операций |
| Events / WebSocket | KEEP | Общий transport и durable history |
| Module Registry / SDK | KEEP | Контракт внешних модулей |
| System / hardware inventory | KEEP | Общие host capabilities |
| Storage foundation | KEEP | Базовое управление физическим storage |
| Network management | KEEP | Общая системная функция узла |
| Core updater | KEEP | Обновление доверенного control plane |
| Privileged helper | KEEP | Ограниченный root boundary |
| Web UI shell | KEEP | Единый интерфейс ядра и модульной навигации |
| Debian installer | KEEP | Первоначальное развёртывание |
| Windows file client | KEEP | Существующий клиент Core |

## Не добавлять в Core

- функциональные сервисы конкретного продукта;
- vendor-specific integrations;
- media/model runtimes;
- отдельные workload supervisors;
- сторонние package/runtime stacks, нужные только одному модулю;
- прямое управление произвольными Docker-контейнерами из непривилегированного Core;
- product-specific таблицы SQLite и permissions в Core migrations.

## Docker boundary

Core не получает `docker.sock`. Container lifecycle выполняется через ограниченный privileged helper с allowlist/validation.

Каждый модуль хранит собственные данные отдельно от Core DB. Core хранит только metadata, необходимую для установки, статуса, разрешений, совместимости и orchestration.

## Критерий чистого Core

Core считается чистым, если удаление любого внешнего модуля не требует пересборки Core и если Core может запускаться на чистом сервере без зависимостей конкретного модуля.
