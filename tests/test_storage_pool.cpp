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
