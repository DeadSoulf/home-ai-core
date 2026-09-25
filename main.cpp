#include "core/logging/Logger.h"
#include "core/modules/Module.h"
#include "core/modules/ModuleManager.h"
#include "core/runtime/CoreRuntime.h"
#include "security/auth/SecurityManager.h"
#include "server/storage/DiskOperations.h"
#include "server/storage/StorageMonitor.h"
#include "server/system/SystemMonitor.h"
#include "server/update/UpdateManager.h"
#include "web/server/WebServer.h"

#include <atomic>
#include <chrono>
#include <csignal>
#include <cstdint>
#include <filesystem>
#include <memory>
#include <string>
#include <system_error>
#include <thread>
#include <unistd.h>
#include <vector>

static std::atomic<bool>
    stop_requested{false};

static void signal_handler(int)
{
    stop_requested = true;
}

int main()
{
    std::signal(
        SIGINT,
        signal_handler
    );

    std::signal(
        SIGTERM,
        signal_handler
    );

    const std::filesystem::path
        runtime_config =
            "runtime/home-ai.conf";

    const std::filesystem::path
        template_config =
            "config/home-ai.conf";

    std::error_code config_error;

    std::filesystem::create_directories(
        runtime_config.parent_path(),
        config_error
    );

    if (
        !std::filesystem::exists(
            runtime_config
        )
    ) {
        config_error.clear();

        std::filesystem::copy_file(
            template_config,
            runtime_config,
            std::filesystem::copy_options::
                overwrite_existing,
            config_error
        );

        if (config_error) {
            homeai::Logger::instance().error(
                "Unable to create runtime configuration: "
                + config_error.message()
            );

            return 1;
        }
    }

    homeai::CoreRuntime runtime;

    if (
        !runtime.initialize(
            runtime_config.string()
        )
    ) {
        homeai::Logger::instance().error(
            "Core initialization failed"
        );

        return 1;
    }

#ifdef HOMEAI_VERSION
    runtime.config().set(
        "core.version",
        HOMEAI_VERSION
    );
#endif

    const auto security_database =
        runtime.config().get(
            "security.database_file",
            "runtime/security/security.db"
        );

    const auto users_file =
        runtime.config().get(
            "security.users_file",
            "runtime/security/users.db"
        );

    const auto audit_file =
        runtime.config().get(
            "security.audit_file",
            "runtime/security/audit.log"
        );

    const auto update_repository =
        runtime.config().get(
            "update.repository",
            "/srv/home-ai-core"
        );

    const auto update_remote =
        runtime.config().get(
            "update.remote",
            "origin"
        );

    const auto update_branch =
        runtime.config().get(
            "update.branch",
            "develop"
        );

    const int update_interval =
        runtime.config().getInt(
            "update.check_interval_seconds",
            60
        );

    const auto web_bind =
        runtime.config().get(
            "web.bind",
            "0.0.0.0"
        );

    int web_port =
        runtime.config().getInt(
            "web.port",
            8080
        );

    if (
        web_port < 1 ||
        web_port > 65535
    ) {
        web_port = 8080;
    }

    homeai::SecurityManager security;
    homeai::UpdateManager updates;
    homeai::SystemMonitor system_monitor;
    homeai::StorageMonitor storage_monitor;
    homeai::ModuleManager modules;

    homeai::WebServer web(
        runtime,
        security,
        updates,
        modules
    );

    std::string module_error;

    if (
        !modules.registerModule(
            std::make_unique<
                homeai::CallbackModule
            >(
                "security",
                std::vector<std::string>{},
                [&](std::string& error) {
                    if (
                        security.initialize(
                            security_database,
                            users_file,
                            audit_file
                        )
                    ) {
                        return true;
                    }

                    error =
                        "Security Core initialization failed.";

                    return false;
                },
                [](std::string&) {
                    return true;
                },
                []() {},
                []() {
                    return
                        homeai::ModuleHealth::
                            Healthy;
                },
                []() {
                    return
                        "Authentication and sessions are ready.";
                }
            ),
            module_error
        )
    ) {
        homeai::Logger::instance().error(
            module_error
        );

        return 1;
    }

    if (
        !modules.registerModule(
            std::make_unique<
                homeai::CallbackModule
            >(
                "update",
                std::vector<std::string>{},
                [&](std::string& error) {
                    if (
                        updates.initialize(
                            update_repository,
                            update_remote,
                            update_branch,
                            update_interval
                        )
                    ) {
                        return true;
                    }

                    error =
                        "Update Manager initialization failed.";

                    return false;
                },
                [&](std::string&) {
                    updates.start();
                    return true;
                },
                [&]() {
                    updates.stop();
                },
                [&]() {
                    const auto status =
                        updates.status();

                    if (
                        status.state ==
                        homeai::UpdateState::Error
                    ) {
                        return
                            homeai::ModuleHealth::
                                Degraded;
                    }

                    return
                        homeai::ModuleHealth::
                            Healthy;
                },
                [&]() {
                    return
                        updates.status()
                            .message;
                }
            ),
            module_error
        )
    ) {
        homeai::Logger::instance().error(
            module_error
        );

        return 1;
    }

    if (
        !modules.registerModule(
            std::make_unique<
                homeai::CallbackModule
            >(
                "system-monitor",
                std::vector<std::string>{},
                [](std::string&) {
                    return true;
                },
                [](std::string&) {
                    return true;
                },
                []() {},
                [&]() {
                    const auto stats =
                        system_monitor.snapshot();

                    if (
                        stats.memory_total_bytes == 0
                        ||
                        stats.disk_total_bytes == 0
                    ) {
                        return
                            homeai::ModuleHealth::
                                Unhealthy;
                    }

                    if (
                        stats.memory_percent >= 95.0
                        ||
                        stats.disk_percent >= 95.0
                    ) {
                        return
                            homeai::ModuleHealth::
                                Degraded;
                    }

                    return
                        homeai::ModuleHealth::
                            Healthy;
                },
                []() {
                    return
                        "CPU, RAM, load and root filesystem monitoring.";
                }
            ),
            module_error
        )
    ) {
        homeai::Logger::instance().error(
            module_error
        );

        return 1;
    }

    if (
        !modules.registerModule(
            std::make_unique<
                homeai::CallbackModule
            >(
                "storage-monitor",
                std::vector<std::string>{},
                [](std::string&) {
                    return true;
                },
                [&](std::string&) {
                    const auto volumes =
                        storage_monitor.snapshot(
                            runtime.config().get(
                                "storage.video_mounts",
                                ""
                            ),
                            runtime.config().get(
                                "storage.personal_mounts",
                                ""
                            )
                        );

                    homeai::DiskOperations
                        operations;

                    for (const auto& volume : volumes) {
                        if (
                            volume.status != "offline"
                            ||
                            volume.mount_point.rfind(
                                "/mnt/home-ai/storage/",
                                0
                            ) != 0
                        ) {
                            continue;
                        }

                        const auto uuid =
                            std::filesystem::path(
                                volume.mount_point
                            )
                            .filename()
                            .string();

                        const auto device =
                            homeai::DiskOperations::
                                deviceForUuid(
                                    uuid
                                );

                        if (device.empty()) {
                            homeai::Logger::instance().warning(
                                "Storage UUID not present: "
                                + uuid
                            );

                            continue;
                        }

                        const auto result =
                            operations.mountAt(
                                device,
                                volume.mount_point,
                                false
                            );

                        if (!result.success) {
                            homeai::Logger::instance().warning(
                                "Unable to restore storage "
                                + uuid
                                + ": "
                                + result.message
                            );
                        }
                        else {
                            homeai::Logger::instance().info(
                                "Restored managed storage "
                                + uuid
                                + " at "
                                + volume.mount_point
                            );
                        }
                    }

                    return true;
                },
                []() {},
                [&]() {
                    const auto volumes =
                        storage_monitor.snapshot(
                            runtime.config().get(
                                "storage.video_mounts",
                                ""
                            ),
                            runtime.config().get(
                                "storage.personal_mounts",
                                ""
                            )
                        );

                    for (
                        const auto& volume :
                        volumes
                    ) {
                        const bool managed =
                            volume.role ==
                                "video"
                            ||
                            volume.role ==
                                "personal"
                            ||
                            volume.role ==
                                "video+personal";

                        if (!managed)
                            continue;

                        if (
                            volume.status !=
                                "online"
                            ||
                            volume.read_only
                            ||
                            volume.used_percent >=
                                95.0
                        ) {
                            return
                                homeai::ModuleHealth::
                                    Degraded;
                        }
                    }

                    return
                        homeai::ModuleHealth::
                            Healthy;
                },
                [&]() {
                    const auto volumes =
                        storage_monitor.snapshot(
                            runtime.config().get(
                                "storage.video_mounts",
                                ""
                            ),
                            runtime.config().get(
                                "storage.personal_mounts",
                                ""
                            )
                        );

                    int managed = 0;
                    int problems = 0;

                    for (
                        const auto& volume :
                        volumes
                    ) {
                        const bool is_managed =
                            volume.role ==
                                "video"
                            ||
                            volume.role ==
                                "personal"
                            ||
                            volume.role ==
                                "video+personal";

                        if (!is_managed)
                            continue;

                        ++managed;

                        if (
                            volume.status !=
                                "online"
                            ||
                            volume.read_only
                            ||
                            volume.used_percent >=
                                95.0
                        ) {
                            ++problems;
                        }
                    }

                    return
                        "Managed storage: "
                        + std::to_string(
                            managed
                        )
                        + ", problems: "
                        + std::to_string(
                            problems
                        )
                        + ".";
                }
            ),
            module_error
        )
    ) {
        homeai::Logger::instance().error(
            module_error
        );

        return 1;
    }

    if (
        !modules.registerModule(
            std::make_unique<
                homeai::CallbackModule
            >(
                "web",
                std::vector<std::string>{
                    "security",
                    "update",
                    "system-monitor",
                    "storage-monitor"
                },
                [](std::string&) {
                    return true;
                },
                [&](std::string& error) {
                    if (
                        web.start(
                            web_bind,
                            static_cast<
                                std::uint16_t
                            >(
                                web_port
                            )
                        )
                    ) {
                        return true;
                    }

                    error =
                        "Unable to start Web Core.";

                    return false;
                },
                [&]() {
                    web.stop();
                },
                [&]() {
                    return
                        web.isRunning()
                        ? homeai::ModuleHealth::
                            Healthy
                        : homeai::ModuleHealth::
                            Unhealthy;
                },
                [&]() {
                    return
                        web.isRunning()
                        ? "HTTP interface is accepting connections."
                        : "HTTP interface is stopped.";
                }
            ),
            module_error
        )
    ) {
        homeai::Logger::instance().error(
            module_error
        );

        return 1;
    }

    if (
        !modules.initializeAll(
            module_error
        )
    ) {
        homeai::Logger::instance().error(
            module_error
        );

        return 1;
    }

    runtime.start();

    if (
        !modules.startAll(
            module_error
        )
    ) {
        homeai::Logger::instance().error(
            module_error
        );

        runtime.stop();

        return 1;
    }

    int tick_ms =
        runtime.config().getInt(
            "runtime.tick_ms",
            250
        );

    if (tick_ms < 10)
        tick_ms = 10;

    bool restart_requested = false;

    while (!stop_requested) {
        if (
            updates.consumeRestartRequest()
        ) {
            restart_requested = true;
            break;
        }

        std::this_thread::sleep_for(
            std::chrono::milliseconds(
                tick_ms
            )
        );
    }

    modules.stopAll();
    runtime.stop();

    if (restart_requested) {
        const auto binary =
            updates.restartBinaryPath();

        homeai::Logger::instance().info(
            "Restarting Home AI Core with updated binary"
        );

        ::execl(
            binary.c_str(),
            binary.c_str(),
            static_cast<char*>(nullptr)
        );

        homeai::Logger::instance().error(
            "Unable to restart updated Home AI Core"
        );

        return 1;
    }

    return 0;
}
