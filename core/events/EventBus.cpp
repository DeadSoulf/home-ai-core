#include "core/events/EventBus.h"

namespace homeai {

void EventBus::subscribe(
    const std::string& topic,
    Handler handler
)
{
    std::lock_guard<std::mutex> lock(mutex_);
    handlers_[topic].push_back(std::move(handler));
}

void EventBus::publish(const Event& event)
{
    std::vector<Handler> handlers;

    {
        std::lock_guard<std::mutex> lock(mutex_);

        auto it = handlers_.find(event.topic);

        if (it == handlers_.end())
            return;

        handlers = it->second;
    }

    for (const auto& handler : handlers)
        handler(event);
}

}
