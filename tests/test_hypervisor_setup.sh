#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/../scripts/setup-hypervisor.sh"
# All system service operations are mocked; tests install nothing.
mode=monolithic
selected=()
account_value=coreuser
systemctl() {
    case "$1" in
        show) echo "$account_value" ;;
        is-active) [[ $mode == modular && $3 == virtqemud.service ]] ;;
        cat) [[ $mode != absent ]] ;;
        enable) selected+=("$3") ;;
        *) return 1 ;;
    esac
}
id() { echo 1000; }
service_account
[[ $account == coreuser ]]
account_value=root
if service_account; then echo 'Accepted root account'; exit 1; fi
account_value='invalid;name'
if service_account; then echo 'Accepted invalid account'; exit 1; fi
enable_daemons
[[ ${selected[*]} == libvirtd.service ]]
selected=()
mode=modular
enable_daemons
[[ ${selected[0]} == virtqemud.service && ${#selected[@]} == 6 ]]
[[ ${selected[*]} != *libvirtd* ]]
selected=()
mode=absent
if enable_daemons; then echo 'Accepted missing daemon'; exit 1; fi
[[ ${#selected[@]} == 0 ]]
if (main unknown); then echo 'Accepted unknown action'; exit 1; fi
if [[ $EUID -ne 0 ]]; then
    if (main install); then echo 'Accepted unprivileged install'; exit 1; fi
fi
echo 'Hypervisor setup tests passed'
