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

    std::cout
        << "Web UI test passed\n";

    return 0;
}
