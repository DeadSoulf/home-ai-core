#!/usr/bin/env bash
# Run explicitly on the Debian virtualization host, not through Web/API.
set -euo pipefail
service=home-ai-core.service

service_account() {
    account=$(systemctl show --property=User --value "$service")
    [[ $account =~ ^[a-z_][a-z0-9_-]*\$?$ && $account != root ]] || {
        echo 'Install home-ai-core.service with a non-root User first.' >&2; return 1;
    }
    [[ $(id -u "$account") != 0 ]]
}

check_host() {
    local failed=0
    if [[ -c /dev/kvm ]]; then
        echo 'KVM device: present'
    else
        echo 'KVM device: missing. Check firmware virtualization or nested virtualization in the parent hypervisor.'
        failed=1
    fi
    command -v qemu-system-x86_64 >/dev/null || { echo 'QEMU: missing'; failed=1; }
    if command -v virsh >/dev/null; then
        if [[ $EUID -eq 0 ]]; then
            service_account
            echo "Checking libvirt as service account: $account"
            runuser -u "$account" -- timeout 15 virsh --connect qemu:///system list --all || failed=1
            runuser -u "$account" -- test -r /dev/kvm || failed=1
            runuser -u "$account" -- test -w /dev/kvm || failed=1
        else
            echo "Checking libvirt as current account: $(id -un)"
            timeout 15 virsh --connect qemu:///system list --all || failed=1
            [[ -r /dev/kvm && -w /dev/kvm ]] || failed=1
        fi
    else
        echo 'libvirt clients: missing'
        failed=1
    fi
    if (( failed )); then
        echo 'Setup is incomplete. Review the checks above; no VM was changed.'
    else
        echo 'QEMU, KVM access and the libvirt connection checks passed. VM-specific permissions are checked when an action is requested.'
    fi
    return "$failed"
}

enable_daemons() {
    local daemon
    local -a daemons
    if systemctl is-active --quiet virtqemud.service || systemctl is-active --quiet virtqemud.socket; then
        daemons=(virtqemud virtnetworkd virtstoraged virtnodedevd virtnwfilterd virtsecretd)
    elif systemctl cat libvirtd.service >/dev/null 2>&1; then
        daemons=(libvirtd)
    else
        daemons=(virtqemud virtnetworkd virtstoraged virtnodedevd virtnwfilterd virtsecretd)
    fi
    systemctl cat "${daemons[0]}.service" >/dev/null 2>&1 || {
        echo 'No supported libvirt daemon unit found.' >&2; return 1;
    }
    for daemon in "${daemons[@]}"; do
        if systemctl cat "$daemon.service" >/dev/null 2>&1; then
            systemctl enable --now "$daemon.service"
        fi
    done
}

main() {
case "${1:-help}" in
    check) check_host ;;
    install)
        [[ $EUID -eq 0 ]] || { echo 'Run: sudo bash scripts/setup-hypervisor.sh install' >&2; exit 1; }
        # Validate the destination before any package or permission changes.
        . /etc/os-release
        [[ ${ID:-} == debian ]] || { echo 'Automatic setup supports Debian only.' >&2; exit 1; }
        service_account
        echo "Installing virtualization packages and granting libvirt/kvm access to $account."
        apt-get update
        apt-get install -y qemu-system-x86 libvirt-daemon-system libvirt-clients
        usermod -aG libvirt,kvm "$account"
        # Preserve an existing daemon architecture; never migrate a running host.
        enable_daemons
        # Restart only Core to load the library and refreshed supplementary groups.
        systemctl restart "$service"
        systemctl is-active --quiet "$service"
        check_host
        ;;
    help)
        echo 'Read-only check: sudo bash scripts/setup-hypervisor.sh check'
        echo 'Debian installation: sudo bash scripts/setup-hypervisor.sh install'
        echo 'Install grants VM management access to the Core service account and restarts Core.'
        ;;
    *) echo 'Unknown action. Use check, install or help.' >&2; exit 2 ;;
esac
}

if [[ ${BASH_SOURCE[0]} == "$0" ]]; then main "$@"; fi
