#include "core/logging/Logger.h"
#include "core/runtime/CoreRuntime.h"
#include "security/auth/SecurityManager.h"
#include "web/server/WebServer.h"

#include <atomic>
#include <chrono>
#include <csignal>
#include <cstdint>
#include <thread>

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

    runtime.start();

    homeai::WebServer web(
        runtime,
        security
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

    while (!stop_requested) {
        std::this_thread::sleep_for(
            std::chrono::milliseconds(
                tick_ms
            )
        );
    }

    web.stop();
    runtime.stop();

    return 0;
}
