#include "core/logging/Logger.h"
#include "core/runtime/CoreRuntime.h"
#include "security/auth/SecurityManager.h"
#include "server/update/UpdateManager.h"
#include "web/server/WebServer.h"

#include <atomic>
#include <chrono>
#include <csignal>
#include <cstdint>
#include <thread>
#include <unistd.h>

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

    homeai::CoreRuntime runtime;

    if (
        !runtime.initialize(
            "config/home-ai.conf"
        )
    ) {
        homeai::Logger::instance().error(
            "Core initialization failed"
        );

        return 1;
    }

    homeai::SecurityManager security;

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

    if (
        !security.initialize(
            users_file,
            audit_file
        )
    ) {
        homeai::Logger::instance().error(
            "Security Core initialization failed"
        );

        return 1;
    }

    homeai::UpdateManager updates;

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

    if (
        !updates.initialize(
            update_repository,
            update_remote,
            update_branch,
            update_interval
        )
    ) {
        homeai::Logger::instance().error(
            "Update Manager initialization failed"
        );

        return 1;
    }

    updates.start();

    runtime.start();

    homeai::WebServer web(
        runtime,
        security,
        updates
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

    if (
        !web.start(
            web_bind,
            static_cast<std::uint16_t>(
                web_port
            )
        )
    ) {
        homeai::Logger::instance().error(
            "Unable to start Web Core"
        );
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

    web.stop();
    updates.stop();
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
