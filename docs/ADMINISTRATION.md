# Администрирование / Administration

## GPU и ускоритель AI

Раздел **Администрирование** доступен только администратору. Ядро читает
`class`, `vendor`, `device` и ссылку `driver` в `/sys/bus/pci/devices`.
Отображаются PCI-адрес, производитель, идентификаторы устройства и загруженный драйвер.
Инвентаризация включает VGA, 3D и другие устройства PCI класса Display (03).
В виртуальной машине видны только предоставленные ей устройства, включая виртуальные карты.
Интегрированные GPU также отображаются. Идентификатор модели выводится как PCI device ID;
база маркетинговых названий карт не требуется.

При открытии интерфейса и затем каждые 10 секунд выполняется повторное чтение.
Администратор получает уведомление о найденных картах и ссылку на настройку.
Физически устанавливать карту следует при выключенном сервере, если оборудование
не поддерживает горячее подключение. После включения карта обнаруживается через sysfs.

**Назначить для AI** сохраняет `ai.accelerator.pci_address` в `runtime/home-ai.conf`.
**Снять назначение GPU** очищает параметр, в том числе когда карта уже отсутствует.
Выбор сохраняется после перезапуска. Отсутствующая выбранная карта отмечается предупреждением;
другая карта автоматически не назначается. После изменения PCI-топологии проверьте назначение.

Это настройка будущего AI runtime, а не запуск вычислений и не подтверждение совместимости.
Драйверы, CUDA/ROCm, прошивки, PCI bind/unbind и параметры оборудования не изменяются.
Карты без драйвера можно выбрать, но настройка программного стека выполняется отдельно.
Все операции назначения требуют администратора и записываются в журнал аудита.

## HTTP API

Существующая сессионная cookie обязательна; без входа возвращается 401, без прав — 403.

- `GET /api/admin/gpus`: `available`, `selected`, `selected_present`, `devices`.
  Поля устройства: `pci_address`, `vendor`, `vendor_id`, `device_id`, `driver`.
  `available=false` означает недоступный sysfs; пустой список при `available=true` — отсутствие карт.
- `POST /api/admin/accelerator`: `Content-Type: application/x-www-form-urlencoded`,
  заголовок `X-HomeAI-Request: 1`, поле `pci_address=0000:01:00.0` или пустое значение.
  Проверяется формат и наличие карты в свежем снимке. 200 — сохранено, 400 — неверный/отсутствующий
  адрес, 403 — нет прав/защитного заголовка, 500 — ошибка сохранения.
  CORS не включён; пользовательский заголовок предотвращает сторонние HTML form POST.

## Language / Язык

Русский и English доступны на всех страницах, включая вход и первоначальную настройку.
Выбор хранится в `localStorage` текущего браузера для этого адреса сервера;
при блокировке хранилища используется cookie. Переключение перезагружает страницу:
несохранённую форму сначала сохраните. Начальный язык — русский.

Каталог: `web/ui/translations.tsv`, механизм: `web/ui/localization.js`.
CMake встраивает оба файла в бинарный файл, поэтому дополнительные файлы на сервере не нужны.
Перевод охватывает меню, страницы, диалоги, статусы и основные сообщения API.
Имена пользователей, значения форм, пути, PCI-идентификаторы и исходный вывод внешних
программ (Git, CMake, драйверы, системные утилиты) сохраняются как данные.
Не добавляйте в TSV обратные кавычки или `${...}`: каталог встроен в JavaScript template literal.

## English

The administrator-only page detects display-class PCI devices through read-only Linux sysfs.
It reports vendor/device IDs, PCI address and the bound driver; it also detects driverless,
integrated and guest-visible virtual GPUs. Inventory refreshes every 10 seconds while the UI is open.
Selection persists as `ai.accelerator.pci_address`. Missing hardware produces a warning;
the selection can be cleared without the device. Selection does not install drivers or enable
AI execution. Driver/runtime compatibility must be checked separately.

Russian and English cover navigation, pages, dialogs and application status messages.
Language is stored per browser/origin and switching reloads the current page.
Raw external diagnostics and user data are not translated.

## Проверка / Verification

```bash
cmake -S . -B build -G Ninja
cmake --build build
ctest --test-dir build --output-on-failure
# Requires Node.js, Playwright and an installed browser:
HOMEAI_BROWSER_CHANNEL=chrome node tests/test_localization.cjs
```

GPU tests use a temporary sysfs fixture; API tests start a loopback HTTP server and verify
authorization, invalid/stale selection, persistence, removal, clear and failed-save rollback.
Browser tests use rendered fixtures and mocked APIs, covering every page, persisted language,
dynamic messages and GPU actions. No hardware or drivers are modified by these tests.

Reference: [Linux PCI sysfs documentation](https://docs.kernel.org/PCI/sysfs-pci.html).
