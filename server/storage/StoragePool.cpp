#include "server/storage/StoragePool.h"

#include <algorithm>
#include <cstdint>
#include <limits>

namespace homeai {

namespace {

bool hasRole(
    const StorageVolume& volume,
    const std::string& role
)
{
    if (role == "video") {
        return
            volume.role == "video"
            ||
            volume.role == "video+personal";
    }

    if (role == "personal") {
        return
            volume.role == "personal"
            ||
            volume.role == "video+personal";
    }

    return false;
}

std::uint64_t reserveFor(
    const StorageVolume& volume,
    const StoragePoolOptions& options
)
{
    const int reserve_percent =
        std::clamp(
            options.reserve_percent,
            0,
            95
        );

    const auto percent_reserve =
        static_cast<std::uint64_t>(
            (
                static_cast<long double>(
                    volume.total_bytes
                )
                *
                static_cast<long double>(
                    reserve_percent
                )
            )
            /
            100.0L
        );

    return std::max(
        percent_reserve,
        options.reserve_bytes
    );
}

bool eligible(
    const StorageVolume& volume,
    const std::string& role,
    const StoragePoolOptions& options
)
{
    if (
        !hasRole(
            volume,
            role
        )
        ||
        volume.status != "online"
        ||
        volume.read_only
        ||
        !volume.capacity_available
    ) {
        return false;
    }

    return
        volume.free_bytes >
        reserveFor(
            volume,
            options
        );
}

std::uint64_t usableFree(
    const StorageVolume& volume,
    const StoragePoolOptions& options
)
{
    const auto reserve =
        reserveFor(
            volume,
            options
        );

    if (volume.free_bytes <= reserve)
        return 0;

    return
        volume.free_bytes -
        reserve;
}

}

StoragePoolSummary
StoragePoolSelector::summarize(
    const std::vector<StorageVolume>& volumes,
    const std::string& role
)
{
    StoragePoolSummary summary;

    for (const auto& volume : volumes) {
        if (
            !hasRole(
                volume,
                role
            )
        ) {
            continue;
        }

        ++summary.assigned_volumes;

        if (volume.status != "online")
            continue;

        ++summary.online_volumes;

        summary.total_bytes +=
            volume.total_bytes > 0
            ? volume.total_bytes
            : volume.device_size_bytes;

        if (volume.capacity_available) {
            summary.free_bytes +=
                volume.free_bytes;
        }
        else {
            summary.free_bytes_complete =
                false;
        }
    }

    return summary;
}

StoragePoolPolicy
StoragePoolSelector::policyFromString(
    const std::string& value
)
{
    if (value == "sequential")
        return StoragePoolPolicy::Sequential;

    if (value == "balanced")
        return StoragePoolPolicy::Balanced;

    if (value == "pinned")
        return StoragePoolPolicy::Pinned;

    return StoragePoolPolicy::MostFree;
}

std::string
StoragePoolSelector::policyToString(
    StoragePoolPolicy policy
)
{
    switch (policy) {
        case StoragePoolPolicy::MostFree:
            return "most_free";

        case StoragePoolPolicy::Sequential:
            return "sequential";

        case StoragePoolPolicy::Balanced:
            return "balanced";

        case StoragePoolPolicy::Pinned:
            return "pinned";
    }

    return "most_free";
}

std::optional<StorageVolume>
StoragePoolSelector::select(
    const std::vector<StorageVolume>& volumes,
    const std::string& role,
    const StoragePoolOptions& options
)
{
    std::vector<const StorageVolume*>
        candidates;

    for (const auto& volume : volumes) {
        if (
            eligible(
                volume,
                role,
                options
            )
        ) {
            candidates.push_back(
                &volume
            );
        }
    }

    if (candidates.empty())
        return std::nullopt;

    if (
        options.policy ==
            StoragePoolPolicy::Pinned
        &&
        !options.preferred_mount.empty()
    ) {
        const auto it =
            std::find_if(
                candidates.begin(),
                candidates.end(),
                [&](const StorageVolume* volume) {
                    return
                        volume->mount_point ==
                        options.preferred_mount;
                }
            );

        if (it != candidates.end())
            return **it;
    }

    if (
        options.policy ==
        StoragePoolPolicy::Sequential
    ) {
        const auto it =
            std::min_element(
                candidates.begin(),
                candidates.end(),
                [](const StorageVolume* a,
                   const StorageVolume* b) {
                    return
                        a->mount_point <
                        b->mount_point;
                }
            );

        return **it;
    }

    if (
        options.policy ==
        StoragePoolPolicy::Balanced
    ) {
        const auto it =
            std::min_element(
                candidates.begin(),
                candidates.end(),
                [](const StorageVolume* a,
                   const StorageVolume* b) {
                    if (
                        a->used_percent !=
                        b->used_percent
                    ) {
                        return
                            a->used_percent <
                            b->used_percent;
                    }

                    return
                        a->mount_point <
                        b->mount_point;
                }
            );

        return **it;
    }

    const auto it =
        std::max_element(
            candidates.begin(),
            candidates.end(),
            [&](const StorageVolume* a,
                const StorageVolume* b) {
                const auto a_free =
                    usableFree(
                        *a,
                        options
                    );

                const auto b_free =
                    usableFree(
                        *b,
                        options
                    );

                if (a_free != b_free)
                    return a_free < b_free;

                return
                    a->mount_point >
                    b->mount_point;
            }
        );

    return **it;
}

}
