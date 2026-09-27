#include "core/events/NotificationCenter.h"

#include <algorithm>
#include <chrono>

namespace homeai {

namespace {

bool isNotificationSeverity(
    const std::string& severity
)
{
    return
        severity == "info"
        ||
        severity == "warning"
        ||
        severity == "error"
        ||
        severity == "critical";
}

}

NotificationCenter::NotificationCenter(
    EventBus& event_bus,
    std::size_t capacity
)
    : capacity_(
          std::max<std::size_t>(
              1,
              capacity
          )
      )
{
    event_bus.subscribeAll(
        [this](const Event& event) {
            onEvent(event);
        }
    );
}

void NotificationCenter::onEvent(
    const Event& event
)
{
    if (
        !isNotificationSeverity(
            event.severity
        )
    ) {
        return;
    }

    const auto now =
        std::chrono::system_clock::now();

    const auto timestamp =
        std::chrono::duration_cast<
            std::chrono::milliseconds
        >(
            now.time_since_epoch()
        ).count();

    Notification notification;

    {
        std::lock_guard<std::mutex>
            lock(mutex_);

        notification.id =
            next_id_++;

        notification.timestamp_ms =
            timestamp;

        notification.topic =
            event.topic;

        notification.source =
            event.source.empty()
            ? "core"
            : event.source;

        notification.severity =
            event.severity;

        notification.message =
            event.data;

        notifications_.push_back(
            std::move(notification)
        );

        while (
            notifications_.size() >
            capacity_
        ) {
            notifications_.pop_front();
        }
    }
}

std::vector<Notification>
NotificationCenter::snapshot(
    std::size_t limit
) const
{
    std::vector<Notification> result;

    if (limit == 0)
        return result;

    std::lock_guard<std::mutex>
        lock(mutex_);

    limit =
        std::min(
            limit,
            notifications_.size()
        );

    result.reserve(limit);

    auto it =
        notifications_.rbegin();

    for (
        std::size_t i = 0;
        i < limit;
        ++i, ++it
    ) {
        result.push_back(*it);
    }

    return result;
}

std::size_t NotificationCenter::size() const
{
    std::lock_guard<std::mutex>
        lock(mutex_);

    return notifications_.size();
}

}
