#include "core/events/EventBus.h"

namespace homeai {

void EventBus::subscribe(
    const std::string& topic,
    Handler handler
)
{
    std::lock_guard<std::mutex>
        lock(mutex_);

    handlers_[topic].push_back(
        std::move(handler)
    );
}

void EventBus::subscribeAll(
    Handler handler
)
{
    std::lock_guard<std::mutex>
        lock(mutex_);

    all_handlers_.push_back(
        std::move(handler)
    );
}

void EventBus::publish(
    const Event& event
)
{
    std::vector<Handler> handlers;
    std::vector<Handler> all_handlers;

    {
        std::lock_guard<std::mutex>
            lock(mutex_);

        const auto it =
            handlers_.find(event.topic);

        if (it != handlers_.end()) {
            handlers = it->second;
        }

        all_handlers =
            all_handlers_;
    }

    for (const auto& handler : handlers)
        handler(event);

    for (
        const auto& handler :
        all_handlers
    ) {
        handler(event);
    }
}

}
