#include "server/system/SystemMonitor.h"

#include <fstream>
#include <sstream>
#include <string>
#include <sys/statvfs.h>

namespace homeai {

bool SystemMonitor::readCpu(
    std::uint64_t& total,
    std::uint64_t& idle
)
{
    std::ifstream file("/proc/stat");

    if (!file.is_open())
        return false;

    std::string cpu;

    std::uint64_t user = 0;
    std::uint64_t nice = 0;
    std::uint64_t system = 0;
    std::uint64_t idle_ticks = 0;
    std::uint64_t iowait = 0;
    std::uint64_t irq = 0;
    std::uint64_t softirq = 0;
    std::uint64_t steal = 0;

    file
        >> cpu
        >> user
        >> nice
        >> system
        >> idle_ticks
        >> iowait
        >> irq
        >> softirq
        >> steal;

    if (cpu != "cpu")
        return false;

    idle =
        idle_ticks +
        iowait;

    total =
        user +
        nice +
        system +
        idle_ticks +
        iowait +
        irq +
        softirq +
        steal;

    return true;
}

double SystemMonitor::calculateCpu()
{
    std::uint64_t total = 0;
    std::uint64_t idle = 0;

    if (!readCpu(total, idle))
        return 0.0;

    std::lock_guard<std::mutex> lock(mutex_);

    if (previous_total_ == 0) {
        previous_total_ = total;
        previous_idle_ = idle;
        return 0.0;
    }

    const auto total_delta =
        total - previous_total_;

    const auto idle_delta =
        idle - previous_idle_;

    previous_total_ = total;
    previous_idle_ = idle;

    if (total_delta == 0)
        return 0.0;

    const double busy =
        static_cast<double>(
            total_delta - idle_delta
        );

    return
        (busy /
         static_cast<double>(total_delta))
        * 100.0;
}

SystemStats SystemMonitor::snapshot()
{
    SystemStats stats;

    stats.cpu_percent =
        calculateCpu();

    {
        std::ifstream file(
            "/proc/meminfo"
        );

        std::string key;
        std::uint64_t value = 0;
        std::string unit;

        std::uint64_t total_kb = 0;
        std::uint64_t available_kb = 0;

        while (file >> key >> value >> unit) {
            if (key == "MemTotal:")
                total_kb = value;
            else if (key == "MemAvailable:")
                available_kb = value;

            if (
                total_kb > 0 &&
                available_kb > 0
            ) {
                break;
            }
        }

        stats.memory_total_bytes =
            total_kb * 1024ULL;

        stats.memory_available_bytes =
            available_kb * 1024ULL;

        if (total_kb > 0) {
            const auto used =
                total_kb - available_kb;

            stats.memory_percent =
                (static_cast<double>(used) /
                 static_cast<double>(total_kb))
                * 100.0;
        }
    }

    {
        struct statvfs filesystem {};

        if (
            statvfs(
                "/",
                &filesystem
            ) == 0
        ) {
            stats.disk_total_bytes =
                static_cast<std::uint64_t>(
                    filesystem.f_blocks
                ) *
                filesystem.f_frsize;

            stats.disk_free_bytes =
                static_cast<std::uint64_t>(
                    filesystem.f_bavail
                ) *
                filesystem.f_frsize;

            if (stats.disk_total_bytes > 0) {
                const auto used =
                    stats.disk_total_bytes -
                    stats.disk_free_bytes;

                stats.disk_percent =
                    (
                        static_cast<double>(used) /
                        static_cast<double>(
                            stats.disk_total_bytes
                        )
                    ) * 100.0;
            }
        }
    }

    {
        std::ifstream file(
            "/proc/uptime"
        );

        double uptime = 0.0;

        if (file >> uptime) {
            stats.uptime_seconds =
                static_cast<std::uint64_t>(
                    uptime
                );
        }
    }

    {
        std::ifstream file(
            "/proc/loadavg"
        );

        file
            >> stats.load_1
            >> stats.load_5
            >> stats.load_15;
    }

    return stats;
}

}
