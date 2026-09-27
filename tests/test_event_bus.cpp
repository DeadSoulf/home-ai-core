#include "core/events/EventBus.h"
#include "core/events/NotificationCenter.h"

#include <iostream>

int main()
{
    homeai::EventBus bus;
    homeai::NotificationCenter center(
        bus,
        2
    );

    bool topic_received = false;
    int all_received = 0;

    bus.subscribe(
        "test.event",
        [&topic_received](
            const homeai::Event& event
        ) {
            if (event.data == "hello")
                topic_received = true;
        }
    );

    bus.subscribeAll(
        [&all_received](
            const homeai::Event&
        ) {
            ++all_received;
        }
    );

    bus.publish({
        "test.event",
        "hello"
    });

    if (
        !topic_received
        ||
        all_received != 1
        ||
        center.size() != 0
    ) {
        std::cerr
            << "Plain EventBus delivery failed\n";
        return 1;
    }

    bus.publish({
        "storage.warning",
        "Disk nearly full.",
        "storage",
        "warning"
    });

    bus.publish({
        "module.recovery",
        "Camera module restarted.",
        "module-manager",
        "info"
    });

    bus.publish({
        "security.alert",
        "Authentication source blocked.",
        "security",
        "error"
    });

    if (
        all_received != 4
        ||
        center.size() != 2
    ) {
        std::cerr
            << "Notification capture/capacity failed\n";
        return 1;
    }

    const auto notifications =
        center.snapshot();

    if (
        notifications.size() != 2
        ||
        notifications[0].topic !=
            "security.alert"
        ||
        notifications[0].severity !=
            "error"
        ||
        notifications[1].topic !=
            "module.recovery"
        ||
        notifications[0].id <=
            notifications[1].id
    ) {
        std::cerr
            << "Notification ordering failed\n";
        return 1;
    }

    std::cout
        << "Event/notification test passed\n";

    return 0;
}
