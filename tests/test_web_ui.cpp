#include "web/ui/WebUi.h"

#include <iostream>
#include <string>

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
    ) {
        std::cerr
            << "Update controls are missing from System page\n";

        return 1;
    }

    std::cout
        << "Web UI test passed\n";

    return 0;
}
