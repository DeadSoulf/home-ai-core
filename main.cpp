#include "core/logging/Logger.h"
#include "core/runtime/CoreRuntime.h"

#include <atomic>
#include <chrono>
#include <csignal>
#include <thread>

static std::atomic<bool> stop_requested{false};

static void signal_handler(int)
{
    stop_requested = true;
}

int main()
{
    std::signal(SIGINT, signal_handler);
    std::signal(SIGTERM, signal_handler);

    homeai::CoreRuntime runtime;

    if (!runtime.initialize("config/home-ai.conf")) {
        homeai::Logger::instance().error(
            "Core initialization failed"
        );

        return 1;
    }

    runtime.start();

    const int tick_ms =
        runtime.config().getInt(
            "runtime.tick_ms",
            250
        );

    while (!stop_requested) {
        std::this_thread::sleep_for(
            std::chrono::milliseconds(tick_ms)
        );
    }

    runtime.stop();

    return 0;
}
