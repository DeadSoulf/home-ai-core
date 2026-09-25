#pragma once

#include <cstdint>
#include <mutex>

namespace homeai {

struct SystemStats {
    double cpu_percent{0.0};
    double memory_percent{0.0};
    double disk_percent{0.0};

    double load_1{0.0};
    double load_5{0.0};
    double load_15{0.0};

    std::uint64_t memory_total_bytes{0};
    std::uint64_t memory_available_bytes{0};

    std::uint64_t disk_total_bytes{0};
    std::uint64_t disk_free_bytes{0};

    std::uint64_t uptime_seconds{0};
};

class SystemMonitor {
public:
    SystemStats snapshot();

private:
    bool readCpu(
        std::uint64_t& total,
        std::uint64_t& idle
    );

    double calculateCpu();

    std::mutex mutex_;

    std::uint64_t previous_total_{0};
    std::uint64_t previous_idle_{0};
};

}
