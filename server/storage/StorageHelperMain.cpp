#include "server/storage/StorageMonitor.h"

#include <algorithm>
#include <cerrno>
#include <cctype>
#include <filesystem>
#include <iostream>
#include <string>
#include <sys/stat.h>
#include <sys/wait.h>
#include <unistd.h>
#include <vector>

namespace {

bool safeDevicePath(
    const std::string& device
)
{
    if (
        device.rfind(
            "/dev/",
            0
        ) != 0
        ||
        device.find("..") !=
            std::string::npos
    ) {
        return false;
    }

    for (
        const unsigned char c :
        device
    ) {
        if (
            std::isalnum(c)
            ||
            c == '/'
            ||
            c == '-'
            ||
            c == '_'
            ||
            c == '.'
        ) {
            continue;
        }

        return false;
    }

    struct stat status{};

    if (
        ::stat(
            device.c_str(),
            &status
        ) != 0
    ) {
        return false;
    }

    return
        S_ISBLK(
            status.st_mode
        );
}

bool safeMountPoint(
    const std::string& path
)
{
    const std::string video_prefix =
        "/mnt/home-ai/video/";

    const std::string files_prefix =
        "/mnt/home-ai/files/";

    const bool prefix_ok =
        path.rfind(
            video_prefix,
            0
        ) == 0
        ||
        path.rfind(
            files_prefix,
            0
        ) == 0;

    if (
        !prefix_ok
        ||
        path.find("..") !=
            std::string::npos
    ) {
        return false;
    }

    for (
        const unsigned char c :
        path
    ) {
        if (
            std::isalnum(c)
            ||
            c == '/'
            ||
            c == '-'
            ||
            c == '_'
            ||
            c == '.'
        ) {
            continue;
        }

        return false;
    }

    return true;
}

bool safeLabel(
    const std::string& label
)
{
    if (
        label.empty()
        ||
        label.size() > 32
    ) {
        return false;
    }

    return std::all_of(
        label.begin(),
        label.end(),
        [](unsigned char c) {
            return
                std::isalnum(c)
                ||
                c == '-'
                ||
                c == '_'
                ||
                c == '.';
        }
    );
}

const homeai::BlockDeviceInfo*
findDevice(
    const std::vector<
        homeai::BlockDeviceInfo
    >& devices,
    const std::string& device
)
{
    for (
        const auto& item :
        devices
    ) {
        if (
            item.device ==
            device
        ) {
            return &item;
        }
    }

    return nullptr;
}

std::string findExecutable(
    const std::vector<std::string>& paths
)
{
    for (const auto& path : paths) {
        if (
            ::access(
                path.c_str(),
                X_OK
            ) == 0
        ) {
            return path;
        }
    }

    return {};
}

int runCommand(
    const std::string& executable,
    const std::vector<std::string>& arguments
)
{
    if (executable.empty())
        return 127;

    const pid_t child =
        ::fork();

    if (child < 0)
        return 126;

    if (child == 0) {
        std::vector<std::string>
            storage;

        storage.reserve(
            arguments.size() + 1
        );

        storage.push_back(
            executable
        );

        for (
            const auto& argument :
            arguments
        ) {
            storage.push_back(
                argument
            );
        }

        std::vector<char*>
            argv;

        argv.reserve(
            storage.size() + 1
        );

        for (auto& item : storage)
            argv.push_back(
                item.data()
            );

        argv.push_back(nullptr);

        ::execv(
            executable.c_str(),
            argv.data()
        );

        _exit(127);
    }

    int status = 0;

    if (
        ::waitpid(
            child,
            &status,
            0
        ) < 0
    ) {
        return 125;
    }

    if (WIFEXITED(status))
        return WEXITSTATUS(status);

    return 124;
}

bool mountedInsideHomeAI(
    const homeai::BlockDeviceInfo& info
)
{
    return
        info.mounted
        &&
        (
            info.mount_point.rfind(
                "/mnt/home-ai/video/",
                0
            ) == 0
            ||
            info.mount_point.rfind(
                "/mnt/home-ai/files/",
                0
            ) == 0
        );
}

int fail(
    const std::string& message
)
{
    std::cerr
        << message
        << '\n';

    return 1;
}

}

