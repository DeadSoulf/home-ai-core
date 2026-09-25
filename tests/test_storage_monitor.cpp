#include "server/storage/StorageMonitor.h"

#include <iostream>

int main()
{
    homeai::StorageMonitor monitor;

    const auto volumes =
        monitor.snapshot(
            "/mnt/home-ai-video-test",
            "/mnt/home-ai-files-test"
        );

    bool video_offline = false;
    bool files_offline = false;

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
    }

    if (
        !video_offline
        ||
        !files_offline
    ) {
        std::cerr
            << "Configured offline storage was not reported\n";

        return 1;
    }

    std::cout
        << "Storage Monitor test passed\n";

    return 0;
}
