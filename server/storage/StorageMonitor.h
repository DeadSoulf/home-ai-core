#pragma once

#include <cstdint>
#include <string>
#include <vector>

namespace homeai {

struct StorageVolume {
    std::string source;
    std::string mount_point;
    std::string filesystem;
    std::string role;
    std::string status;

    std::uint64_t device_size_bytes{0};
    std::uint64_t total_bytes{0};
    std::uint64_t used_bytes{0};
    std::uint64_t free_bytes{0};

    double used_percent{0.0};

    bool capacity_available{false};
    bool read_only{false};
};

struct BlockDeviceInfo {
    std::string name;
    std::string device;
    std::string parent;
    std::string type;
    std::string model;
    std::string vendor;
    std::string serial;
    std::string mount_point;

    std::uint64_t size_bytes{0};

    bool removable{false};
    bool mounted{false};
    bool in_use{false};
    bool has_partitions{false};
    bool candidate{false};
};

class StorageMonitor {
public:
    std::vector<StorageVolume> snapshot(
        const std::string& video_mounts,
        const std::string& personal_mounts
    ) const;

    std::vector<BlockDeviceInfo>
    blockDevices() const;

private:
    static std::vector<std::string> parseMountList(
        const std::string& value
    );

    static std::string normalizeMount(
        const std::string& value
    );

    static bool containsMount(
        const std::vector<std::string>& mounts,
        const std::string& mount_point
    );

    static std::string classifyRole(
        const std::string& mount_point,
        const std::vector<std::string>& video_mounts,
        const std::vector<std::string>& personal_mounts
    );
};

}