int main(
    int argc,
    char** argv
)
{
    if (::geteuid() != 0) {
        return fail(
            "Storage helper must run as root."
        );
    }

    if (argc < 3) {
        return fail(
            "Invalid storage helper arguments."
        );
    }

    const std::string action =
        argv[1];

    const std::string device =
        argv[2];

    if (!safeDevicePath(device)) {
        return fail(
            "Invalid block device."
        );
    }

    homeai::StorageMonitor monitor;

    const auto devices =
        monitor.blockDevices();

    const auto* info =
        findDevice(
            devices,
            device
        );

    if (info == nullptr) {
        return fail(
            "Device is not present in the current block inventory."
        );
    }

    if (action == "mount") {
        if (argc != 5) {
            return fail(
                "Invalid mount arguments."
            );
        }

        const std::string mount_point =
            argv[3];

        const std::string mode =
            argv[4];

        if (
            !safeMountPoint(
                mount_point
            )
            ||
            (
                mode != "ro"
                &&
                mode != "rw"
            )
        ) {
            return fail(
                "Invalid mount request."
            );
        }

        if (
            info->mounted
            ||
            info->in_use
        ) {
            return fail(
                "Device is already in use."
            );
        }

        if (!info->candidate) {
            return fail(
                "Device is not eligible for mounting."
            );
        }

        std::error_code error;

        std::filesystem::create_directories(
            mount_point,
            error
        );

        if (error) {
            return fail(
                "Unable to create mount point."
            );
        }

        ::chmod(
            mount_point.c_str(),
            0750
        );

        const auto mount_bin =
            findExecutable(
                {
                    "/usr/bin/mount",
                    "/bin/mount"
                }
            );

        std::string options =
            "nodev,nosuid,noexec";

        if (mode == "ro")
            options += ",ro";

        const int code =
            runCommand(
                mount_bin,
                {
                    "-o",
                    options,
                    device,
                    mount_point
                }
            );

        if (code != 0) {
            return fail(
                "Mount failed. The disk may not contain a supported filesystem."
            );
        }

        std::cout
            << "Диск смонтирован: "
            << device
            << " -> "
            << mount_point
            << '\n';

        return 0;
    }

    if (action == "unmount") {
        if (argc != 3) {
            return fail(
                "Invalid unmount arguments."
            );
        }

        if (
            !mountedInsideHomeAI(
                *info
            )
        ) {
            return fail(
                "Only Home AI managed mount points can be unmounted."
            );
        }

        const auto umount_bin =
            findExecutable(
                {
                    "/usr/bin/umount",
                    "/bin/umount"
                }
            );

        const int code =
            runCommand(
                umount_bin,
                {
                    device
                }
            );

        if (code != 0) {
            return fail(
                "Unmount failed. The filesystem may still be busy."
            );
        }

        std::cout
            << "Диск размонтирован: "
            << device
            << '\n';

        return 0;
    }

    if (action == "format-ext4") {
        if (argc != 4) {
            return fail(
                "Invalid format arguments."
            );
        }

        const std::string label =
            argv[3];

        if (!safeLabel(label)) {
            return fail(
                "Invalid filesystem label."
            );
        }

        if (
            info->mounted
            ||
            info->in_use
            ||
            !info->candidate
        ) {
            return fail(
                "Device is in use or not eligible for formatting."
            );
        }

        const auto mkfs =
            findExecutable(
                {
                    "/usr/sbin/mkfs.ext4",
                    "/sbin/mkfs.ext4"
                }
            );

        const int code =
            runCommand(
                mkfs,
                {
                    "-F",
                    "-L",
                    label,
                    device
                }
            );

        if (code != 0) {
            return fail(
                "EXT4 formatting failed."
            );
        }

        std::cout
            << "EXT4 создан на "
            << device
            << " с меткой "
            << label
            << '\n';

        return 0;
    }

    if (action == "wipefs") {
        if (argc != 3) {
            return fail(
                "Invalid wipe arguments."
            );
        }

        if (
            info->mounted
            ||
            info->in_use
            ||
            !info->candidate
        ) {
            return fail(
                "Device is in use or not eligible for signature removal."
            );
        }

        const auto wipefs =
            findExecutable(
                {
                    "/usr/sbin/wipefs",
                    "/sbin/wipefs"
                }
            );

        const int code =
            runCommand(
                wipefs,
                {
                    "-a",
                    device
                }
            );

        if (code != 0) {
            return fail(
                "Filesystem signature removal failed."
            );
        }

        std::cout
            << "Сигнатуры файловой системы удалены с "
            << device
            << '\n';

        return 0;
    }

    return fail(
        "Unsupported storage action."
    );
}
