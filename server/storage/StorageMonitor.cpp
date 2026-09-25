#include "server/storage/StorageMonitor.h"

#include <algorithm>
#include <cctype>
#include <cstdio>
#include <mntent.h>
#include <set>
#include <string>
#include <sys/statvfs.h>

namespace homeai {

namespace {

std::string trimCopy(
    std::string value
)
{
    auto first = std::find_if_not(
        value.begin(),
        value.end(),
        [](unsigned char c) {
            return std::isspace(c);
        }
    );

    auto last = std::find_if_not(
        value.rbegin(),
        value.rend(),
        [](unsigned char c) {
            return std::isspace(c);
        }
    ).base();

    if (first >= last)
        return {};

    return std::string(
        first,
        last
    );
}

bool isBlockDeviceSource(
    const std::string& source
)
{
    return source.rfind(
        "/dev/",
        0
    ) == 0;
}

}

std::vector<std::string>
StorageMonitor::parseMountList(
    const std::string& value
)
{
    std::vector<std::string> result;

    std::size_t start = 0;

    while (start <= value.size()) {
        auto end =
            value.find(
                ',',
                start
            );

        if (end == std::string::npos)
            end = value.size();

        auto item =
            normalizeMount(
                trimCopy(
                    value.substr(
                        start,
                        end - start
                    )
                )
            );

        if (
            !item.empty()
            &&
            std::find(
                result.begin(),
                result.end(),
                item
            ) == result.end()
        ) {
            result.push_back(
                std::move(item)
            );
        }

        if (end == value.size())
            break;

        start = end + 1;
    }

    return result;
}

std::string StorageMonitor::normalizeMount(
    const std::string& value
)
{
    auto result =
        trimCopy(value);

    while (
        result.size() > 1
        &&
        result.back() == '/'
    ) {
        result.pop_back();
    }

    return result;
}

bool StorageMonitor::containsMount(
    const std::vector<std::string>& mounts,
    const std::string& mount_point
)
{
    const auto normalized =
        normalizeMount(
            mount_point
        );

    return std::find(
        mounts.begin(),
        mounts.end(),
        normalized
    ) != mounts.end();
}

std::string StorageMonitor::classifyRole(
    const std::string& mount_point,
    const std::vector<std::string>& video_mounts,
    const std::vector<std::string>& personal_mounts
)
{
    const bool video =
        containsMount(
            video_mounts,
            mount_point
        );

    const bool personal =
        containsMount(
            personal_mounts,
            mount_point
        );

    if (video && personal)
        return "video+personal";

    if (video)
        return "video";

    if (personal)
        return "personal";

    if (
        normalizeMount(
            mount_point
        ) == "/"
    ) {
        return "system";
    }

    return "unassigned";
}

std::vector<StorageVolume>
StorageMonitor::snapshot(
    const std::string& video_mounts_value,
    const std::string& personal_mounts_value
) const
{
    const auto video_mounts =
        parseMountList(
            video_mounts_value
        );

    const auto personal_mounts =
        parseMountList(
            personal_mounts_value
        );

    std::vector<StorageVolume>
        volumes;

    std::set<std::string>
        discovered_mounts;

    FILE* mounts =
        setmntent(
            "/proc/self/mounts",
            "r"
        );

    if (mounts != nullptr) {
        mntent entry{};
        char buffer[4096];

        while (
            getmntent_r(
                mounts,
                &entry,
                buffer,
                sizeof(buffer)
            ) != nullptr
        ) {
            const std::string source =
                entry.mnt_fsname
                ? entry.mnt_fsname
                : "";

            const std::string mount_point =
                normalizeMount(
                    entry.mnt_dir
                    ? entry.mnt_dir
                    : ""
                );

            if (
                source.empty()
                ||
                mount_point.empty()
                ||
                !isBlockDeviceSource(
                    source
                )
            ) {
                continue;
            }

            if (
                !discovered_mounts.insert(
                    mount_point
                ).second
            ) {
                continue;
            }

            StorageVolume volume;

            volume.source =
                source;

            volume.mount_point =
                mount_point;

            volume.filesystem =
                entry.mnt_type
                ? entry.mnt_type
                : "";

            volume.role =
                classifyRole(
                    mount_point,
                    video_mounts,
                    personal_mounts
                );

            volume.status =
                "online";

            const std::string options =
                entry.mnt_opts
                ? entry.mnt_opts
                : "";

            volume.read_only =
                options == "ro"
                ||
                options.find(
                    "ro,"
                ) == 0
                ||
                options.find(
                    ",ro,"
                ) != std::string::npos
                ||
                (
                    options.size() >= 3
                    &&
                    options.rfind(",ro") ==
                        options.size() - 3
                );

            struct statvfs filesystem{};

            if (
                statvfs(
                    mount_point.c_str(),
                    &filesystem
                ) == 0
            ) {
                const auto block_size =
                    static_cast<std::uint64_t>(
                        filesystem.f_frsize
                        ? filesystem.f_frsize
                        : filesystem.f_bsize
                    );

                volume.total_bytes =
                    static_cast<std::uint64_t>(
                        filesystem.f_blocks
                    ) * block_size;

                volume.free_bytes =
                    static_cast<std::uint64_t>(
                        filesystem.f_bavail
                    ) * block_size;

                if (
                    volume.free_bytes >
                    volume.total_bytes
                ) {
                    volume.free_bytes =
                        volume.total_bytes;
                }

                volume.used_bytes =
                    volume.total_bytes -
                    volume.free_bytes;

                if (
                    volume.total_bytes > 0
                ) {
                    volume.used_percent =
                        (
                            static_cast<double>(
                                volume.used_bytes
                            )
                            /
                            static_cast<double>(
                                volume.total_bytes
                            )
                        ) * 100.0;
                }
            }

            volumes.push_back(
                std::move(volume)
            );
        }

        endmntent(mounts);
    }

    auto addOffline =
        [&](const std::string& mount_point,
            const std::string& role) {
            if (
                discovered_mounts.contains(
                    mount_point
                )
            ) {
                return;
            }

            StorageVolume volume;

            volume.mount_point =
                mount_point;

            volume.role =
                role;

            volume.status =
                "offline";

            volumes.push_back(
                std::move(volume)
            );

            discovered_mounts.insert(
                mount_point
            );
        };

    for (const auto& mount : video_mounts) {
        const bool also_personal =
            containsMount(
                personal_mounts,
                mount
            );

        addOffline(
            mount,
            also_personal
                ? "video+personal"
                : "video"
        );
    }

    for (const auto& mount : personal_mounts) {
        addOffline(
            mount,
            "personal"
        );
    }

    std::sort(
        volumes.begin(),
        volumes.end(),
        [](const StorageVolume& a,
           const StorageVolume& b) {
            if (
                a.mount_point == "/"
                &&
                b.mount_point != "/"
            ) {
                return true;
            }

            if (
                a.mount_point != "/"
                &&
                b.mount_point == "/"
            ) {
                return false;
            }

            return
                a.mount_point <
                b.mount_point;
        }
    );

    return volumes;
}

}
