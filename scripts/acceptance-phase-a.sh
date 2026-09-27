#!/usr/bin/env bash
set -u
set -o pipefail

ROOT="${1:-$(pwd)}"
LOGIN_USER="${HOMEAI_ACCEPTANCE_USER:-}"
BUILD_DIR="${HOMEAI_ACCEPTANCE_BUILD_DIR:-$ROOT/build-acceptance}"

pass=0
warn=0
fail=0

green='\033[32m'
yellow='\033[33m'
red='\033[31m'
reset='\033[0m'

say_pass() {
    pass=$((pass + 1))
    printf "%bPASS%b  %s\n" "$green" "$reset" "$*"
}

say_warn() {
    warn=$((warn + 1))
    printf "%bWARN%b  %s\n" "$yellow" "$reset" "$*"
}

say_fail() {
    fail=$((fail + 1))
    printf "%bFAIL%b  %s\n" "$red" "$reset" "$*"
}

config_value() {
    local key=$1
    local file=$2
    awk -F= -v key="$key" '
        $0 !~ /^[[:space:]]*#/ && $1 == key {
            sub(/^[^=]*=/, "", $0)
            print $0
            exit
        }
    ' "$file"
}

run_check() {
    local label=$1
    shift

    if "$@" >/tmp/home-ai-acceptance.out 2>&1; then
        say_pass "$label"
        return 0
    fi

    say_fail "$label"
    sed -n '1,20p' /tmp/home-ai-acceptance.out | sed 's/^/      /'
    return 1
}

printf '\nHome AI Core — Phase A acceptance\n'
printf 'Repository: %s\n\n' "$ROOT"

if [[ ! -d "$ROOT/.git" || ! -f "$ROOT/VERSION" ]]; then
    say_fail "Repository root is invalid: $ROOT"
    exit 1
fi

cd "$ROOT" || exit 1

if [[ $EUID -eq 0 ]]; then
    say_warn "Acceptance is running as root; build/runtime ownership should normally use the service account"
else
    say_pass "Acceptance runs as non-root user $(id -un)"
fi

if [[ -r /etc/os-release ]]; then
    . /etc/os-release
    if [[ "${ID:-}" == "debian" && "${VERSION_ID:-}" == "13" ]]; then
        say_pass "Operating system is Debian 13"
    else
        say_fail "Expected Debian 13, found ${PRETTY_NAME:-unknown}"
    fi
else
    say_fail "Cannot read /etc/os-release"
fi

if [[ "$(uname -m)" == "x86_64" ]]; then
    say_pass "Architecture is x86_64"
else
    say_fail "Expected x86_64, found $(uname -m)"
fi

