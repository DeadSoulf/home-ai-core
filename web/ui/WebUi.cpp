#include "web/ui/WebUi.h"

#include <algorithm>
#include <sstream>
#include <string>

namespace homeai {

namespace {

std::string htmlEscape(
    const std::string& value
)
{
    std::string result;

    for (char c : value) {
        switch (c) {
            case '&':
                result += "&amp;";
                break;
            case '<':
                result += "&lt;";
                break;
            case '>':
                result += "&gt;";
                break;
            case '"':
                result += "&quot;";
                break;
            case '\'':
                result += "&#39;";
                break;
            default:
                result += c;
                break;
        }
    }

    return result;
}

bool uiHasPermission(
    const WebUiContext& context,
    const std::string& permission
)
{
    return
        std::find(
            context.permissions.begin(),
            context.permissions.end(),
            permission
        ) != context.permissions.end();
}

void navLink(
    std::ostringstream& page,
    const WebUiContext& context,
    const std::string& target,
    const std::string& label,
    const std::string& icon
)
{
    page
        << "<a class=\"nav-link"
        << (
            context.page == target
            ? " active"
            : ""
        )
        << "\" href=\""
        << target
        << "\">"
        << "<span class=\"nav-icon\">"
        << icon
        << "</span>"
        << "<span>"
        << htmlEscape(label)
        << "</span>"
        << "</a>";
}

std::string roleDisplay(
    const std::string& role
)
{
    if (role == "admin")
        return "Администратор";

    if (role == "operator")
        return "Оператор";

    return "Наблюдатель";
}

std::string pageTitle(
    const std::string& page
)
{
    if (page == "/")
        return "Главная";

    if (page == "/network")
        return "Сеть";

    if (page == "/storage")
        return "Хранилище";

    if (page == "/files")
        return "Файлы";

    if (page == "/cameras")
        return "Камеры";

    if (page == "/smart-home")
        return "Умный дом";

    if (page == "/ai")
        return "AI";

    if (page == "/users")
        return "Пользователи";

    if (page == "/automation")
        return "Автоматизация";

    if (page == "/hypervisor")
        return "Виртуализация";

    if (page == "/system")
        return "Система";

    if (page == "/admin")
        return "AI / GPU";

    if (page == "/settings")
        return "Настройки";

    return "Home AI Core";
}

void renderSystemStats(
    std::ostringstream& page,
    bool include_core_cards
)
{
    page << R"HTML(
<div class="section-card">
<div class="section-title">
<h2>Статистика</h2>
<span class="section-hint">Обновление каждые 2 секунды</span>
</div>

<div class="stats-grid">
)HTML";

    if (include_core_cards) {
        page << R"HTML(
<div class="stat-card">
<span class="stat-label">Ядро</span>
<strong class="status-ok">RUNNING</strong>
</div>

<div class="stat-card">
<span class="stat-label">Web Core</span>
<strong class="status-ok">RUNNING</strong>
</div>

<div class="stat-card">
<span class="stat-label">Security Core</span>
<strong class="status-ok">RUNNING</strong>
</div>
)HTML";
    }

    page << R"HTML(
<div class="stat-card">
<span class="stat-label">CPU</span>
<strong id="cpu-value">...</strong>
</div>

<div class="stat-card">
<span class="stat-label">RAM</span>
<strong id="ram-value">...</strong>
</div>

<div class="stat-card">
<span class="stat-label">Системный диск</span>
<strong id="disk-value">...</strong>
</div>

<div class="stat-card">
<span class="stat-label">Uptime</span>
<strong id="uptime-value">...</strong>
</div>

<div class="stat-card">
<span class="stat-label">Load Average</span>
<strong id="load-value">...</strong>
</div>
</div>
</div>
)HTML";
}

void renderPlaceholder(
    std::ostringstream& page,
    const std::string& title,
    const std::string& description,
    const std::string& items
)
{
    page
        << "<div class=\"section-card\">"
        << "<h2>"
        << htmlEscape(title)
        << "</h2>"
        << "<p class=\"muted\">"
        << htmlEscape(description)
        << "</p>"
        << "<div class=\"placeholder-grid\">"
        << items
        << "</div>"
        << "</div>";
}

}

bool isWebUiPath(
    const std::string& path
)
{
    return
        path == "/"
        ||
        path == "/system"
        ||
        path == "/network"
        ||
        path == "/storage"
        ||
        path == "/files"
        ||
        path == "/cameras"
        ||
        path == "/smart-home"
        ||
        path == "/automation"
        ||
        path == "/ai"
        ||
        path == "/users"
        ||
        path == "/hypervisor"
        ||
        path == "/settings" || path == "/admin";
}

std::string renderWebUi(
    const WebUiContext& context
)
{
    std::ostringstream page;

    const auto title =
        pageTitle(context.page);

    page << R"HTML(
<!doctype html>
<html lang="ru">
<head>
<script src="/assets/i18n.js"></script>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<title>)HTML";

    page
        << htmlEscape(title)
        << " — "
        << htmlEscape(context.core_name);

    page << R"HTML(</title>
<style>
* {
    box-sizing: border-box;
}

:root {
    color-scheme: dark;
    --bg: #0e1015;
    --sidebar: #15181f;
    --surface: #191d25;
    --surface-2: #11151b;
    --border: #2b313c;
    --text: #edf0f6;
    --muted: #9da5b2;
    --accent: #6aa7ff;
    --accent-soft: #1d2b40;
    --ok: #68dc8b;
    --warn: #f2d784;
    --danger: #ff7e88;
}

html,
body {
    margin: 0;
    min-height: 100%;
    background: var(--bg);
    color: var(--text);
    font-family:
        Inter,
        ui-sans-serif,
        system-ui,
        -apple-system,
        BlinkMacSystemFont,
        "Segoe UI",
        sans-serif;
}

html {
    -webkit-text-size-adjust: 100%;
    text-size-adjust: 100%;
}

body {
    min-height: 100vh;
    min-height: 100dvh;
    overflow-x: hidden;
    overscroll-behavior-y: none;
}

button,
a,
input,
select,
textarea {
    -webkit-tap-highlight-color: transparent;
}

button,
.nav-link,
a {
    touch-action: manipulation;
}

button:focus-visible,
a:focus-visible,
input:focus-visible,
select:focus-visible,
textarea:focus-visible {
    outline: 2px solid var(--accent);
    outline-offset: 2px;
}

pre,
code {
    max-width: 100%;
}

pre {
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
}

body.mobile-menu-open {
    overflow: hidden;
}

a {
    color: inherit;
}

img,
video,
canvas,
svg {
    max-width: 100%;
}

.section-card,
.stat-card,
.placeholder-card,
.storage-card,
.user-card,
.audit-entry {
    min-width: 0;
    overflow-wrap: anywhere;
}

.app-shell {
    min-height: 100vh;
}

.sidebar {
    position: fixed;
    inset: 0 auto 0 0;
    width: 250px;
    padding: 18px 14px;
    background: var(--sidebar);
    border-right: 1px solid var(--border);
    overflow-y: auto;
    overscroll-behavior: contain;
    z-index: 100;
}

.mobile-menu-backdrop {
    display: none;
}

.brand {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 8px 10px 18px;
    border-bottom: 1px solid var(--border);
    margin-bottom: 14px;
}

.brand-mark {
    display: grid;
    place-items: center;
    width: 36px;
    height: 36px;
    border-radius: 10px;
    background: var(--accent-soft);
    color: var(--accent);
    font-weight: 800;
}

.brand-text strong {
    display: block;
    font-size: 0.98rem;
}

.brand-text small {
    color: var(--muted);
}

.nav-group {
    margin-top: 10px;
}

.nav-caption {
    padding: 10px 12px 6px;
    color: #6f7886;
    font-size: 0.72rem;
    font-weight: 800;
    letter-spacing: 0.08em;
    text-transform: uppercase;
}

.nav-link {
    display: flex;
    align-items: center;
    gap: 11px;
    padding: 10px 12px;
    border-radius: 9px;
    color: #b9c0ca;
    text-decoration: none;
    margin: 2px 0;
}

.nav-link:hover {
    background: #1c212a;
    color: white;
}

.nav-link.active {
    background: var(--accent-soft);
    color: #dbeaff;
}

.nav-icon {
    width: 22px;
    text-align: center;
    font-size: 1rem;
}

.sidebar-user {
    margin-top: 16px;
    padding: 12px;
    border-top: 1px solid var(--border);
}

.sidebar-user strong {
    display: block;
}

.sidebar-user small {
    color: var(--muted);
}

.sidebar-user form {
    margin: 10px 0 0;
}

.sidebar-user button {
    width: 100%;
}

.content {
    margin-left: 250px;
    min-height: 100vh;
}

.topbar {
    min-height: 72px;
    padding: 16px 28px;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 20px;
    background: rgba(14, 16, 21, 0.94);
    border-bottom: 1px solid var(--border);
    position: sticky;
    top: 0;
    z-index: 50;
    backdrop-filter: blur(10px);
}

.topbar-left {
    min-width: 0;
    display: flex;
    align-items: center;
    gap: 12px;
}

.topbar-title {
    min-width: 0;
}

.topbar h1 {
    margin: 0;
    font-size: 1.35rem;
    overflow-wrap: anywhere;
}

.topbar-meta {
    color: var(--muted);
    font-size: 0.9rem;
}

.topbar-actions {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 12px;
    min-width: 0;
}

.mobile-menu-button {
    display: none;
    flex: 0 0 auto;
    width: 44px;
    height: 44px;
    padding: 0;
    align-items: center;
    justify-content: center;
    background: #252b35;
    color: var(--text);
    border: 1px solid #38404c;
    font-size: 1.35rem;
    line-height: 1;
}

.page {
    max-width: 1320px;
    margin: 0 auto;
    padding: 26px 28px 50px;
}

.section-card {
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 13px;
    padding: 20px;
    margin-bottom: 18px;
}

.section-card h2,
.section-card h3 {
    margin-top: 0;
}

.section-title {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 14px;
    margin-bottom: 16px;
}

.section-title h2 {
    margin: 0;
}

.section-hint,
.muted,
small {
    color: var(--muted);
}

.stats-grid,
.placeholder-grid,
.storage-grid {
    display: grid;
    grid-template-columns:
        repeat(auto-fit, minmax(210px, 1fr));
    gap: 12px;
}

.stat-card,
.placeholder-card,
.storage-card {
    background: var(--surface-2);
    border: 1px solid #242a34;
    border-radius: 10px;
    padding: 15px;
}

.camera-discovery-list {
    display: grid;
    gap: 8px;
}

.camera-discovery-row {
    display: grid;
    grid-template-columns:
        minmax(0, 1fr)
        auto;
    align-items: center;
    gap: 12px;
    padding: 10px 12px;
    background: var(--surface-2);
    border: 1px solid #242a34;
    border-radius: 8px;
}

.camera-discovery-main {
    min-width: 0;
}

.camera-discovery-row strong {
    min-width: 0;
    overflow-wrap: anywhere;
}

.camera-discovery-meta {
    margin-top: 4px;
    color: var(--muted);
    font-size: 14px;
    line-height: 1.4;
    overflow-wrap: anywhere;
}

.camera-discovery-row button {
    margin: 0;
}

.stat-card strong {
    display: block;
    margin-top: 8px;
    font-size: 1.25rem;
}

.stat-label {
    color: var(--muted);
}

.status-ok {
    color: var(--ok);
}

.status-warn {
    color: var(--warn);
}

.status-error {
    color: var(--danger);
}

.error-list {
    display: grid;
    gap: 9px;
}

.error-item {
    padding: 11px 12px;
    border-radius: 8px;
    background: #30191d;
    color: #ffc2c7;
    border: 1px solid #5b2a31;
}

.error-ok {
    padding: 11px 12px;
    border-radius: 8px;
    background: #14261b;
    color: #9ce0ae;
    border: 1px solid #285137;
}

.form-grid {
    display: grid;
    grid-template-columns:
        repeat(auto-fit, minmax(240px, 1fr));
    gap: 14px;
}

label {
    display: block;
    margin-bottom: 6px;
    color: #c8ced8;
}

input,
select,
textarea {
    width: 100%;
    padding: 10px 11px;
    border-radius: 8px;
    border: 1px solid #363d49;
    background: #0f1217;
    color: white;
}

textarea {
    resize: vertical;
}

#vpn-profile-config {
    min-height: 340px;
    font-family:
        ui-monospace,
        SFMono-Regular,
        Menlo,
        Monaco,
        Consolas,
        "Liberation Mono",
        monospace;
    line-height: 1.45;
}

button {
    min-height: 42px;
    padding: 10px 14px;
    border: 0;
    border-radius: 8px;
    cursor: pointer;
    font-weight: 700;
}

button:disabled {
    cursor: not-allowed;
    opacity: 0.45;
}

.button-row {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: 15px;
}

.secondary {
    background: #303641;
    color: white;
}

.danger {
    background: #8c2d36;
    color: white;
}

.storage-toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 14px;
    flex-wrap: wrap;
}

.storage-toolbar h2 {
    margin: 0;
}

.hotplug-alert {
    display: none;
    margin: 14px 0;
    padding: 12px;
    border-radius: 8px;
    border: 1px solid #755d26;
    background: #2b2515;
    color: var(--warn);
}

.device-actions,
.disk-menu-actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: 12px;
}

.device-actions button,
.disk-menu-actions button {
    margin: 0;
}

.device-note {
    margin-top: 10px;
    color: var(--muted);
    font-size: 0.92rem;
}

.disk-menu-overlay {
    display: none;
    position: fixed;
    inset: 0;
    z-index: 1000;
    background: rgba(0, 0, 0, 0.72);
    align-items: center;
    justify-content: center;
    padding: 20px;
}

.disk-menu-panel {
    width: min(650px, 100%);
    max-height: 90vh;
    overflow: auto;
    background: var(--surface);
    border: 1px solid #363c49;
    border-radius: 14px;
    padding: 22px;
}

.disk-menu-info {
    padding: 12px;
    border-radius: 8px;
    background: #101319;
    line-height: 1.6;
    margin-bottom: 14px;
}

.disk-menu-group {
    border-top: 1px solid var(--border);
    margin-top: 14px;
    padding-top: 14px;
}

.disk-menu-warning {
    margin-top: 12px;
    color: var(--warn);
}

.disk-menu-result {
    display: none;
    margin-top: 12px;
    padding: 10px;
    border-radius: 8px;
    background: #15261b;
    color: #9ce0ae;
}

.disk-menu-result.error {
    background: #35191d;
    color: #ffb8c0;
}

.kv {
    display: grid;
    grid-template-columns: 180px 1fr;
    gap: 8px 14px;
}

.kv div:nth-child(odd) {
    color: var(--muted);
}

.users-grid {
    display: grid;
    gap: 12px;
}

.user-card {
    background: var(--surface-2);
    border: 1px solid #242a34;
    border-radius: 10px;
    padding: 15px;
}

.user-card-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 12px;
    flex-wrap: wrap;
}

.user-card-header h3 {
    margin: 0;
}

.user-meta {
    display: grid;
    grid-template-columns:
        repeat(auto-fit, minmax(170px, 1fr));
    gap: 8px 14px;
    margin-top: 12px;
    color: var(--muted);
}

.permission-grid {
    display: grid;
    grid-template-columns:
        repeat(auto-fit, minmax(260px, 1fr));
    gap: 8px;
    margin-top: 12px;
}

.permission-row {
    display: grid;
    grid-template-columns:
        minmax(0, 1fr) 120px;
    gap: 10px;
    align-items: center;
    padding: 8px;
    border-radius: 8px;
    background: #0f1217;
}

.permission-chip-list {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    margin-top: 10px;
}

.permission-chip {
    padding: 4px 7px;
    border-radius: 999px;
    background: #202632;
    color: #c7d0dc;
    font-size: 0.78rem;
}

.audit-list {
    display: grid;
    gap: 8px;
}

.audit-entry {
    border-left: 3px solid #3b4658;
    background: var(--surface-2);
    padding: 10px 12px;
    border-radius: 8px;
}

.audit-entry strong {
    display: inline-block;
    margin-right: 8px;
}

.update-progress-shell {
    margin-top: 16px;
    padding: 14px;
    border-radius: 10px;
    background: var(--surface-2);
    border: 1px solid #242a34;
}

.update-progress-head {
    display: flex;
    align-items: center;
    gap: 12px;
}

.update-progress-track {
    flex: 1;
    height: 12px;
    overflow: hidden;
    border-radius: 999px;
    background: #0d1015;
    border: 1px solid #2d3440;
}

.update-progress-bar {
    width: 0%;
    height: 100%;
    background: var(--accent);
    transition: width 0.3s ease;
}

.update-progress-percent {
    min-width: 48px;
    text-align: right;
    font-weight: 800;
}

.update-progress-detail {
    margin-top: 8px;
    color: var(--muted);
    font-size: 0.9rem;
}

.update-stage-list {
    display: grid;
    gap: 7px;
    margin-top: 14px;
}

.update-stage-row {
    display: grid;
    grid-template-columns: 24px minmax(0, 1fr) 48px;
    align-items: center;
    gap: 8px;
    padding: 8px 10px;
    border-radius: 8px;
    background: #11151b;
    border: 1px solid #242a34;
}

.update-stage-row.current {
    border-color: #45638b;
    background: #162033;
}

.update-stage-row.done .update-stage-icon {
    color: var(--ok);
}

.update-stage-row.error {
    border-color: #6b3138;
    background: #2a171b;
}

.update-stage-row.error .update-stage-icon {
    color: var(--danger);
}

.update-stage-row.pending {
    opacity: 0.68;
}

@media (max-width: 860px) {
    .sidebar {
        position: fixed;
        inset: 0 auto 0 0;
        width: min(86vw, 320px);
        height: 100vh;
        height: 100dvh;
        padding:
            max(14px, env(safe-area-inset-top))
            12px
            max(14px, env(safe-area-inset-bottom));
        border-right: 1px solid var(--border);
        border-bottom: 0;
        transform: translateX(-105%);
        transition: transform 0.22s ease;
        box-shadow: 18px 0 45px rgba(0, 0, 0, 0.34);
        z-index: 1000;
    }

    body.mobile-menu-open .sidebar {
        transform: translateX(0);
    }

    .mobile-menu-backdrop {
        display: block;
        position: fixed;
        inset: 0;
        background: rgba(0, 0, 0, 0.62);
        opacity: 0;
        visibility: hidden;
        pointer-events: none;
        transition:
            opacity 0.22s ease,
            visibility 0.22s ease;
        z-index: 900;
    }

    body.mobile-menu-open .mobile-menu-backdrop {
        opacity: 1;
        visibility: visible;
        pointer-events: auto;
    }

    .content {
        margin-left: 0;
        min-width: 0;
    }

    .mobile-menu-button {
        display: inline-flex;
    }

    .nav-link {
        min-height: 48px;
        padding: 12px 13px;
        font-size: 0.98rem;
    }

    .nav-caption {
        padding-top: 14px;
    }

    .brand {
        position: sticky;
        top: 0;
        z-index: 2;
        background: var(--sidebar);
        padding-top: 6px;
    }

    .topbar {
        min-height: 64px;
        padding:
            max(10px, env(safe-area-inset-top))
            14px
            10px;
        position: sticky;
        top: 0;
        gap: 10px;
    }

    .topbar-left {
        flex: 1 1 auto;
    }

    .topbar-actions {
        flex: 0 1 auto;
    }

    .topbar h1 {
        font-size: 1.12rem;
    }

    .topbar-title > .topbar-meta {
        font-size: 0.78rem;
    }

    .topbar > label {
        order: 3;
        padding: 0 !important;
        margin: 0 !important;
        font-size: 0;
    }

    .topbar > label select {
        width: auto;
        min-width: 76px;
        min-height: 42px;
        padding: 8px 9px;
        font-size: 0.9rem;
    }

    .topbar-user-summary {
        display: none;
    }

    #update-badge {
        margin-right: 0 !important;
        font-size: 0.82rem;
        white-space: nowrap;
    }

    .page {
        width: 100%;
        padding:
            16px
            14px
            max(34px, env(safe-area-inset-bottom));
    }

    .section-card {
        padding: 16px;
        margin-bottom: 14px;
        border-radius: 11px;
    }

    .stats-grid,
    .placeholder-grid,
    .storage-grid,
    .form-grid,
    .user-meta,
    .permission-grid {
        grid-template-columns: minmax(0, 1fr);
    }

    input,
    select,
    textarea {
        min-width: 0;
        font-size: 16px;
    }

    input:not([type="checkbox"]):not([type="radio"]),
    select,
    button {
        min-height: 48px;
    }

    input[type="checkbox"],
    input[type="radio"] {
        min-height: 22px;
        min-width: 22px;
    }

    .section-card p,
    .device-note,
    .muted {
        line-height: 1.5;
    }

    .stat-card,
    .placeholder-card,
    .storage-card,
    .user-card,
    .audit-entry {
        padding: 14px;
    }

    .user-card,
    .audit-entry,
    .storage-card {
        overflow: hidden;
    }

    .user-card *,
    .audit-entry *,
    .storage-card * {
        min-width: 0;
    }

    .kv {
        grid-template-columns: minmax(0, 1fr);
        gap: 4px;
    }

    .kv div:nth-child(even) {
        margin-bottom: 10px;
    }

    .permission-row {
        grid-template-columns: minmax(0, 1fr);
    }

    .disk-menu-overlay {
        align-items: flex-end;
        padding:
            8px
            8px
            max(8px, env(safe-area-inset-bottom));
    }

    .disk-menu-panel {
        width: 100%;
        max-height: calc(100dvh - 16px);
        padding: 16px;
        border-radius: 16px 16px 10px 10px;
    }

    #vpn-profile-config {
        min-height: 280px;
    }
}

@media (max-width: 600px) {
    .topbar {
        display: grid;
        grid-template-columns:
            minmax(0, 1fr)
            auto;
        align-items: center;
        gap: 8px 10px;
    }

    .topbar-left {
        grid-column: 1;
        grid-row: 1;
        width: auto;
        min-width: 0;
    }

    .topbar-actions {
        grid-column: 1 / -1;
        grid-row: 2;
        justify-content: flex-start;
        margin: 0;
        min-height: 0;
    }

    .topbar-actions:has(#update-badge[style*="display:none"]) {
        display: none;
    }

    .topbar > label {
        grid-column: 2;
        grid-row: 1;
        order: initial;
    }

    .mobile-menu-button {
        width: 48px;
        height: 48px;
    }

    .topbar-title {
        overflow: hidden;
    }

    .topbar h1 {
        overflow: hidden;
        text-overflow: ellipsis;
        white-space: nowrap;
    }

    .section-title,
    .storage-toolbar,
    .user-card-header {
        align-items: stretch;
        flex-direction: column;
    }

    .section-title > button,
    .storage-toolbar > button {
        width: 100%;
    }

    .button-row {
        display: grid;
        grid-template-columns: minmax(0, 1fr);
    }

    .button-row > button,
    .button-row > a,
    .device-actions > button,
    .disk-menu-actions > button {
        width: 100%;
        min-height: 48px;
    }

    .button-row > a {
        display: inline-flex;
        align-items: center;
        justify-content: center;
        padding: 10px 14px;
        border-radius: 8px;
        text-decoration: none;
    }

    .kv > div:nth-child(odd) {
        color: var(--muted);
        font-size: 0.85rem;
        margin-top: 4px;
    }

    .kv > div:nth-child(even) {
        padding-bottom: 9px;
        border-bottom: 1px solid #242a34;
    }

    .kv > div:nth-last-child(-n + 2) {
        border-bottom: 0;
    }

    .device-actions,
    .disk-menu-actions {
        display: grid;
        grid-template-columns: minmax(0, 1fr);
    }

    .update-progress-head {
        align-items: stretch;
        flex-direction: column;
    }

    .update-progress-percent {
        min-width: 0;
        text-align: left;
    }

    .update-stage-row {
        grid-template-columns:
            22px
            minmax(0, 1fr)
            44px;
        padding: 8px;
    }

    .sidebar-user {
        padding-left: 10px;
        padding-right: 10px;
    }

    .sidebar-user button {
        min-height: 44px;
    }
}

@media (max-width: 420px) {
    .topbar-title > .topbar-meta {
        display: none;
    }

    .topbar > label select {
        min-width: 70px;
        padding-left: 7px;
        padding-right: 7px;
    }

    .section-title h2,
    .storage-toolbar h2 {
        font-size: 1.12rem;
    }
}

@media (max-width: 380px) {
    .page {
        padding-left: 10px;
        padding-right: 10px;
    }

    .section-card {
        padding: 13px;
    }

    .topbar {
        padding-left: 10px;
        padding-right: 10px;
    }

    .topbar h1 {
        font-size: 1rem;
    }

    #update-badge {
        white-space: normal;
        text-align: right;
    }
}
</style>
</head>
<body>
<div class="app-shell">

