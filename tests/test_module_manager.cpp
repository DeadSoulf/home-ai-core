#include "core/modules/Module.h"
#include "core/modules/ModuleManager.h"

#include <chrono>
#include <iostream>
#include <memory>
#include <string>
#include <vector>

int main()
{
    homeai::ModuleManager manager;
    std::vector<std::string> events;
    std::string error;

    if (
        !manager.registerManifest(
            {
                "test-stack",
                "Test Stack",
                "1.0.0",
                "Module catalog test entry.",
                {},
                {"system.view"},
                {"database", "api", "web"},
                false,
                true,
                false
            },
            error
        )
    ) {
        std::cerr
            << "Manifest registration failed: "
            << error << '\n';
        return 1;
    }

    std::string duplicate_error;

    if (
        manager.registerManifest(
            {
                "test-stack",
                "Duplicate",
                "1.0.0"
            },
            duplicate_error
        )
        ||
        duplicate_error.empty()
    ) {
        std::cerr
            << "Duplicate manifest was accepted\n";
        return 1;
    }

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
            make_module("database", {}),
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
            << error << '\n';
        return 1;
    }

    if (!manager.initializeAll(error)) {
        std::cerr
            << "Initialization failed: "
            << error << '\n';
        return 1;
    }

    if (!manager.startAll(error)) {
        std::cerr
            << "Start failed: "
            << error << '\n';
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

    const auto catalog =
        manager.catalogSnapshot();

    if (
        catalog.size() != 1
        ||
        catalog.front().manifest.id !=
            "test-stack"
        ||
        !catalog.front().installed
        ||
        !catalog.front().running
        ||
        catalog.front().state !=
            "running"
    ) {
        std::cerr
            << "Module catalog snapshot is invalid\n";
        return 1;
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

    homeai::ModuleManager recovery;
    std::vector<std::string>
        recovery_events;

    homeai::ModuleHealth
        dependency_health =
            homeai::ModuleHealth::
                Healthy;

    int dependency_starts = 0;

    if (
        !recovery.registerModule(
            std::make_unique<
                homeai::CallbackModule
            >(
                "dependency",
                std::vector<std::string>{},
                [](std::string&) {
                    return true;
                },
                [&](std::string&) {
                    ++dependency_starts;
                    recovery_events.push_back(
                        "start:dependency"
                    );

                    if (dependency_starts > 1) {
                        dependency_health =
                            homeai::ModuleHealth::
                                Healthy;
                    }

                    return true;
                },
                [&]() {
                    recovery_events.push_back(
                        "stop:dependency"
                    );
                },
                [&]() {
                    return dependency_health;
                }
            ),
            error
        )
        ||
        !recovery.registerModule(
            std::make_unique<
                homeai::CallbackModule
            >(
                "dependent",
                std::vector<std::string>{
                    "dependency"
                },
                [](std::string&) {
                    return true;
                },
                [&](std::string&) {
                    recovery_events.push_back(
                        "start:dependent"
                    );
                    return true;
                },
                [&]() {
                    recovery_events.push_back(
                        "stop:dependent"
                    );
                },
                []() {
                    return
                        homeai::ModuleHealth::
                            Healthy;
                }
            ),
            error
        )
        ||
        !recovery.initializeAll(error)
        ||
        !recovery.startAll(error)
    ) {
        std::cerr
            << "Recovery setup failed: "
            << error << '\n';
        return 1;
    }

    recovery_events.clear();

    dependency_health =
        homeai::ModuleHealth::
            Unhealthy;

    const auto recovered =
        recovery.watchdogPass(
            2,
            std::chrono::seconds(0)
        );

    const std::vector<std::string>
        expected_recovery = {
            "stop:dependent",
            "stop:dependency",
            "start:dependency",
            "start:dependent"
        };

    if (
        recovered != 1
        ||
        recovery_events !=
            expected_recovery
    ) {
        std::cerr
            << "Watchdog restart order is invalid\n";
        return 1;
    }

    const auto recovery_status =
        recovery.snapshot();

    bool dependency_restarted = false;
    bool dependent_restarted = false;

    for (
        const auto& module :
        recovery_status
    ) {
        if (
            module.name == "dependency"
            &&
            module.restart_count == 1
            &&
            module.state ==
                homeai::ModuleState::Running
            &&
            module.health ==
                homeai::ModuleHealth::Healthy
        ) {
            dependency_restarted = true;
        }

        if (
            module.name == "dependent"
            &&
            module.restart_count == 1
            &&
            module.state ==
                homeai::ModuleState::Running
        ) {
            dependent_restarted = true;
        }
    }

    if (
        !dependency_restarted
        ||
        !dependent_restarted
    ) {
        std::cerr
            << "Restart counters/status are invalid\n";
        return 1;
    }

    recovery.stopAll();

    homeai::ModuleManager stuck;

    if (
        !stuck.registerModule(
            std::make_unique<
                homeai::CallbackModule
            >(
                "stuck",
                std::vector<std::string>{},
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
                            Unhealthy;
                }
            ),
            error
        )
        ||
        !stuck.initializeAll(error)
        ||
        !stuck.startAll(error)
    ) {
        std::cerr
            << "Stuck module setup failed: "
            << error << '\n';
        return 1;
    }

    if (
        stuck.watchdogPass(
            1,
            std::chrono::seconds(0)
        ) != 1
        ||
        stuck.watchdogPass(
            1,
            std::chrono::seconds(0)
        ) != 0
    ) {
        std::cerr
            << "Watchdog retry limit is invalid\n";
        return 1;
    }

    stuck.stopAll();

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
        broken.initializeAll(error)
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
