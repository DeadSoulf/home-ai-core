#include "core/events/EventBus.h"

#include <iostream>

int main()
{
    homeai::EventBus bus;

    bool received = false;

    bus.subscribe(
        "test.event",
        [&received](const homeai::Event& event) {
            if (event.data == "hello")
                received = true;
        }
    );

    bus.publish({
        "test.event",
        "hello"
    });

    if (!received) {
        std::cerr << "EventBus test failed\n";
        return 1;
    }

    std::cout << "EventBus test passed\n";
    return 0;
}
