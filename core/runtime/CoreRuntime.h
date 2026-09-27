#pragma once

#include "core/config/ConfigManager.h"
#include "core/events/EventBus.h"
#include "core/events/NotificationCenter.h"

#include <atomic>
#include <string>

namespace homeai {

class CoreRuntime {
public:
    bool initialize(const std::string& config_file);

    void start();
    void stop();

    bool isRunning() const;

    EventBus& events();
    NotificationCenter& notifications();
    ConfigManager& config();

private:
    ConfigManager config_;
    EventBus event_bus_;
    NotificationCenter notification_center_{
        event_bus_
    };

    std::atomic<bool> running_{false};
};

}
