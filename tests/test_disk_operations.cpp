#include "server/storage/DiskOperations.h"

#include <iostream>

int main()
{
    const auto video =
        homeai::DiskOperations::
        defaultMountPoint(
            "/dev/sdb1",
            "video"
        );

    const auto personal =
        homeai::DiskOperations::
        defaultMountPoint(
            "/dev/nvme0n1p1",
            "personal"
        );

    const auto managed =
        homeai::DiskOperations::
        defaultMountPoint(
            "/dev/sdd1",
            "storage"
        );

    const auto invalid =
        homeai::DiskOperations::
        defaultMountPoint(
            "/dev/sdc1",
            "unknown"
        );

    if (
        video !=
            "/mnt/home-ai/video/sdb1"
        ||
        personal !=
            "/mnt/home-ai/files/nvme0n1p1"
        ||
        managed !=
            "/mnt/home-ai/storage/sdd1"
        ||
        !invalid.empty()
    ) {
        std::cerr
            << "Disk operation mount point test failed\n";

        return 1;
    }

    std::cout
        << "Disk Operations test passed\n";

    return 0;
}