<aside id="sidebar-navigation" class="sidebar">
<div class="brand">
<div class="brand-mark">AI</div>
<div class="brand-text">
<strong>)HTML";

    page
        << "<span data-i18n-skip>" << htmlEscape(context.core_name) << "</span>"
        << "</strong><small>v"
        << htmlEscape(
            context.version
        )
        << "</small></div></div>";

    page << "<nav>";

    const bool show_overview =
        uiHasPermission(
            context,
            "system.view"
        );

    const bool show_server =
        uiHasPermission(
            context,
            "system.view"
        )
        ||
        uiHasPermission(
            context,
            "network.view"
        )
        ||
        uiHasPermission(
            context,
            "storage.view"
        )
        ||
        uiHasPermission(
            context,
            "hypervisor.view"
        );

    const bool show_services =
        uiHasPermission(
            context,
            "files.read"
        )
        ||
        uiHasPermission(
            context,
            "cameras.view"
        )
        ||
        uiHasPermission(
            context,
            "smart_home.view"
        )
        ||
        uiHasPermission(
            context,
            "automation.view"
        );

    const bool show_ai =
        uiHasPermission(
            context,
            "ai.use"
        )
        ||
        uiHasPermission(
            context,
            "ai.manage"
        );

    const bool show_management =
        uiHasPermission(
            context,
            "users.view"
        )
        ||
        uiHasPermission(
            context,
            "system.manage"
        );

    if (show_overview) {
        page << "<div class=\"nav-group\">";
        page << "<div class=\"nav-caption\">Обзор</div>";

        navLink(
            page,
            context,
            "/",
            "Главная",
            "⌂"
        );

        page << "</div>";
    }

    if (show_server) {
        page << "<div class=\"nav-group\">";
        page << "<div class=\"nav-caption\">Сервер</div>";

        if (
            uiHasPermission(
                context,
                "system.view"
            )
        ) {
            navLink(
                page,
                context,
                "/system",
                "Система",
                "▣"
            );
        }

        if (
            uiHasPermission(
                context,
                "network.view"
            )
        ) {
            navLink(
                page,
                context,
                "/network",
                "Сеть",
                "⇄"
            );
        }

        if (
            uiHasPermission(
                context,
                "storage.view"
            )
        ) {
            navLink(
                page,
                context,
                "/storage",
                "Хранилище",
                "◫"
            );
        }

        if (
            uiHasPermission(
                context,
                "hypervisor.view"
            )
        ) {
            navLink(
                page,
                context,
                "/hypervisor",
                "Виртуализация",
                "▤"
            );
        }

        page << "</div>";
    }

    if (show_services) {
        page << "<div class=\"nav-group\">";
        page << "<div class=\"nav-caption\">Сервисы</div>";

        if (
            uiHasPermission(
                context,
                "files.read"
            )
        ) {
            navLink(
                page,
                context,
                "/files",
                "Файлы",
                "▱"
            );
        }

        if (
            uiHasPermission(
                context,
                "cameras.view"
            )
        ) {
            navLink(
                page,
                context,
                "/cameras",
                "Камеры",
                "◉"
            );
        }

        if (
            uiHasPermission(
                context,
                "smart_home.view"
            )
        ) {
            navLink(
                page,
                context,
                "/smart-home",
                "Умный дом",
                "⌁"
            );
        }

        if (
            uiHasPermission(
                context,
                "automation.view"
            )
        ) {
            navLink(
                page,
                context,
                "/automation",
                "Автоматизация",
                "⚙"
            );
        }

        page << "</div>";
    }

    if (show_ai) {
        page << "<div class=\"nav-group\">";
        page << "<div class=\"nav-caption\">AI</div>";

        if (
            uiHasPermission(
                context,
                "ai.use"
            )
        ) {
            navLink(
                page,
                context,
                "/ai",
                "AI",
                "✦"
            );
        }

        if (
            uiHasPermission(
                context,
                "ai.manage"
            )
        ) {
            navLink(
                page,
                context,
                "/admin",
                "AI / GPU",
                "⚒"
            );
        }

        page << "</div>";
    }

    if (show_management) {
        page << "<div class=\"nav-group\">";
        page << "<div class=\"nav-caption\">Управление</div>";

        if (
            uiHasPermission(
                context,
                "users.view"
            )
        ) {
            navLink(
                page,
                context,
                "/users",
                "Пользователи",
                "♙"
            );
        }

        if (
            uiHasPermission(
                context,
                "system.manage"
            )
        ) {
            navLink(
                page,
                context,
                "/settings",
                "Настройки",
                "⚙"
            );
        }

        page << "</div>";
    }

    page << "</nav>";

    page
        << "<div class=\"sidebar-user\">"
        << "<div><strong>"
        << "<span data-i18n-skip>" << htmlEscape(context.username) << "</span>"
        << "</strong><small>"
        << htmlEscape(
            roleDisplay(
                context.role
            )
        )
        << "</small></div>"
        << "<form method=\"POST\" action=\"/logout\">"
        << "<button type=\"submit\" class=\"secondary\">Выйти</button>"
        << "</form></div>";

    page << R"HTML(
</aside>
<div
    id="mobile-menu-backdrop"
    class="mobile-menu-backdrop"
    aria-hidden="true"></div>

<div class="content">
<header class="topbar">
<div class="topbar-left">
<button
    id="mobile-menu-button"
    class="mobile-menu-button"
    type="button"
    aria-label="Открыть меню"
    aria-controls="sidebar-navigation"
    aria-expanded="false">☰</button>
<div class="topbar-title">
<h1>)HTML";

    page
        << htmlEscape(title)
        << "</h1>"
        << "<div class=\"topbar-meta\">"
        << "<span data-i18n-skip>" << htmlEscape(context.core_name) << "</span>"
        << " · "
        << htmlEscape(
            context.version
        )
        << "</div></div></div>"
        << "<div class=\"topbar-meta topbar-actions\">"
        << "<a id=\"update-badge\" href=\"/system\" "
           "style=\"display:none;margin-right:14px;color:#f2d784;text-decoration:none;font-weight:700\">"
           "Доступно обновление</a>"
        << "<span class=\"topbar-user-summary\">"
        << "<span data-i18n-skip>" << htmlEscape(context.username) << "</span>"
        << " · "
        << htmlEscape(
            roleDisplay(
                context.role
            )
        )
        << "</span></div>"
        << "</header><main class=\"page\">";

    if (
        uiHasPermission(
            context,
            "ai.manage"
        )
    ) {
        page << R"HTML(<div id="gpu-notice" role="status" class="section-card" hidden></div>)HTML";
    }

    if (
        context.page == "/admin"
        &&
        uiHasPermission(
            context,
            "ai.manage"
        )
    ) {
        page << R"HTML(
<div class="section-card">
<h2>GPU / AI accelerator</h2>
<p>Выберите видеокарту для AI. Выбор сохраняется в конфигурации; совместимость с AI требует отдельной проверки. Драйверы автоматически не устанавливаются.</p>
<div id="gpu-list"></div>
<p id="gpu-selection"></p>
<button type="button" id="gpu-clear">Снять назначение GPU</button>
<p id="gpu-action" role="status"></p>
</div>)HTML";
    }
    else if (context.page == "/") {
        renderSystemStats(
            page,
            true
        );

        page << R"HTML(
<div class="section-card">
<div class="section-title">
<h2>Ошибки и предупреждения</h2>
<span class="section-hint">Текущие контролируемые проверки</span>
</div>
<div id="home-errors" class="error-list">
<div class="error-ok">Проверка состояния...</div>
</div>
</div>
)HTML";
    }
    else if (
        context.page == "/system"
    ) {
        renderSystemStats(
            page,
            true
        );

        page << R"HTML(
<div class="section-card">
<h2>Состояние ядра</h2>
<div class="kv">
<div>Core Runtime</div><div class="status-ok">RUNNING</div>
<div>Web Core</div><div class="status-ok">RUNNING</div>
<div>Security Core</div><div class="status-ok">RUNNING</div>
<div>Версия</div><div>)HTML";

        page
            << htmlEscape(
                context.version
            )
            << R"HTML(</div>
</div>
</div>

<div class="section-card">
<div class="section-title">
<h2>Модули ядра</h2>
<span class="section-hint">Module Manager lifecycle / health</span>
</div>

<div id="module-list" class="placeholder-grid">
<div class="placeholder-card">Загрузка состояния модулей...</div>
</div>
</div>

<div class="section-card">
<div class="section-title">
<h2>Обновление сервера</h2>
<span class="section-hint">GitHub → build → tests → restart</span>
</div>

<div class="kv">
<div>Ветка</div><div id="update-branch">...</div>
<div>Локальная версия</div><div id="update-local">...</div>
<div>GitHub версия</div><div id="update-remote">...</div>
<div>Состояние</div><div id="update-state">...</div>
</div>

<div id="update-message" class="muted" style="margin-top:14px">
Проверка состояния обновлений...
</div>

<div class="update-progress-shell">
<div class="update-progress-head">
<div class="update-progress-track">
<div id="update-progress-bar" class="update-progress-bar"></div>
</div>
<div id="update-progress-percent" class="update-progress-percent">0%</div>
</div>

<div id="update-progress-detail" class="update-progress-detail">
Ожидание...
</div>

<div id="update-stage-list" class="update-stage-list">
<div class="update-stage-row pending" data-update-stage="check" data-threshold="5">
<span class="update-stage-icon">○</span><span>Проверка GitHub</span><span>5%</span>
</div>
<div class="update-stage-row pending" data-update-stage="download" data-threshold="15">
<span class="update-stage-icon">○</span><span>Получение изменений</span><span>15%</span>
</div>
<div class="update-stage-row pending" data-update-stage="configure" data-threshold="25">
<span class="update-stage-icon">○</span><span>Подготовка CMake</span><span>25%</span>
</div>
<div class="update-stage-row pending" data-update-stage="build" data-threshold="70">
<span class="update-stage-icon">○</span><span>Сборка</span><span>70%</span>
</div>
<div class="update-stage-row pending" data-update-stage="tests" data-threshold="90">
<span class="update-stage-icon">○</span><span>Тестирование</span><span>90%</span>
</div>
<div class="update-stage-row pending" data-update-stage="activation" data-threshold="95">
<span class="update-stage-icon">○</span><span>Активация новой версии</span><span>95%</span>
</div>
<div class="update-stage-row pending" data-update-stage="restart" data-threshold="100">
<span class="update-stage-icon">○</span><span>Перезапуск</span><span>100%</span>
</div>
</div>
</div>

<div class="button-row">
<button id="update-check-btn" type="button" class="secondary">
Проверить обновления
</button>
)HTML";

        if (
            uiHasPermission(
                context,
                "system.manage"
            )
        ) {
            page << R"HTML(
<button id="update-apply-btn" type="button" disabled>
Обновить сервер
</button>
<button id="update-restart-btn" type="button" class="secondary" disabled>
Перезапустить сервер
</button>
)HTML";
        }

        page << R"HTML(
</div>

<details id="update-log-details" style="margin-top:14px">
<summary>Показать подробный журнал</summary>
<pre id="update-output"
style="display:none;white-space:pre-wrap;background:#0f1217;padding:12px;border-radius:8px;overflow:auto;max-height:360px"></pre>
</details>
</div>
)HTML";
    }
    else if (
        context.page == "/network"
    ) {
        page << R"HTML(
<div class="section-card">
<h2>Web / сеть управления</h2>
<div class="kv">
<div>Bind address</div><div>)HTML";

        page
            << htmlEscape(
                context.web_bind
            )
            << "</div><div>Web port</div><div>"
            << htmlEscape(
                context.web_port
            )
            << R"HTML(</div>
<div>Web Core</div><div class="status-ok">RUNNING</div>
</div>
</div>
)HTML";

        if (
            uiHasPermission(
                context,
                "network.manage"
            )
        ) {
            page << R"HTML(
<div class="section-card">
<h2>Настройки сети</h2>
<form method="POST" action="/api/config">
<input type="hidden" name="return_to" value="/network">

<div class="form-grid">
<div>
<label>Web bind address</label>
<input name="web.bind" value=")HTML";

            page
                << htmlEscape(
                    context.web_bind
                )
                << R"HTML(">
</div>

<div>
<label>Web port</label>
<input
    type="number"
    min="1"
    max="65535"
    name="web.port"
    value=")HTML";

            page
                << htmlEscape(
                    context.web_port
                )
                << R"HTML(">
</div>
</div>

<div class="button-row">
<button type="submit">Сохранить</button>
</div>

<p class="muted">
Изменение bind address или порта вступит в силу после перезапуска ядра.
</p>
</form>
</div>
)HTML";
        }

        page << R"HTML(
<div class="section-card">
<div class="section-title">
<h2>Сетевые интерфейсы</h2>
<span class="section-hint">DHCP / статический IPv4</span>
</div>

<p class="muted">
Home AI Core показывает текущие IPv4-адреса интерфейсов.
Для каждого интерфейса можно выбрать DHCP или статический IPv4 с адресом,
маской, шлюзом и DNS. Если этот интерфейс используется для Web,
после применения настроек адрес сервера может измениться.
</p>

<div id="network-helper-message" class="muted" style="margin-bottom:12px"></div>
<div id="network-interface-list" class="storage-grid" data-can-manage=")HTML";

        page
            << (
                uiHasPermission(
                    context,
                    "network.manage"
                )
                ? "1"
                : "0"
            )
            << R"HTML(">
<div class="storage-card">Загрузка сетевых интерфейсов...</div>
</div>
</div>

<div class="section-card">
<div class="section-title">
<h2>WireGuard</h2>
<span class="section-hint">VPN-подключения сервера</span>
</div>

