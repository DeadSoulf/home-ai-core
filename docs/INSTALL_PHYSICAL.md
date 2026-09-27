# Home AI Core — установка на физический Debian 13 сервер

Эта инструкция предназначена для первого bare-metal acceptance текущей ветки `develop`.
Пока ветка не промотирована в `main`, это **development/acceptance installation**, а не
объявление стабильного production-релиза.

## 1. Требования

- Debian 13 x86-64.
- Intel VT-x или AMD-V/SVM включены в BIOS/UEFI, если нужен Hypervisor Core.
- Проводное подключение к доверенной LAN желательно на первом этапе.
- Постоянный IP или DNS-имя рекомендуется определить заранее для TLS-сертификата.
- Не запускайте Home AI Core от root.

Проверьте аппаратную виртуализацию:

```bash
grep -Eoc '(vmx|svm)' /proc/cpuinfo
```

Для физического гипервизора результат должен быть больше нуля.

## 2. Минимальный bootstrap и клонирование

На чистом Debian сначала нужны только CA-сертификаты и Git. Выполните от обычного пользователя
с sudo-доступом:

```bash
sudo apt-get update
sudo apt-get install -y --no-install-recommends ca-certificates git

sudo install -d -m 0755 -o "$(id -un)" -g "$(id -gn)" /srv/home-ai-core
git clone --branch develop https://github.com/DeadSoulf/home-ai-core.git /srv/home-ai-core
cd /srv/home-ai-core
```

HTTPS remote выбран намеренно: публичный репозиторий может обновляться без SSH-ключа
и без интерактивного ввода пароля.

## 3. Системные зависимости

```bash
sudo bash scripts/prepare-debian-host.sh
```

Скрипт не выполняет `apt upgrade`, не меняет сеть и не устанавливает Hypervisor автоматически.

## 4. Чистая сборка и тесты

```bash
cmake -S . -B build -G Ninja -DCMAKE_BUILD_TYPE=RelWithDebInfo
cmake --build build
ctest --test-dir build --output-on-failure
```

Продолжайте только при полном прохождении CTest.

## 5. Runtime и HTTPS до первого запуска

Чтобы первый Web-запуск сразу был по HTTPS, подготовьте runtime-конфигурацию до установки службы:

```bash
install -d -m 0700 runtime
cp config/home-ai.conf runtime/home-ai.conf

SERVER_NAME="192.168.1.50"   # замените на реальный IP или DNS-имя
bash scripts/setup-tls.sh "$SERVER_NAME"

sed -i 's/^web\.tls_enabled=.*/web.tls_enabled=true/' runtime/home-ai.conf
```

Примеры:

```bash
bash scripts/setup-tls.sh home-ai-core.local
# или
bash scripts/setup-tls.sh 192.168.1.50
```

Встроенный генератор создаёт self-signed сертификат. Браузер будет предупреждать о доверии,
пока сертификат/CA не добавлен в доверенные. Для постоянного удалённого администрирования
используйте сертификат доверенного internal/public CA.

## 6. Privileged helpers

После успешной сборки установите helpers для того же обычного пользователя:

```bash
sudo sh scripts/install-storage-helper.sh "$(id -un)"
sudo sh scripts/install-network-helper.sh "$(id -un)"
```

Core при этом остаётся непривилегированным; root-доступ получает только валидирующий helper.

## 7. systemd

```bash
sudo bash scripts/home-ai-service.sh install "$(id -un)" /srv/home-ai-core
bash scripts/home-ai-service.sh status
bash scripts/home-ai-service.sh autostart
```

Ожидается `active (running)` и `Autostart: enabled`.

## 8. Hypervisor Core

Только если физический сервер должен управлять KVM/QEMU/libvirt:

```bash
sudo bash scripts/setup-hypervisor.sh install
sudo bash scripts/setup-hypervisor.sh check
```

Скрипт устанавливает QEMU/libvirt, добавляет service account в группы `libvirt` и `kvm`
и перезапускает только Home AI Core.

## 9. Первый администратор

Откройте:

```text
https://SERVER_IP_OR_NAME:8080/setup
```

Создайте первого администратора. Заводского логина или пароля нет.

Не публикуйте порт управления напрямую в Интернет. Используйте доверенную LAN/VPN,
а для удалённого доступа — TLS и, предпочтительно, WireGuard.

## 10. Phase A acceptance

После входа и настройки запустите acceptance от service account:

```bash
cd /srv/home-ai-core
ADMIN_LOGIN="admin"          # замените на созданного администратора
HOMEAI_ACCEPTANCE_USER="$ADMIN_LOGIN" \
    bash scripts/acceptance-phase-a.sh /srv/home-ai-core
```

Пароль запрашивается интерактивно и не передаётся через переменную окружения.

Для физического сервера целевое состояние:

```text
FAIL=0
```

Предупреждения о nested virtualization, допустимые на development VM, на bare-metal хосте
не должны заменять рабочие VMX/SVM и `/dev/kvm`.

## 11. Проверка после перезагрузки

```bash
sudo reboot
```

После загрузки:

```bash
systemctl is-enabled home-ai-core.service
systemctl is-active home-ai-core.service
sudo journalctl -u home-ai-core.service -n 50 --no-pager
```

Проверьте Web UI, HTTPS, вход, Storage/Network readiness и Hypervisor readiness.

## 12. Обновления

До промоушена релиза физический acceptance-хост может отслеживать `develop`.
После перехода на стабильный канал одновременно переключите Git branch и:

```text
update.branch=main
```

Web UI должен показывать только две пользовательские версии: текущую на сервере и доступную
на GitHub. Git SHA, build progress и подробные compiler/test logs остаются внутренней диагностикой.
