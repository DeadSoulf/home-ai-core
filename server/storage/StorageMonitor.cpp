#include "server/storage/StorageMonitor.h"

#include <algorithm>
#include <cctype>
#include <cstdio>
#include <filesystem>
#include <fstream>
#include <map>
#include <mntent.h>
#include <set>
#include <sstream>
#include <string>
#include <sys/statvfs.h>
#include <utility>

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

bool ignoredKernelDevice(
    const std::string& name
)
{
    const char* prefixes[] = {
        "loop",
        "ram",
        "zram",
        "fd",
        "sr",
        "dm-",
        "md"
    };

    for (const auto* prefix : prefixes) {
        if (
            name.rfind(
                prefix,
                0
            ) == 0
        ) {
            return true;
        }
    }

    return false;
}

std::string readTextFile(
    const std::filesystem::path& path
)
{
    std::ifstream file(path);

    if (!file.is_open())
        return {};

    std::string value;

    std::getline(
        file,
        value
    );

    return trimCopy(value);
}

bool directoryHasEntries(
    const std::filesystem::path& path
)
{
    std::error_code error;

    if (
        !std::filesystem::exists(
            path,
            error
        )
        ||
        error
    ) {
        return false;
    }

    const auto begin =
        std::filesystem::directory_iterator(
            path,
            error
        );

    if (error)
        return false;

    return begin !=
        std::filesystem::directory_iterator();
}

std::uint64_t readUnsigned(
    const std::filesystem::path& path
)
{
    const auto value =
        readTextFile(path);

    if (value.empty())
        return 0;

    try {
        return static_cast<std::uint64_t>(
            std::stoull(value)
        );
    }
    catch (...) {
        return 0;
    }
}

std::map<std::string, std::string>
mountedDevices()
{
    std::map<std::string, std::string>
        mounted;

    FILE* mounts =
        setmntent(
            "/proc/self/mounts",
            "r"
        );

    if (mounts == nullptr)
        return mounted;

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
            entry.mnt_dir
            ? entry.mnt_dir
            : "";

        if (
            isBlockDeviceSource(source)
            &&
            !mount_point.empty()
        ) {
            mounted[source] =
                mount_point;

            std::error_code error;

            const auto canonical =
                std::filesystem::canonical(
                    source,
                    error
                );

            if (!error) {
                mounted[
                    canonical.string()
                ] = mount_point;
            }
        }
    }

    endmntent(mounts);

    return mounted;
}

std::uint64_t blockDeviceSizeBytes(
    const std::string& source
)
{
    if (
        source.empty()
        ||
        source.rfind(
            "/dev/",
            0
        ) != 0
    ) {
        return 0;
    }

    std::filesystem::path device_path =
        source;

    std::error_code error;

    const auto canonical =
        std::filesystem::canonical(
            device_path,
            error
        );

    if (!error)
        device_path = canonical;

    const auto name =
        device_path
            .filename()
            .string();

    if (name.empty())
        return 0;

    const auto sectors =
        readUnsigned(
            std::filesystem::path(
                "/sys/class/block"
            )
            /
            name
            /
            "size"
        );

    return
        sectors * 512ULL;
}

std::set<std::string>
swapDevices()
{
    std::set<std::string> result;

    std::ifstream file(
        "/proc/swaps"
    );

    if (!file.is_open())
        return result;

    std::string line;

    std::getline(
        file,
        line
    );

    while (
        std::getline(
            file,
            line
        )
    ) {
        std::istringstream stream(line);
        std::string source;

        if (stream >> source) {
            if (
                isBlockDeviceSource(
                    source
                )
            ) {
                result.insert(source);
            }
        }
    }

    return result;
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

            volume.device_size_bytes =
                blockDeviceSizeBytes(
                    source
                );

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

                    volume.capacity_available =
                        true;
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

std::vector<BlockDeviceInfo>
StorageMonitor::blockDevices() const
{
    std::vector<BlockDeviceInfo>
        devices;

    const std::filesystem::path sys_block =
        "/sys/class/block";

    if (
        !std::filesystem::exists(
            sys_block
        )
    ) {
        return devices;
    }

    const auto mounted =
        mountedDevices();

    const auto swaps =
        swapDevices();

    std::set<std::string>
        partition_parents;

    for (
        const auto& entry :
        std::filesystem::directory_iterator(
            sys_block
        )
    ) {
        const auto name =
            entry.path().filename().string();

        if (
            ignoredKernelDevice(name)
        ) {
            continue;
        }

        const auto partition_value =
            readTextFile(
                entry.path() /
                "partition"
            );

        if (partition_value.empty())
            continue;

        std::error_code error;

        const auto canonical =
            std::filesystem::canonical(
                entry.path(),
                error
            );

        if (error)
            continue;

        const auto parent =
            canonical.parent_path()
                .filename()
                .string();

        if (!parent.empty())
            partition_parents.insert(
                parent
            );
    }

    for (
        const auto& entry :
        std::filesystem::directory_iterator(
            sys_block
        )
    ) {
        const auto name =
            entry.path().filename().string();

        if (
            ignoredKernelDevice(name)
        ) {
            continue;
        }

        BlockDeviceInfo device;

        device.name =
            name;

        device.device =
            "/dev/" + name;

        const bool partition =
            !readTextFile(
                entry.path() /
                "partition"
            ).empty();

        device.type =
            partition
            ? "partition"
            : "disk";

        if (partition) {
            std::error_code error;

            const auto canonical =
                std::filesystem::canonical(
                    entry.path(),
                    error
                );

            if (!error) {
                device.parent =
                    canonical.parent_path()
                        .filename()
                        .string();
            }
        }

        const auto metadata_name =
            partition
            && !device.parent.empty()
            ? device.parent
            : name;

        const auto metadata_path =
            sys_block /
            metadata_name;

        device.model =
            readTextFile(
                metadata_path /
                "device/model"
            );

        device.vendor =
            readTextFile(
                metadata_path /
                "device/vendor"
            );

        device.serial =
            readTextFile(
                metadata_path /
                "device/serial"
            );

        device.size_bytes =
            readUnsigned(
                entry.path() /
                "size"
            ) * 512ULL;

        device.removable =
            readUnsigned(
                metadata_path /
                "removable"
            ) != 0;

        device.has_partitions =
            partition_parents.contains(
                name
            );

        auto mounted_it =
            mounted.find(
                device.device
            );

        if (
            mounted_it ==
            mounted.end()
        ) {
            std::error_code error;

            const auto canonical =
                std::filesystem::canonical(
                    device.device,
                    error
                );

            if (!error) {
                mounted_it =
                    mounted.find(
                        canonical.string()
                    );
            }
        }

        if (
            mounted_it !=
            mounted.end()
        ) {
            device.mounted = true;

            device.mount_point =
                mounted_it->second;
        }

        const bool is_swap =
            swaps.contains(
                device.device
            );

        device.in_use =
            device.mounted
            ||
            is_swap
            ||
            directoryHasEntries(
                entry.path() /
                "holders"
            );

        device.candidate =
            !device.in_use
            &&
            device.size_bytes > 0
            &&
            (
                partition
                ||
                !device.has_partitions
            );

        devices.push_back(
            std::move(device)
        );
    }

    std::sort(
        devices.begin(),
        devices.end(),
        [](const BlockDeviceInfo& a,
           const BlockDeviceInfo& b) {
            if (
                a.candidate !=
                b.candidate
            ) {
                return
                    a.candidate >
                    b.candidate;
            }

            return
                a.device <
                b.device;
        }
    );

    return devices;
}

}