<p class="muted">
Профили WireGuard читаются из runtime/wireguard/*.conf.
Для подключения на Debian должен быть установлен пакет wireguard-tools,
а процесс Home AI Core должен иметь системное разрешение на управление
сетевыми интерфейсами.
</p>

<div class="button-row">
<button id="vpn-refresh-btn" type="button" class="secondary">
Обновить WireGuard
</button>
</div>

<div id="vpn-message" class="muted" style="margin-top:12px"></div>

<div id="vpn-list" class="placeholder-grid" style="margin-top:14px">
<div class="placeholder-card">Загрузка профилей WireGuard...</div>
</div>
</div>
)HTML";

        if (
            uiHasPermission(
                context,
                "network.manage"
            )
        ) {
            page << R"HTML(
<div class="section-card" id="vpn-editor">
<div class="section-title">
<h2>Редактор WireGuard</h2>
<span class="section-hint">Создание и изменение *.conf</span>
</div>

<p class="muted">
Конфигурация загружается только при нажатии «Редактировать».
PrivateKey отображается в редакторе и сохраняется в runtime/wireguard
с правами 0600. Для активного туннеля сохранённые изменения вступят
в силу после переподключения профиля.
</p>

<div class="form-grid">
<div>
<label for="vpn-profile-name">Имя профиля</label>
<input
    id="vpn-profile-name"
    type="text"
    maxlength="32"
    autocomplete="off"
    placeholder="wg0">
</div>
</div>

<div style="margin-top:14px">
<label for="vpn-profile-config">Конфигурация WireGuard</label>
<textarea
    id="vpn-profile-config"
    spellcheck="false"
    autocomplete="off"
    placeholder="[Interface]"></textarea>
</div>

<div class="button-row">
<button id="vpn-new-btn" type="button" class="secondary">
Новый профиль
</button>
<button id="vpn-save-btn" type="button">
Сохранить
</button>
<button id="vpn-delete-btn" type="button" class="danger" disabled>
Удалить
</button>
</div>

<div id="vpn-editor-message" class="muted" style="margin-top:12px">
Выберите «Редактировать» у существующего профиля или создайте новый.
</div>
</div>
)HTML";
        }

        renderPlaceholder(
            page,
            "Network Core",
            "Интерфейсы и DHCP уже доступны. Следующие этапы — маршруты, DNS и диагностика сети.",
            "<div class=\"placeholder-card\">Интерфейсы / DHCP — READY</div>"
            "<div class=\"placeholder-card\">Маршруты — PLANNED</div>"
            "<div class=\"placeholder-card\">DNS — PLANNED</div>"
            "<div class=\"placeholder-card\">Диагностика — PLANNED</div>"
        );
    }
    else if (
        context.page == "/storage"
    ) {
        page << R"HTML(
<div class="section-card">
<div class="storage-toolbar">
<h2>Диски и хранилища</h2>
<button id="scan-storage-btn" type="button">
Проверить новые диски
</button>
</div>

<p class="muted">
Здесь находятся все операции с дисками, назначение хранилищ для камер
и личных файлов, а также hot-plug обнаружение.
</p>

<div id="storage-hotplug-alert" class="hotplug-alert"></div>

<h3>Пулы хранения</h3>
<div id="storage-pools" class="storage-grid">
<div class="storage-card">Загрузка пулов хранения...</div>
</div>

<h3 style="margin-top:22px">Подключённые хранилища</h3>
<div id="storage-list" class="storage-grid">
<div class="storage-card">Загрузка информации о дисках...</div>
</div>

<h3 style="margin-top:22px">Новые / неиспользуемые диски</h3>
<div id="storage-candidates" class="storage-grid">
<div class="storage-card">Поиск новых дисков...</div>
</div>
</div>
)HTML";

        if (
            uiHasPermission(
                context,
                "storage.manage"
            )
        ) {
            page << R"HTML(
<div class="section-card">
<h2>Настройки пулов хранения</h2>
<form method="POST" action="/api/config">
<input type="hidden" name="return_to" value="/storage">

<div class="form-grid">
<div>
<label>Стратегия видео</label>
<select name="storage.video_policy">
<option value="most_free")HTML";

            page
                << (
                    context.storage_video_policy ==
                        "most_free"
                    ? " selected"
                    : ""
                )
                << R"HTML(>Больше всего свободного места</option>
<option value="sequential")HTML";

            page
                << (
                    context.storage_video_policy ==
                        "sequential"
                    ? " selected"
                    : ""
                )
                << R"HTML(>Заполнять диски по очереди</option>
<option value="balanced")HTML";

            page
                << (
                    context.storage_video_policy ==
                        "balanced"
                    ? " selected"
                    : ""
                )
                << R"HTML(>Равномерная загрузка</option>
<option value="pinned")HTML";

            page
                << (
                    context.storage_video_policy ==
                        "pinned"
                    ? " selected"
                    : ""
                )
                << R"HTML(>Закрепление за диском</option>
</select>
</div>

<div>
<label>Стратегия файлов</label>
<select name="storage.files_policy">
<option value="most_free")HTML";

            page
                << (
                    context.storage_files_policy ==
                        "most_free"
                    ? " selected"
                    : ""
                )
                << R"HTML(>Больше всего свободного места</option>
<option value="sequential")HTML";

            page
                << (
                    context.storage_files_policy ==
                        "sequential"
                    ? " selected"
                    : ""
                )
                << R"HTML(>Заполнять диски по очереди</option>
<option value="balanced")HTML";

            page
                << (
                    context.storage_files_policy ==
                        "balanced"
                    ? " selected"
                    : ""
                )
                << R"HTML(>Равномерная загрузка</option>
<option value="pinned")HTML";

            page
                << (
                    context.storage_files_policy ==
                        "pinned"
                    ? " selected"
                    : ""
                )
                << R"HTML(>Закрепление за диском</option>
</select>
</div>

<div>
<label>Резерв видео, %</label>
<input
    type="number"
    min="0"
    max="95"
    name="storage.video_reserve_percent"
    value=")HTML";

            page
                << htmlEscape(
                    context.storage_video_reserve_percent
                )
                << R"HTML(">
</div>

<div>
<label>Резерв видео, GB</label>
<input
    type="number"
    min="0"
    name="storage.video_reserve_gb"
    value=")HTML";

            page
                << htmlEscape(
                    context.storage_video_reserve_gb
                )
                << R"HTML(">
</div>

<div>
<label>Резерв файлов, %</label>
<input
    type="number"
    min="0"
    max="95"
    name="storage.files_reserve_percent"
    value=")HTML";

            page
                << htmlEscape(
                    context.storage_files_reserve_percent
                )
                << R"HTML(">
</div>

<div>
<label>Резерв файлов, GB</label>
<input
    type="number"
    min="0"
    name="storage.files_reserve_gb"
    value=")HTML";

            page
                << htmlEscape(
                    context.storage_files_reserve_gb
                )
                << R"HTML(">
</div>

<div>
<label>Закреплённый диск для видео</label>
<select
    id="storage-video-pinned"
    name="storage.video_pinned_mount"
    data-selected=")HTML";

            page
                << htmlEscape(
                    context.storage_video_pinned_mount
                )
                << R"HTML(">
<option value="">Автоматически</option>
</select>
</div>

<div>
<label>Закреплённый диск для файлов</label>
<select
    id="storage-files-pinned"
    name="storage.files_pinned_mount"
    data-selected=")HTML";

            page
                << htmlEscape(
                    context.storage_files_pinned_mount
                )
                << R"HTML(">
<option value="">Автоматически</option>
</select>
</div>
</div>

<p class="muted">
Один физический диск можно одновременно добавить в пул видео и в пул домашних файлов.
Для новых дисков Home AI Core использует стабильный путь на основе UUID файловой системы.
</p>

<div class="button-row">
<button type="submit">Сохранить настройки пулов</button>
</div>
</form>
</div>
)HTML";
        }

        page << R"HTML(
<div id="disk-menu-overlay" class="disk-menu-overlay">
<div class="disk-menu-panel">
<h3>Управление диском</h3>

<div id="disk-menu-info" class="disk-menu-info">
Устройство не выбрано.
</div>

<div id="disk-menu-warning" class="disk-menu-warning"></div>
<div id="disk-menu-result" class="disk-menu-result"></div>

<div class="disk-menu-group">
<h4>Как использовать диск</h4>

<div id="disk-assignment-help" class="device-note">
Выберите одно или несколько назначений.
</div>

<div class="form-grid" style="margin-top:12px">
<label>
<input id="disk-role-video" type="checkbox" style="width:auto;margin-right:8px">
Использовать для видео
</label>
<label>
<input id="disk-role-personal" type="checkbox" style="width:auto;margin-right:8px">
Использовать для домашних файлов
</label>
</div>

<div class="disk-menu-actions">
<button id="disk-apply-roles" type="button">
Применить назначение
</button>
</div>
</div>

<div class="disk-menu-group">
<h4>Стандартные операции</h4>
<div class="disk-menu-actions">
<button id="disk-refresh" type="button" class="secondary">
Обновить информацию
</button>
<button id="disk-unmount" type="button" class="secondary">
Размонтировать диск
</button>
<button id="disk-format-ext4" type="button" class="danger">
Форматировать EXT4
</button>
<button id="disk-wipefs" type="button" class="danger">
Удалить сигнатуры
</button>
</div>
</div>

<p class="muted">
Форматирование и удаление сигнатур уничтожают существующие данные.
Для этих операций требуется ввести точное имя устройства.
</p>

<div class="disk-menu-actions">
<button id="disk-menu-close" type="button" class="secondary">
Закрыть
</button>
</div>
</div>
</div>
)HTML";
    }
    else if (
        context.page == "/files"
    ) {
        page << R"HTML(
<div class="section-card">
<div class="section-title">
<h2>Домашнее облако</h2>
<span class="section-hint">Личное файловое хранилище</span>
</div>

<div class="kv">
<div>Корневая папка</div><div>)HTML";

        page
            << htmlEscape(
                context.files_root
            )
            << R"HTML(</div>
<div>Диски личных файлов</div><div>)HTML";

        page
            << (
                context.storage_personal_mounts.empty()
                ? "Не назначены"
                : htmlEscape(
                    context.storage_personal_mounts
                )
            )
            << R"HTML(</div>
</div>

<p class="muted" style="margin-top:14px">
Файлы домашнего облака будут храниться только в выбранном каталоге.
Физические диски для личных данных назначаются в разделе «Хранилище».
</p>
</div>
)HTML";

        if (
            uiHasPermission(
                context,
                "files.manage"
            )
        ) {
            page << R"HTML(
<div class="section-card">
<h2>Настройки хранения файлов</h2>

<form method="POST" action="/api/config">
<input type="hidden" name="return_to" value="/files">

<div class="form-grid">
<div>
<label>Корневая папка домашнего облака</label>
<input
    name="files.root"
    placeholder="/mnt/home-ai/files"
    value=")HTML";

            page
                << htmlEscape(
                    context.files_root
                )
                << R"HTML(">
</div>
</div>

<div class="button-row">
<button type="submit">Сохранить</button>
</div>
</form>

<p class="muted">
Следующим этапом здесь появятся браузер файлов, папки пользователей,
корзина, квоты, версии файлов и общий доступ внутри домашней сети.
</p>
</div>
)HTML";
        }

        renderPlaceholder(
            page,
            "Файловый сервис",
            "Раздел подготовлен как центр будущего домашнего облака.",
            "<div class=\"placeholder-card\">Файловый браузер — NEXT</div>"
            "<div class=\"placeholder-card\">Папки пользователей — NEXT</div>"
            "<div class=\"placeholder-card\">Квоты — NEXT</div>"
            "<div class=\"placeholder-card\">Корзина и версии — NEXT</div>"
        );
    }
    else if (
        context.page == "/cameras"
    ) {
        const bool can_manage_cameras =
            uiHasPermission(
                context,
                "cameras.manage"
            );

        page << R"HTML(
<div id="camera-root" data-can-manage=")HTML";

        page
            << (
                can_manage_cameras
                ? "1"
                : "0"
            )
            << R"HTML(">

<div class="section-card">
<div class="section-title">
<h2>Камеры</h2>
<span class="section-hint">Camera Core 0.0.20</span>
</div>

<div class="stats-grid">
<div class="stat-card">
<span class="stat-label">Всего</span>
<strong id="camera-total">0</strong>
</div>
<div class="stat-card">
<span class="stat-label">В сети</span>
<strong id="camera-online" class="status-ok">0</strong>
</div>
<div class="stat-card">
<span class="stat-label">Не в сети</span>
<strong id="camera-offline" class="status-error">0</strong>
</div>
<div class="stat-card">
<span class="stat-label">Отключено</span>
<strong id="camera-disabled">0</strong>
</div>
</div>
</div>
)HTML";

        if (can_manage_cameras) {
            page << R"HTML(
<div class="section-card">
<div class="section-title">
<h2>Поиск камер</h2>
<button
    id="camera-discover-btn"
    type="button"
    class="secondary">
Найти камеры
</button>
</div>

<p class="muted">
Поиск выполняется по локальной сети.
Камеры находятся даже с выключенным ONVIF.
</p>

<div
    id="camera-discovery-message"
    class="muted"
    role="status"></div>

<div
    id="camera-discovery-list"
    class="camera-discovery-list"
    style="margin-top:14px"></div>
</div>

<div class="section-card">
<div class="section-title">
<h2 id="camera-form-title">Добавить камеру</h2>
<span class="section-hint">RTSP / ONVIF</span>
</div>

<input id="camera-id" type="hidden">
<input id="camera-manufacturer" type="hidden">
<input id="camera-model" type="hidden">
<input id="camera-firmware-version" type="hidden">
<input id="camera-serial-number" type="hidden">
<input id="camera-hardware-id" type="hidden">
<input id="camera-ptz-xaddr" type="hidden">
<input id="camera-ptz-profile-token" type="hidden">

<div class="form-grid">
<div>
<label for="camera-name">Название</label>
<input
    id="camera-name"
    maxlength="128"
    autocomplete="off"
    placeholder="Вход">
</div>

<div>
<label for="camera-rtsp-url">RTSP URL (определяется автоматически)</label>
<input
    id="camera-rtsp-url"
    maxlength="2048"
    autocomplete="off"
    inputmode="url"
    placeholder="Будет заполнен через ONVIF">
</div>

<input
    id="camera-onvif-xaddr"
    type="hidden">

<div>
<label for="camera-username">Логин</label>
<input
    id="camera-username"
    maxlength="128"
    autocomplete="off"
    placeholder="admin">
</div>

<div>
<label for="camera-password">Пароль</label>
<input
    id="camera-password"
    type="password"
    maxlength="512"
    autocomplete="new-password"
    placeholder="Оставьте пустым, чтобы не менять">
</div>
</div>

<label style="margin-top:14px">
<input
    id="camera-clear-password"
    type="checkbox"
    style="width:auto;margin-right:8px">
Удалить сохранённый пароль
</label>

<label style="margin-top:14px">
<input
    id="camera-enabled"
    type="checkbox"
    checked
    style="width:auto;margin-right:8px">
Камера включена
</label>

<p class="muted">
Для найденных камер Home AI Core пытается определить RTSP автоматически.
ONVIF используется, если он включён; Hikvision и совместимые камеры могут
обнаруживаться и при выключенном ONVIF.
</p>

<div class="button-row">
<button
    id="camera-auto-stream-btn"
    type="button"
    class="secondary">
Определить поток автоматически
</button>
</div>

<div
    id="camera-profile-message"
    class="muted"
    role="status"
    style="margin-top:12px"></div>

<div
    id="camera-profile-list"
    class="placeholder-grid"
    style="margin-top:12px"></div>

<div
    id="camera-device-info"
    class="placeholder-card"
    style="display:none;margin-top:12px">
<strong>Информация о камере</strong>
<div class="kv" style="margin-top:10px">
<div>Производитель</div>
<div id="camera-info-manufacturer">—</div>
<div>Модель</div>
<div id="camera-info-model">—</div>
<div>Версия прошивки</div>
<div id="camera-info-firmware">—</div>
<div>Серийный номер</div>
<div id="camera-info-serial">—</div>
<div>Hardware ID</div>
<div id="camera-info-hardware">—</div>
</div>
</div>

<div class="button-row">
<button id="camera-save-btn" type="button">
Сохранить камеру
</button>
<button id="camera-cancel-btn" type="button" class="secondary">
Очистить форму
</button>
</div>

<div
    id="camera-form-message"
    class="muted"
    role="status"
    style="margin-top:12px"></div>
</div>
)HTML";
        }

        page << R"HTML(
<div class="section-card">
<div class="section-title">
<h2>Список камер</h2>
<button
    id="camera-refresh-btn"
    type="button"
    class="secondary">
Обновить
</button>
</div>

<p class="muted">
«Проверить» — быстрая проверка RTSP-порта.
«Поток» запускает ffprobe и показывает реальный видеокодек,
разрешение и FPS. Snapshot создаётся через ffmpeg.
</p>

<div
    id="camera-list"
    class="placeholder-grid">
<div class="placeholder-card">
Загрузка камер...
</div>
</div>
</div>

<div class="section-card">
<div class="section-title">
<h2>Следующие этапы</h2>
<span class="section-hint">Video / NVR</span>
</div>
<div class="placeholder-grid">
<div class="placeholder-card">Запись видео — NEXT</div>
<div class="placeholder-card">Архив / таймлайн — NEXT</div>
<div class="placeholder-card">Непрерывный видеопоток — NEXT</div>
<div class="placeholder-card">Аналитика — NEXT</div>
</div>
</div>

</div>
)HTML";
    }
    else if (
        context.page == "/smart-home"
    ) {
        renderPlaceholder(
            page,
            "Умный дом",
            "Устройства, комнаты, состояния и сценарии умного дома будут находиться здесь.",
            "<div class=\"placeholder-card\">Устройства — PLANNED</div>"
            "<div class=\"placeholder-card\">Комнаты — PLANNED</div>"
            "<div class=\"placeholder-card\">MQTT / Zigbee — PLANNED</div>"
            "<div class=\"placeholder-card\">Состояния — PLANNED</div>"
        );
    }
    else if (
        context.page == "/ai"
    ) {
        renderPlaceholder(
            page,
            "AI",
            "Управление локальным AI, памятью, задачами и навыками.",
            "<div class=\"placeholder-card\">AI Brain — PLANNED</div>"
            "<div class=\"placeholder-card\">Memory Core — PLANNED</div>"
            "<div class=\"placeholder-card\">Planner — PLANNED</div>"
            "<div class=\"placeholder-card\">Skills — PLANNED</div>"
        );
    }
    else if (
        context.page == "/users"
    ) {
        const bool can_manage_users =
            uiHasPermission(
                context,
                "users.manage"
            );

        page << R"HTML(
<div id="users-root" class="section-card" data-can-manage=")HTML";

        page
            << (
                can_manage_users
                ? "1"
                : "0"
            )
            << R"HTML(">
<h2>Текущий пользователь</h2>
<div class="kv">
<div>Имя</div><div>)HTML";

        page
            << "<span data-i18n-skip>"
            << htmlEscape(
                context.username
            )
            << "</span>"
            << "</div><div>Роль</div><div>"
            << htmlEscape(
                roleDisplay(
                    context.role
                )
            )
            << R"HTML(</div>
<div>Сессия</div><div class="status-ok">ACTIVE</div>
</div>
</div>
)HTML";

        if (can_manage_users) {
            page << R"HTML(
<div class="section-card">
<h2>Создать пользователя</h2>

<div class="form-grid">
<div>
<label>Имя пользователя</label>
<input id="new-user-name" maxlength="32" autocomplete="off">
</div>

<div>
<label>Пароль</label>
<input id="new-user-password" type="password" minlength="12" maxlength="256" autocomplete="new-password">
</div>

<div>
<label>Роль</label>
<select id="new-user-role">
<option value="viewer">Наблюдатель</option>
<option value="operator">Оператор</option>
<option value="admin">Администратор</option>
</select>
</div>
</div>

<div class="button-row">
<button id="user-create-btn" type="button">Создать пользователя</button>
</div>

<div id="user-action-message" class="muted" role="status" style="margin-top:12px"></div>
</div>
)HTML";
        }

        page << R"HTML(
<div class="section-card">
<div class="section-title">
<h2>Пользователи</h2>
<button id="users-refresh-btn" type="button" class="secondary">Обновить</button>
</div>

<div id="users-list" class="users-grid">
<div class="user-card">Загрузка пользователей...</div>
</div>
</div>
)HTML";

        if (can_manage_users) {
            page << R"HTML(
<div class="section-card">
<div class="section-title">
<h2>Активные сессии</h2>
<button id="sessions-refresh-btn" type="button" class="secondary">Обновить</button>
</div>

<div id="sessions-list" class="users-grid">
<div class="user-card">Загрузка активных сессий...</div>
</div>
</div>

<div class="section-card">
<div class="section-title">
<h2>Журнал безопасности</h2>
<span class="section-hint">Последние события Security Core</span>
</div>

<div class="form-grid">
<div>
<label>Пользователь</label>
<input id="audit-user-filter" placeholder="admin">
</div>

<div>
<label>Событие</label>
<input id="audit-event-filter" placeholder="login.success">
</div>
</div>

<div class="button-row">
<button id="audit-filter-btn" type="button">Применить фильтр</button>
<button id="audit-prev-btn" type="button" class="secondary">Назад</button>
<button id="audit-next-btn" type="button" class="secondary">Далее</button>
</div>

<div id="audit-list" class="audit-list" style="margin-top:16px">
<div class="user-card">Загрузка журнала безопасности...</div>
</div>
</div>
)HTML";
        }
    }
    else if (
        context.page == "/automation"
    ) {
        renderPlaceholder(
            page,
            "Автоматизация",
            "Правила, триггеры и сценарии системы будут собраны здесь.",
            "<div class=\"placeholder-card\">Правила — PLANNED</div>"
            "<div class=\"placeholder-card\">Триггеры — PLANNED</div>"
            "<div class=\"placeholder-card\">Сценарии — PLANNED</div>"
            "<div class=\"placeholder-card\">История выполнения — PLANNED</div>"
        );
    }
    else if (
        context.page == "/hypervisor"
    ) {
        renderPlaceholder(
            page,
            "Виртуализация",
            "Управление KVM, виртуальными машинами, сетями и снапшотами.",
            "<div class=\"placeholder-card\">Виртуальные машины — PLANNED</div>"
            "<div class=\"placeholder-card\">Диски VM — PLANNED</div>"
            "<div class=\"placeholder-card\">Сети VM — PLANNED</div>"
            "<div class=\"placeholder-card\">Snapshots — PLANNED</div>"
        );
    }
    else if (
        context.page == "/settings"
    ) {
        if (
            uiHasPermission(
                context,
                "system.manage"
            )
        ) {
            page << R"HTML(
<div class="section-card">
<h2>Основные настройки</h2>
<form method="POST" action="/api/config">
<input type="hidden" name="return_to" value="/settings">

<div class="form-grid">
<div>
<label>Название ядра</label>
<input name="core.name" value=")HTML";

            page
                << "<span data-i18n-skip>" << htmlEscape(context.core_name) << "</span>"
                << R"HTML(">
</div>

<div>
<label>Уровень журналирования</label>
<select name="log.level">
<option value="debug")HTML";

            if (
                context.log_level ==
                "debug"
            ) {
                page << " selected";
            }

            page << ">Debug</option>"
                 << "<option value=\"info\"";

            if (
                context.log_level ==
                "info"
            ) {
                page << " selected";
            }

            page << ">Info</option>"
                 << "<option value=\"warning\"";

            if (
                context.log_level ==
                "warning"
            ) {
                page << " selected";
            }

            page << ">Warning</option>"
                 << "<option value=\"error\"";

            if (
                context.log_level ==
                "error"
            ) {
                page << " selected";
            }

            page << R"HTML(>Error</option>
</select>
</div>

<div>
<label>Runtime Tick, ms</label>
<input
    type="number"
    min="10"
    max="10000"
    name="runtime.tick_ms"
    value=")HTML";

            page
                << htmlEscape(
                    context.tick_ms
                )
                << R"HTML(">
</div>
</div>

<div class="button-row">
<button type="submit">Сохранить</button>
</div>
</form>
</div>
)HTML";
        }
        else {
            page << R"HTML(
<div class="section-card">
<h2>Настройки</h2>
<p class="muted">Изменение настроек доступно только администратору.</p>
</div>
)HTML";
        }
    }

    page << R"HTML(
</main>
</div>
</div>

<script>

function setMobileMenuOpen(open) {
    const button =
        document.getElementById(
            "mobile-menu-button"
        );

    const backdrop =
        document.getElementById(
            "mobile-menu-backdrop"
        );

    const sidebar =
        document.getElementById(
            "sidebar-navigation"
        );

    const mobile =
        window.matchMedia(
            "(max-width: 860px)"
        ).matches;

    const english =
        document.documentElement.lang ===
        "en";

    document.body.classList.toggle(
        "mobile-menu-open",
        open
    );

    if (button) {
        button.setAttribute(
            "aria-expanded",
            open
                ? "true"
                : "false"
        );

        button.setAttribute(
            "aria-label",
            open
                ? (
                    english
                    ? "Close menu"
                    : "Закрыть меню"
                )
                : (
                    english
                    ? "Open menu"
                    : "Открыть меню"
                )
        );
    }

    if (sidebar) {
        sidebar.inert =
            mobile
            &&
            !open;

        sidebar.setAttribute(
            "aria-hidden",
            (
                mobile
                &&
                !open
            )
                ? "true"
                : "false"
        );
    }

    if (backdrop) {
        backdrop.setAttribute(
            "aria-hidden",
            open
                ? "false"
                : "true"
        );
    }
}

window.addEventListener(
    "pagehide",
    function() {
        for (
            const id of
            Array.from(
                cameraLiveActive
            )
        ) {
            stopCameraLive(id);
        }
    }
);

document.addEventListener(
    "DOMContentLoaded",
    function() {
        const button =
            document.getElementById(
                "mobile-menu-button"
            );

        const backdrop =
            document.getElementById(
                "mobile-menu-backdrop"
            );

        const sidebar =
            document.getElementById(
                "sidebar-navigation"
            );

        setMobileMenuOpen(
            false
        );

        if (button) {
            button.addEventListener(
                "click",
                function() {
                    setMobileMenuOpen(
                        !document.body.classList.contains(
                            "mobile-menu-open"
                        )
                    );
                }
            );
        }

        if (backdrop) {
            backdrop.addEventListener(
                "click",
                function() {
                    setMobileMenuOpen(
                        false
                    );
                }
            );
        }

        if (sidebar) {
            sidebar.addEventListener(
                "click",
                function(event) {
                    const link =
                        event.target.closest(
                            "a.nav-link"
                        );

                    if (link) {
                        setMobileMenuOpen(
                            false
                        );
                    }
                }
            );
        }

        document.addEventListener(
            "keydown",
            function(event) {
                if (
                    event.key === "Escape"
                    &&
                    document.body.classList.contains(
                        "mobile-menu-open"
                    )
                ) {
                    setMobileMenuOpen(
                        false
                    );

                    if (button) {
                        button.focus();
                    }
                }
            }
        );

        const media =
            window.matchMedia(
                "(min-width: 861px)"
            );

        const closeOnDesktop =
            function(event) {
                if (event.matches) {
                    setMobileMenuOpen(
                        false
                    );
                }
            };

        if (media.addEventListener) {
            media.addEventListener(
                "change",
                closeOnDesktop
            );
        }
        else if (media.addListener) {
            media.addListener(
                closeOnDesktop
            );
        }
    }
);

let usersPermissionCatalog = [];
let auditOffset = 0;
const auditLimit = 50;

function userCanManage() {
    const root =
        document.getElementById(
            "users-root"
        );

    return (
        root
        &&
        root.dataset.canManage === "1"
    );
}

function userRoleLabel(role) {
    if (role === "admin")
        return tr("Администратор");

    if (role === "operator")
        return tr("Оператор");

    return tr("Наблюдатель");
}

function permissionLabel(permission) {
    const labels = {
        "files.read": "Файлы: чтение",
        "files.write": "Файлы: запись",
        "files.manage": "Файлы: управление",
        "cameras.view": "Камеры: просмотр",
        "cameras.manage": "Камеры: управление",
        "smart_home.view": "Умный дом: просмотр",
        "smart_home.control": "Умный дом: управление устройствами",
        "smart_home.manage": "Умный дом: настройка",
        "ai.use": "AI: использование",
        "ai.manage": "AI: управление",
        "users.view": "Пользователи: просмотр",
        "users.manage": "Пользователи: управление",
        "system.view": "Система: просмотр",
        "system.manage": "Система: управление",
        "storage.view": "Диски: просмотр",
        "storage.manage": "Диски: управление",
        "network.view": "Сеть: просмотр",
        "network.manage": "Сеть: управление",
        "hypervisor.view": "Виртуализация: просмотр",
        "hypervisor.manage": "Виртуализация: управление",
        "automation.view": "Автоматизация: просмотр",
        "automation.manage": "Автоматизация: управление"
    };

    return tr(
        labels[permission]
        || permission
    );
}

function formatUserTime(value) {
    const timestamp =
        Number(value);

    if (
        !Number.isFinite(timestamp)
        ||
        timestamp <= 0
    ) {
        return tr("Никогда");
    }

    return new Date(
        timestamp * 1000
    ).toLocaleString();
}

function setUserActionMessage(
    message,
    isError = false
) {
    const output =
        document.getElementById(
            "user-action-message"
        );

    if (!output)
        return;

    output.textContent =
        tr(
            message || ""
        );

    output.className =
        isError
        ? "status-error"
        : "muted";
}

async function userApiPost(
    url,
    values
) {
    const response =
        await fetch(
            url,
            {
                method: "POST",
                headers: {
                    "Content-Type":
                        "application/x-www-form-urlencoded",
                    "X-HomeAI-Request":
                        "1"
                },
                body:
                    new URLSearchParams(
                        values
                    ).toString()
            }
        );

    if (response.status === 401) {
        window.location =
            "/login";

        return null;
    }

    const data =
        await response.json();

    if (!response.ok) {
        throw new Error(
            data.message
            || data.error
            || "Операция не выполнена."
        );
    }

    return data;
}

function appendUserMeta(
    card,
    label,
    value,
    skipTranslation = false
) {
    const item =
        document.createElement(
            "div"
        );

    const strong =
        document.createElement(
            "strong"
        );

    strong.textContent =
        tr(label) + ": ";

    item.appendChild(strong);

    const span =
        document.createElement(
            "span"
        );

    span.textContent =
        value;

    if (skipTranslation)
        span.dataset.i18nSkip = "";

    item.appendChild(span);

    card.appendChild(item);
}

function renderPermissionEditor(
    user,
    container
) {
    const details =
        document.createElement(
            "details"
        );

    details.style.marginTop =
        "14px";

    const summary =
        document.createElement(
            "summary"
        );

    summary.textContent =
        tr(
            "Индивидуальные права"
        );

    details.appendChild(
        summary
    );

    const grid =
        document.createElement(
            "div"
        );

    grid.className =
        "permission-grid";

    for (
        const permission of
        usersPermissionCatalog
    ) {
        const row =
            document.createElement(
                "div"
            );

        row.className =
            "permission-row";

        const label =
            document.createElement(
                "span"
            );

        label.textContent =
            permissionLabel(
                permission
            );

        row.appendChild(label);

        const select =
            document.createElement(
                "select"
            );

        for (
            const [
                value,
                text
            ] of [
                ["0", "Наследовать"],
                ["1", "Разрешить"],
                ["-1", "Запретить"]
            ]
        ) {
            const option =
                document.createElement(
                    "option"
                );

            option.value =
                value;

            option.textContent =
                tr(text);

            select.appendChild(
                option
            );
        }

        const current =
            Object.prototype.
                hasOwnProperty.call(
                    user.permission_overrides
                        || {},
                    permission
                )
            ? String(
                user.permission_overrides[
                    permission
                ]
              )
            : "0";

        select.value =
            current;

        select.addEventListener(
            "change",
            async function() {
                select.disabled =
                    true;

                try {
                    const result =
                        await userApiPost(
                            "/api/users/permission",
                            {
                                user_id:
                                    String(
                                        user.id
                                    ),
                                permission:
                                    permission,
                                decision:
                                    select.value
                            }
                        );

                    if (result) {
                        setUserActionMessage(
                            result.message
                            || "Permission updated"
                        );

                        await updateUsers();
                    }
                }
                catch (error) {
                    setUserActionMessage(
                        error.message,
                        true
                    );
                }
                finally {
                    select.disabled =
                        false;
                }
            }
        );

        row.appendChild(
            select
        );

        grid.appendChild(
            row
        );
    }

    details.appendChild(
        grid
    );

    container.appendChild(
        details
    );
}

function renderUserCard(
    user
) {
    const card =
        document.createElement(
            "div"
        );

    card.className =
        "user-card";

    const header =
        document.createElement(
            "div"
        );

    header.className =
        "user-card-header";

    const title =
        document.createElement(
            "h3"
        );

    title.textContent =
        user.username;

    title.dataset.i18nSkip =
        "";

    header.appendChild(
        title
    );

    const state =
        document.createElement(
            "strong"
        );

    state.className =
        user.enabled
        ? "status-ok"
        : "status-error";

    state.textContent =
        tr(
            user.enabled
            ? "ACTIVE"
            : "DISABLED"
        );

    header.appendChild(
        state
    );

    card.appendChild(
        header
    );

    const meta =
        document.createElement(
            "div"
        );

    meta.className =
        "user-meta";

    appendUserMeta(
        meta,
        "Роль",
        userRoleLabel(
            user.role
        )
    );

    appendUserMeta(
        meta,
        "Создан",
        formatUserTime(
            user.created_at
        ),
        true
    );

    appendUserMeta(
        meta,
        "Последний вход",
        formatUserTime(
            user.last_login_at
        ),
        true
    );

    card.appendChild(
        meta
    );

    const permissionsTitle =
        document.createElement(
            "div"
        );

    permissionsTitle.className =
        "muted";

    permissionsTitle.style.marginTop =
        "12px";

    permissionsTitle.textContent =
        tr(
            "Эффективные права"
        );

    card.appendChild(
        permissionsTitle
    );

    const chips =
        document.createElement(
            "div"
        );

    chips.className =
        "permission-chip-list";

    for (
        const permission of
        (
            Array.isArray(
                user.effective_permissions
            )
            ? user.effective_permissions
            : []
        )
    ) {
        const chip =
            document.createElement(
                "span"
            );

        chip.className =
            "permission-chip";

        chip.textContent =
            permissionLabel(
                permission
            );

        chips.appendChild(
            chip
        );
    }

    if (!chips.childNodes.length) {
        const chip =
            document.createElement(
                "span"
            );

        chip.className =
            "permission-chip";

        chip.textContent =
            tr("Нет разрешений");

        chips.appendChild(
            chip
        );
    }

    card.appendChild(
        chips
    );

    if (userCanManage()) {
        const controls =
            document.createElement(
                "div"
            );

        controls.className =
            "form-grid";

        controls.style.marginTop =
            "14px";

        const roleBox =
            document.createElement(
                "div"
            );

        const roleLabel =
            document.createElement(
                "label"
            );

        roleLabel.textContent =
            tr("Роль");

        const roleSelect =
            document.createElement(
                "select"
            );

        for (
            const [
                value,
                label
            ] of [
                ["viewer", "Наблюдатель"],
                ["operator", "Оператор"],
                ["admin", "Администратор"]
            ]
        ) {
            const option =
                document.createElement(
                    "option"
                );

            option.value =
                value;

            option.textContent =
                tr(label);

            option.selected =
                value === user.role;

            roleSelect.appendChild(
                option
            );
        }

        roleBox.append(
            roleLabel,
            roleSelect
        );

        controls.appendChild(
            roleBox
        );

        const enabledBox =
            document.createElement(
                "div"
            );

        const enabledLabel =
            document.createElement(
                "label"
            );

        enabledLabel.textContent =
            tr("Учётная запись");

        const enabledSelect =
            document.createElement(
                "select"
            );

        for (
            const [
                value,
                label
            ] of [
                ["1", "Активна"],
                ["0", "Отключена"]
            ]
        ) {
            const option =
                document.createElement(
                    "option"
                );

            option.value =
                value;

            option.textContent =
                tr(label);

            option.selected =
                (
                    value === "1"
                ) === Boolean(
                    user.enabled
                );

            enabledSelect.appendChild(
                option
            );
        }

        enabledBox.append(
            enabledLabel,
            enabledSelect
        );

        controls.appendChild(
            enabledBox
        );

        card.appendChild(
            controls
        );

        const actions =
            document.createElement(
                "div"
            );

        actions.className =
            "button-row";

        const save =
            document.createElement(
                "button"
            );

        save.type = "button";
        save.textContent =
            tr("Сохранить");

        save.addEventListener(
            "click",
            async function() {
                save.disabled = true;

                try {
                    const result =
                        await userApiPost(
                            "/api/users/update",
                            {
                                user_id:
                                    String(
                                        user.id
                                    ),
                                role:
                                    roleSelect.value,
                                enabled:
                                    enabledSelect.value
                            }
                        );

                    if (result) {
                        setUserActionMessage(
                            result.message
                            || "User updated"
                        );

                        await updateUsers();
                        await updateUserSessions();
                    }
                }
                catch (error) {
                    setUserActionMessage(
                        error.message,
                        true
                    );
                }
                finally {
                    save.disabled = false;
                }
            }
        );

        actions.appendChild(
            save
        );

        const password =
            document.createElement(
                "button"
            );

        password.type =
            "button";

        password.className =
            "secondary";

        password.textContent =
            tr("Сменить пароль");

        password.addEventListener(
            "click",
            async function() {
                const next =
                    window.prompt(
                        "Введите новый пароль (минимум 12 символов)"
                    );

                if (next === null)
                    return;

                try {
                    const result =
                        await userApiPost(
                            "/api/users/password",
                            {
                                user_id:
                                    String(
                                        user.id
                                    ),
                                password:
                                    next
                            }
                        );

                    if (result) {
                        setUserActionMessage(
                            result.message
                            || "Password changed"
                        );

                        await updateUserSessions();
                    }
                }
                catch (error) {
                    setUserActionMessage(
                        error.message,
                        true
                    );
                }
            }
        );

        actions.appendChild(
            password
        );

        const revoke =
            document.createElement(
                "button"
            );

        revoke.type =
            "button";

        revoke.className =
            "secondary";

        revoke.textContent =
            tr(
                "Завершить все сессии"
            );

        revoke.addEventListener(
            "click",
            async function() {
                if (
                    !window.confirm(
                        "Завершить все активные сессии этого пользователя?"
                    )
                ) {
                    return;
                }

                try {
                    const result =
                        await userApiPost(
                            "/api/users/sessions/revoke",
                            {
                                user_id:
                                    String(
                                        user.id
                                    )
                            }
                        );

                    if (result) {
                        setUserActionMessage(
                            result.message
                            || "User sessions revoked"
                        );

                        await updateUserSessions();
                    }
                }
                catch (error) {
                    setUserActionMessage(
                        error.message,
                        true
                    );
                }
            }
        );

        actions.appendChild(
            revoke
        );

        const remove =
            document.createElement(
                "button"
            );

        remove.type =
            "button";

        remove.className =
            "danger";

        remove.textContent =
            tr(
                "Удалить пользователя"
            );

        remove.addEventListener(
            "click",
            async function() {
                if (
                    !window.confirm(
                        "Удалить пользователя и все его активные сессии?"
                    )
                ) {
                    return;
                }

                try {
                    const result =
                        await userApiPost(
                            "/api/users/delete",
                            {
                                user_id:
                                    String(
                                        user.id
                                    )
                            }
                        );

                    if (result) {
                        setUserActionMessage(
                            result.message
                            || "User deleted"
                        );

                        await updateUsers();
                        await updateUserSessions();
                        await updateAuditLog();
                    }
                }
                catch (error) {
                    setUserActionMessage(
                        error.message,
                        true
                    );
                }
            }
        );

        actions.appendChild(
            remove
        );

        card.appendChild(
            actions
        );

        renderPermissionEditor(
            user,
            card
        );
    }

    return card;
}

async function updateUsers() {
    const container =
        document.getElementById(
            "users-list"
        );

    if (!container)
        return;

    try {
        const response =
            await fetch(
                "/api/users",
                {
                    cache: "no-store"
                }
            );

        if (response.status === 401) {
            window.location =
                "/login";

            return;
        }

        if (!response.ok) {
            throw new Error(
                "Не удалось получить список пользователей."
            );
        }

        const data =
            await response.json();

        usersPermissionCatalog =
            Array.isArray(
                data.permission_catalog
            )
            ? data.permission_catalog
            : [];

        container.replaceChildren();

        const users =
            Array.isArray(
                data.users
            )
            ? data.users
            : [];

        for (const user of users) {
            container.appendChild(
                renderUserCard(
                    user
                )
            );
        }

        if (users.length === 0) {
            const empty =
                document.createElement(
                    "div"
                );

            empty.className =
                "user-card";

            empty.textContent =
                tr(
                    "Пользователи не найдены."
                );

            container.appendChild(
                empty
            );
        }
    }
    catch (error) {
        container.textContent =
            tr(error.message);
    }
}

async function updateUserSessions() {
    const container =
        document.getElementById(
            "sessions-list"
        );

    if (!container)
        return;

    try {
        const response =
            await fetch(
                "/api/users/sessions",
                {
                    cache: "no-store"
                }
            );

        if (response.status === 401) {
            window.location =
                "/login";

            return;
        }

        if (!response.ok) {
            throw new Error(
                "Не удалось получить активные сессии."
            );
        }

        const data =
            await response.json();

        const sessions =
            Array.isArray(
                data.sessions
            )
            ? data.sessions
            : [];

        container.replaceChildren();

        for (
            const session of
            sessions
        ) {
            const card =
                document.createElement(
                    "div"
                );

            card.className =
                "user-card";

            const header =
                document.createElement(
                    "div"
                );

            header.className =
                "user-card-header";

            const username =
                document.createElement(
                    "strong"
                );

            username.textContent =
                session.username;

            username.dataset.i18nSkip =
                "";

            header.appendChild(
                username
            );

            const id =
                document.createElement(
                    "span"
                );

            id.className =
                "muted";

            id.textContent =
                "Session #"
                + session.id;

            id.dataset.i18nSkip =
                "";

            header.appendChild(id);
            card.appendChild(header);

            const meta =
                document.createElement(
                    "div"
                );

            meta.className =
                "user-meta";

            appendUserMeta(
                meta,
                "Создана",
                formatUserTime(
                    session.created_at
                ),
                true
            );

            appendUserMeta(
                meta,
                "Последняя активность",
                formatUserTime(
                    session.last_seen_at
                ),
                true
            );

            appendUserMeta(
                meta,
                "Истекает",
                formatUserTime(
                    session.expires_at
                ),
                true
            );

            card.appendChild(meta);

            const actions =
                document.createElement(
                    "div"
                );

            actions.className =
                "button-row";

            const revoke =
                document.createElement(
                    "button"
                );

            revoke.type =
                "button";

            revoke.className =
                "danger";

            revoke.textContent =
                tr("Завершить сессию");

            revoke.addEventListener(
                "click",
                async function() {
                    try {
                        const result =
                            await userApiPost(
                                "/api/users/session/revoke",
                                {
                                    user_id:
                                        String(
                                            session.user_id
                                        ),
                                    session_id:
                                        String(
                                            session.id
                                        )
                                }
                            );

                        if (result) {
                            setUserActionMessage(
                                result.message
                                || "Session revoked"
                            );

                            await updateUserSessions();
                        }
                    }
                    catch (error) {
                        setUserActionMessage(
                            error.message,
                            true
                        );
                    }
                }
            );

            actions.appendChild(
                revoke
            );

            card.appendChild(
                actions
            );

            container.appendChild(
                card
            );
        }

        if (sessions.length === 0) {
            const empty =
                document.createElement(
                    "div"
                );

            empty.className =
                "user-card";

            empty.textContent =
                tr(
                    "Активных сессий нет."
                );

            container.appendChild(
                empty
            );
        }
    }
    catch (error) {
        container.textContent =
            tr(error.message);
    }
}

async function updateAuditLog() {
    const container =
        document.getElementById(
            "audit-list"
        );

    if (!container)
        return;

    const username =
        document.getElementById(
            "audit-user-filter"
        )?.value || "";

    const event =
        document.getElementById(
            "audit-event-filter"
        )?.value || "";

    const query =
        new URLSearchParams(
            {
                limit:
                    String(
                        auditLimit
                    ),
                offset:
                    String(
                        auditOffset
                    ),
                username:
                    username,
                event:
                    event
            }
        );

    try {
        const response =
            await fetch(
                "/api/users/audit?"
                + query.toString(),
                {
                    cache: "no-store"
                }
            );

        if (response.status === 401) {
            window.location =
                "/login";

            return;
        }

        if (!response.ok) {
            throw new Error(
                "Не удалось получить журнал безопасности."
            );
        }

        const data =
            await response.json();

        const entries =
            Array.isArray(
                data.entries
            )
            ? data.entries
            : [];

        container.replaceChildren();

        for (const entry of entries) {
            const card =
                document.createElement(
                    "div"
                );

            card.className =
                "audit-entry";

            const header =
                document.createElement(
                    "div"
                );

            const time =
                document.createElement(
                    "strong"
                );

            time.textContent =
                formatUserTime(
                    entry.created_at
                );

            time.dataset.i18nSkip =
                "";

            header.appendChild(time);

            const eventName =
                document.createElement(
                    "span"
                );

            eventName.textContent =
                entry.event;

            eventName.dataset.i18nSkip =
                "";

            header.appendChild(
                eventName
            );

            card.appendChild(
                header
            );

            const userLine =
                document.createElement(
                    "div"
                );

            userLine.className =
                "muted";

            userLine.textContent =
                tr("Пользователь")
                + ": "
                + entry.username;

            userLine.dataset.i18nSkip =
                "";

            card.appendChild(
                userLine
            );

            const details =
                document.createElement(
                    "div"
                );

            details.textContent =
                entry.details;

            details.dataset.i18nSkip =
                "";

            card.appendChild(
                details
            );

            container.appendChild(
                card
            );
        }

        if (entries.length === 0) {
            const empty =
                document.createElement(
                    "div"
                );

            empty.className =
                "user-card";

            empty.textContent =
                tr(
                    "События не найдены."
                );

            container.appendChild(
                empty
            );
        }

        const previous =
            document.getElementById(
                "audit-prev-btn"
            );

        const next =
            document.getElementById(
                "audit-next-btn"
            );

        if (previous)
            previous.disabled =
                auditOffset <= 0;

        if (next)
            next.disabled =
                entries.length <
                auditLimit;
    }
    catch (error) {
        container.textContent =
            tr(error.message);
    }
}

document.addEventListener(
    "DOMContentLoaded",
    function() {
        updateUsers();
        updateUserSessions();
        updateAuditLog();

        const refresh =
            document.getElementById(
                "users-refresh-btn"
            );

        if (refresh) {
            refresh.addEventListener(
                "click",
                updateUsers
            );
        }

        const sessionsRefresh =
            document.getElementById(
                "sessions-refresh-btn"
            );

        if (sessionsRefresh) {
            sessionsRefresh.addEventListener(
                "click",
                updateUserSessions
            );
        }

        const create =
            document.getElementById(
                "user-create-btn"
            );

        if (create) {
            create.addEventListener(
                "click",
                async function() {
                    const username =
                        document.getElementById(
                            "new-user-name"
                        ).value;

                    const password =
                        document.getElementById(
                            "new-user-password"
                        ).value;

                    const role =
                        document.getElementById(
                            "new-user-role"
                        ).value;

                    create.disabled = true;

                    try {
                        const result =
                            await userApiPost(
                                "/api/users/create",
                                {
                                    username:
                                        username,
                                    password:
                                        password,
                                    role:
                                        role
                                }
                            );

                        if (result) {
                            document.getElementById(
                                "new-user-name"
                            ).value = "";

                            document.getElementById(
                                "new-user-password"
                            ).value = "";

                            setUserActionMessage(
                                result.message
                                || "User created"
                            );

                            await updateUsers();
                            await updateAuditLog();
                        }
                    }
                    catch (error) {
                        setUserActionMessage(
                            error.message,
                            true
                        );
                    }
                    finally {
                        create.disabled = false;
                    }
                }
            );
        }

        const auditFilter =
            document.getElementById(
                "audit-filter-btn"
            );

        if (auditFilter) {
            auditFilter.addEventListener(
                "click",
                function() {
                    auditOffset = 0;
                    updateAuditLog();
                }
            );
        }

        const auditPrevious =
            document.getElementById(
                "audit-prev-btn"
            );

        if (auditPrevious) {
            auditPrevious.addEventListener(
                "click",
                function() {
                    auditOffset =
                        Math.max(
                            0,
                            auditOffset
                            - auditLimit
                        );

                    updateAuditLog();
                }
            );
        }

        const auditNext =
            document.getElementById(
                "audit-next-btn"
            );

        if (auditNext) {
            auditNext.addEventListener(
                "click",
                function() {
                    auditOffset +=
                        auditLimit;

                    updateAuditLog();
                }
            );
        }
    }
);

let knownGpus = null;
async function selectGpu(address) {
    const output = document.getElementById('gpu-action');
    try {
        const response = await fetch('/api/admin/accelerator', {
            method: 'POST', headers: {'X-HomeAI-Request':'1', 'Content-Type':'application/x-www-form-urlencoded'},
            body: new URLSearchParams({pci_address:address})
        });
        const data = await response.json();
        if (!response.ok) throw new Error(data.error || 'request_failed');
        output.textContent = tr('Назначение GPU сохранено.');
        await updateGpus();
    } catch (error) { output.textContent = tr(error.message); }
}
async function updateGpus() {
    const notice = document.getElementById('gpu-notice');
    if (!notice) return;
    try {
        const response = await fetch('/api/admin/gpus', {cache:'no-store'});
        if (response.status === 401) { window.location = '/login'; return; }
        if (!response.ok) throw new Error('request_failed');
        const data = await response.json();
        const addresses = data.devices.map(gpu => gpu.pci_address);
        const added = addresses.filter(address => knownGpus === null || !knownGpus.has(address));
        knownGpus = new Set(addresses);
        notice.hidden = false;
        if (!data.available) notice.textContent = tr('Информация PCI недоступна.');
        else if (data.selected && !data.selected_present) notice.textContent = tr('Назначенная GPU отсутствует: ') + data.selected;
        else if (added.length) {
            notice.replaceChildren(document.createTextNode(tr('Обнаружена GPU, доступна настройка AI: ') + added.join(', ') + ' '));
            const link = document.createElement('a'); link.href = '/admin'; link.textContent = tr('Администрирование'); notice.appendChild(link);
        } else if (!addresses.length) notice.textContent = tr('Видеокарты не обнаружены.');
        else {
            notice.replaceChildren(document.createTextNode(tr('Обнаружены GPU: ') + addresses.join(', ') + ' '));
            const link = document.createElement('a'); link.href = '/admin'; link.textContent = tr('Администрирование'); notice.appendChild(link);
        }
        const list = document.getElementById('gpu-list');
        if (!list) return;
        list.replaceChildren();
        for (const gpu of data.devices) {
            const card = document.createElement('div'); card.className = 'storage-card';
            const info = document.createElement('p'); info.dataset.i18nSkip = '';
            info.textContent = gpu.vendor + ' ' + gpu.vendor_id + ':' + gpu.device_id + ' · ' + gpu.pci_address + ' · ' + tr('Драйвер: ') + (gpu.driver || tr('не загружен'));
            const button = document.createElement('button'); button.type = 'button';
            button.textContent = tr(gpu.pci_address === data.selected ? 'Назначена для AI' : 'Назначить для AI');
            button.disabled = gpu.pci_address === data.selected;
            button.addEventListener('click', () => selectGpu(gpu.pci_address));
            card.append(info, button); list.appendChild(card);
        }
        document.getElementById('gpu-selection').textContent = tr('Назначенная GPU: ') + (data.selected || tr('Не назначена'));
    } catch (error) { notice.hidden = false; notice.textContent = tr('Не удалось получить сведения о GPU.'); }
}
document.addEventListener('DOMContentLoaded', () => {
    updateGpus(); setInterval(updateGpus, 10000);
    const clear = document.getElementById('gpu-clear');
    if (clear) clear.addEventListener('click', () => selectGpu(''));
});

let cameraCache = [];
const cameraMediaCache =
    new Map();
const cameraSnapshotCache =
    new Map();
let onvifDiscoveryCache = [];
let onvifProfileCache = [];

function cameraCanManage() {
    const root =
        document.getElementById(
            "camera-root"
        );

    return (
        root
        &&
        root.dataset.canManage === "1"
    );
}

function cameraStatusLabel(camera) {
    if (!camera.enabled)
        return tr("Отключено");

    if (camera.status === "online")
        return tr("В сети");

    if (camera.status === "offline")
        return tr("Не в сети");

    return tr("Неизвестно");
}

function cameraStatusClass(camera) {
    if (!camera.enabled)
        return "muted";

    if (camera.status === "online")
        return "status-ok";

    if (camera.status === "offline")
        return "status-error";

    return "status-warn";
}

function setCameraDeviceInfo(info) {
    const fields = {
        "camera-manufacturer":
            info?.manufacturer || "",
        "camera-model":
            info?.model || "",
        "camera-firmware-version":
            info?.firmware_version || "",
        "camera-serial-number":
            info?.serial_number || "",
        "camera-hardware-id":
            info?.hardware_id || ""
    };

    for (
        const [id, value] of
        Object.entries(fields)
    ) {
        const element =
            document.getElementById(
                id
            );

        if (element)
            element.value = value;
    }

    const values = {
        "camera-info-manufacturer":
            fields["camera-manufacturer"],
        "camera-info-model":
            fields["camera-model"],
        "camera-info-firmware":
            fields["camera-firmware-version"],
        "camera-info-serial":
            fields["camera-serial-number"],
        "camera-info-hardware":
            fields["camera-hardware-id"]
    };

    let hasInfo = false;

    for (
        const [id, value] of
        Object.entries(values)
    ) {
        const element =
            document.getElementById(
                id
            );

        if (element) {
            element.textContent =
                value || "—";

            if (value)
                hasInfo = true;
        }
    }

    const container =
        document.getElementById(
            "camera-device-info"
        );

    if (container) {
        container.style.display =
            hasInfo
            ? "block"
            : "none";
    }
}

function clearCameraForm() {
    const id =
        document.getElementById(
            "camera-id"
        );

    if (!id)
        return;

    id.value = "";

    document.getElementById(
        "camera-name"
    ).value = "";

    document.getElementById(
        "camera-rtsp-url"
    ).value = "";

    document.getElementById(
        "camera-onvif-xaddr"
    ).value = "";

    document.getElementById(
        "camera-username"
    ).value = "";

    document.getElementById(
        "camera-password"
    ).value = "";

    setCameraDeviceInfo({});

    document.getElementById(
        "camera-ptz-xaddr"
    ).value = "";

    document.getElementById(
        "camera-ptz-profile-token"
    ).value = "";

    document.getElementById(
        "camera-enabled"
    ).checked = true;

    document.getElementById(
        "camera-clear-password"
    ).checked = false;

    onvifProfileCache = [];

    const profileMessage =
        document.getElementById(
            "camera-profile-message"
        );

    if (profileMessage)
        profileMessage.textContent = "";

    const profileList =
        document.getElementById(
            "camera-profile-list"
        );

    if (profileList)
        profileList.replaceChildren();

    const title =
        document.getElementById(
            "camera-form-title"
        );

    if (title)
        title.textContent =
            tr("Добавить камеру");

    const message =
        document.getElementById(
            "camera-form-message"
        );

    if (message)
        message.textContent = "";
}

function editCamera(id) {
    const camera =
        cameraCache.find(
            item =>
                Number(item.id) ===
                Number(id)
        );

    if (
        !camera
        ||
        !cameraCanManage()
    ) {
        return;
    }

    document.getElementById(
        "camera-id"
    ).value = camera.id;

    document.getElementById(
        "camera-name"
    ).value = camera.name || "";

    document.getElementById(
        "camera-rtsp-url"
    ).value = camera.rtsp_url || "";

    document.getElementById(
        "camera-onvif-xaddr"
    ).value =
        camera.onvif_xaddr || "";

    document.getElementById(
        "camera-username"
    ).value = camera.username || "";

    document.getElementById(
        "camera-password"
    ).value = "";

    setCameraDeviceInfo(
        {
            manufacturer:
                camera.manufacturer || "",
            model:
                camera.model || "",
            firmware_version:
                camera.firmware_version || "",
            serial_number:
                camera.serial_number || "",
            hardware_id:
                camera.hardware_id || ""
        }
    );

    document.getElementById(
        "camera-ptz-xaddr"
    ).value =
        camera.ptz_xaddr || "";

    document.getElementById(
        "camera-ptz-profile-token"
    ).value =
        camera.ptz_profile_token || "";

    document.getElementById(
        "camera-enabled"
    ).checked = !!camera.enabled;

    document.getElementById(
        "camera-clear-password"
    ).checked = false;

    const title =
        document.getElementById(
            "camera-form-title"
        );

    if (title) {
        title.textContent =
            tr("Редактировать камеру");
    }

    const message =
        document.getElementById(
            "camera-form-message"
        );

    if (message) {
        message.textContent =
            camera.has_password
            ? tr("Пароль сохранён. Оставьте поле пустым, чтобы не менять его.")
            : tr("Пароль не задан.");
    }

    const form =
        document.getElementById(
            "camera-form-title"
        );

    if (form) {
        form.scrollIntoView(
            {
                behavior: "smooth",
                block: "start"
            }
        );
    }
}

async function cameraPost(
    path,
    parameters = new URLSearchParams()
) {
    const response =
        await fetch(
            path,
            {
                method: "POST",
                headers: {
                    "Content-Type":
                        "application/x-www-form-urlencoded",
                    "X-HomeAI-Request":
                        "1"
                },
                body:
                    parameters.toString()
            }
        );

    if (response.status === 401) {
        window.location = "/login";
        return null;
    }

    const data =
        await response.json();

    if (!response.ok) {
        const error =
            new Error(
                data.message
                || data.error
                || "Camera operation failed"
            );

        error.data = data;
        throw error;
    }

    return data;
}

function useOnvifProfile(index) {
    const profile =
        onvifProfileCache[
            index
        ];

    if (!profile)
        return;

    const rtsp =
        document.getElementById(
            "camera-rtsp-url"
        );

    if (rtsp) {
        rtsp.value =
            profile.rtsp_uri
            || "";
    }

    const message =
        document.getElementById(
            "camera-profile-message"
        );

    if (message) {
        message.className =
            "status-ok";

        message.textContent =
            tr("RTSP поток выбран автоматически.")
            +
            (
                profile.name
                ? (
                    " "
                    + profile.name
                )
                : ""
            );
    }
}

function renderOnvifProfiles(
    recommendedIndex
) {
    const list =
        document.getElementById(
            "camera-profile-list"
        );

    if (!list)
        return;

    list.replaceChildren();

    for (
        let index = 0;
        index <
            onvifProfileCache.length;
        ++index
    ) {
        const profile =
            onvifProfileCache[
                index
            ];

        const card =
            document.createElement(
                "div"
            );

        card.className =
            "placeholder-card";

        const title =
            document.createElement(
                "strong"
            );

        title.textContent =
            profile.name
            || (
                tr("Профиль")
                + " "
                + (
                    index + 1
                )
            );

        title.dataset.i18nSkip = "";
        card.appendChild(title);

        const details =
            document.createElement(
                "div"
            );

        details.className =
            "muted";
        details.style.marginTop =
            "8px";

        const parts = [];

        if (profile.encoding)
            parts.push(
                profile.encoding
            );

        if (
            profile.width
            &&
            profile.height
        ) {
            parts.push(
                profile.width
                + "×"
                + profile.height
            );
        }

        if (
            Number(
                profile.fps
            ) > 0
        ) {
            parts.push(
                Number(
                    profile.fps
                ).toFixed(2)
                + " FPS"
            );
        }

        details.textContent =
            parts.join(" · ")
            || tr("ONVIF профиль");

        details.dataset.i18nSkip = "";
        card.appendChild(details);

        const uri =
            document.createElement(
                "div"
            );

        uri.className =
            "muted";
        uri.style.marginTop =
            "6px";
        uri.textContent =
            profile.rtsp_uri
            || "";
        uri.dataset.i18nSkip = "";

        card.appendChild(uri);

        const actions =
            document.createElement(
                "div"
            );

        actions.className =
            "button-row";

        const use =
            document.createElement(
                "button"
            );

        use.type = "button";
        use.className =
            index ===
                recommendedIndex
            ? ""
            : "secondary";

        use.textContent =
            index ===
                recommendedIndex
            ? tr("Основной поток")
            : tr("Использовать этот поток");

        use.addEventListener(
            "click",
            function() {
                useOnvifProfile(
                    index
                );
            }
        );

        actions.appendChild(
            use
        );

        card.appendChild(
            actions
        );

        list.appendChild(
            card
        );
    }
}

async function autoDetectCameraStream() {
    const xaddr =
        document.getElementById(
            "camera-onvif-xaddr"
        ).value.trim();

    const username =
        document.getElementById(
            "camera-username"
        ).value.trim();

    const password =
        document.getElementById(
            "camera-password"
        ).value;

    const message =
        document.getElementById(
            "camera-profile-message"
        );

    const button =
        document.getElementById(
            "camera-auto-stream-btn"
        );

    if (!xaddr) {
        const rtsp =
            document.getElementById(
                "camera-rtsp-url"
            ).value.trim();

        if (rtsp) {
            if (message) {
                message.className =
                    "status-ok";
                message.textContent =
                    tr("RTSP адрес уже определён по сетевому обнаружению.");
            }

            return;
        }

        if (message) {
            message.className =
                "status-error";
            message.textContent =
                tr("Для этой камеры ONVIF выключен и RTSP адрес не удалось определить автоматически.");
        }

        return;
    }

    if (button)
        button.disabled = true;

    if (message) {
        message.className =
            "muted";
        message.textContent =
            tr("Получение ONVIF Media Profiles...");
    }

    const parameters =
        new URLSearchParams();

    parameters.set(
        "onvif_xaddr",
        xaddr
    );

    parameters.set(
        "username",
        username
    );

    parameters.set(
        "password",
        password
    );

    try {
        const result =
            await cameraPost(
                "/api/cameras/onvif-streams",
                parameters
            );

        if (!result)
            return;

        onvifProfileCache =
            Array.isArray(
                result.profiles
            )
            ? result.profiles
            : [];

        const deviceInfo =
            result.device_info
            || {};

        setCameraDeviceInfo(
            deviceInfo
        );

        document.getElementById(
            "camera-ptz-xaddr"
        ).value =
            result.ptz_xaddr || "";

        document.getElementById(
            "camera-ptz-profile-token"
        ).value =
            result.ptz_profile_token || "";

        const cameraName =
            document.getElementById(
                "camera-name"
            );

        if (
            cameraName
            &&
            (
                !cameraName.value.trim()
                ||
                cameraName.value.startsWith(
                    "ONVIF "
                )
            )
        ) {
            const detectedName =
                [
                    deviceInfo.manufacturer,
                    deviceInfo.model
                ]
                .filter(Boolean)
                .join(" ")
                .trim();

            if (detectedName) {
                cameraName.value =
                    detectedName;
            }
        }

        const recommended =
            Math.max(
                0,
                Math.min(
                    Number(
                        result.recommended_index
                    ) || 0,
                    Math.max(
                        0,
                        onvifProfileCache.length
                        - 1
                    )
                )
            );

        renderOnvifProfiles(
            recommended
        );

        if (
            onvifProfileCache.length
        ) {
            useOnvifProfile(
                recommended
            );
        }

        if (message) {
            message.className =
                "status-ok";
            message.textContent =
                result.message
                || tr("RTSP поток определён автоматически.");
        }
    }
    catch (error) {
        onvifProfileCache = [];

        if (
            error.data
            &&
            error.data.device_info
        ) {
            setCameraDeviceInfo(
                error.data.device_info
            );
        }

        const list =
            document.getElementById(
                "camera-profile-list"
            );

        if (list)
            list.replaceChildren();

        if (message) {
            message.className =
                "status-error";
            message.textContent =
                tr("Не удалось определить RTSP автоматически: ")
                + error.message;
        }
    }
    finally {
        if (button)
            button.disabled = false;
    }
}

async function saveCamera() {
    if (!cameraCanManage())
        return;

    const id =
        document.getElementById(
            "camera-id"
        ).value;

    const name =
        document.getElementById(
            "camera-name"
        ).value.trim();

    const rtspUrl =
        document.getElementById(
            "camera-rtsp-url"
        ).value.trim();

    const onvifXaddr =
        document.getElementById(
            "camera-onvif-xaddr"
        ).value.trim();

    const manufacturer =
        document.getElementById(
            "camera-manufacturer"
        ).value.trim();

    const model =
        document.getElementById(
            "camera-model"
        ).value.trim();

    const firmwareVersion =
        document.getElementById(
            "camera-firmware-version"
        ).value.trim();

    const serialNumber =
        document.getElementById(
            "camera-serial-number"
        ).value.trim();

    const hardwareId =
        document.getElementById(
            "camera-hardware-id"
        ).value.trim();

    const ptzXaddr =
        document.getElementById(
            "camera-ptz-xaddr"
        ).value.trim();

    const ptzProfileToken =
        document.getElementById(
            "camera-ptz-profile-token"
        ).value.trim();

    const username =
        document.getElementById(
            "camera-username"
        ).value.trim();

    const password =
        document.getElementById(
            "camera-password"
        ).value;

    const enabled =
        document.getElementById(
            "camera-enabled"
        ).checked;

    const clearPassword =
        document.getElementById(
            "camera-clear-password"
        ).checked;

    const message =
        document.getElementById(
            "camera-form-message"
        );

    if (
        !name
        ||
        !rtspUrl
    ) {
        if (message) {
            message.textContent =
                tr("Заполните название и RTSP URL.");
        }

        return;
    }

    const parameters =
        new URLSearchParams();

    if (id)
        parameters.set("id", id);

    parameters.set("name", name);
    parameters.set("rtsp_url", rtspUrl);
    parameters.set(
        "onvif_xaddr",
        onvifXaddr
    );
    parameters.set(
        "manufacturer",
        manufacturer
    );
    parameters.set(
        "model",
        model
    );
    parameters.set(
        "firmware_version",
        firmwareVersion
    );
    parameters.set(
        "serial_number",
        serialNumber
    );
    parameters.set(
        "hardware_id",
        hardwareId
    );
    parameters.set(
        "ptz_xaddr",
        ptzXaddr
    );
    parameters.set(
        "ptz_profile_token",
        ptzProfileToken
    );
    parameters.set("username", username);
    parameters.set("password", password);
    parameters.set(
        "enabled",
        enabled ? "1" : "0"
    );

    if (
        !id
        ||
        password.length > 0
        ||
        clearPassword
    ) {
        parameters.set(
            "update_password",
            "1"
        );

        if (clearPassword) {
            parameters.set(
                "password",
                ""
            );
        }
    }

    const button =
        document.getElementById(
            "camera-save-btn"
        );

    if (button)
        button.disabled = true;

    try {
        const result =
            await cameraPost(
                "/api/cameras/save",
                parameters
            );

        if (!result)
            return;

        if (message)
            message.textContent =
                result.message
                || tr("Камера сохранена.");

        clearCameraForm();
        await updateCameras();
    }
    catch (error) {
        if (message) {
            message.textContent =
                tr("Ошибка камеры: ")
                + error.message;
        }
    }
    finally {
        if (button)
            button.disabled = false;
    }
}

async function probeCamera(id) {
    const parameters =
        new URLSearchParams();

    parameters.set(
        "id",
        String(id)
    );

    try {
        await cameraPost(
            "/api/cameras/probe",
            parameters
        );
    }
    catch (_) {
    }

    await updateCameras();
}

async function mediaProbeCamera(id) {
    const output =
        document.getElementById(
            "camera-media-"
            + id
        );

    if (output) {
        output.className =
            "muted";
        output.textContent =
            tr("Проверка видеопотока...");
    }

    const parameters =
        new URLSearchParams();

    parameters.set(
        "id",
        String(id)
    );

    try {
        const result =
            await cameraPost(
                "/api/cameras/media-probe",
                parameters
            );

        if (!result)
            return;

        cameraMediaCache.set(
            Number(id),
            result
        );

        renderCameraMedia(
            id
        );
    }
    catch (error) {
        const data =
            error.data || {
                success: false,
                message: error.message
            };

        cameraMediaCache.set(
            Number(id),
            data
        );

        renderCameraMedia(
            id
        );
    }
}

function renderCameraMedia(id) {
    const output =
        document.getElementById(
            "camera-media-"
            + id
        );

    if (!output)
        return;

    const result =
        cameraMediaCache.get(
            Number(id)
        );

    if (!result) {
        output.textContent = "";
        return;
    }

    if (!result.success) {
        output.className =
            "status-error";
        output.textContent =
            result.message
            || tr("Не удалось проверить видеопоток.");

        return;
    }

    const video =
        (
            result.video_codec
            || "-"
        )
        +
        (
            result.width
            &&
            result.height
            ? (
                " · "
                + result.width
                + "×"
                + result.height
            )
            : ""
        )
        +
        (
            Number(result.fps) > 0
            ? (
                " · "
                + Number(
                    result.fps
                ).toFixed(2)
                + " FPS"
            )
            : ""
        );

    const audio =
        result.audio_codec
        ? (
            " · "
            + tr("Аудио")
            + ": "
            + result.audio_codec
        )
        : "";

    output.className =
        "status-ok";
    output.textContent =
        tr("Поток")
        + ": "
        + video
        + audio;
}

async function loadCameraSnapshot(id) {
    const output =
        document.getElementById(
            "camera-snapshot-"
            + id
        );

    if (output) {
        output.replaceChildren();

        const loading =
            document.createElement(
                "div"
            );

        loading.className =
            "muted";
        loading.textContent =
            tr("Получение snapshot...");

        output.appendChild(
            loading
        );
    }

    try {
        const response =
            await fetch(
                "/api/cameras/snapshot?id="
                + encodeURIComponent(
                    String(id)
                ),
                {
                    cache: "no-store"
                }
            );

        if (response.status === 401) {
            window.location = "/login";
            return;
        }

        if (!response.ok) {
            let message =
                tr("Не удалось получить snapshot.");

            try {
                const data =
                    await response.json();

                message =
                    data.message
                    || message;
            }
            catch (_) {
            }

            throw new Error(message);
        }

        const blob =
            await response.blob();

        const dataUrl =
            await new Promise(
                function(resolve, reject) {
                    const reader =
                        new FileReader();

                    reader.onload =
                        function() {
                            resolve(
                                reader.result
                            );
                        };

                    reader.onerror =
                        reject;

                    reader.readAsDataURL(
                        blob
                    );
                }
            );

        cameraSnapshotCache.set(
            Number(id),
            dataUrl
        );

        renderCameraSnapshot(
            id
        );
    }
    catch (error) {
        if (output) {
            output.replaceChildren();

            const message =
                document.createElement(
                    "div"
                );

            message.className =
                "status-error";
            message.textContent =
                error.message;

            output.appendChild(
                message
            );
        }
    }
}

function renderCameraSnapshot(id) {
    const output =
        document.getElementById(
            "camera-snapshot-"
            + id
        );

    if (!output)
        return;

    const dataUrl =
        cameraSnapshotCache.get(
            Number(id)
        );

    if (!dataUrl)
        return;

    output.replaceChildren();

    const image =
        document.createElement(
            "img"
        );

    image.src = dataUrl;
    image.alt =
        tr("Snapshot камеры");
    image.style.width =
        "100%";
    image.style.maxHeight =
        "320px";
    image.style.objectFit =
        "contain";
    image.style.marginTop =
        "12px";
    image.style.borderRadius =
        "8px";
    image.style.background =
        "#0f1217";

    output.appendChild(
        image
    );
}

async function discoverOnvifCameras() {
    const message =
        document.getElementById(
            "camera-discovery-message"
        );

    const list =
        document.getElementById(
            "camera-discovery-list"
        );

    const button =
        document.getElementById(
            "camera-discover-btn"
        );

    if (
        !message
        ||
        !list
    ) {
        return;
    }

    if (button)
        button.disabled = true;

    message.textContent =
        tr("Поиск камер в локальной сети...");

    list.replaceChildren();

    const parameters =
        new URLSearchParams();

    parameters.set(
        "timeout_ms",
        "2000"
    );

    try {
        const result =
            await cameraPost(
                "/api/cameras/discover",
                parameters
            );

        if (!result)
            return;

        onvifDiscoveryCache =
            Array.isArray(
                result.devices
            )
            ? result.devices
            : [];

        message.textContent =
            onvifDiscoveryCache.length
            ? (
                tr("Найдено камер: ")
                + onvifDiscoveryCache.length
            )
            : tr("Камеры не найдены.");

        for (
            let index = 0;
            index <
                onvifDiscoveryCache.length;
            ++index
        ) {
            const device =
                onvifDiscoveryCache[
                    index
                ];

            const row =
                document.createElement(
                    "div"
                );

            row.className =
                "camera-discovery-row";

            const main =
                document.createElement(
                    "div"
                );

            main.className =
                "camera-discovery-main";

            const title =
                document.createElement(
                    "strong"
                );

            title.textContent =
                device.remote_address
                || tr("Камера");

            title.dataset.i18nSkip = "";

            main.appendChild(title);

            const identityParts = [];

            if (device.vendor_hint) {
                identityParts.push(
                    device.vendor_hint
                );
            }
            else if (device.onvif) {
                identityParts.push(
                    "ONVIF"
                );
            }
            else if (device.sadp) {
                identityParts.push(
                    "SADP"
                );
            }

            if (device.model_hint) {
                identityParts.push(
                    device.model_hint
                );
            }

            if (identityParts.length) {
                const identity =
                    document.createElement(
                        "div"
                    );

                identity.className =
                    "camera-discovery-meta";

                identity.textContent =
                    identityParts.join(
                        " · "
                    );

                identity.dataset.i18nSkip =
                    "";

                main.appendChild(
                    identity
                );
            }

            const detailParts = [];

            if (device.firmware_hint) {
                detailParts.push(
                    tr("Прошивка")
                    + ": "
                    + device.firmware_hint
                );
            }

            if (device.serial_hint) {
                detailParts.push(
                    tr("S/N")
                    + ": "
                    + device.serial_hint
                );
            }

            if (detailParts.length) {
                const details =
                    document.createElement(
                        "div"
                    );

                details.className =
                    "camera-discovery-meta";

                details.textContent =
                    detailParts.join(
                        " · "
                    );

                details.dataset.i18nSkip =
                    "";

                main.appendChild(
                    details
                );
            }

            row.appendChild(main);

            const use =
                document.createElement(
                    "button"
                );

            use.type = "button";
            use.textContent =
                tr("Использовать");

            use.addEventListener(
                "click",
                function() {
                    const current =
                        onvifDiscoveryCache[
                            index
                        ];

                    if (!current)
                        return;

                    const onvif =
                        document.getElementById(
                            "camera-onvif-xaddr"
                        );

                    const name =
                        document.getElementById(
                            "camera-name"
                        );

                    if (onvif) {
                        onvif.value =
                            current.xaddr
                            || "";
                    }

                    const rtsp =
                        document.getElementById(
                            "camera-rtsp-url"
                        );

                    if (rtsp) {
                        rtsp.value =
                            current.suggested_rtsp_url
                            || "";
                    }

                    onvifProfileCache = [];

                    setCameraDeviceInfo(
                        {
                            manufacturer:
                                current.vendor_hint
                                || "",
                            model:
                                current.model_hint
                                || "",
                            firmware_version:
                                current.firmware_hint
                                || "",
                            serial_number:
                                current.serial_hint
                                || "",
                            hardware_id:
                                ""
                        }
                    );

                    const profileList =
                        document.getElementById(
                            "camera-profile-list"
                        );

                    if (profileList) {
                        profileList.replaceChildren();
                    }

                    const profileMessage =
                        document.getElementById(
                            "camera-profile-message"
                        );

                    if (profileMessage) {
                        profileMessage.className =
                            "muted";

                        if (current.xaddr) {
                            profileMessage.textContent =
                                tr("Введите логин/пароль камеры и нажмите «Определить поток автоматически».");
                        }
                        else if (
                            current.suggested_rtsp_url
                        ) {
                            profileMessage.textContent =
                                current.sadp
                                ? tr("Hikvision найдена через SADP. RTSP адрес подготовлен автоматически. Введите логин/пароль и сохраните камеру.")
                                : tr("ONVIF выключен. RTSP адрес подготовлен автоматически. Введите логин/пароль и сохраните камеру.");
                        }
                        else {
                            profileMessage.textContent =
                                tr("Камера найдена без ONVIF. Укажите RTSP адрес вручную.");
                        }
                    }

                    if (
                        name
                        &&
                        !name.value.trim()
                    ) {
                        const detectedName =
                            [
                                current.vendor_hint,
                                current.model_hint
                            ]
                            .filter(Boolean)
                            .join(" ")
                            .trim();

                        const prefix =
                            detectedName
                            || tr("Камера");

                        name.value =
                            current.remote_address
                            ? (
                                prefix
                                + " "
                                + current.remote_address
                            )
                            : prefix;
                    }

                    const form =
                        document.getElementById(
                            "camera-form-title"
                        );

                    if (form) {
                        form.scrollIntoView(
                            {
                                behavior:
                                    "smooth",
                                block:
                                    "start"
                            }
                        );
                    }
                }
            );

            row.appendChild(
                use
            );

            list.appendChild(
                row
            );
        }
    }
    catch (error) {
        message.textContent =
            tr("Ошибка ONVIF discovery: ")
            + error.message;
    }
    finally {
        if (button)
            button.disabled = false;
    }
}

function liveCameraFrame(id) {
    if (
        !cameraLiveActive.has(
            Number(id)
        )
    ) {
        return;
    }

    if (document.hidden) {
        const timer =
            window.setTimeout(
                function() {
                    liveCameraFrame(id);
                },
                1500
            );

        cameraLiveTimers.set(
            Number(id),
            timer
        );

        return;
    }

    const container =
        document.getElementById(
            "camera-live-"
            + id
        );

    if (!container)
        return;

    let image =
        container.querySelector(
            "img"
        );

    if (!image) {
        container.replaceChildren();

        image =
            document.createElement(
                "img"
            );

        image.alt =
            tr("Live View камеры");
        image.style.width =
            "100%";
        image.style.maxHeight =
            "420px";
        image.style.objectFit =
            "contain";
        image.style.marginTop =
            "12px";
        image.style.borderRadius =
            "8px";
        image.style.background =
            "#0f1217";

        container.appendChild(
            image
        );
    }

    const scheduleNext =
        function(delay) {
            if (
                !cameraLiveActive.has(
                    Number(id)
                )
            ) {
                return;
            }

            const timer =
                window.setTimeout(
                    function() {
                        liveCameraFrame(
                            id
                        );
                    },
                    delay
                );

            cameraLiveTimers.set(
                Number(id),
                timer
            );
        };

    image.onload =
        function() {
            scheduleNext(900);
        };

    image.onerror =
        function() {
            const status =
                document.getElementById(
                    "camera-live-status-"
                    + id
                );

            if (status) {
                status.className =
                    "status-error";
                status.textContent =
                    tr("Live View: не удалось получить кадр.");
            }

            scheduleNext(2000);
        };

    image.src =
        "/api/cameras/snapshot?id="
        + encodeURIComponent(
            String(id)
        )
        + "&live="
        + Date.now();
}

function startCameraLive(id) {
    const numericId =
        Number(id);

    if (
        cameraLiveActive.has(
            numericId
        )
    ) {
        return;
    }

    cameraLiveActive.add(
        numericId
    );

    const status =
        document.getElementById(
            "camera-live-status-"
            + id
        );

    if (status) {
        status.className =
            "status-ok";
        status.textContent =
            tr("Live View включён.");
    }

    liveCameraFrame(id);
}

function stopCameraLive(id) {
    const numericId =
        Number(id);

    cameraLiveActive.delete(
        numericId
    );

    const timer =
        cameraLiveTimers.get(
            numericId
        );

    if (timer)
        window.clearTimeout(timer);

    cameraLiveTimers.delete(
        numericId
    );

    const container =
        document.getElementById(
            "camera-live-"
            + id
        );

    if (container)
        container.replaceChildren();

    const status =
        document.getElementById(
            "camera-live-status-"
            + id
        );

    if (status) {
        status.className =
            "muted";
        status.textContent =
            tr("Live View остановлен.");
    }
}

async function runPtzCommand(
    id,
    action
) {
    const status =
        document.getElementById(
            "camera-ptz-status-"
            + id
        );

    const parameters =
        new URLSearchParams();

    parameters.set(
        "id",
        String(id)
    );

    parameters.set(
        "action",
        action
    );

    parameters.set(
        "speed",
        "0.55"
    );

    try {
        const result =
            await cameraPost(
                "/api/cameras/ptz",
                parameters
            );

        if (
            status
            &&
            action !== "stop"
        ) {
            status.className =
                "status-ok";
            status.textContent =
                result.message
                || tr("PTZ команда выполнена.");
        }
    }
    catch (error) {
        if (status) {
            status.className =
                "status-error";
            status.textContent =
                error.message;
        }
    }
}

function ptzCommand(
    id,
    action
) {
    const numericId =
        Number(id);

    const previous =
        cameraPtzQueues.get(
            numericId
        )
        || Promise.resolve();

    const next =
        previous
            .catch(
                function() {
                }
            )
            .then(
                function() {
                    return runPtzCommand(
                        id,
                        action
                    );
                }
            );

    cameraPtzQueues.set(
        numericId,
        next
    );

    next.finally(
        function() {
            if (
                cameraPtzQueues.get(
                    numericId
                ) === next
            ) {
                cameraPtzQueues.delete(
                    numericId
                );
            }
        }
    );
}

function bindPtzButton(
    button,
    id,
    action
) {
    if (!button)
        return;

    let moving = false;

    const start =
        function(event) {
            event.preventDefault();

            if (moving)
                return;

            moving = true;

            button.setPointerCapture?.(
                event.pointerId
            );

            ptzCommand(
                id,
                action
            );
        };

    const stop =
        function(event) {
            event.preventDefault();

            if (!moving)
                return;

            moving = false;

            ptzCommand(
                id,
                "stop"
            );
        };

    button.addEventListener(
        "pointerdown",
        start
    );

    button.addEventListener(
        "pointerup",
        stop
    );

    button.addEventListener(
        "pointercancel",
        stop
    );

    button.addEventListener(
        "lostpointercapture",
        stop
    );

    button.addEventListener(
        "contextmenu",
        function(event) {
            event.preventDefault();
        }
    );
}

function createPtzControls(camera) {
    const wrapper =
        document.createElement(
            "div"
        );

    wrapper.style.marginTop =
        "12px";

    const title =
        document.createElement(
            "strong"
        );

    title.textContent =
        tr("PTZ управление");

    wrapper.appendChild(title);

    const grid =
        document.createElement(
            "div"
        );

    grid.style.display =
        "grid";
    grid.style.gridTemplateColumns =
        "repeat(3, minmax(48px, 72px))";
    grid.style.gap =
        "8px";
    grid.style.marginTop =
        "8px";

    const controls = [
        null,
        ["↑", "up"],
        null,
        ["←", "left"],
        ["■", "stop"],
        ["→", "right"],
        ["−", "zoom_out"],
        ["↓", "down"],
        ["+", "zoom_in"]
    ];

    for (
        const control of
        controls
    ) {
        if (!control) {
            const spacer =
                document.createElement(
                    "div"
                );

            grid.appendChild(
                spacer
            );

            continue;
        }

        const button =
            document.createElement(
                "button"
            );

        button.type = "button";
        button.className =
            "secondary";
        button.textContent =
            control[0];
        button.style.minHeight =
            "48px";
        button.style.touchAction =
            "none";

        if (control[1] === "stop") {
            button.addEventListener(
                "click",
                function() {
                    ptzCommand(
                        camera.id,
                        "stop"
                    );
                }
            );
        }
        else {
            bindPtzButton(
                button,
                camera.id,
                control[1]
            );
        }

        grid.appendChild(
            button
        );
    }

    wrapper.appendChild(grid);

    const status =
        document.createElement(
            "div"
        );

    status.id =
        "camera-ptz-status-"
        + camera.id;
    status.className =
        "muted";
    status.style.marginTop =
        "8px";
    status.textContent =
        tr("Удерживайте кнопку для движения.");

    wrapper.appendChild(
        status
    );

    return wrapper;
}

async function deleteCamera(id, name) {
    if (
        !window.confirm(
            tr("Удалить камеру")
            + " "
            + name
            + "?"
        )
    ) {
        return;
    }

    stopCameraLive(id);

    const parameters =
        new URLSearchParams();

    parameters.set(
        "id",
        String(id)
    );

    try {
        await cameraPost(
            "/api/cameras/delete",
            parameters
        );

        cameraMediaCache.delete(
            Number(id)
        );

        cameraSnapshotCache.delete(
            Number(id)
        );

        clearCameraForm();
        await updateCameras();
    }
    catch (error) {
        const message =
            document.getElementById(
                "camera-form-message"
            );

        if (message) {
            message.textContent =
                tr("Ошибка камеры: ")
                + error.message;
        }
    }
}

function renderCameraCard(camera) {
    const card =
        document.createElement(
            "div"
        );

    card.className =
        "placeholder-card";

    const title =
        document.createElement(
            "strong"
        );

    title.textContent =
        camera.name;
    title.dataset.i18nSkip = "";

    card.appendChild(title);

    const state =
        document.createElement(
            "div"
        );

    state.className =
        cameraStatusClass(
            camera
        );

    state.style.marginTop =
        "8px";

    state.textContent =
        cameraStatusLabel(
            camera
        );

    card.appendChild(state);

    const address =
        document.createElement(
            "div"
        );

    address.className =
        "muted";
    address.style.marginTop =
        "8px";
    address.textContent =
        camera.rtsp_url;
    address.dataset.i18nSkip = "";

    card.appendChild(address);

    if (camera.onvif_xaddr) {
        const onvif =
            document.createElement(
                "div"
            );

        onvif.className =
            "muted";
        onvif.style.marginTop =
            "6px";
        onvif.textContent =
            "ONVIF: "
            + camera.onvif_xaddr;
        onvif.dataset.i18nSkip = "";

        card.appendChild(onvif);
    }

    const deviceDetails = [];

    if (camera.manufacturer) {
        deviceDetails.push(
            tr("Производитель")
            + ": "
            + camera.manufacturer
        );
    }

    if (camera.model) {
        deviceDetails.push(
            tr("Модель")
            + ": "
            + camera.model
        );
    }

    if (camera.firmware_version) {
        deviceDetails.push(
            tr("Версия прошивки")
            + ": "
            + camera.firmware_version
        );
    }

    if (camera.serial_number) {
        deviceDetails.push(
            tr("Серийный номер")
            + ": "
            + camera.serial_number
        );
    }

    if (deviceDetails.length) {
        const info =
            document.createElement(
                "div"
            );

        info.className =
            "muted";
        info.style.marginTop =
            "8px";
        info.style.whiteSpace =
            "pre-line";
        info.textContent =
            deviceDetails.join("\n");
        info.dataset.i18nSkip = "";

        card.appendChild(info);
    }

    if (camera.username) {
        const user =
            document.createElement(
                "div"
            );

        user.className =
            "muted";

        user.textContent =
            tr("Логин")
            + ": "
            + camera.username;

        user.dataset.i18nSkip = "";

        card.appendChild(user);
    }

    if (camera.last_error) {
        const error =
            document.createElement(
                "div"
            );

        error.className =
            "status-error";
        error.style.marginTop =
            "8px";
        error.textContent =
            camera.last_error;

        card.appendChild(error);
    }

    if (
        camera.last_seen_at
        &&
        camera.last_seen_at > 0
    ) {
        const lastSeen =
            document.createElement(
                "div"
            );

        lastSeen.className =
            "muted";
        lastSeen.style.marginTop =
            "6px";

        lastSeen.textContent =
            tr("Последняя связь")
            + ": "
            + new Date(
                Number(
                    camera.last_seen_at
                ) * 1000
            ).toLocaleString();

        card.appendChild(lastSeen);
    }

    const media =
        document.createElement(
            "div"
        );

    media.id =
        "camera-media-"
        + camera.id;
    media.className =
        "muted";
    media.style.marginTop =
        "8px";

    card.appendChild(media);

    const liveStatus =
        document.createElement(
            "div"
        );

    liveStatus.id =
        "camera-live-status-"
        + camera.id;
    liveStatus.className =
        "muted";
    liveStatus.style.marginTop =
        "8px";
    liveStatus.textContent =
        cameraLiveActive.has(
            Number(camera.id)
        )
        ? tr("Live View включён.")
        : "";

    card.appendChild(
        liveStatus
    );

    const live =
        document.createElement(
            "div"
        );

    live.id =
        "camera-live-"
        + camera.id;

    card.appendChild(live);

    const snapshot =
        document.createElement(
            "div"
        );

    snapshot.id =
        "camera-snapshot-"
        + camera.id;

    card.appendChild(snapshot);

    const actions =
        document.createElement(
            "div"
        );

    actions.className =
        "button-row";

    const liveButton =
        document.createElement(
            "button"
        );

    liveButton.type =
        "button";
    liveButton.textContent =
        tr("Live");

    liveButton.addEventListener(
        "click",
        function() {
            startCameraLive(
                camera.id
            );
        }
    );

    actions.appendChild(
        liveButton
    );

    const stopLiveButton =
        document.createElement(
            "button"
        );

    stopLiveButton.type =
        "button";
    stopLiveButton.className =
        "secondary";
    stopLiveButton.textContent =
        tr("Стоп Live");

    stopLiveButton.addEventListener(
        "click",
        function() {
            stopCameraLive(
                camera.id
            );
        }
    );

    actions.appendChild(
        stopLiveButton
    );

    const snapshotButton =
        document.createElement(
            "button"
        );

    snapshotButton.type =
        "button";
    snapshotButton.className =
        "secondary";
    snapshotButton.textContent =
        tr("Snapshot");

    snapshotButton.addEventListener(
        "click",
        function() {
            loadCameraSnapshot(
                camera.id
            );
        }
    );

    actions.appendChild(
        snapshotButton
    );

    if (cameraCanManage()) {
        const probe =
            document.createElement(
                "button"
            );

        probe.type = "button";
        probe.className =
            "secondary";
        probe.textContent =
            tr("Проверить");

        probe.addEventListener(
            "click",
            function() {
                probeCamera(
                    camera.id
                );
            }
        );

        actions.appendChild(
            probe
        );

        const mediaProbe =
            document.createElement(
                "button"
            );

        mediaProbe.type =
            "button";
        mediaProbe.className =
            "secondary";
        mediaProbe.textContent =
            tr("Поток");

        mediaProbe.addEventListener(
            "click",
            function() {
                mediaProbeCamera(
                    camera.id
                );
            }
        );

        actions.appendChild(
            mediaProbe
        );

        const edit =
            document.createElement(
                "button"
            );

        edit.type = "button";
        edit.className =
            "secondary";
        edit.textContent =
            tr("Редактировать");

        edit.addEventListener(
            "click",
            function() {
                editCamera(
                    camera.id
                );
            }
        );

        actions.appendChild(
            edit
        );

        const remove =
            document.createElement(
                "button"
            );

        remove.type = "button";
        remove.className =
            "danger";
        remove.textContent =
            tr("Удалить");

        remove.addEventListener(
            "click",
            function() {
                deleteCamera(
                    camera.id,
                    camera.name
                );
            }
        );

        actions.appendChild(
            remove
        );
    }

    card.appendChild(
        actions
    );

    if (
        cameraCanManage()
        &&
        camera.ptz_supported
    ) {
        card.appendChild(
            createPtzControls(
                camera
            )
        );
    }
    else if (
        cameraCanManage()
        &&
        camera.onvif_xaddr
    ) {
        const ptzUnavailable =
            document.createElement(
                "div"
            );

        ptzUnavailable.className =
            "muted";
        ptzUnavailable.style.marginTop =
            "10px";
        ptzUnavailable.textContent =
            tr("PTZ не поддерживается этой камерой.");

        card.appendChild(
            ptzUnavailable
        );
    }

    window.setTimeout(
        function() {
            renderCameraMedia(
                camera.id
            );

            renderCameraSnapshot(
                camera.id
            );

            if (
                cameraLiveActive.has(
                    Number(
                        camera.id
                    )
                )
            ) {
                const existingTimer =
                    cameraLiveTimers.get(
                        Number(
                            camera.id
                        )
                    );

                if (!existingTimer) {
                    liveCameraFrame(
                        camera.id
                    );
                }
            }
        },
        0
    );

    return card;
}

async function updateCameras() {
    const container =
        document.getElementById(
            "camera-list"
        );

    if (!container)
        return;

    try {
        const response =
            await fetch(
                "/api/cameras",
                {
                    cache: "no-store"
                }
            );

        if (response.status === 401) {
            window.location = "/login";
            return;
        }

        if (!response.ok) {
            throw new Error(
                "Unable to load cameras"
            );
        }

        const data =
            await response.json();

        cameraCache =
            Array.isArray(
                data.cameras
            )
            ? data.cameras
            : [];

        const ids =
            new Set(
                cameraCache.map(
                    item =>
                        Number(item.id)
                )
            );

        for (
            const id of
            Array.from(
                cameraLiveActive
            )
        ) {
            if (!ids.has(id))
                stopCameraLive(id);
        }

        for (
            const id of
            cameraMediaCache.keys()
        ) {
            if (!ids.has(id))
                cameraMediaCache.delete(id);
        }

        for (
            const id of
            cameraSnapshotCache.keys()
        ) {
            if (!ids.has(id)) {
                cameraSnapshotCache.delete(
                    id
                );
            }
        }

        let online = 0;
        let offline = 0;
        let disabled = 0;

        for (
            const camera of
            cameraCache
        ) {
            if (!camera.enabled)
                ++disabled;
            else if (
                camera.status ===
                    "online"
            ) {
                ++online;
            }
            else if (
                camera.status ===
                    "offline"
            ) {
                ++offline;
            }
        }

        const counters = {
            "camera-total":
                cameraCache.length,
            "camera-online":
                online,
            "camera-offline":
                offline,
            "camera-disabled":
                disabled
        };

        for (
            const [id, value] of
            Object.entries(counters)
        ) {
            const element =
                document.getElementById(
                    id
                );

            if (element) {
                element.textContent =
                    String(value);
            }
        }

        container.replaceChildren();

        if (
            cameraCache.length === 0
        ) {
            const empty =
                document.createElement(
                    "div"
                );

            empty.className =
                "placeholder-card";

            empty.textContent =
                cameraCanManage()
                ? tr("Камеры не добавлены. Добавьте первую RTSP-камеру.")
                : tr("Камеры не добавлены.");

            container.appendChild(
                empty
            );

            return;
        }

        for (
            const camera of
            cameraCache
        ) {
            container.appendChild(
                renderCameraCard(
                    camera
                )
            );
        }
    }
    catch (error) {
        container.replaceChildren();

        const card =
            document.createElement(
                "div"
            );

        card.className =
            "placeholder-card status-error";

        card.textContent =
            tr("Ошибка загрузки камер: ")
            + error;

        container.appendChild(
            card
        );
    }
}

document.addEventListener(
    "DOMContentLoaded",
    function() {
        if (
            !document.getElementById(
                "camera-list"
            )
        ) {
            return;
        }

        updateCameras();

        const refresh =
            document.getElementById(
                "camera-refresh-btn"
            );

        if (refresh) {
            refresh.addEventListener(
                "click",
                updateCameras
            );
        }

        const discover =
            document.getElementById(
                "camera-discover-btn"
            );

        if (discover) {
            discover.addEventListener(
                "click",
                discoverOnvifCameras
            );
        }

        const autoStream =
            document.getElementById(
                "camera-auto-stream-btn"
            );

        if (autoStream) {
            autoStream.addEventListener(
                "click",
                autoDetectCameraStream
            );
        }

        const save =
            document.getElementById(
                "camera-save-btn"
            );

        if (save) {
            save.addEventListener(
                "click",
                saveCamera
            );
        }

        const cancel =
            document.getElementById(
                "camera-cancel-btn"
            );

        if (cancel) {
            cancel.addEventListener(
                "click",
                clearCameraForm
            );
        }

        setInterval(
            updateCameras,
            10000
        );
    }
);

function formatUptime(seconds) {
    seconds = Number(seconds);

    const days =
        Math.floor(
            seconds / 86400
        );

    const hours =
        Math.floor(
            (seconds % 86400) / 3600
        );

    const minutes =
        Math.floor(
            (seconds % 3600) / 60
        );

    if (days > 0) {
        return (
            days + tr(" д ")
            + hours + tr(" ч ")
            + minutes + tr(" мин")
        );
    }

    return (
        hours + tr(" ч ")
        + minutes + tr(" мин")
    );
}

function shortSha(value) {
    if (!value)
        return "-";

    return String(value).slice(0, 12);
}

async function postUpdateAction(
    url,
    parameters = null
) {
    const options = {
        method: "POST"
    };

    if (parameters) {
        options.headers = {
            "Content-Type":
                "application/x-www-form-urlencoded"
        };

        options.body =
            parameters.toString();
    }

    const response =
        await fetch(
            url,
            options
        );

    if (
        response.status === 401
    ) {
        window.location =
            "/login";

        return null;
    }

    return await response.json();
}

function renderUpdateProgress(data) {
    const percent =
        Math.max(
            0,
            Math.min(
                100,
                Number(
                    data.progress_percent
                    || 0
                )
            )
        );

    const bar =
        document.getElementById(
            "update-progress-bar"
        );

    if (bar)
        bar.style.width =
            percent + "%";

    const percentLabel =
        document.getElementById(
            "update-progress-percent"
        );

    if (percentLabel)
        percentLabel.textContent =
            percent + "%";

    const detail =
        document.getElementById(
            "update-progress-detail"
        );

    if (detail) {
        if (
            Number(data.progress_total) > 0
        ) {
            detail.textContent =
                tr("Выполнено")
                + ": "
                + Number(
                    data.progress_current
                )
                + " / "
                + Number(
                    data.progress_total
                );
        }
        else {
            detail.textContent =
                data.message
                || tr("Ожидание...");
        }
    }

    const order = [
        "check",
        "download",
        "configure",
        "build",
        "tests",
        "activation",
        "restart"
    ];

    const stage =
        String(
            data.progress_stage
            || ""
        );

    let currentIndex =
        order.indexOf(stage);

    if (
        data.state === "up_to_date"
        ||
        data.state === "update_available"
    ) {
        currentIndex = 0;
    }
    else if (
        data.state ===
        "ready_to_restart"
    ) {
        currentIndex = 6;
    }

    const failed =
        data.state === "error";

    const rows =
        document.querySelectorAll(
            "[data-update-stage]"
        );

    rows.forEach(
        function(row) {
            const rowStage =
                row.dataset.updateStage;

            const index =
                order.indexOf(
                    rowStage
                );

            const icon =
                row.querySelector(
                    ".update-stage-icon"
                );

            row.classList.remove(
                "done",
                "current",
                "error",
                "pending"
            );

            if (
                index < currentIndex
            ) {
                row.classList.add(
                    "done"
                );

                if (icon)
                    icon.textContent =
                        "✓";

                return;
            }

            if (
                index === currentIndex
            ) {
                if (failed) {
                    row.classList.add(
                        "error"
                    );

                    if (icon)
                        icon.textContent =
                            "!";
                }
                else if (
                    data.state ===
                        "up_to_date"
                    ||
                    data.state ===
                        "update_available"
                ) {
                    row.classList.add(
                        "done"
                    );

                    if (icon)
                        icon.textContent =
                            "✓";
                }
                else {
                    row.classList.add(
                        "current"
                    );

                    if (icon)
                        icon.textContent =
                            "●";
                }

                return;
            }

            row.classList.add(
                "pending"
            );

            if (icon)
                icon.textContent =
                    "○";
        }
    );
}

async function updateServerUpdateStatus() {
    try {
        const response =
            await fetch(
                "/api/update/status",
                {
                    cache: "no-store"
                }
            );

        if (
            response.status === 401
        ) {
            window.location =
                "/login";

            return;
        }

        if (!response.ok)
            return;

        const data =
            await response.json();

        const badge =
            document.getElementById(
                "update-badge"
            );

        if (badge) {
            if (
                data.restart_required
            ) {
                badge.style.display =
                    "inline";

                badge.textContent =
                    "Требуется перезапуск";
            }
            else if (
                data.update_available
            ) {
                badge.style.display =
                    "inline";

                badge.textContent =
                    "Доступно обновление";
            }
            else {
                badge.style.display =
                    "none";
            }
        }

        const branch =
            document.getElementById(
                "update-branch"
            );

        if (!branch)
            return;

        branch.textContent =
            data.branch || "-";

        document.getElementById(
            "update-local"
        ).textContent =
            shortSha(
                data.local_sha
            );

        document.getElementById(
            "update-remote"
        ).textContent =
            shortSha(
                data.remote_sha
            );

        document.getElementById(
            "update-state"
        ).textContent =
            data.state || "-";

        document.getElementById(
            "update-message"
        ).textContent =
            data.message || "";

        renderUpdateProgress(
            data
        );

        const output =
            document.getElementById(
                "update-output"
            );

        if (output) {
            if (data.last_output) {
                output.style.display =
                    "block";

                output.textContent =
                    data.last_output;
            }
            else {
                output.style.display =
                    "none";
            }
        }

        const checkButton =
            document.getElementById(
                "update-check-btn"
            );

        if (checkButton)
            checkButton.disabled =
                Boolean(data.busy);

        const applyButton =
            document.getElementById(
                "update-apply-btn"
            );

        if (applyButton) {
            applyButton.disabled =
                Boolean(data.busy)
                ||
                !data.update_available;
        }

        const restartButton =
            document.getElementById(
                "update-restart-btn"
            );

        if (restartButton) {
            restartButton.disabled =
                !data.restart_required;
        }
    }
    catch (error) {
        console.error(
            "Update status error:",
            error
        );
    }
}

function prefixToNetmask(prefix) {
    const value =
        Number(prefix);

    if (
        !Number.isInteger(value)
        ||
        value < 1
        ||
        value > 32
    ) {
        return "255.255.255.0";
    }

    const mask =
        value === 32
        ? 0xffffffff
        : (
            0xffffffff
            <<
            (
                32 - value
            )
        ) >>> 0;

    return [
        (mask >>> 24) & 255,
        (mask >>> 16) & 255,
        (mask >>> 8) & 255,
        mask & 255
    ].join(".");
}

function currentIpv4Parts(item) {
    const addresses =
        Array.isArray(
            item.ipv4_addresses
        )
        ? item.ipv4_addresses
        : [];

    if (addresses.length === 0) {
        return {
            address: "",
            netmask:
                "255.255.255.0"
        };
    }

    const parts =
        String(
            addresses[0]
        ).split("/");

    return {
        address:
            parts[0]
            || "",
        netmask:
            prefixToNetmask(
                Number(
                    parts[1]
                    || 24
                )
            )
    };
}

async function applyIpv4Configuration(
    item,
    controls,
    button
) {
    const mode =
        controls.mode.value;

    if (
        mode !== "dhcp"
        &&
        mode !== "static"
    ) {
        const message =
            document.getElementById(
                "network-helper-message"
            );

        if (message) {
            message.textContent =
                tr(
                    "Выберите режим IPv4."
                );
        }

        return;
    }

    let warning =
        tr(
            "Применить сетевые настройки для интерфейса "
        )
        + item.name
        + "?";

    if (item.default_route) {
        warning +=
            "\n\n"
            + tr(
                "Это интерфейс маршрута по умолчанию. Текущее Web-подключение может быть потеряно."
            );
    }
    else {
        warning +=
            "\n\n"
            + tr(
                "Если Web использует этот интерфейс, адрес сервера может измениться."
            );
    }

    if (
        !window.confirm(
            warning
        )
    ) {
        return;
    }

    const message =
        document.getElementById(
            "network-helper-message"
        );

    button.disabled = true;

    if (message) {
        message.textContent =
            mode === "dhcp"
            ? tr(
                "Запрос DHCP выполняется..."
            )
            : tr(
                "Применяется статический IPv4..."
            );
    }

    const parameters =
        new URLSearchParams();

    parameters.set(
        "interface",
        item.name
    );

    parameters.set(
        "mode",
        mode
    );

    if (mode === "static") {
        parameters.set(
            "address",
            controls.address.value.trim()
        );

        parameters.set(
            "netmask",
            controls.netmask.value.trim()
        );

        parameters.set(
            "gateway",
            controls.gateway.value.trim()
        );

        parameters.set(
            "dns_primary",
            controls.dnsPrimary.value.trim()
        );

        parameters.set(
            "dns_secondary",
            controls.dnsSecondary.value.trim()
        );
    }

    try {
        const response =
            await fetch(
                "/api/network/ipv4",
                {
                    method: "POST",
                    headers: {
                        "X-HomeAI-Request":
                            "1",
                        "Content-Type":
                            "application/x-www-form-urlencoded"
                    },
                    body:
                        parameters.toString()
                }
            );

        if (
            response.status === 401
        ) {
            window.location =
                "/login";

            return;
        }

        const data =
            await response.json();

        if (message) {
            message.textContent =
                data.message
                || tr(
                    "Сетевая конфигурация применена."
                );
        }

        if (!response.ok)
            return;

        window.setTimeout(
            function() {
                updateNetworkInterfaces(
                    true
                );
            },
            1500
        );
    }
    catch (error) {
        if (message) {
            message.textContent =
                tr(
                    "Ошибка настройки IPv4: "
                )
                + error;
        }
    }
    finally {
        button.disabled = false;
    }
}

async function updateNetworkInterfaces(
    force
) {
    const container =
        document.getElementById(
            "network-interface-list"
        );

    if (!container)
        return;

    if (
        !force
        &&
        (
            (
                document.activeElement
                &&
                container.contains(
                    document.activeElement
                )
            )
            ||
            container.querySelector(
                "[data-network-dirty=\"1\"]"
            )
        )
    ) {
        return;
    }

    const canManage =
        container.dataset.canManage ===
        "1";

    const message =
        document.getElementById(
            "network-helper-message"
        );

    try {
        const response =
            await fetch(
                "/api/network/interfaces",
                {
                    cache: "no-store"
                }
            );

        if (
            response.status ===
            401
        ) {
            window.location =
                "/login";

            return;
        }

        if (!response.ok)
            return;

        const data =
            await response.json();

        if (message) {
            message.textContent =
                data.helper_installed
                ? tr(
                    "Network Helper готов."
                )
                : tr(
                    "Для изменения IPv4 установите Network Helper."
                );
        }

        const interfaces =
            Array.isArray(
                data.interfaces
            )
            ? data.interfaces
            : [];

        container.replaceChildren();

        if (interfaces.length === 0) {
            const empty =
                document.createElement(
                    "div"
                );

            empty.className =
                "storage-card";

            empty.textContent =
                tr(
                    "Сетевые интерфейсы не обнаружены."
                );

            container.appendChild(
                empty
            );

            return;
        }

        for (const item of interfaces) {
            const card =
                document.createElement(
                    "div"
                );

            card.className =
                "storage-card";

            const title =
                document.createElement(
                    "strong"
                );

            title.textContent =
                item.name;

            title.dataset.i18nSkip =
                "";

            card.appendChild(title);

            const state =
                document.createElement(
                    "div"
                );

            state.className =
                item.up
                ? "status-ok"
                : "muted";

            state.textContent =
                tr("Состояние")
                + ": "
                + (
                    item.up
                    ? "UP"
                    : "DOWN"
                )
                + (
                    item.carrier
                    ? " · LINK"
                    : ""
                );

            card.appendChild(state);

            const addresses =
                document.createElement(
                    "div"
                );

            const ipv4 =
                Array.isArray(
                    item.ipv4_addresses
                )
                ? item.ipv4_addresses
                : [];

            addresses.textContent =
                "IPv4: "
                + (
                    ipv4.length > 0
                    ? ipv4.join(", ")
                    : tr(
                        "адрес не назначен"
                    )
                );

            addresses.dataset.i18nSkip =
                "";

            card.appendChild(
                addresses
            );

            const mac =
                document.createElement(
                    "div"
                );

            mac.textContent =
                "MAC: "
                + (
                    item.mac_address
                    || "-"
                );

            mac.dataset.i18nSkip =
                "";

            card.appendChild(mac);

            const details =
                document.createElement(
                    "div"
                );

            details.className =
                "muted";

            details.textContent =
                "MTU "
                + Number(
                    item.mtu
                    || 0
                )
                + (
                    item.default_route
                    ? " · "
                        + tr(
                            "маршрут по умолчанию"
                        )
                    : ""
                );

            card.appendChild(
                details
            );

            if (
                canManage
                &&
                !item.loopback
            ) {
                const current =
                    currentIpv4Parts(
                        item
                    );

                const config =
                    document.createElement(
                        "div"
                    );

                config.className =
                    "form-grid";

                config.style.marginTop =
                    "12px";

                config.dataset.networkDirty =
                    "0";

                const makeField =
                    function(
                        labelText,
                        value,
                        type
                    ) {
                        const wrapper =
                            document.createElement(
                                "div"
                            );

                        const label =
                            document.createElement(
                                "label"
                            );

                        label.textContent =
                            tr(labelText);

                        const input =
                            document.createElement(
                                type === "select"
                                ? "select"
                                : "input"
                            );

                        wrapper.appendChild(
                            label
                        );

                        wrapper.appendChild(
                            input
                        );

                        return {
                            wrapper:
                                wrapper,
                            input:
                                input,
                            value:
                                value
                        };
                    };

                const modeField =
                    makeField(
                        "Режим IPv4",
                        item.ipv4_method,
                        "select"
                    );

                const modeOptions = [];

                if (
                    item.ipv4_method !==
                        "dhcp"
                    &&
                    item.ipv4_method !==
                        "static"
                ) {
                    modeOptions.push(
                        {
                            value:
                                "",
                            label:
                                tr(
                                    "Выберите режим"
                                )
                        }
                    );
                }

                modeOptions.push(
                    {
                        value:
                            "dhcp",
                        label:
                            tr("DHCP")
                    },
                    {
                        value:
                            "static",
                        label:
                            tr(
                                "Статический IP"
                            )
                    }
                );

                for (
                    const optionData of
                    modeOptions
                ) {
                    const option =
                        document.createElement(
                            "option"
                        );

                    option.value =
                        optionData.value;

                    option.textContent =
                        optionData.label;

                    modeField.input.appendChild(
                        option
                    );
                }

                modeField.input.value =
                    item.ipv4_method ===
                        "static"
                    ? "static"
                    : (
                        item.ipv4_method ===
                            "dhcp"
                        ? "dhcp"
                        : ""
                    );

                config.appendChild(
                    modeField.wrapper
                );

                const addressField =
                    makeField(
                        "IP-адрес",
                        current.address
                    );

                addressField.input.value =
                    current.address;

                config.appendChild(
                    addressField.wrapper
                );

                const netmaskField =
                    makeField(
                        "Маска сети",
                        current.netmask
                    );

                netmaskField.input.value =
                    current.netmask;

                config.appendChild(
                    netmaskField.wrapper
                );

                const gatewayField =
                    makeField(
                        "Шлюз",
                        item.gateway
                        || ""
                    );

                gatewayField.input.value =
                    item.gateway
                    || "";

                config.appendChild(
                    gatewayField.wrapper
                );

                const dns =
                    Array.isArray(
                        item.dns_servers
                    )
                    ? item.dns_servers
                    : [];

                const dnsPrimaryField =
                    makeField(
                        "Основной DNS",
                        dns[0]
                        || ""
                    );

                dnsPrimaryField.input.value =
                    dns[0]
                    || "";

                config.appendChild(
                    dnsPrimaryField.wrapper
                );

                const dnsSecondaryField =
                    makeField(
                        "Дополнительный DNS",
                        dns[1]
                        || ""
                    );

                dnsSecondaryField.input.value =
                    dns[1]
                    || "";

                config.appendChild(
                    dnsSecondaryField.wrapper
                );

                const staticInputs = [
                    addressField.input,
                    netmaskField.input,
                    gatewayField.input,
                    dnsPrimaryField.input,
                    dnsSecondaryField.input
                ];

                const refreshMode =
                    function() {
                        const staticMode =
                            modeField.input.value ===
                            "static";

                        for (
                            const input of
                            staticInputs
                        ) {
                            input.disabled =
                                !staticMode
                                ||
                                !data.helper_installed;
                        }
                    };

                modeField.input.disabled =
                    !data.helper_installed;

                const markDirty =
                    function() {
                        config.dataset.networkDirty =
                            "1";
                    };

                modeField.input.addEventListener(
                    "change",
                    function() {
                        markDirty();
                        refreshMode();
                    }
                );

                for (
                    const input of
                    staticInputs
                ) {
                    input.addEventListener(
                        "input",
                        markDirty
                    );
                }

                refreshMode();

                card.appendChild(
                    config
                );

                const actions =
                    document.createElement(
                        "div"
                    );

                actions.className =
                    "button-row";

                const apply =
                    document.createElement(
                        "button"
                    );

                apply.type =
                    "button";

                apply.textContent =
                    tr(
                        "Применить IPv4"
                    );

                apply.disabled =
                    !data.helper_installed;

                apply.addEventListener(
                    "click",
                    function() {
                        applyIpv4Configuration(
                            item,
                            {
                                mode:
                                    modeField.input,
                                address:
                                    addressField.input,
                                netmask:
                                    netmaskField.input,
                                gateway:
                                    gatewayField.input,
                                dnsPrimary:
                                    dnsPrimaryField.input,
                                dnsSecondary:
                                    dnsSecondaryField.input
                            },
                            apply
                        );
                    }
                );

                actions.appendChild(
                    apply
                );

                card.appendChild(
                    actions
                );
            }

            container.appendChild(
                card
            );
        }
    }
    catch (error) {
        if (message) {
            message.textContent =
                tr(
                    "Ошибка получения сетевых интерфейсов: "
                )
                + error;
        }
    }
}

let vpnEditorProfile = "";
let vpnEditorWasActive = false;

function newVpnProfile() {
    const editor =
        document.getElementById(
            "vpn-editor"
        );

    if (!editor)
        return;

    const name =
        document.getElementById(
            "vpn-profile-name"
        );

    const config =
        document.getElementById(
            "vpn-profile-config"
        );

    const removeButton =
        document.getElementById(
            "vpn-delete-btn"
        );

    const editorMessage =
        document.getElementById(
            "vpn-editor-message"
        );

    vpnEditorProfile = "";
    vpnEditorWasActive = false;

    if (name) {
        name.disabled = false;
        name.value = "";
    }

    if (config) {
        config.value =
            "[Interface]\n"
            + "PrivateKey = \n"
            + "Address = \n"
            + "\n"
            + "[Peer]\n"
            + "PublicKey = \n"
            + "AllowedIPs = \n"
            + "Endpoint = \n";
    }

    if (removeButton) {
        removeButton.disabled =
            true;
    }

    if (editorMessage) {
        editorMessage.textContent =
            "Новый профиль. Заполните имя и конфигурацию.";
    }

    if (name) {
        name.focus();
    }
}

async function editVpnProfile(
    profileName,
    active
) {
    const editor =
        document.getElementById(
            "vpn-editor"
        );

    if (!editor)
        return;

    const name =
        document.getElementById(
            "vpn-profile-name"
        );

    const config =
        document.getElementById(
            "vpn-profile-config"
        );

    const removeButton =
        document.getElementById(
            "vpn-delete-btn"
        );

    const editorMessage =
        document.getElementById(
            "vpn-editor-message"
        );

    if (editorMessage) {
        editorMessage.textContent =
            "Загрузка профиля...";
    }

    try {
        const response =
            await fetch(
                "/api/network/vpn/profile?profile="
                + encodeURIComponent(
                    profileName
                ),
                {
                    cache: "no-store"
                }
            );

        if (response.status === 401) {
            window.location =
                "/login";

            return;
        }

        const result =
            await response.json();

        if (!response.ok) {
            throw new Error(
                result.message
                || "Не удалось загрузить профиль."
            );
        }

        vpnEditorProfile =
            profileName;

        vpnEditorWasActive =
            !!active;

        if (name) {
            name.value =
                result.profile
                || profileName;

            name.disabled =
                true;
        }

        if (config) {
            config.value =
                result.config
                || "";
        }

        if (removeButton) {
            removeButton.disabled =
                false;
        }

        if (editorMessage) {
            editorMessage.textContent =
                active
                ? "Профиль активен. Сохранение не перезапускает туннель."
                : "Профиль загружен.";
        }

        editor.scrollIntoView(
            {
                behavior: "smooth",
                block: "start"
            }
        );
    }
    catch (error) {
        if (editorMessage) {
            editorMessage.textContent =
                "Ошибка WireGuard: "
                + error;
        }
    }
}

async function saveVpnProfile() {
    const name =
        document.getElementById(
            "vpn-profile-name"
        );

    const config =
        document.getElementById(
            "vpn-profile-config"
        );

    const saveButton =
        document.getElementById(
            "vpn-save-btn"
        );

    const removeButton =
        document.getElementById(
            "vpn-delete-btn"
        );

    const editorMessage =
        document.getElementById(
            "vpn-editor-message"
        );

    if (
        !name
        ||
        !config
        ||
        !saveButton
    ) {
        return;
    }

    const profileName =
        name.value.trim();

    if (
        !/^[A-Za-z0-9_-]{1,32}$/.test(
            profileName
        )
    ) {
        if (editorMessage) {
            editorMessage.textContent =
                "Имя профиля: 1–32 символа, только буквы, цифры, - и _.";
        }

        return;
    }

    if (
        !config.value
        ||
        !config.value.includes(
            "[Interface]"
        )
    ) {
        if (editorMessage) {
            editorMessage.textContent =
                "Конфигурация должна содержать секцию [Interface].";
        }

        return;
    }

    const parameters =
        new URLSearchParams();

    parameters.set(
        "action",
        "save"
    );

    parameters.set(
        "profile",
        profileName
    );

    parameters.set(
        "config",
        config.value
    );

    saveButton.disabled =
        true;

    try {
        const response =
            await fetch(
                "/api/network/vpn/profile",
                {
                    method: "POST",
                    headers: {
                        "Content-Type":
                            "application/x-www-form-urlencoded",
                        "X-HomeAI-Request":
                            "1"
                    },
                    body:
                        parameters.toString()
                }
            );

        if (response.status === 401) {
            window.location =
                "/login";

            return;
        }

        const result =
            await response.json();

        if (editorMessage) {
            editorMessage.textContent =
                result.message
                || (
                    response.ok
                    ? "Профиль сохранён."
                    : "Не удалось сохранить профиль."
                );
        }

        if (!response.ok)
            return;

        vpnEditorProfile =
            profileName;

        name.disabled =
            true;

        if (removeButton) {
            removeButton.disabled =
                false;
        }

        if (
            vpnEditorWasActive
            &&
            editorMessage
        ) {
            editorMessage.textContent +=
                " Для применения изменений отключите и снова подключите профиль.";
        }

        await updateVpnProfiles();
    }
    catch (error) {
        if (editorMessage) {
            editorMessage.textContent =
                "Ошибка WireGuard: "
                + error;
        }
    }
    finally {
        saveButton.disabled =
            false;
    }
}

async function deleteVpnProfile() {
    if (!vpnEditorProfile)
        return;

    const removeButton =
        document.getElementById(
            "vpn-delete-btn"
        );

    const editorMessage =
        document.getElementById(
            "vpn-editor-message"
        );

    if (
        !window.confirm(
            "Удалить профиль WireGuard "
            + vpnEditorProfile
            + "?"
        )
    ) {
        return;
    }

    const parameters =
        new URLSearchParams();

    parameters.set(
        "action",
        "remove"
    );

    parameters.set(
        "profile",
        vpnEditorProfile
    );

    if (removeButton) {
        removeButton.disabled =
            true;
    }

    try {
        const response =
            await fetch(
                "/api/network/vpn/profile",
                {
                    method: "POST",
                    headers: {
                        "Content-Type":
                            "application/x-www-form-urlencoded",
                        "X-HomeAI-Request":
                            "1"
                    },
                    body:
                        parameters.toString()
                }
            );

        if (response.status === 401) {
            window.location =
                "/login";

            return;
        }

        const result =
            await response.json();

        if (editorMessage) {
            editorMessage.textContent =
                result.message
                || (
                    response.ok
                    ? "Профиль удалён."
                    : "Не удалось удалить профиль."
                );
        }

        if (!response.ok)
            return;

        newVpnProfile();

        await updateVpnProfiles();
    }
    catch (error) {
        if (editorMessage) {
            editorMessage.textContent =
                "Ошибка WireGuard: "
                + error;
        }
    }
    finally {
        if (
            removeButton
            &&
            vpnEditorProfile
        ) {
            removeButton.disabled =
                false;
        }
    }
}

async function updateVpnProfiles() {
    const container =
        document.getElementById(
            "vpn-list"
        );

    if (!container)
        return;

    const message =
        document.getElementById(
            "vpn-message"
        );

    const canManage =
        document.getElementById(
            "vpn-editor"
        ) !== null;

    try {
        const response =
            await fetch(
                "/api/network/vpn",
                {
                    cache: "no-store"
                }
            );

        if (
            response.status === 401
        ) {
            window.location =
                "/login";

            return;
        }

        if (!response.ok)
            return;

        const data =
            await response.json();

        container.replaceChildren();

        if (message) {
            if (!data.available) {
                message.textContent =
                    "wg-quick не найден. Установите wireguard-tools.";
            }
            else if (data.error) {
                message.textContent =
                    data.error;
            }
            else {
                message.textContent =
                    "WireGuard готов.";
            }
        }

        const profiles =
            Array.isArray(
                data.profiles
            )
            ? data.profiles
            : [];

        if (profiles.length === 0) {
            const empty =
                document.createElement(
                    "div"
                );

            empty.className =
                "placeholder-card";

            empty.textContent =
                canManage
                ? "Профили не найдены. Создайте профиль в редакторе ниже."
                : "Профили WireGuard не найдены.";

            container.appendChild(
                empty
            );

            return;
        }

        for (const profile of profiles) {
            const card =
                document.createElement(
                    "div"
                );

            card.className =
                "placeholder-card";

            const title =
                document.createElement(
                    "strong"
                );

            title.textContent =
                profile.name;
            title.dataset.i18nSkip = '';

            card.appendChild(title);

            const state =
                document.createElement(
                    "div"
                );

            state.className =
                profile.active
                ? "status-ok"
                : "muted";

            state.textContent =
                profile.active
                ? "CONNECTED"
                : "DISCONNECTED";

            card.appendChild(state);

            if (canManage) {
                const actions =
                    document.createElement(
                        "div"
                    );

                actions.className =
                    "button-row";

                const button =
                    document.createElement(
                        "button"
                    );

                button.type =
                    "button";

                button.textContent =
                    profile.active
                    ? "Отключить"
                    : "Подключить";

                if (profile.active) {
                    button.className =
                        "secondary";
                }

                button.addEventListener(
                    "click",
                    async function() {
                        const parameters =
                            new URLSearchParams();

                        parameters.set(
                            "profile",
                            profile.name
                        );

                        parameters.set(
                            "action",
                            profile.active
                                ? "disconnect"
                                : "connect"
                        );

                        button.disabled = true;

                        try {
                            const actionResponse =
                                await fetch(
                                    "/api/network/vpn/action",
                                    {
                                        method: "POST",
                                        headers: {
                                            "Content-Type":
                                                "application/x-www-form-urlencoded",
                                            "X-HomeAI-Request":
                                                "1"
                                        },
                                        body:
                                            parameters.toString()
                                    }
                                );

                            const result =
                                await actionResponse.json();

                            if (message) {
                                message.textContent =
                                    result.message
                                    || "Операция завершена.";
                            }

                            await updateVpnProfiles();
                        }
                        catch (error) {
                            if (message) {
                                message.textContent =
                                    "Ошибка WireGuard: "
                                    + error;
                            }
                        }
                        finally {
                            button.disabled =
                                false;
                        }
                    }
                );

                actions.appendChild(
                    button
                );

                const editButton =
                    document.createElement(
                        "button"
                    );

                editButton.type =
                    "button";

                editButton.className =
                    "secondary";

                editButton.textContent =
                    "Редактировать";

                editButton.addEventListener(
                    "click",
                    function() {
                        editVpnProfile(
                            profile.name,
                            profile.active
                        );
                    }
                );

                actions.appendChild(
                    editButton
                );

                card.appendChild(
                    actions
                );
            }

            container.appendChild(
                card
            );
        }
    }
    catch (error) {
        if (message) {
            message.textContent =
                "Ошибка WireGuard: "
                + error;
        }
    }
}

function moduleDisplayName(name) {
    const names = {
        "security": "Security Core",
        "update": "Update Manager",
        "system-monitor": "System Monitor",
        "storage-monitor": "Storage Monitor",
        "cameras": "Camera Core",
        "web": "Web Core"
    };

    return names[name] || name;
}

function moduleHealthLabel(health) {
    if (health === "healthy")
        return "HEALTHY";

    if (health === "degraded")
        return "DEGRADED";

    if (health === "unhealthy")
        return "UNHEALTHY";

    return "UNKNOWN";
}

function moduleHealthClass(health) {
    if (health === "healthy")
        return "status-ok";

    if (health === "degraded")
        return "status-warn";

    if (health === "unhealthy")
        return "status-error";

    return "muted";
}

async function updateModuleStatus() {
    const container =
        document.getElementById(
            "module-list"
        );

    if (!container)
        return;

    try {
        const response =
            await fetch(
                "/api/modules",
                {
                    cache: "no-store"
                }
            );

        if (
            response.status === 401
        ) {
            window.location =
                "/login";

            return;
        }

        if (!response.ok)
            return;

        const data =
            await response.json();

        const modules =
            Array.isArray(
                data.modules
            )
            ? data.modules
            : [];

        container.replaceChildren();

        for (const module of modules) {
            const card =
                document.createElement(
                    "div"
                );

            card.className =
                "placeholder-card";

            const title =
                document.createElement(
                    "strong"
                );

            title.textContent =
                moduleDisplayName(
                    module.name
                );

            card.appendChild(title);

            const lifecycle =
                document.createElement(
                    "div"
                );

            lifecycle.className =
                "muted";

            lifecycle.textContent =
                "Lifecycle: "
                + (
                    module.state
                    || "unknown"
                );

            card.appendChild(
                lifecycle
            );

            const health =
                document.createElement(
                    "div"
                );

            health.className =
                moduleHealthClass(
                    module.health
                );

            health.textContent =
                "Health: "
                + moduleHealthLabel(
                    module.health
                );

            card.appendChild(
                health
            );

            if (module.message) {
                const message =
                    document.createElement(
                        "div"
                    );

                message.className =
                    "muted";

                message.style.marginTop =
                    "8px";

                message.textContent =
                    module.message;

                card.appendChild(
                    message
                );
            }

            if (
                Array.isArray(
                    module.dependencies
                )
                &&
                module.dependencies
                    .length > 0
            ) {
                const dependencies =
                    document.createElement(
                        "div"
                    );

                dependencies.className =
                    "muted";

                dependencies.style.marginTop =
                    "8px";

                dependencies.textContent =
                    "Зависимости: "
                    + module.dependencies
                        .join(", ");

                card.appendChild(
                    dependencies
                );
            }

            container.appendChild(
                card
            );
        }

        if (modules.length === 0) {
            const empty =
                document.createElement(
                    "div"
                );

            empty.className =
                "placeholder-card";

            empty.textContent =
                "Модули не зарегистрированы.";

            container.appendChild(
                empty
            );
        }
    }
    catch (error) {
        console.error(
            "Module status error:",
            error
        );
    }
}

function formatBytes(value) {
    const bytes =
        Number(value);

    if (
        !Number.isFinite(bytes)
        ||
        bytes <= 0
    ) {
        return "0 B";
    }

    const units = [
        "B",
        "KiB",
        "MiB",
        "GiB",
        "TiB",
        "PiB"
    ];

    let size = bytes;
    let index = 0;

    while (
        size >= 1024
        &&
        index <
            units.length - 1
    ) {
        size /= 1024;
        ++index;
    }

    return (
        size.toFixed(
            index === 0
            ? 0
            : 1
        )
        + " "
        + units[index]
    );
}

async function updateSystemStats() {
    const cpu =
        document.getElementById(
            "cpu-value"
        );

    if (!cpu)
        return;

    try {
        const response =
            await fetch(
                "/api/system",
                {
                    method: "GET",
                    cache: "no-store"
                }
            );

        if (
            response.status === 401
        ) {
            window.location =
                "/login";

            return;
        }

        if (!response.ok)
            return;

        const data =
            await response.json();

        document.getElementById(
            "cpu-value"
        ).textContent =
            Number(
                data.cpu_percent
            ).toFixed(1)
            + "%";

        document.getElementById(
            "ram-value"
        ).textContent =
            Number(
                data.memory_percent
            ).toFixed(1)
            + "%";

        document.getElementById(
            "disk-value"
        ).textContent =
            Number(
                data.disk_percent
            ).toFixed(1)
            + "%";

        document.getElementById(
            "uptime-value"
        ).textContent =
            formatUptime(
                data.uptime_seconds
            );

        document.getElementById(
            "load-value"
        ).textContent =
            Number(
                data.load_1
            ).toFixed(2)
            + " / "
            + Number(
                data.load_5
            ).toFixed(2)
            + " / "
            + Number(
                data.load_15
            ).toFixed(2);
    }
    catch (error) {
        console.error(
            "System monitor error:",
            error
        );
    }
}

async function updateHomeErrors() {
    const container =
        document.getElementById(
            "home-errors"
        );

    if (!container)
        return;

    const errors = [];

    try {
        const systemResponse =
            await fetch(
                "/api/system",
                {
                    cache: "no-store"
                }
            );

        if (!systemResponse.ok) {
            errors.push(
                "Не удалось получить системную статистику."
            );
        }
        else {
            const stats =
                await systemResponse.json();

            if (
                Number(
                    stats.memory_percent
                ) >= 95
            ) {
                errors.push(
                    "Критически высокая загрузка RAM."
                );
            }

            if (
                Number(
                    stats.disk_percent
                ) >= 95
            ) {
                errors.push(
                    "Системный диск заполнен более чем на 95%."
                );
            }
        }

        const storageResponse =
            await fetch(
                "/api/storage",
                {
                    cache: "no-store"
                }
            );

        if (!storageResponse.ok) {
            errors.push(
                "Не удалось получить состояние хранилищ."
            );
        }
        else {
            const storage =
                await storageResponse.json();

            for (
                const volume of
                (
                    Array.isArray(
                        storage.volumes
                    )
                    ? storage.volumes
                    : []
                )
            ) {
                if (
                    volume.status ===
                    "offline"
                ) {
                    errors.push(
                        "Хранилище OFFLINE: "
                        + (
                            volume.mount_point
                            || "неизвестно"
                        )
                    );
                }

                if (
                    volume.status ===
                        "online"
                    &&
                    Number(
                        volume.used_percent
                    ) >= 95
                ) {
                    errors.push(
                        "Хранилище заполнено более чем на 95%: "
                        + (
                            volume.mount_point
                            || volume.source
                            || "неизвестно"
                        )
                    );
                }

                if (
                    volume.read_only
                    &&
                    volume.status ===
                        "online"
                ) {
                    errors.push(
                        "Хранилище доступно только для чтения: "
                        + (
                            volume.mount_point
                            || volume.source
                        )
                    );
                }
            }
        }

        const modulesResponse =
            await fetch(
                "/api/modules",
                {
                    cache: "no-store"
                }
            );

        if (!modulesResponse.ok) {
            errors.push(
                "Не удалось получить состояние модулей ядра."
            );
        }
        else {
            const modulesData =
                await modulesResponse.json();

            for (
                const module of
                (
                    Array.isArray(
                        modulesData.modules
                    )
                    ? modulesData.modules
                    : []
                )
            ) {
                if (
                    module.state === "failed"
                    ||
                    module.health === "unhealthy"
                ) {
                    errors.push(
                        "Ошибка модуля "
                        + moduleDisplayName(
                            module.name
                        )
                        + ": "
                        + (
                            module.message
                            || module.health
                            || module.state
                        )
                    );
                }
                else if (
                    module.health ===
                    "degraded"
                ) {
                    errors.push(
                        "Предупреждение модуля "
                        + moduleDisplayName(
                            module.name
                        )
                        + ": "
                        + (
                            module.message
                            || "degraded"
                        )
                    );
                }
            }
        }
    }
    catch (error) {
        errors.push(
            "Ошибка проверки состояния: "
            + error
        );
    }

    container.replaceChildren();

    if (errors.length === 0) {
        const item =
            document.createElement(
                "div"
            );

        item.className =
            "error-ok";

        item.textContent =
            "Активных ошибок не обнаружено.";

        container.appendChild(
            item
        );

        return;
    }

    for (const message of errors) {
        const item =
            document.createElement(
                "div"
            );

        item.className =
            "error-item";

        item.textContent =
            message;

        container.appendChild(
            item
        );
    }
}

function storageRoleLabel(role) {
    if (role === "video")
        return "Видео";

    if (role === "personal")
        return "Личные файлы";

    if (
        role ===
        "video+personal"
    ) {
        return "Видео + личные файлы";
    }

    if (role === "system")
        return "Системный";

    return "Не назначен";
}

const ignoredStorageDevices =
    new Set();

let knownStorageCandidates =
    null;

let storageDeviceInventory =
    [];

let storageVolumeInventory =
    [];

let storageHelperInstalled =
    false;

let currentDiskDevice =
    "";

function storageDeviceTitle(device) {
    const model =
        (
            (device.vendor || "")
            + " "
            + (device.model || "")
        ).trim();

    return (
        model
        || device.device
    );
}

function findStorageDevice(
    devicePath
) {
    return storageDeviceInventory
        .find(
            function(device) {
                return (
                    device.device ===
                    devicePath
                );
            }
        );
}

function setDiskMenuResult(
    message,
    isError = false
) {
    const result =
        document.getElementById(
            "disk-menu-result"
        );

    if (!result)
        return;

    result.style.display =
        "block";

    result.className =
        isError
        ? "disk-menu-result error"
        : "disk-menu-result";

    result.textContent =
        message;
}

function showStorageAlert(message) {
    const alertBox =
        document.getElementById(
            "storage-hotplug-alert"
        );

    if (!alertBox)
        return;

    alertBox.style.display =
        "block";

    alertBox.textContent =
        message;
}

function closeDiskMenu() {
    const overlay =
        document.getElementById(
            "disk-menu-overlay"
        );

    if (overlay)
        overlay.style.display =
            "none";

    currentDiskDevice = "";
}

function updateDiskMenuState(device) {
    const warning =
        document.getElementById(
            "disk-menu-warning"
        );

    const result =
        document.getElementById(
            "disk-menu-result"
        );

    if (result)
        result.style.display =
            "none";

    const canPrivileged =
        storageHelperInstalled;

    const candidate =
        Boolean(
            device &&
            device.candidate
        );

    const mounted =
        Boolean(
            device &&
            device.mounted
        );

    if (warning) {
        warning.textContent =
            !mounted &&
            !canPrivileged
            ? tr(
                "Для подключения нового диска нужно установить привилегированный Storage Helper."
            )
            : "";
    }

    const assignmentHelp =
        document.getElementById(
            "disk-assignment-help"
        );

    if (assignmentHelp) {
        if (mounted) {
            assignmentHelp.textContent =
                tr(
                    "Диск уже подключён к Linux в "
                )
                + (
                    device.mount_point
                    || tr(
                        "текущей точке"
                    )
                )
                + tr(
                    ". Можно выбрать одно или оба назначения. Диск перемонтирован не будет."
                );
        }
        else if (candidate) {
            assignmentHelp.textContent =
                tr(
                    "Диск ещё не подключён. Выберите одно или оба назначения — Home AI Core смонтирует его один раз и добавит в выбранные пулы."
                );
        }
        else {
            assignmentHelp.textContent =
                tr(
                    "Этот диск сейчас используется системой или недоступен для безопасного подключения."
                );
        }
    }

    const assignedVolume =
        mounted
        ? storageVolumeInventory.find(
            function(volume) {
                return (
                    volume.mount_point ===
                    device.mount_point
                );
            }
        )
        : null;

    const videoRole =
        document.getElementById(
            "disk-role-video"
        );

    const personalRole =
        document.getElementById(
            "disk-role-personal"
        );

    if (videoRole) {
        videoRole.checked =
            Boolean(
                assignedVolume
                &&
                (
                    assignedVolume.role ===
                        "video"
                    ||
                    assignedVolume.role ===
                        "video+personal"
                )
            );

        videoRole.disabled =
            !mounted
            &&
            (
                !candidate
                ||
                !canPrivileged
            );
    }

    if (personalRole) {
        personalRole.checked =
            Boolean(
                assignedVolume
                &&
                (
                    assignedVolume.role ===
                        "personal"
                    ||
                    assignedVolume.role ===
                        "video+personal"
                )
            );

        personalRole.disabled =
            !mounted
            &&
            (
                !candidate
                ||
                !canPrivileged
            );
    }

    const applyRoles =
        document.getElementById(
            "disk-apply-roles"
        );

    if (applyRoles) {
        applyRoles.disabled =
            !mounted
            &&
            (
                !candidate
                ||
                !canPrivileged
            );
    }

    const managedMount =
        mounted
        &&
        (
            device.mount_point
                .startsWith(
                    "/mnt/home-ai/video/"
                )
            ||
            device.mount_point
                .startsWith(
                    "/mnt/home-ai/files/"
                )
            ||
            device.mount_point
                .startsWith(
                    "/mnt/home-ai/storage/"
                )
        );

    const setDisabled =
        function(id, disabled) {
            const button =
                document.getElementById(
                    id
                );

            if (button)
                button.disabled =
                    disabled;
        };

    setDisabled(
        "disk-unmount",
        !managedMount ||
        !canPrivileged
    );

    setDisabled(
        "disk-format-ext4",
        !candidate ||
        !canPrivileged
    );

    setDisabled(
        "disk-wipefs",
        !candidate ||
        !canPrivileged
    );
}

function openDiskMenu(devicePath) {
    const device =
        findStorageDevice(
            devicePath
        );

    if (!device) {
        showStorageAlert(
            "Не удалось получить информацию об устройстве "
            + devicePath
            + ". Обновите список дисков."
        );

        return;
    }

    currentDiskDevice =
        device.device;

    const overlay =
        document.getElementById(
            "disk-menu-overlay"
        );

    const info =
        document.getElementById(
            "disk-menu-info"
        );

    if (info) {
        info.textContent =
            storageDeviceTitle(
                device
            )
            + "\nУстройство: "
            + device.device
            + "\nРазмер: "
            + formatBytes(
                device.size_bytes
            )
            + "\nСтатус: "
            + (
                device.mounted
                ? "смонтирован в "
                    + device.mount_point
                : (
                    device.in_use
                    ? "используется системой"
                    : "не смонтирован"
                )
            )
            + (
                device.uuid
                ? "\nUUID: "
                    + device.uuid
                : ""
            )
            + (
                device.serial
                ? "\nSerial: "
                    + device.serial
                : ""
            );

        info.style.whiteSpace =
            "pre-line";
    }

    updateDiskMenuState(
        device
    );

    if (overlay)
        overlay.style.display =
            "flex";
}

async function runDiskAction(action) {
    const device =
        findStorageDevice(
            currentDiskDevice
        );

    if (!device) {
        setDiskMenuResult(
            "Устройство больше не найдено.",
            true
        );

        return;
    }

    const parameters =
        new URLSearchParams();

    parameters.set(
        "action",
        action
    );

    parameters.set(
        "device",
        device.device
    );

    if (
        action === "apply-roles"
    ) {
        const videoRole =
            document.getElementById(
                "disk-role-video"
            );

        const personalRole =
            document.getElementById(
                "disk-role-personal"
            );

        parameters.set(
            "video",
            videoRole &&
            videoRole.checked
            ? "1"
            : "0"
        );

        parameters.set(
            "personal",
            personalRole &&
            personalRole.checked
            ? "1"
            : "0"
        );
    }

    if (
        action === "format-ext4"
        ||
        action === "wipefs"
    ) {
        const confirmation =
            window.prompt(
                "ОПАСНАЯ ОПЕРАЦИЯ. Данные могут быть уничтожены. "
                + "Для подтверждения введите точное имя устройства: "
                + device.device
            );

        if (
            confirmation !==
            device.device
        ) {
            setDiskMenuResult(
                "Операция отменена: подтверждение не совпало.",
                true
            );

            return;
        }

        parameters.set(
            "confirm",
            confirmation
        );
    }

    if (
        action ===
        "format-ext4"
    ) {
        const label =
            window.prompt(
                "Метка EXT4 (латиница, цифры, -, _, .):",
                "homeai-data"
            );

        if (label === null)
            return;

        parameters.set(
            "label",
            label
        );
    }

    setDiskMenuResult(
        "Выполняется операция..."
    );

    try {
        const response =
            await fetch(
                "/api/storage/action",
                {
                    method: "POST",
                    headers: {
                        "Content-Type":
                            "application/x-www-form-urlencoded"
                    },
                    body:
                        parameters.toString()
                }
            );

        if (
            response.status ===
            401
        ) {
            window.location =
                "/login";

            return;
        }

        const data =
            await response.json();

        setDiskMenuResult(
            data.message
            || "Операция завершена.",
            !data.success
        );

        if (data.success) {
            await updateStorageCandidates(
                true
            );

            await updateStorageStats();

            const refreshed =
                findStorageDevice(
                    device.device
                );

            if (refreshed) {
                updateDiskMenuState(
                    refreshed
                );
            }
        }
    }
    catch (error) {
        setDiskMenuResult(
            "Ошибка выполнения операции: "
            + error,
            true
        );
    }
}

function renderStorageCandidate(device) {
    const card =
        document.createElement(
            "div"
        );

    card.className =
        "storage-card";

    const title =
        document.createElement(
            "strong"
        );

    title.textContent =
        storageDeviceTitle(
            device
        );

    card.appendChild(title);

    const state =
        document.createElement(
            "div"
        );

    state.textContent =
        "НОВЫЙ / НЕ СМОНТИРОВАН";

    card.appendChild(state);

    const deviceLine =
        document.createElement(
            "div"
        );

    deviceLine.textContent =
        "Устройство: "
        + device.device;

    card.appendChild(
        deviceLine
    );

    const typeLine =
        document.createElement(
            "div"
        );

    typeLine.textContent =
        "Тип: "
        + (
            device.type ===
                "partition"
            ? "раздел"
            : "диск"
        )
        + (
            device.removable
            ? " · hot-plug/removable"
            : ""
        );

    card.appendChild(
        typeLine
    );

    const capacity =
        document.createElement(
            "div"
        );

    capacity.textContent =
        "Размер: "
        + formatBytes(
            device.size_bytes
        );

    card.appendChild(
        capacity
    );

    if (device.serial) {
        const serial =
            document.createElement(
                "div"
            );

        serial.textContent =
            "Serial: "
            + device.serial;

        card.appendChild(
            serial
        );
    }

    const actions =
        document.createElement(
            "div"
        );

    actions.className =
        "device-actions";

    const manage =
        document.createElement(
            "button"
        );

    manage.type =
        "button";

    manage.textContent =
        "Управление диском";

    manage.addEventListener(
        "click",
        function() {
            openDiskMenu(
                device.device
            );
        }
    );

    actions.appendChild(
        manage
    );

    const ignore =
        document.createElement(
            "button"
        );

    ignore.type =
        "button";

    ignore.className =
        "secondary";

    ignore.textContent =
        "Пока не использовать";

    ignore.addEventListener(
        "click",
        function() {
            ignoredStorageDevices.add(
                device.device
            );

            card.remove();
        }
    );

    actions.appendChild(
        ignore
    );

    card.appendChild(
        actions
    );

    const note =
        document.createElement(
            "div"
        );

    note.className =
        "device-note";

    note.textContent =
        "Откройте управление диском для стандартных операций и выбора назначения.";

    card.appendChild(
        note
    );

    return card;
}

async function updateStorageCandidates(
    manual = false
) {
    const container =
        document.getElementById(
            "storage-candidates"
        );

    if (!container)
        return;

    try {
        const response =
            await fetch(
                "/api/storage/devices",
                {
                    method: "GET",
                    cache: "no-store"
                }
            );

        if (
            response.status ===
            401
        ) {
            window.location =
                "/login";

            return;
        }

        if (!response.ok)
            return;

        const data =
            await response.json();

        storageDeviceInventory =
            Array.isArray(
                data.devices
            )
            ? data.devices
            : [];

        storageHelperInstalled =
            Boolean(
                data.helper_installed
            );

        const candidates =
            storageDeviceInventory.filter(
                function(device) {
                    return (
                        device.candidate
                        &&
                        !ignoredStorageDevices
                            .has(
                                device.device
                            )
                    );
                }
            );

        const currentSet =
            new Set(
                candidates.map(
                    function(device) {
                        return device.device;
                    }
                )
            );

        const added =
            knownStorageCandidates ===
                null
            ? []
            : candidates.filter(
                function(device) {
                    return !knownStorageCandidates
                        .has(
                            device.device
                        );
                }
            );

        knownStorageCandidates =
            currentSet;

        if (added.length > 0) {
            showStorageAlert(
                "Обнаружен новый диск: "
                + added.map(
                    function(device) {
                        return device.device;
                    }
                ).join(", ")
                + "."
            );
        }
        else if (manual) {
            showStorageAlert(
                candidates.length > 0
                ? "Доступных дисков: "
                    + candidates.length
                : "Новых неиспользуемых дисков не обнаружено."
            );
        }

        container.replaceChildren();

        if (
            candidates.length === 0
        ) {
            const empty =
                document.createElement(
                    "div"
                );

            empty.className =
                "storage-card";

            empty.textContent =
                "Новых неиспользуемых дисков нет.";

            container.appendChild(
                empty
            );

            return;
        }

        for (
            const device of
            candidates
        ) {
            container.appendChild(
                renderStorageCandidate(
                    device
                )
            );
        }
    }
    catch (error) {
        console.error(
            "Storage hotplug scan error:",
            error
        );
    }
}

function storagePolicyLabel(policy) {
    if (policy === "sequential")
        return tr("Заполнять диски по очереди");

    if (policy === "balanced")
        return tr("Равномерная загрузка");

    if (policy === "pinned")
        return tr("Закрепление за диском");

    return tr("Больше всего свободного места");
}

function populatePinnedStorageSelects(volumes) {
    const populate =
        function(id, role) {
            const select =
                document.getElementById(
                    id
                );

            if (!select)
                return;

            const selected =
                select.dataset.selected !==
                    undefined
                &&
                select.dataset.selected !==
                    ""
                ? select.dataset.selected
                : (
                    select.value
                    || ""
                );

            const options =
                [
                    {
                        value: "",
                        label:
                            tr("Автоматически")
                    }
                ];

            for (const volume of volumes) {
                const matches =
                    volume.role === role
                    ||
                    volume.role ===
                        "video+personal";

                if (
                    !matches
                    ||
                    !volume.mount_point
                ) {
                    continue;
                }

                options.push(
                    {
                        value:
                            volume.mount_point,
                        label:
                            (
                                volume.uuid
                                ? volume.uuid
                                : (
                                    volume.source
                                    || volume.mount_point
                                )
                            )
                            + " · "
                            + formatBytes(
                                volume.total_bytes
                                || volume.device_size_bytes
                                || 0
                            )
                    }
                );
            }

            select.replaceChildren();

            for (const item of options) {
                const option =
                    document.createElement(
                        "option"
                    );

                option.value =
                    item.value;

                option.textContent =
                    item.label;

                option.dataset.i18nSkip =
                    "";

                if (
                    item.value ===
                    selected
                ) {
                    option.selected =
                        true;
                }

                select.appendChild(
                    option
                );
            }

            // Use the server-rendered value only for the first population.
            // Afterwards preserve any unsaved selection while periodic
            // storage refreshes continue in the background.
            if (
                select.dataset.selected !==
                undefined
            ) {
                select.dataset.selected =
                    "";
            }
        };

    populate(
        "storage-video-pinned",
        "video"
    );

    populate(
        "storage-files-pinned",
        "personal"
    );
}

function renderStoragePools(data) {
    const container =
        document.getElementById(
            "storage-pools"
        );

    if (!container)
        return;

    const volumes =
        Array.isArray(
            data.volumes
        )
        ? data.volumes
        : [];

    populatePinnedStorageSelects(
        volumes
    );

    const renderPool =
        function(
            title,
            role,
            policy,
            target,
            summary
        ) {
            const matching =
                volumes.filter(
                    function(volume) {
                        return (
                            volume.role === role
                            ||
                            volume.role ===
                                "video+personal"
                        );
                    }
                );

            const online =
                matching.filter(
                    function(volume) {
                        return (
                            volume.status ===
                            "online"
                        );
                    }
                );

            const fallbackTotal =
                online.reduce(
                    function(sum, volume) {
                        return (
                            sum
                            +
                            Number(
                                volume.total_bytes
                                ||
                                volume.device_size_bytes
                                ||
                                0
                            )
                        );
                    },
                    0
                );

            const fallbackFree =
                online.reduce(
                    function(sum, volume) {
                        return (
                            sum
                            +
                            (
                                volume.capacity_available
                                ? Number(
                                    volume.free_bytes
                                    || 0
                                )
                                : 0
                            )
                        );
                    },
                    0
                );

            const total =
                summary
                ? Number(
                    summary.total_bytes
                    || 0
                )
                : fallbackTotal;

            const free =
                summary
                ? Number(
                    summary.free_bytes
                    || 0
                )
                : fallbackFree;

            const assignedCount =
                summary
                ? Number(
                    summary.assigned_volumes
                    || 0
                )
                : matching.length;

            const onlineCount =
                summary
                ? Number(
                    summary.online_volumes
                    || 0
                )
                : online.length;

            const freeComplete =
                summary
                ? Boolean(
                    summary.free_bytes_complete
                )
                : online.every(
                    function(volume) {
                        return Boolean(
                            volume.capacity_available
                        );
                    }
                );

            const card =
                document.createElement(
                    "div"
                );

            card.className =
                "storage-card";

            const heading =
                document.createElement(
                    "strong"
                );

            heading.textContent =
                title;

            card.appendChild(
                heading
            );

            const count =
                document.createElement(
                    "div"
                );

            count.textContent =
                tr("Дисков в пуле")
                + ": "
                + assignedCount
                + " · "
                + tr("в сети")
                + ": "
                + onlineCount;

            card.appendChild(
                count
            );

            const policyLine =
                document.createElement(
                    "div"
                );

            policyLine.textContent =
                tr("Стратегия")
                + ": "
                + storagePolicyLabel(
                    policy
                );

            card.appendChild(
                policyLine
            );

            const capacity =
                document.createElement(
                    "div"
                );

            capacity.textContent =
                tr("Общая ёмкость")
                + ": "
                + formatBytes(total)
                + ", "
                + tr("свободно")
                + " "
                + (
                    freeComplete
                    ? formatBytes(free)
                    : (
                        formatBytes(free)
                        + " + "
                        + tr(
                            "данные части дисков недоступны"
                        )
                    )
                );

            card.appendChild(
                capacity
            );

            const targetLine =
                document.createElement(
                    "div"
                );

            targetLine.className =
                target
                ? "status-ok"
                : "status-warn";

            targetLine.textContent =
                tr("Следующая запись")
                + ": "
                + (
                    target
                    || tr(
                        "нет доступного диска"
                    )
                );

            card.appendChild(
                targetLine
            );

            return card;
        };

    container.replaceChildren(
        renderPool(
            tr("Видео-пул"),
            "video",
            data.video_policy,
            data.video_target,
            data.video_summary
        ),
        renderPool(
            tr("Пул домашних файлов"),
            "personal",
            data.files_policy,
            data.files_target,
            data.files_summary
        )
    );
}

async function updateStorageStats() {
    const container =
        document.getElementById(
            "storage-list"
        );

    if (!container)
        return;

    try {
        const response =
            await fetch(
                "/api/storage",
                {
                    method: "GET",
                    cache: "no-store"
                }
            );

        if (
            response.status ===
            401
        ) {
            window.location =
                "/login";

            return;
        }

        if (!response.ok)
            return;

        const data =
            await response.json();

        renderStoragePools(
            data
        );

        container.replaceChildren();

        const volumes =
            Array.isArray(
                data.volumes
            )
            ? data.volumes
            : [];

        storageVolumeInventory =
            volumes;

        if (
            volumes.length === 0
        ) {
            const empty =
                document.createElement(
                    "div"
                );

            empty.className =
                "storage-card";

            empty.textContent =
                "Блочные хранилища не обнаружены.";

            container.appendChild(
                empty
            );

            return;
        }

        for (
            const volume of
            volumes
        ) {
            const card =
                document.createElement(
                    "div"
                );

            card.className =
                "storage-card";

            const title =
                document.createElement(
                    "strong"
                );

            title.textContent =
                storageRoleLabel(
                    volume.role
                );

            card.appendChild(
                title
            );

            const status =
                document.createElement(
                    "div"
                );

            status.textContent =
                "Статус: "
                + (
                    volume.status ===
                        "online"
                    ? "ONLINE"
                    : "OFFLINE"
                )
                + (
                    volume.read_only
                    ? " · READ ONLY"
                    : ""
                );

            card.appendChild(
                status
            );

            const device =
                document.createElement(
                    "div"
                );

            device.textContent =
                "Устройство: "
                + (
                    volume.source
                    || "не смонтировано"
                );

            card.appendChild(
                device
            );

            const mount =
                document.createElement(
                    "div"
                );

            mount.textContent =
                "Точка: "
                + (
                    volume.mount_point
                    || "-"
                );

            card.appendChild(
                mount
            );

            const filesystem =
                document.createElement(
                    "div"
                );

            filesystem.textContent =
                "ФС: "
                + (
                    volume.filesystem
                    || "-"
                );

            card.appendChild(
                filesystem
            );

            if (volume.uuid) {
                const uuid =
                    document.createElement(
                        "div"
                    );

                uuid.textContent =
                    "UUID: "
                    + volume.uuid;

                uuid.dataset.i18nSkip =
                    "";

                card.appendChild(
                    uuid
                );
            }

            const physicalSize =
                document.createElement(
                    "div"
                );

            physicalSize.textContent =
                "Размер диска: "
                + formatBytes(
                    volume.device_size_bytes
                    || volume.total_bytes
                    || 0
                );

            card.appendChild(
                physicalSize
            );

            const capacity =
                document.createElement(
                    "div"
                );

            if (
                volume.status ===
                    "online"
                &&
                volume.capacity_available
            ) {
                capacity.textContent =
                    "Занято: "
                    + formatBytes(
                        volume.used_bytes
                    )
                    + " / "
                    + formatBytes(
                        volume.total_bytes
                    )
                    + " ("
                    + Number(
                        volume.used_percent
                    ).toFixed(1)
                    + "%), свободно "
                    + formatBytes(
                        volume.free_bytes
                    );
            }
            else if (
                volume.status ===
                "online"
            ) {
                capacity.textContent =
                    "Использование ФС: данные временно недоступны";
            }
            else {
                capacity.textContent =
                    "Ёмкость недоступна";
            }

            card.appendChild(
                capacity
            );

            if (volume.source) {
                const actions =
                    document.createElement(
                        "div"
                    );

                actions.className =
                    "device-actions";

                const manage =
                    document.createElement(
                        "button"
                    );

                manage.type =
                    "button";

                manage.textContent =
                    "Управление диском";

                manage.addEventListener(
                    "click",
                    function() {
                        openDiskMenu(
                            volume.source
                        );
                    }
                );

                actions.appendChild(
                    manage
                );

                card.appendChild(
                    actions
                );
            }

            container.appendChild(
                card
            );
        }
    }
    catch (error) {
        console.error(
            "Storage monitor error:",
            error
        );
    }
}

document.addEventListener(
    "DOMContentLoaded",
    function() {
        updateSystemStats();
        updateHomeErrors();
        updateModuleStatus();
        updateStorageStats();
        updateStorageCandidates();
        updateServerUpdateStatus();
        updateNetworkInterfaces();
        updateVpnProfiles();

        const updateCheckButton =
            document.getElementById(
                "update-check-btn"
            );

        if (updateCheckButton) {
            updateCheckButton.addEventListener(
                "click",
                async function() {
                    await postUpdateAction(
                        "/api/update/check"
                    );

                    setTimeout(
                        updateServerUpdateStatus,
                        400
                    );
                }
            );
        }

        const updateApplyButton =
            document.getElementById(
                "update-apply-btn"
            );

        if (updateApplyButton) {
            updateApplyButton.addEventListener(
                "click",
                async function() {
                    const accepted =
                        window.confirm(
                            "Обновить репозиторий с GitHub, собрать новую версию и запустить тесты?"
                        );

                    if (!accepted)
                        return;

                    const parameters =
                        new URLSearchParams();

                    parameters.set(
                        "confirm",
                        "UPDATE"
                    );

                    const result =
                        await postUpdateAction(
                            "/api/update/apply",
                            parameters
                        );

                    if (
                        result
                        &&
                        result.message
                    ) {
                        const message =
                            document.getElementById(
                                "update-message"
                            );

                        if (message) {
                            message.textContent =
                                result.message;
                        }
                    }

                    setTimeout(
                        updateServerUpdateStatus,
                        500
                    );
                }
            );
        }

        const updateRestartButton =
            document.getElementById(
                "update-restart-btn"
            );

        if (updateRestartButton) {
            updateRestartButton.addEventListener(
                "click",
                async function() {
                    const accepted =
                        window.confirm(
                            "Перезапустить Home AI Core и применить новую сборку?"
                        );

                    if (!accepted)
                        return;

                    await postUpdateAction(
                        "/api/update/restart"
                    );

                    const message =
                        document.getElementById(
                            "update-message"
                        );

                    if (message) {
                        message.textContent =
                            "Сервер перезапускается...";
                    }

                    setTimeout(
                        function() {
                            window.location.reload();
                        },
                        3500
                    );
                }
            );
        }

        const vpnRefreshButton =
            document.getElementById(
                "vpn-refresh-btn"
            );

        if (vpnRefreshButton) {
            vpnRefreshButton.addEventListener(
                "click",
                updateVpnProfiles
            );
        }

        const vpnNewButton =
            document.getElementById(
                "vpn-new-btn"
            );

        if (vpnNewButton) {
            vpnNewButton.addEventListener(
                "click",
                newVpnProfile
            );
        }

        const vpnSaveButton =
            document.getElementById(
                "vpn-save-btn"
            );

        if (vpnSaveButton) {
            vpnSaveButton.addEventListener(
                "click",
                saveVpnProfile
            );
        }

        const vpnDeleteButton =
            document.getElementById(
                "vpn-delete-btn"
            );

        if (vpnDeleteButton) {
            vpnDeleteButton.addEventListener(
                "click",
                deleteVpnProfile
            );
        }

        const scanButton =
            document.getElementById(
                "scan-storage-btn"
            );

        if (scanButton) {
            scanButton.addEventListener(
                "click",
                function() {
                    updateStorageStats();

                    updateStorageCandidates(
                        true
                    );
                }
            );
        }

        const actionMap = {
            "disk-apply-roles":
                "apply-roles",
            "disk-unmount":
                "unmount",
            "disk-format-ext4":
                "format-ext4",
            "disk-wipefs":
                "wipefs"
        };

        for (
            const [id, action]
            of Object.entries(
                actionMap
            )
        ) {
            const button =
                document.getElementById(
                    id
                );

            if (button) {
                button.addEventListener(
                    "click",
                    function() {
                        runDiskAction(
                            action
                        );
                    }
                );
            }
        }

        const refreshButton =
            document.getElementById(
                "disk-refresh"
            );

        if (refreshButton) {
            refreshButton.addEventListener(
                "click",
                async function() {
                    await updateStorageCandidates(
                        true
                    );

                    await updateStorageStats();

                    const refreshed =
                        findStorageDevice(
                            currentDiskDevice
                        );

                    if (refreshed) {
                        openDiskMenu(
                            refreshed.device
                        );
                    }
                }
            );
        }

        const closeButton =
            document.getElementById(
                "disk-menu-close"
            );

        if (closeButton) {
            closeButton.addEventListener(
                "click",
                closeDiskMenu
            );
        }

        const overlay =
            document.getElementById(
                "disk-menu-overlay"
            );

        if (overlay) {
            overlay.addEventListener(
                "click",
                function(event) {
                    if (
                        event.target ===
                        overlay
                    ) {
                        closeDiskMenu();
                    }
                }
            );
        }

        setInterval(
            updateSystemStats,
            2000
        );

        setInterval(
            updateHomeErrors,
            5000
        );

        setInterval(
            updateModuleStatus,
            5000
        );

        setInterval(
            updateServerUpdateStatus,
            1000
        );

        setInterval(
            updateNetworkInterfaces,
            5000
        );

        setInterval(
            updateVpnProfiles,
            10000
        );

        setInterval(
            updateStorageStats,
            5000
        );

        setInterval(
            updateStorageCandidates,
            3000
        );
    }
);
</script>

<footer style="padding:16px;text-align:center">Copyright © TexNik</footer>
</body>
</html>
)HTML";

    return page.str();
}

}
