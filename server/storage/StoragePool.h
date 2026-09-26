#pragma once

#include "server/storage/StorageMonitor.h"

#include <cstddef>
#include <cstdint>
#include <optional>
#include <string>
#include <vector>

namespace homeai {

enum class StoragePoolPolicy {
    MostFree,
    Sequential,
    Balanced,
    Pinned
};

struct StoragePoolOptions {
    StoragePoolPolicy policy{
        StoragePoolPolicy::MostFree
    };

    int reserve_percent{10};
    std::uint64_t reserve_bytes{0};

    std::string preferred_mount;
};

struct StoragePoolSummary {
    std::size_t assigned_volumes{0};
    std::size_t online_volumes{0};

    std::uint64_t total_bytes{0};
    std::uint64_t free_bytes{0};

    bool free_bytes_complete{true};
};

class StoragePoolSelector {
public:
    static StoragePoolPolicy policyFromString(
        const std::string& value
    );

    static std::string policyToString(
        StoragePoolPolicy policy
    );

    static StoragePoolSummary summarize(
        const std::vector<StorageVolume>& volumes,
        const std::string& role
    );

    static std::optional<StorageVolume>
    select(
        const std::vector<StorageVolume>& volumes,
        const std::string& role,
        const StoragePoolOptions& options,
        std::uint64_t required_bytes = 0
    );
};

}
