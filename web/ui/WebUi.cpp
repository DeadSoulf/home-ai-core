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

std::string pageTitle(
    const std::string& page
)
{
    if (page == "/")
        return "Главная";

    if (page == "/network")
        return "Сеть";

    if (page == "/storage")
        return "Диски";

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
        return "Администрирование";

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
<meta name="viewport" content="width=device-width, initial-scale=1">
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

body {
    min-height: 100vh;
}

a {
    color: inherit;
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
    z-index: 100;
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

.topbar h1 {
    margin: 0;
    font-size: 1.35rem;
}

.topbar-meta {
    color: var(--muted);
    font-size: 0.9rem;
}

.mobile-menu-button {
    display: none;
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
select {
    width: 100%;
    padding: 10px 11px;
    border-radius: 8px;
    border: 1px solid #363d49;
    background: #0f1217;
    color: white;
}

button {
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

@media (max-width: 860px) {
    .sidebar {
        position: static;
        width: 100%;
        border-right: 0;
        border-bottom: 1px solid var(--border);
    }

    .content {
        margin-left: 0;
    }

    .nav-group {
        display: grid;
        grid-template-columns:
            repeat(auto-fit, minmax(150px, 1fr));
        gap: 4px;
    }

    .nav-caption {
        grid-column: 1 / -1;
    }

    .sidebar-user {
        display: flex;
        align-items: center;
        justify-content: space-between;
        gap: 12px;
    }

    .sidebar-user form {
        margin: 0;
    }

    .topbar {
        position: static;
        padding: 14px 18px;
    }

    .page {
        padding: 18px;
    }
}
</style>
</head>
<body>
<div class="app-shell">

<aside class="sidebar">
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

    page << "<div class=\"nav-group\">";
    page << "<div class=\"nav-caption\">Обзор</div>";

    if (
        uiHasPermission(
            context,
            "system.view"
        )
    ) {
        navLink(page, context, "/", "Главная", "⌂");
        navLink(page, context, "/system", "Система", "▣");
    }

    if (
        uiHasPermission(
            context,
            "network.view"
        )
    ) {
        navLink(page, context, "/network", "Сеть", "⇄");
    }

    if (
        uiHasPermission(
            context,
            "storage.view"
        )
    ) {
        navLink(page, context, "/storage", "Диски", "◫");
    }

    if (
        uiHasPermission(
            context,
            "files.read"
        )
    ) {
        navLink(page, context, "/files", "Файлы", "▱");
    }

    page << "</div>";

    page << "<div class=\"nav-group\">";
    page << "<div class=\"nav-caption\">Дом и сервисы</div>";

    if (
        uiHasPermission(
            context,
            "cameras.view"
        )
    ) {
        navLink(page, context, "/cameras", "Камеры", "◉");
    }

    if (
        uiHasPermission(
            context,
            "smart_home.view"
        )
    ) {
        navLink(page, context, "/smart-home", "Умный дом", "⌁");
    }

    if (
        uiHasPermission(
            context,
            "automation.view"
        )
    ) {
        navLink(page, context, "/automation", "Автоматизация", "⚙");
    }

    page << "</div>";

    page << "<div class=\"nav-group\">";
    page << "<div class=\"nav-caption\">Интеллект</div>";

    if (
        uiHasPermission(
            context,
            "ai.use"
        )
    ) {
        navLink(page, context, "/ai", "AI", "✦");
    }

    page << "</div>";

    page << "<div class=\"nav-group\">";
    page << "<div class=\"nav-caption\">Администрирование</div>";

    if (
        uiHasPermission(
            context,
            "ai.manage"
        )
    ) {
        navLink(page, context, "/admin", "Администрирование", "⚒");
    }

    if (
        uiHasPermission(
            context,
            "users.view"
        )
    ) {
        navLink(page, context, "/users", "Пользователи", "♙");
    }

    if (
        uiHasPermission(
            context,
            "hypervisor.view"
        )
    ) {
        navLink(page, context, "/hypervisor", "Виртуализация", "▤");
    }

    if (
        uiHasPermission(
            context,
            "system.manage"
        )
    ) {
        navLink(page, context, "/settings", "Настройки", "⚙");
    }

    page << "</div></nav>";

    page
        << "<div class=\"sidebar-user\">"
        << "<div><strong>"
        << "<span data-i18n-skip>" << htmlEscape(context.username) << "</span>"
        << "</strong><small>"
        << htmlEscape(
            context.role
        )
        << "</small></div>"
        << "<form method=\"POST\" action=\"/logout\">"
        << "<button type=\"submit\" class=\"secondary\">Выйти</button>"
        << "</form></div>";

    page << R"HTML(
</aside>

<div class="content">
<header class="topbar">
<div>
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
        << "</div></div>"
        << "<div class=\"topbar-meta\">"
        << "<a id=\"update-badge\" href=\"/system\" "
           "style=\"display:none;margin-right:14px;color:#f2d784;text-decoration:none;font-weight:700\">"
           "Доступно обновление</a>"
        << "<span data-i18n-skip>" << htmlEscape(context.username) << "</span>"
        << " · "
        << htmlEscape(
            context.role
        )
        << "</div>"
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

<pre id="update-output"
style="display:none;white-space:pre-wrap;background:#0f1217;padding:12px;border-radius:8px;overflow:auto"></pre>
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

        renderPlaceholder(
            page,
            "Network Core",
            "Здесь будет управление интерфейсами, адресами, маршрутами, DNS и диагностикой сети.",
            "<div class=\"placeholder-card\">Интерфейсы — NEXT</div>"
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

<h3>Подключённые хранилища</h3>
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
<h2>Настройки хранилищ</h2>
<form method="POST" action="/api/config">
<input type="hidden" name="return_to" value="/storage">

<div class="form-grid">
<div>
<label>Диски для видео</label>
<input
    name="storage.video_mounts"
    placeholder="/mnt/home-ai/video/sdb1"
    value=")HTML";

            page
                << htmlEscape(
                    context.storage_video_mounts
                )
                << R"HTML(">
</div>

<div>
<label>Диски для личных файлов</label>
<input
    name="storage.personal_mounts"
    placeholder="/mnt/home-ai/files/sdc1"
    value=")HTML";

            page
                << htmlEscape(
                    context.storage_personal_mounts
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
<h4>Назначение и подключение</h4>
<div class="disk-menu-actions">
<button id="disk-mount-video" type="button">
Подключить для видео
</button>
<button id="disk-mount-personal" type="button">
Подключить для личных файлов
</button>
<button id="disk-assign-video" type="button" class="secondary">
Назначить как видео
</button>
<button id="disk-assign-personal" type="button" class="secondary">
Назначить как личные файлы
</button>
<button id="disk-unassign" type="button" class="secondary">
Снять назначение
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
Размонтировать
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
Физические диски для личных данных назначаются в разделе «Диски».
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
        renderPlaceholder(
            page,
            "Камеры",
            "Все функции видеонаблюдения будут собраны в этом разделе.",
            "<div class=\"placeholder-card\">Камеры RTSP / ONVIF — PLANNED</div>"
            "<div class=\"placeholder-card\">Live View — PLANNED</div>"
            "<div class=\"placeholder-card\">Архив — PLANNED</div>"
            "<div class=\"placeholder-card\">Аналитика — PLANNED</div>"
        );
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
        page << R"HTML(
<div class="section-card">
<h2>Текущий пользователь</h2>
<div class="kv">
<div>Имя</div><div>)HTML";

        page
            << "<span data-i18n-skip>" << htmlEscape(context.username) << "</span>"
            << "</div><div>Роль</div><div>"
            << htmlEscape(
                context.role
            )
            << R"HTML(</div>
<div>Сессия</div><div class="status-ok">ACTIVE</div>
</div>
</div>
)HTML";

        renderPlaceholder(
            page,
            "Управление пользователями",
            "Создание пользователей, изменение ролей, сброс паролей и активные сессии будут находиться здесь.",
            "<div class=\"placeholder-card\">Список пользователей — NEXT</div>"
            "<div class=\"placeholder-card\">Роли и права — NEXT</div>"
            "<div class=\"placeholder-card\">Сессии — NEXT</div>"
            "<div class=\"placeholder-card\">Audit — NEXT</div>"
        );
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
                "Профили не найдены. Добавьте *.conf в runtime/wireguard/";

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
                                            "application/x-www-form-urlencoded"
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

            card.appendChild(
                actions
            );

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

    if (warning) {
        warning.textContent =
            storageHelperInstalled
            ? ""
            : "Для монтирования, форматирования и размонтирования нужно установить привилегированный Storage Helper.";
    }

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

    setDisabled(
        "disk-mount-video",
        !candidate ||
        !canPrivileged
    );

    setDisabled(
        "disk-mount-personal",
        !candidate ||
        !canPrivileged
    );

    setDisabled(
        "disk-assign-video",
        !mounted
    );

    setDisabled(
        "disk-assign-personal",
        !mounted
    );

    setDisabled(
        "disk-unassign",
        !mounted
    );

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
        );

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

        container.replaceChildren();

        const volumes =
            Array.isArray(
                data.volumes
            )
            ? data.volumes
            : [];

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

            const capacity =
                document.createElement(
                    "div"
                );

            if (
                volume.status ===
                "online"
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
            "disk-mount-video":
                "mount-video",
            "disk-mount-personal":
                "mount-personal",
            "disk-assign-video":
                "assign-video",
            "disk-assign-personal":
                "assign-personal",
            "disk-unassign":
                "unassign",
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
            15000
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
