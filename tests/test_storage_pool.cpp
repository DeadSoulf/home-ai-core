#include "server/storage/StoragePool.h"

#include <iostream>
#include <vector>

int main()
{
    using namespace homeai;

    StorageVolume a;
    a.mount_point = "/pool/a";
    a.role = "video";
    a.status = "online";
    a.capacity_available = true;
    a.total_bytes = 1000;
    a.free_bytes = 500;
    a.used_percent = 50.0;

    StorageVolume b;
    b.mount_point = "/pool/b";
    b.role = "video+personal";
    b.status = "online";
    b.capacity_available = true;
    b.total_bytes = 2000;
    b.free_bytes = 1200;
    b.used_percent = 40.0;

    StorageVolume c;
    c.mount_point = "/pool/c";
    c.role = "personal";
    c.status = "online";
    c.capacity_available = true;
    c.total_bytes = 1000;
    c.free_bytes = 900;
    c.used_percent = 10.0;

    const std::vector<StorageVolume> volumes{
        a,
        b,
        c
    };

    const auto video_summary =
        StoragePoolSelector::summarize(
            volumes,
            "video"
        );

    if (
        video_summary.assigned_volumes != 2
        ||
        video_summary.online_volumes != 2
        ||
        video_summary.total_bytes != 3000
        ||
        video_summary.free_bytes != 1700
        ||
        !video_summary.free_bytes_complete
    ) {
        std::cerr
            << "Video pool summary failed\n";

        return 1;
    }

    StorageVolume pending;
    pending.mount_point =
        "/pool/pending";
    pending.role =
        "video";
    pending.status =
        "online";
    pending.device_size_bytes =
        4000;
    pending.capacity_available =
        false;

    auto volumes_with_pending =
        volumes;

    volumes_with_pending.push_back(
        pending
    );

    const auto expanded_summary =
        StoragePoolSelector::summarize(
            volumes_with_pending,
            "video"
        );

    if (
        expanded_summary.assigned_volumes != 3
        ||
        expanded_summary.online_volumes != 3
        ||
        expanded_summary.total_bytes != 7000
        ||
        expanded_summary.free_bytes != 1700
        ||
        expanded_summary.free_bytes_complete
    ) {
        std::cerr
            << "Pool capacity fallback failed\n";

        return 1;
    }

    StoragePoolOptions options;
    options.reserve_percent = 10;

    const auto most_free =
        StoragePoolSelector::select(
            volumes,
            "video",
            options
        );

    if (
        !most_free
        ||
        most_free->mount_point != "/pool/b"
    ) {
        std::cerr
            << "Most-free policy failed\n";

        return 1;
    }

    options.policy =
        StoragePoolPolicy::Sequential;

    const auto sequential =
        StoragePoolSelector::select(
            volumes,
            "video",
            options
        );

    if (
        !sequential
        ||
        sequential->mount_point != "/pool/a"
    ) {
        std::cerr
            << "Sequential policy failed\n";

        return 1;
    }

    options.policy =
        StoragePoolPolicy::Balanced;

    const auto balanced =
        StoragePoolSelector::select(
            volumes,
            "video",
            options
        );

    if (
        !balanced
        ||
        balanced->mount_point != "/pool/b"
    ) {
        std::cerr
            << "Balanced policy failed\n";

        return 1;
    }

    options.policy =
        StoragePoolPolicy::Pinned;

    options.preferred_mount =
        "/pool/a";

    const auto pinned =
        StoragePoolSelector::select(
            volumes,
            "video",
            options
        );

    if (
        !pinned
        ||
        pinned->mount_point != "/pool/a"
    ) {
        std::cerr
            << "Pinned policy failed\n";

        return 1;
    }

    const auto personal =
        StoragePoolSelector::select(
            volumes,
            "personal",
            StoragePoolOptions{}
        );

    if (
        !personal
        ||
        personal->mount_point != "/pool/b"
    ) {
        std::cerr
            << "Personal pool role filter failed\n";

        return 1;
    }

    StoragePoolOptions reserve_options;
    reserve_options.reserve_percent = 95;

    const auto reserved =
        StoragePoolSelector::select(
            volumes,
            "video",
            reserve_options
        );

    if (reserved) {
        std::cerr
            << "Reserve threshold should exclude full pool\n";

        return 1;
    }

    StoragePoolOptions pinned_fallback;
    pinned_fallback.policy =
        StoragePoolPolicy::Pinned;
    pinned_fallback.preferred_mount =
        "/pool/missing";

    const auto fallback =
        StoragePoolSelector::select(
            volumes,
            "video",
            pinned_fallback
        );

    if (
        !fallback
        ||
        fallback->mount_point != "/pool/b"
    ) {
        std::cerr
            << "Pinned fallback policy failed\n";

        return 1;
    }

    std::cout
        << "Storage Pool test passed\n";

    return 0;
}
