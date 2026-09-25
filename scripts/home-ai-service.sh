#!/usr/bin/env bash
# Manage a system service; only service management needs root.
set -euo pipefail
action=${1:-help}
unit=/etc/systemd/system/home-ai-core.service
case "$action" in
  install)
    [[ $EUID -eq 0 ]] || { echo 'Use sudo for install.' >&2; exit 1; }
    account=${2:-${SUDO_USER:-}}
    root=${3:-/srv/home-ai-core}
    [[ $account =~ ^[a-z_][a-z0-9_-]*\$?$ && $account != root ]] || { echo 'Specify a non-root service account.' >&2; exit 1; }
    [[ $(id -u "$account") != 0 ]] || exit 1
    root=$(realpath -e -- "$root")
    # Keep systemd paths unambiguous (no specifiers, escapes or whitespace).
    [[ $root =~ ^/[a-zA-Z0-9_./-]+$ ]] || { echo 'Unsupported repository path.' >&2; exit 1; }
    [[ -x $root/build/home-ai-core && -f $root/config/home-ai.conf ]] || { echo 'Build the repository first.' >&2; exit 1; }
    runuser -u "$account" -- test -x "$root/build/home-ai-core"
    if [[ ! -d $root/runtime ]]; then
      install -d -m 700 -o "$account" -g "$(id -gn "$account")" "$root/runtime"
    fi
    runuser -u "$account" -- test -w "$root/runtime" || { echo 'Service account must own its runtime data.' >&2; exit 1; }
    temp=$(mktemp)
    trap 'rm -f -- "$temp"' EXIT
    cat > "$temp" <<EOF
[Unit]
Description=Home AI Core
Wants=network-online.target
After=network-online.target
StartLimitIntervalSec=60
StartLimitBurst=5

[Service]
Type=simple
User=$account
Group=$(id -gn "$account")
WorkingDirectory=$root
ExecStart=$root/build/home-ai-core
Restart=on-failure
RestartSec=5
TimeoutStopSec=90
UMask=0077
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF
    install -m 644 "$temp" "$unit"
    systemctl daemon-reload
    systemctl enable home-ai-core.service
    systemctl restart home-ai-core.service
    systemctl is-enabled --quiet home-ai-core.service || {
      echo 'Failed to enable Home AI Core autostart.' >&2
      exit 1
    }
    systemctl is-active --quiet home-ai-core.service || {
      echo 'Home AI Core service did not start.' >&2
      systemctl status home-ai-core.service --no-pager || true
      exit 1
    }
    echo 'Installed, enabled at boot, and started.'
    ;;
  uninstall)
    [[ $EUID -eq 0 ]] || { echo 'Use sudo for uninstall.' >&2; exit 1; }
    systemctl disable --now home-ai-core.service
    rm -f -- "$unit"
    systemctl daemon-reload
    echo 'Service removed; repository, configuration and data retained.'
    ;;
  start|stop|restart|status)
    systemctl "$action" home-ai-core.service
    ;;
  autostart)
    if systemctl is-enabled --quiet home-ai-core.service; then
      echo 'Autostart: enabled'
    else
      echo 'Autostart: disabled'
      exit 1
    fi
    ;;
  *)
    echo 'Usage: sudo bash scripts/home-ai-service.sh install NON_ROOT_USER [/srv/home-ai-core]'
    echo '       sudo bash scripts/home-ai-service.sh {start|stop|restart|uninstall}'
    echo '       bash scripts/home-ai-service.sh {status|autostart}'
    [[ $action == help ]]
    ;;
esac
