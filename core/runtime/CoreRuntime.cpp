#include "core/runtime/CoreRuntime.h"
#include "core/logging/Logger.h"

namespace homeai {

bool CoreRuntime::initialize(const std::string& config_file)
{
    auto& logger = Logger::instance();

    logger.info("Initializing Home AI Core");

    if (!config_.load(config_file)) {
        logger.warning(
            "Configuration file not found: " + config_file
        );
    }

    const auto log_level = config_.get(
        "log.level",
        "info"
    );

    if (log_level == "debug")
        logger.setLevel(LogLevel::Debug);
    else if (log_level == "warning")
        logger.setLevel(LogLevel::Warning);
    else if (log_level == "error")
        logger.setLevel(LogLevel::Error);
    else
        logger.setLevel(LogLevel::Info);

    event_bus_.subscribe(
        "core.started",
        [](const Event&) {
            Logger::instance().debug(
                "Event received: core.started"
            );
        }
    );

    logger.info(
        "Core name: " +
        config_.get("core.name", "Home AI Core")
    );

    logger.info(
        "Core version: " +
        config_.get("core.version", "unknown")
    );

    return true;
}

void CoreRuntime::start()
{
    if (running_)
        return;

    running_ = true;

    Logger::instance().info("Core runtime started");

    event_bus_.publish({
        "core.started",
        ""
    });
}

void CoreRuntime::stop()
{
    if (!running_)
        return;

    Logger::instance().info(
        "Stopping core runtime"
    );

    event_bus_.publish({
        "core.stopping",
        ""
    });

    running_ = false;

    Logger::instance().info(
        "Core runtime stopped"
    );
}

bool CoreRuntime::isRunning() const
{
    return running_;
}

EventBus& CoreRuntime::events()
{
    return event_bus_;
}

ConfigManager& CoreRuntime::config()
{
    return config_;
}

}
