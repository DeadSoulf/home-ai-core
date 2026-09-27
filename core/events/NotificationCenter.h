#pragma once

#include "core/events/EventBus.h"

#include <cstddef>
#include <cstdint>
#include <deque>
#include <mutex>
#include <string>
#include <vector>

namespace homeai {

struct Notification {
    std::uint64_t id{0};
    std::int64_t timestamp_ms{0};
    std::string topic;
    std::string source;
    std::string severity;
    std::string message;
};

class NotificationCenter {
public:
    explicit NotificationCenter(
        EventBus& event_bus,
        std::size_t capacity = 200
    );

    std::vector<Notification>
    snapshot(
        std::size_t limit = 50
    ) const;

    std::size_t size() const;

private:
    void onEvent(
        const Event& event
    );

    std::size_t capacity_{200};

    mutable std::mutex mutex_;

    std::deque<Notification>
        notifications_;

    std::uint64_t next_id_{1};
};

}
