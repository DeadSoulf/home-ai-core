#include "server/storage/StorageMonitor.h"

#include <iostream>

int main()
{
    homeai::StorageMonitor monitor;

    const auto volumes =
        monitor.snapshot(
            "/mnt/home-ai-video-test",
            "/mnt/home-ai-files-test",
            "/mnt/home-ai-vm-test"
        );

    bool video_offline = false;
    bool files_offline = false;
    bool vm_offline = false;

    for (const auto& volume : volumes) {
        if (
            volume.mount_point ==
                "/mnt/home-ai-video-test"
            &&
            volume.role == "video"
            &&
            volume.status == "offline"
        ) {
            video_offline = true;
        }

        if (
            volume.mount_point ==
                "/mnt/home-ai-files-test"
            &&
            volume.role == "personal"
            &&
            volume.status == "offline"
        ) {
            files_offline = true;
        }

        if (
            volume.mount_point ==
                "/mnt/home-ai-vm-test"
            &&
            volume.role == "vm"
            &&
            volume.status == "offline"
        ) {
            vm_offline = true;
        }
    }

    if (
        !video_offline
        ||
        !files_offline
        ||
        !vm_offline
    ) {
        std::cerr
            << "Configured offline storage was not reported\n";

        return 1;
    }

    const auto devices =
        monitor.blockDevices();

    for (const auto& device : devices) {
        if (
            device.device.rfind(
                "/dev/",
                0
            ) != 0
        ) {
            std::cerr
                << "Invalid block device path\n";

            return 1;
        }

        if (
            device.type != "disk"
            &&
            device.type != "partition"
        ) {
            std::cerr
                << "Invalid block device type\n";

            return 1;
        }

        if (
            device.candidate
            &&
            (
                device.mounted
                ||
                device.in_use
                ||
                device.size_bytes == 0
            )
        ) {
            std::cerr
                << "Invalid hotplug candidate state\n";

            return 1;
        }
    }

    std::cout
        << "Storage Monitor test passed\n";

    return 0;
}