version=$(tr -d '[:space:]' < VERSION)
if [[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    say_pass "Repository VERSION is $version"
else
    say_fail "Repository VERSION is invalid: ${version:-empty}"
fi

branch=$(git branch --show-current 2>/dev/null || true)
if [[ "$branch" == "develop" ]]; then
    say_pass "Git branch is develop"
else
    say_fail "Expected develop branch, found '${branch:-detached}'"
fi

if [[ -z "$(git status --porcelain --untracked-files=no)" ]]; then
    say_pass "Tracked Git working tree is clean"
else
    say_fail "Tracked Git working tree has local changes"
    git status --short | sed -n '1,20p' | sed 's/^/      /'
fi

for tool in git cmake ninja ctest g++ openssl curl python3; do
    if command -v "$tool" >/dev/null 2>&1; then
        say_pass "Tool available: $tool"
    else
        say_fail "Required acceptance tool missing: $tool"
    fi
done

printf '\n== Clean build and test ==\n'
rm -rf "$BUILD_DIR"

if run_check "CMake configure" cmake -S . -B "$BUILD_DIR" -G Ninja -DCMAKE_BUILD_TYPE=RelWithDebInfo; then
    if run_check "Ninja build" cmake --build "$BUILD_DIR"; then
        run_check "Full CTest suite" ctest --test-dir "$BUILD_DIR" --output-on-failure
    fi
fi

if [[ -d "$ROOT/build" || -L "$ROOT/build" ]]; then
    active_ctest=$(ctest --test-dir "$ROOT/build" -N 2>&1 || true)

    if grep -Fq "Could not find executable" <<<"$active_ctest"; then
        say_fail "Active build contains stale CTest executable paths"
        printf '%s\n' "$active_ctest" | grep -A3 -B1 -F "Could not find executable" | sed -n '1,16p' | sed 's/^/      /'
    else
        say_pass "Active build CTest metadata resolves executable paths"
    fi
else
    say_fail "Active build path is missing: $ROOT/build"
fi

printf '\n== Service ==\n'
if systemctl cat home-ai-core.service >/dev/null 2>&1; then
    say_pass "home-ai-core.service is installed"

    if systemctl is-enabled --quiet home-ai-core.service; then
        say_pass "home-ai-core.service is enabled at boot"
    else
        say_fail "home-ai-core.service is not enabled"
    fi

    if systemctl is-active --quiet home-ai-core.service; then
        say_pass "home-ai-core.service is active"
    else
        say_fail "home-ai-core.service is not active"
        systemctl status home-ai-core.service --no-pager 2>/dev/null | sed -n '1,20p' | sed 's/^/      /'
    fi
else
    say_fail "home-ai-core.service is not installed"
fi

runtime_config="$ROOT/runtime/home-ai.conf"
if [[ -r "$runtime_config" ]]; then
    say_pass "Runtime configuration is readable"
else
    say_fail "Runtime configuration is missing/unreadable: $runtime_config"
fi

printf '\n== Privileged helpers ==\n'
for helper in /usr/local/libexec/home-ai-storage-helper /usr/local/libexec/home-ai-network-helper; do
    if [[ -x "$helper" ]]; then
        owner=$(stat -c '%U:%G %a' "$helper" 2>/dev/null || true)
        if [[ "$owner" == root:* ]]; then
            say_pass "$helper is installed and root-owned ($owner)"
        else
            say_warn "$helper is executable but ownership is unexpected ($owner)"
        fi
    else
        say_fail "$helper is not installed/executable"
    fi
done

printf '\n== Camera/media dependencies ==\n'
for tool in ffmpeg ffprobe; do
    if command -v "$tool" >/dev/null 2>&1; then
        say_pass "$tool is available"
    else
        say_fail "$tool is missing"
    fi
done

printf '\n== Virtualization host ==\n'
virt=$(systemd-detect-virt 2>/dev/null || true)
flags=$(grep -Eoc '(vmx|svm)' /proc/cpuinfo 2>/dev/null || true)

if [[ "${flags:-0}" -gt 0 ]]; then
    say_pass "CPU exposes VMX/SVM flags ($flags)"
else
    if [[ -n "$virt" && "$virt" != "none" ]]; then
        say_warn "No VMX/SVM flags are exposed inside virtualized host '$virt' (nested virtualization unavailable)"
    else
        say_fail "No VMX/SVM flags detected on a non-virtualized host"
    fi
fi

if [[ -e /dev/kvm ]]; then
    if [[ -r /dev/kvm && -w /dev/kvm ]]; then
        say_pass "/dev/kvm exists and is accessible"
    else
        say_warn "/dev/kvm exists but current user cannot read/write it"
    fi
else
    if [[ -n "$virt" && "$virt" != "none" ]]; then
        say_warn "/dev/kvm is absent in virtualized development host"
    else
        say_fail "/dev/kvm is absent"
    fi
fi

if command -v qemu-system-x86_64 >/dev/null 2>&1; then
    say_pass "QEMU system emulator is available"
else
    say_fail "qemu-system-x86_64 is missing"
fi

if command -v virsh >/dev/null 2>&1; then
    if virsh -c qemu:///system uri >/dev/null 2>&1; then
        say_pass "libvirt qemu:///system connection works for current user"
    else
        say_warn "virsh exists but qemu:///system is not accessible to current user"
    fi
else
    say_warn "virsh is not installed; runtime libvirt may still be available through the library"
fi

printf '\n== Web / TLS / security boundary ==\n'
if [[ -r "$runtime_config" ]]; then
    bind=$(config_value web.bind "$runtime_config")
    port=$(config_value web.port "$runtime_config")
    tls=$(config_value web.tls_enabled "$runtime_config")
    cert=$(config_value web.tls_certificate "$runtime_config")
    key=$(config_value web.tls_private_key "$runtime_config")

    [[ -n "$port" ]] || port=8080
    host="$bind"
    if [[ -z "$host" || "$host" == "0.0.0.0" || "$host" == "::" ]]; then
        host=127.0.0.1
    fi

    scheme=http
    curl_tls=()
    if [[ "$tls" == "true" ]]; then
        scheme=https
        curl_tls=(-k)

        if [[ -r "$cert" && -r "$key" ]]; then
            say_pass "TLS is enabled and certificate/key are readable"
        else
            say_fail "TLS is enabled but certificate/key are unreadable"
        fi
    else
        say_fail "TLS is disabled on the real acceptance host"
    fi

    base="$scheme://$host:$port"

    headers_file=$(mktemp)
    body_file=$(mktemp)
    trap 'rm -f "$headers_file" "$body_file" "${cookie_jar:-}" /tmp/home-ai-acceptance.out' EXIT

    runtime_status=$(curl "${curl_tls[@]}" -sS -o "$body_file" -w '%{http_code}' "$base/api/status" 2>/dev/null || true)

    if [[ "$runtime_status" == "200" ]]; then
        running_version=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1])).get("version", ""))' "$body_file" 2>/dev/null || true)

        if [[ "$running_version" == "$version" ]]; then
            say_pass "Running service version matches repository VERSION: $version"
        else
            say_fail "Running service version '${running_version:-unknown}' does not match repository VERSION $version"
        fi
    else
        say_fail "/api/status returned HTTP ${runtime_status:-unreachable}"
    fi

    status=$(curl "${curl_tls[@]}" -sS -D "$headers_file" -o "$body_file" -w '%{http_code}' "$base/login" 2>/dev/null || true)

    if [[ "$status" == "200" ]]; then
        say_pass "Web login page responds at $base"
    else
        say_fail "Web login page returned HTTP ${status:-unreachable} at $base"
    fi

    for header in "X-Content-Type-Options: nosniff" "X-Frame-Options: DENY" "Referrer-Policy: no-referrer"; do
        if grep -Fqi "$header" "$headers_file"; then
            say_pass "Security header present: $header"
        else
            say_fail "Security header missing: $header"
        fi
    done

    if [[ "$scheme" == "https" ]]; then
        if grep -Fqi "Strict-Transport-Security:" "$headers_file"; then
            say_pass "HSTS is present on HTTPS"
        else
            say_fail "HSTS is missing on HTTPS"
        fi
    fi

    unauth=$(curl "${curl_tls[@]}" -sS -o /dev/null -w '%{http_code}' "$base/api/session" 2>/dev/null || true)
    if [[ "$unauth" == "401" ]]; then
        say_pass "Unauthenticated /api/session is rejected with 401"
    else
        say_fail "Unauthenticated /api/session returned HTTP ${unauth:-unreachable}, expected 401"
    fi

    if [[ -n "$LOGIN_USER" ]]; then
        printf '\nAuthenticated read-only API checks for user %s\n' "$LOGIN_USER"
        printf 'Password: '
        IFS= read -r -s LOGIN_PASSWORD
        printf '\n'

        cookie_jar=$(mktemp)
        login_status=$(curl "${curl_tls[@]}" -sS -o /dev/null -w '%{http_code}'             -c "$cookie_jar"             --data-urlencode "username=$LOGIN_USER"             --data-urlencode "password=$LOGIN_PASSWORD"             "$base/login" 2>/dev/null || true)
        unset LOGIN_PASSWORD

        if [[ "$login_status" == "303" ]]; then
            say_pass "Authenticated login succeeded"

            for api in /api/session /api/modules /api/notifications /api/system/readiness; do
                api_status=$(curl "${curl_tls[@]}" -sS -o "$body_file" -w '%{http_code}' -b "$cookie_jar" "$base$api" 2>/dev/null || true)
                if [[ "$api_status" == "200" ]]; then
                    say_pass "Read-only API works: $api"
                else
                    say_fail "Read-only API $api returned HTTP ${api_status:-unreachable}"
                fi
            done
        else
            say_fail "Authenticated login returned HTTP ${login_status:-unreachable}"
        fi
    else
        say_warn "Authenticated API checks skipped; set HOMEAI_ACCEPTANCE_USER to enable them"
    fi
fi

printf '\n== Recent service errors ==\n'
if systemctl is-active --quiet home-ai-core.service; then
    recent_errors=$(journalctl -u home-ai-core.service -p err..alert --since '-10 min' --no-pager -q 2>/dev/null || true)
    if [[ -z "$recent_errors" ]]; then
        say_pass "No recent error-level service log entries"
    else
        say_warn "Recent error-level service log entries exist"
        printf '%s\n' "$recent_errors" | sed -n '1,20p' | sed 's/^/      /'
    fi
fi

printf '\n========================================\n'
printf 'Phase A acceptance result: PASS=%d WARN=%d FAIL=%d\n' "$pass" "$warn" "$fail"

if (( fail > 0 )); then
    printf '%bNOT ACCEPTED%b — fix FAIL items before closing Phase A.\n' "$red" "$reset"
    exit 1
fi

printf '%bACCEPTED%b — Phase A host acceptance passed.\n' "$green" "$reset"
exit 0
