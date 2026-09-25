#include "core/modules/Module.h"

namespace homeai {

CallbackModule::CallbackModule(
    std::string name,
    std::vector<std::string> dependencies,
    Action initialize,
    Action start,
    StopAction stop,
    HealthAction health,
    MessageAction health_message
)
    : name_(std::move(name)),
      dependencies_(
          std::move(dependencies)
      ),
      initialize_(
          std::move(initialize)
      ),
      start_(
          std::move(start)
      ),
      stop_(
          std::move(stop)
      ),
      health_(
          std::move(health)
      ),
      health_message_(
          std::move(
              health_message
          )
      )
{
}

std::string CallbackModule::name() const
{
    return name_;
}

std::vector<std::string>
CallbackModule::dependencies() const
{
    return dependencies_;
}

bool CallbackModule::initialize(
    std::string& error
)
{
    if (!initialize_)
        return true;

    return initialize_(error);
}

bool CallbackModule::start(
    std::string& error
)
{
    if (!start_)
        return true;

    return start_(error);
}

void CallbackModule::stop()
{
    if (stop_)
        stop_();
}

ModuleHealth
CallbackModule::health() const
{
    if (!health_)
        return ModuleHealth::Unknown;

    return health_();
}

std::string
CallbackModule::healthMessage() const
{
    if (!health_message_)
        return {};

    return health_message_();
}

std::string moduleHealthToString(
    ModuleHealth health
)
{
    switch (health) {
        case ModuleHealth::Unknown:
            return "unknown";
        case ModuleHealth::Healthy:
            return "healthy";
        case ModuleHealth::Degraded:
            return "degraded";
        case ModuleHealth::Unhealthy:
            return "unhealthy";
    }

    return "unknown";
}

}
