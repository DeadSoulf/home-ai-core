#include "core/modules/Module.h"
#include "core/modules/ModuleManager.h"

#include <iostream>
#include <memory>
#include <string>
#include <vector>

int main()
{
    homeai::ModuleManager manager;

    std::vector<std::string>
        events;

    std::string error;

    auto make_module =
        [&](const std::string& name,
            std::vector<std::string> dependencies) {
            return std::make_unique<
                homeai::CallbackModule
            >(
                name,
                std::move(dependencies),
                [&, name](std::string&) {
                    events.push_back(
                        "init:" + name
                    );

                    return true;
                },
                [&, name](std::string&) {
                    events.push_back(
                        "start:" + name
                    );

                    return true;
                },
                [&, name]() {
                    events.push_back(
                        "stop:" + name
                    );
                },
                []() {
                    return
                        homeai::ModuleHealth::
                            Healthy;
                },
                [name]() {
                    return
                        name + " healthy";
                }
            );
        };

    if (
        !manager.registerModule(
            make_module(
                "database",
                {}
            ),
            error
        )
        ||
        !manager.registerModule(
            make_module(
                "api",
                {"database"}
            ),
            error
        )
        ||
        !manager.registerModule(
            make_module(
                "web",
                {"api"}
            ),
            error
        )
    ) {
        std::cerr
            << "Registration failed: "
            << error
            << '\n';

        return 1;
    }

    if (
        !manager.initializeAll(
            error
        )
    ) {
        std::cerr
            << "Initialization failed: "
            << error
            << '\n';

        return 1;
    }

    if (
        !manager.startAll(
            error
        )
    ) {
        std::cerr
            << "Start failed: "
            << error
            << '\n';

        return 1;
    }

    const std::vector<std::string>
        expected_start = {
            "init:database",
            "init:api",
            "init:web",
            "start:database",
            "start:api",
            "start:web"
        };

    if (events != expected_start) {
        std::cerr
            << "Dependency order is invalid\n";

        return 1;
    }

    const auto status =
        manager.snapshot();

    if (status.size() != 3) {
        std::cerr
            << "Module snapshot size is invalid\n";

        return 1;
    }

    for (const auto& module : status) {
        if (
            module.state !=
                homeai::ModuleState::Running
            ||
            module.health !=
                homeai::ModuleHealth::Healthy
        ) {
            std::cerr
                << "Running module status is invalid\n";

            return 1;
        }
    }

    manager.stopAll();

    const std::vector<std::string>
        expected_all = {
            "init:database",
            "init:api",
            "init:web",
            "start:database",
            "start:api",
            "start:web",
            "stop:web",
            "stop:api",
            "stop:database"
        };

    if (events != expected_all) {
        std::cerr
            << "Shutdown order is invalid\n";

        return 1;
    }

    homeai::ModuleManager broken;

    if (
        !broken.registerModule(
            std::make_unique<
                homeai::CallbackModule
            >(
                "broken",
                std::vector<std::string>{
                    "missing"
                },
                [](std::string&) {
                    return true;
                },
                [](std::string&) {
                    return true;
                },
                []() {},
                []() {
                    return
                        homeai::ModuleHealth::
                            Healthy;
                }
            ),
            error
        )
    ) {
        std::cerr
            << "Broken module registration unexpectedly failed\n";

        return 1;
    }

    error.clear();

    if (
        broken.initializeAll(
            error
        )
        ||
        error.find("missing") ==
            std::string::npos
    ) {
        std::cerr
            << "Missing dependency was not detected\n";

        return 1;
    }

    std::cout
        << "Module Manager test passed\n";

    return 0;
}
