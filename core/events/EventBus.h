#pragma once

#include <functional>
#include <mutex>
#include <string>
#include <unordered_map>
#include <vector>

namespace homeai {

struct Event {
    std::string topic;
    std::string data;
    std::string source;
    std::string severity;
};

class EventBus {
public:
    using Handler =
        std::function<void(const Event&)>;

    void subscribe(
        const std::string& topic,
        Handler handler
    );

    void subscribeAll(
        Handler handler
    );

    void publish(
        const Event& event
    );

private:
    std::unordered_map<
        std::string,
        std::vector<Handler>
    > handlers_;

    std::vector<Handler>
        all_handlers_;

    std::mutex mutex_;
};

}
