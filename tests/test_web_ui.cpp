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

    context.version =
        "0.0.6";

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
    const auto viewer = homeai::renderWebUi(context);
    if (viewer.find("id=\"gpu-list\"") != std::string::npos || viewer.find("href=\"/admin\"") != std::string::npos) return 1;

    return 0;
}
