#pragma once

#include "server/storage/StorageMonitor.h"

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

class StoragePoolSelector {
public:
    static StoragePoolPolicy policyFromString(
        const std::string& value
    );

    static std::string policyToString(
        StoragePoolPolicy policy
    );

    static std::optional<StorageVolume>
    select(
        const std::vector<StorageVolume>& volumes,
        const std::string& role,
        const StoragePoolOptions& options
    );
};

}
