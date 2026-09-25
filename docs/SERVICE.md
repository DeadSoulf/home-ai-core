# Home AI Core как systemd-служба / systemd service

Служба запускается при загрузке Linux и продолжает работать после выхода из SSH.
Само ядро работает от обычного пользователя. Root нужен только для установки и управления
системной службой. Скрипт отказывает в запуске ядра от UID 0.

## Установка на Debian

Для текущей ветки `develop` Security Core использует встроенную SQLite-библиотеку.
Перед первой сборкой версии с центральной пользовательской БД установите зависимость:

```bash
sudo apt install -y libsqlite3-dev
```

Отдельный сервер базы данных не запускается.

Выполняйте сборку от того же обычного пользователя, который уже владеет репозиторием,
`runtime/`, конфигурацией и ключами доступа Git. Это сохраняет существующие сессии,
пользователей, настройки и механизм обновления. Не запускайте сборку или ядро через sudo.

```bash
cd /srv/home-ai-core
git pull --ff-only origin develop
cmake -S . -B build -G Ninja
cmake --build build
ctest --test-dir build --output-on-failure
```

Если ядро уже запущено вручную, сначала остановите его через Ctrl+C в том терминале.
Это освобождает порт и предотвращает одновременную работу двух экземпляров с одной базой.
Затем из обычной учётной записи:

```bash
sudo bash scripts/home-ai-service.sh install "$(id -un)" /srv/home-ai-core
bash scripts/home-ai-service.sh status
bash scripts/home-ai-service.sh autostart
```

`install` создаёт `/etc/systemd/system/home-ai-core.service`, включает его через
`systemctl enable`, затем выполняет `systemctl restart`, проверяет состояние
`enabled` и сразу запускает актуальную сборку ядра.
Он не меняет владельцев существующих данных
и не устанавливает зависимости. Путь должен быть абсолютным, без пробелов и спецсимволов.
Если вы вошли как root, явно укажите существующего обычного владельца проекта вместо `$(id -un)`.

После статуса `active (running)` можно закрыть SSH. Команда
`bash scripts/home-ai-service.sh autostart` должна вывести `Autostart: enabled`.
После перезагрузки сервера systemd автоматически запускает службу через
`WantedBy=multi-user.target`.

## Управление

```bash
sudo bash scripts/home-ai-service.sh stop
sudo bash scripts/home-ai-service.sh start
sudo bash scripts/home-ai-service.sh restart
bash scripts/home-ai-service.sh status
bash scripts/home-ai-service.sh autostart
sudo journalctl -u home-ai-core.service -n 100 --no-pager
sudo journalctl -u home-ai-core.service -f
sudo bash scripts/home-ai-service.sh uninstall
```

Удаление останавливает службу и отключает автозапуск, сохраняя код, пользователей,
runtime-конфигурацию, облачные файлы и записи. `status` сохраняет стандартный код возврата
systemctl: неактивная служба может вернуть ненулевой код.

Рабочий каталог — корень проекта; бинарный файл — `build/home-ai-core`.
`Restart=on-failure` восстанавливает процесс после ошибки, но не отменяет явную остановку.
Web Update Manager по-прежнему собирает новую версию и перезапускает процесс через exec,
сохраняя управление systemd. Для обновления учётная запись службы должна иметь права на
репозиторий и каталог сборки, а доступ Git должен работать без ввода пароля.

Storage Helper остаётся отдельным привилегированным компонентом. Служба не получает root,
дополнительные capabilities или новые разрешения sudo; существующие настройки helper сохраняются.
WireGuard требует отдельного системного разрешения, как и при ручном запуске.

## English

Build and test as the existing non-root repository owner. Stop any manually running instance,
then install and start with the commands above. The installer enables boot startup and uses
the repository as WorkingDirectory, preserving runtime data and the existing self-update workflow.
The core runs under the selected non-root account; sudo is used only for service management.
Uninstall stops/disables the service and retains all project and user data.
Check `systemctl status home-ai-core` and the Web UI after disconnecting SSH and after a reboot.
Physical server/boot verification must be performed on the target Debian host.


## Network Helper for IPv4 management

The Home AI Core service remains non-root. Web DHCP/static IPv4 changes use a separate validated helper.

After building a version that includes `home-ai-network-helper`, install it once for the
service account:

```bash
sudo sh scripts/install-network-helper.sh texnik
```

This installs:

```text
/usr/local/libexec/home-ai-network-helper
/etc/sudoers.d/home-ai-network-helper
```

The helper is required only for privileged IPv4 changes. Reading interface/IP status does not
require it. Re-run the installer after updating Home AI Core whenever the helper binary changes.
