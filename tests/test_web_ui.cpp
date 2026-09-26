#include "web/ui/WebUi.h"

#include <iostream>
#include <string>
#include <filesystem>
#include <fstream>

int main()
{
    homeai::WebUiContext context;

    context.core_name =
        "Home AI Core";

#ifdef HOMEAI_VERSION
    context.version =
        HOMEAI_VERSION;
#else
    context.version =
        "dev";
#endif

    context.log_level =
        "debug";

    context.tick_ms =
        "250";

    context.web_bind =
        "0.0.0.0";

    context.web_port =
        "8080";

    context.username =
        "admin";

    context.role =
        "admin";

    context.admin =
        true;

    context.permissions = {
        "files.read",
        "files.write",
        "files.manage",
        "cameras.view",
        "cameras.manage",
        "smart_home.view",
        "smart_home.control",
        "smart_home.manage",
        "ai.use",
        "ai.manage",
        "users.view",
        "users.manage",
        "system.view",
        "system.manage",
        "storage.view",
        "storage.manage",
        "network.view",
        "network.manage",
        "hypervisor.view",
        "hypervisor.manage",
        "automation.view",
        "automation.manage"
    };

    context.page = "/";

    const auto home =
        homeai::renderWebUi(
            context
        );

    const auto overview_position =
        home.find(
            "<div class=\"nav-caption\">Обзор</div>"
        );

    const auto server_position =
        home.find(
            "<div class=\"nav-caption\">Сервер</div>"
        );

    const auto services_position =
        home.find(
            "<div class=\"nav-caption\">Сервисы</div>"
        );

    const auto ai_position =
        home.find(
            "<div class=\"nav-caption\">AI</div>"
        );

    const auto management_position =
        home.find(
            "<div class=\"nav-caption\">Управление</div>"
        );

    if (
        home.find("Главная") ==
            std::string::npos
        ||
        home.find(
            "Ошибки и предупреждения"
        ) == std::string::npos
        ||
        home.find(
            "Диски и хранилища"
        ) != std::string::npos
        ||
        overview_position ==
            std::string::npos
        ||
        server_position ==
            std::string::npos
        ||
        services_position ==
            std::string::npos
        ||
        ai_position ==
            std::string::npos
        ||
        management_position ==
            std::string::npos
        ||
        !(
            overview_position <
                server_position
            &&
            server_position <
                services_position
            &&
            services_position <
                ai_position
            &&
            ai_position <
                management_position
        )
        ||
        home.find(
            "<span>Хранилище</span>"
        ) == std::string::npos
        ||
        home.find(
            "<span>AI / GPU</span>"
        ) == std::string::npos
        ||
        home.find(
            "<div class=\"nav-caption\">Дом и сервисы</div>"
        ) != std::string::npos
        ||
        home.find(
            "<div class=\"nav-caption\">Интеллект</div>"
        ) != std::string::npos
        ||
        home.find(
            "name=\"viewport\" content=\"width=device-width, initial-scale=1, viewport-fit=cover\""
        ) == std::string::npos
        ||
        home.find(
            "id=\"mobile-menu-button\""
        ) == std::string::npos
        ||
        home.find(
            "id=\"mobile-menu-backdrop\""
        ) == std::string::npos
        ||
        home.find(
            "id=\"sidebar-navigation\""
        ) == std::string::npos
        ||
        home.find(
            "body.mobile-menu-open .sidebar"
        ) == std::string::npos
        ||
        home.find(
            "@media (max-width: 600px)"
        ) == std::string::npos
        ||
        home.find(
            "font-size: 16px;"
        ) == std::string::npos
        ||
        home.find(
            "min-height: 48px;"
        ) == std::string::npos
        ||
        home.find(
            "@media (max-width: 420px)"
        ) == std::string::npos
        ||
        home.find(
            "viewport-fit=cover"
        ) == std::string::npos
        ||
        home.find(
            "overscroll-behavior-y: none"
        ) == std::string::npos
        ||
        home.find(
            "touch-action: manipulation"
        ) == std::string::npos
        ||
        home.find(
            "setMobileMenuOpen"
        ) == std::string::npos
        ||
        home.find(
            "sidebar.inert"
        ) == std::string::npos
        ||
        home.find(
            "dashboard-shortcuts"
        ) == std::string::npos
        ||
        home.find(
            "mobile-bottom-nav"
        ) == std::string::npos
        ||
        home.find(
            "sidebar-collapse-button"
        ) == std::string::npos
        ||
        home.find(
            "sidebar-collapsed"
        ) == std::string::npos
        ||
        home.find(
            "home-ai.sidebar-collapsed"
        ) == std::string::npos
        ||
        home.find(
            "Home AI Cloud dashboard redesign 0.0.23"
        ) == std::string::npos
    ) {
        std::cerr
            << "Home page structure is invalid\n";

        return 1;
    }

    context.page =
        "/storage";

    const auto storage =
        homeai::renderWebUi(
            context
        );

    if (
        storage.find(
            "Диски и хранилища"
        ) == std::string::npos
        ||
        storage.find(
            "Управление диском"
        ) == std::string::npos
        ||
        storage.find(
            "Пулы хранения"
        ) == std::string::npos
        ||
        storage.find(
            "Применить назначение"
        ) == std::string::npos
        ||
        storage.find(
            "Использовать для видео"
        ) == std::string::npos
        ||
        storage.find(
            "storage-pools"
        ) == std::string::npos
        ||
        storage.find(
            "Проверить новые диски"
        ) == std::string::npos
    ) {
        std::cerr
            << "Storage page structure is invalid\n";

        return 1;
    }

    context.page =
        "/network";

    const auto network =
        homeai::renderWebUi(
            context
        );

    if (
        network.find(
            "Настройки сети"
        ) == std::string::npos
        ||
        network.find(
            "name=\"return_to\" value=\"/network\""
        ) == std::string::npos
        ||
        network.find(
            "Сетевые интерфейсы"
        ) == std::string::npos
        ||
        network.find(
            "network-interface-list"
        ) == std::string::npos
        ||
        network.find(
            "/api/network/ipv4"
        ) == std::string::npos
        ||
        network.find(
            "Режим IPv4"
        ) == std::string::npos
        ||
        network.find(
            "Статический IP"
        ) == std::string::npos
        ||
        network.find(
            "Маска сети"
        ) == std::string::npos
        ||
        network.find(
            "Основной DNS"
        ) == std::string::npos
        ||
        network.find(
            "Редактор WireGuard"
        ) == std::string::npos
        ||
        network.find(
            "vpn-profile-config"
        ) == std::string::npos
        ||
        network.find(
            "/api/network/vpn/profile"
        ) == std::string::npos
    ) {
        std::cerr
            << "Network page structure is invalid\n";

        return 1;
    }

    context.page =
        "/cameras";

    const auto cameras =
        homeai::renderWebUi(
            context
        );

    if (
        cameras.find(
            "id=\"camera-root\""
        ) == std::string::npos
        ||
        cameras.find(
            "id=\"camera-list\""
        ) == std::string::npos
        ||
        cameras.find(
            "id=\"camera-save-btn\""
        ) == std::string::npos
        ||
        cameras.find(
            "id=\"camera-clear-password\""
        ) == std::string::npos
        ||
        cameras.find(
            "/api/cameras/save"
        ) == std::string::npos
        ||
        cameras.find(
            "/api/cameras/delete"
        ) == std::string::npos
        ||
        cameras.find(
            "/api/cameras/probe"
        ) == std::string::npos
        ||
        cameras.find(
            "/api/cameras/media-probe"
        ) == std::string::npos
        ||
        cameras.find(
            "/api/cameras/snapshot"
        ) == std::string::npos
        ||
        cameras.find(
            "/api/cameras/discover"
        ) == std::string::npos
        ||
        cameras.find(
            "/api/cameras/onvif-streams"
        ) == std::string::npos
        ||
        cameras.find(
            "id=\"camera-auto-stream-btn\""
        ) == std::string::npos
        ||
        cameras.find(
            "id=\"camera-profile-list\""
        ) == std::string::npos
        ||
        cameras.find(
            "id=\"camera-device-info\""
        ) == std::string::npos
        ||
        cameras.find(
            "id=\"camera-manufacturer\""
        ) == std::string::npos
        ||
        cameras.find(
            "id=\"camera-ptz-xaddr\""
        ) == std::string::npos
        ||
        cameras.find(
            "/api/cameras/ptz"
        ) == std::string::npos
        ||
        cameras.find(
            "startCameraLive"
        ) == std::string::npos
        ||
        cameras.find(
            "createPtzControls"
        ) == std::string::npos
        ||
        cameras.find(
            "Версия прошивки"
        ) == std::string::npos
        ||
        cameras.find(
            "id=\"camera-onvif-xaddr\""
        ) == std::string::npos
        ||
        cameras.find(
            "id=\"camera-discover-btn\""
        ) == std::string::npos
        ||
        cameras.find(
            "class=\"camera-discovery-list\""
        ) == std::string::npos
        ||
        cameras.find(
            "camera-discovery-row"
        ) == std::string::npos
        ||
        cameras.find(
            "Найти камеры"
        ) == std::string::npos
        ||
        cameras.find(
            "выключенным ONVIF"
        ) == std::string::npos
        ||
        cameras.find(
            "current.suggested_rtsp_url"
        ) == std::string::npos
        ||
        cameras.find(
            "current.sadp"
        ) == std::string::npos
        ||
        cameras.find(
            "current.model_hint"
        ) == std::string::npos
        ||
        cameras.find(
            "camera-discovery-meta"
        ) == std::string::npos
        ||
        cameras.find(
            "device.firmware_hint"
        ) == std::string::npos
        ||
        cameras.find(
            "device.serial_hint"
        ) == std::string::npos
        ||
        cameras.find(
            "Прошивка"
        ) == std::string::npos
        ||
        cameras.find(
            "Hikvision найдена через SADP"
        ) == std::string::npos
        ||
        cameras.find(
            "device.scopes"
        ) != std::string::npos
        ||
        cameras.find(
            "Найденный XAddr"
        ) != std::string::npos
        ||
        cameras.find(
            "Camera Core 0.0.23"
        ) == std::string::npos
        ||
        cameras.find(
            "Камеры RTSP / ONVIF — PLANNED"
        ) != std::string::npos
    ) {
        std::cerr
            << "Camera page structure is invalid\n";

        return 1;
    }

    const std::string routes[] = {
        "/",
        "/system",
        "/network",
        "/storage",
        "/cameras",
        "/smart-home",
        "/automation",
        "/ai",
        "/users",
        "/hypervisor",
        "/settings"
        ,"/files", "/admin"
    };

    for (const auto& route : routes) {
        if (
            !homeai::isWebUiPath(
                route
            )
        ) {
            std::cerr
                << "Sidebar route is not registered: "
                << route
                << '\n';

            return 1;
        }
    }

    if (
        homeai::isWebUiPath(
            "/missing"
        )
    ) {
        std::cerr
            << "Unknown Web UI route was accepted\n";

        return 1;
    }

    context.page =
        "/users";

    const auto users =
        homeai::renderWebUi(
            context
        );

    if (
        users.find(
            "Создать пользователя"
        ) == std::string::npos
        ||
        users.find(
            "users-list"
        ) == std::string::npos
        ||
        users.find(
            "sessions-list"
        ) == std::string::npos
        ||
        users.find(
            "audit-list"
        ) == std::string::npos
    ) {
        std::cerr
            << "Users administration page is incomplete\n";

        return 1;
    }

    context.page =
        "/system";

    const auto system =
        homeai::renderWebUi(
            context
        );

    if (
        system.find(
            "Модули ядра"
        ) == std::string::npos
        ||
        system.find(
            "module-list"
        ) == std::string::npos
        ||
        system.find(
            "Обновление сервера"
        ) == std::string::npos
        ||
        system.find(
            "update-check-btn"
        ) == std::string::npos
        ||
        system.find(
            "update-apply-btn"
        ) == std::string::npos
        ||
        system.find(
            "update-progress-bar"
        ) == std::string::npos
        ||
        system.find(
            "data-update-stage=\"build\""
        ) == std::string::npos
    ) {
        std::cerr
            << "Update controls are missing from System page\n";

        return 1;
    }

    std::cout
        << "Web UI test passed\n";

    std::filesystem::create_directories("ui-fixtures");
    for (const auto& route : routes) {
        context.page = route;
        const auto html = homeai::renderWebUi(context);
        if (html.find("/assets/i18n.js") == std::string::npos || html.find("Copyright © TexNik") == std::string::npos) return 1;
        std::ofstream("ui-fixtures/" + (route == "/" ? std::string("home") : route.substr(1)) + ".html") << html;
    }
    context.page = "/admin";
    if (homeai::renderWebUi(context).find("gpu-clear") == std::string::npos) return 1;
    context.admin = false;
    context.permissions = {
        "system.view",
        "files.read",
        "cameras.view",
        "smart_home.view",
        "ai.use",
        "storage.view",
        "network.view",
        "automation.view"
    };
    const auto viewer =
        homeai::renderWebUi(
            context
        );

    if (
        viewer.find(
            "id=\"gpu-list\""
        ) != std::string::npos
        ||
        viewer.find(
            "href=\"/admin\""
        ) != std::string::npos
        ||
        viewer.find(
            "<div class=\"nav-caption\">Управление</div>"
        ) != std::string::npos
    ) {
        return 1;
    }

    context.page =
        "/network";

    context.permissions = {
        "network.view"
    };

    const auto network_only =
        homeai::renderWebUi(
            context
        );

    if (
        network_only.find(
            "<div class=\"nav-caption\">Сервер</div>"
        ) == std::string::npos
        ||
        network_only.find(
            "href=\"/network\""
        ) == std::string::npos
        ||
        network_only.find(
            "<div class=\"nav-caption\">Обзор</div>"
        ) != std::string::npos
        ||
        network_only.find(
            "<div class=\"nav-caption\">Сервисы</div>"
        ) != std::string::npos
        ||
        network_only.find(
            "<div class=\"nav-caption\">AI</div>"
        ) != std::string::npos
        ||
        network_only.find(
            "<div class=\"nav-caption\">Управление</div>"
        ) != std::string::npos
    ) {
        std::cerr
            << "Permission-aware sidebar grouping is invalid\n";

        return 1;
    }

    return 0;
}
