#include "server/storage/DiskOperations.h"

#include <array>
#include <cerrno>
#include <cctype>
#include <cstring>
#include <filesystem>
#include <sys/wait.h>
#include <unistd.h>

namespace homeai {

namespace {

constexpr const char* helper_path =
    "/usr/local/libexec/home-ai-storage-helper";

constexpr const char* sudo_path =
    "/usr/bin/sudo";

std::string basenameOf(
    const std::string& device
)
{
    const auto separator =
        device.find_last_of('/');

    if (
        separator ==
        std::string::npos
    ) {
        return device;
    }

    return device.substr(
        separator + 1
    );
}

std::string filesystemUuid(
    const std::string& device
)
{
    const std::filesystem::path root =
        "/dev/disk/by-uuid";

    std::error_code error;

    if (
        !std::filesystem::exists(
            root,
            error
        )
        ||
        error
    ) {
        return {};
    }

    const auto canonical_device =
        std::filesystem::canonical(
            device,
            error
        );

    if (error)
        return {};

    error.clear();

    for (
        const auto& entry :
        std::filesystem::directory_iterator(
            root,
            error
        )
    ) {
        if (error)
            break;

        std::error_code canonical_error;

        const auto canonical =
            std::filesystem::canonical(
                entry.path(),
                canonical_error
            );

        if (
            !canonical_error
            &&
            canonical ==
                canonical_device
        ) {
            return
                entry.path()
                    .filename()
                    .string();
        }
    }

    return {};
}

}

bool DiskOperations::helperInstalled() const
{
    return
        ::access(
            helper_path,
            X_OK
        ) == 0;
}

std::string DiskOperations::defaultMountPoint(
    const std::string& device,
    const std::string& role
)
{
    const auto name =
        basenameOf(device);

    const auto uuid =
        filesystemUuid(
            device
        );

    const auto stable_name =
        uuid.empty()
        ? name
        : uuid;

    if (role == "video") {
        return
            "/mnt/home-ai/video/" +
            stable_name;
    }

    if (role == "personal") {
        return
            "/mnt/home-ai/files/" +
            stable_name;
    }

    if (role == "vm") {
        return
            "/mnt/home-ai/vm/" +
            stable_name;
    }

    if (role == "storage") {
        return
            "/mnt/home-ai/storage/" +
            stable_name;
    }

    return {};
}

DiskOperationResult
DiskOperations::runHelper(
    const std::vector<std::string>& arguments
) const
{
    DiskOperationResult result;

    if (!helperInstalled()) {
        result.code =
            "helper_not_installed";

        result.message =
            "Привилегированный модуль управления дисками не установлен.";

        return result;
    }

    int pipe_fd[2]{};

    if (::pipe(pipe_fd) != 0) {
        result.code =
            "pipe_failed";

        result.message =
            "Не удалось создать канал для операции.";

        return result;
    }

    const pid_t child =
        ::fork();

    if (child < 0) {
        ::close(pipe_fd[0]);
        ::close(pipe_fd[1]);

        result.code =
            "fork_failed";

        result.message =
            "Не удалось запустить операцию.";

        return result;
    }

    if (child == 0) {
        ::close(pipe_fd[0]);

        ::dup2(
            pipe_fd[1],
            STDOUT_FILENO
        );

        ::dup2(
            pipe_fd[1],
            STDERR_FILENO
        );

        ::close(pipe_fd[1]);

        std::vector<std::string>
            storage;

        storage.reserve(
            arguments.size() + 3
        );

        const bool already_root =
            ::geteuid() == 0;

        if (!already_root) {
            storage.emplace_back(
                sudo_path
            );

            storage.emplace_back(
                "-n"
            );
        }

        storage.emplace_back(
            helper_path
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

        for (auto& item : storage) {
            argv.push_back(
                item.data()
            );
        }

        argv.push_back(nullptr);

        const char* executable =
            already_root
            ? helper_path
            : sudo_path;

        ::execv(
            executable,
            argv.data()
        );

        const std::string error =
            "exec failed: " +
            std::string(
                std::strerror(errno)
            );

        ::write(
            STDOUT_FILENO,
            error.data(),
            error.size()
        );

        _exit(127);
    }

    ::close(pipe_fd[1]);

    std::string output;
    std::array<char, 1024>
        buffer{};

    while (output.size() < 16384) {
        const auto count =
            ::read(
                pipe_fd[0],
                buffer.data(),
                buffer.size()
            );

        if (count <= 0)
            break;

        output.append(
            buffer.data(),
            static_cast<std::size_t>(
                count
            )
        );
    }

    ::close(pipe_fd[0]);

    int status = 0;

    if (
        ::waitpid(
            child,
            &status,
            0
        ) < 0
    ) {
        result.code =
            "wait_failed";

        result.message =
            "Не удалось получить результат операции.";

        return result;
    }

    while (
        !output.empty()
        &&
        (
            output.back() == '\n'
            ||
            output.back() == '\r'
        )
    ) {
        output.pop_back();
    }

    if (
        WIFEXITED(status)
        &&
        WEXITSTATUS(status) == 0
    ) {
        result.success = true;
        result.code = "ok";
        result.message =
            output.empty()
            ? "Операция выполнена."
            : output;

        return result;
    }

    result.code =
        "operation_failed";

    result.message =
        output.empty()
        ? "Операция завершилась ошибкой."
        : output;

    return result;
}

DiskOperationResult
DiskOperations::mount(
    const std::string& device,
    const std::string& role,
    bool read_only
) const
{
    const auto mount_point =
        defaultMountPoint(
            device,
            role
        );

    if (mount_point.empty()) {
        return {
            false,
            "invalid_role",
            "Неизвестное назначение диска."
        };
    }

    return runHelper(
        {
            "mount",
            device,
            mount_point,
            read_only
                ? "ro"
                : "rw"
        }
    );
}

DiskOperationResult
DiskOperations::mountAt(
    const std::string& device,
    const std::string& mount_point,
    bool read_only
) const
{
    if (
        mount_point.rfind(
            "/mnt/home-ai/",
            0
        ) != 0
    ) {
        return {
            false,
            "invalid_mount_point",
            "Некорректная точка монтирования."
        };
    }

    return runHelper(
        {
            "mount",
            device,
            mount_point,
            read_only
                ? "ro"
                : "rw"
        }
    );
}

std::string
DiskOperations::deviceForUuid(
    const std::string& uuid
)
{
    if (
        uuid.empty()
        ||
        uuid.find('/') !=
            std::string::npos
        ||
        uuid.find("..") !=
            std::string::npos
    ) {
        return {};
    }

    for (
        const unsigned char c :
        uuid
    ) {
        if (
            std::isalnum(c)
            ||
            c == '-'
            ||
            c == '_'
            ||
            c == '.'
        ) {
            continue;
        }

        return {};
    }

    std::error_code error;

    const auto canonical =
        std::filesystem::canonical(
            std::filesystem::path(
                "/dev/disk/by-uuid"
            )
            /
            uuid,
            error
        );

    if (error)
        return {};

    const auto value =
        canonical.string();

    if (
        value.rfind(
            "/dev/",
            0
        ) != 0
    ) {
        return {};
    }

    return value;
}

DiskOperationResult
DiskOperations::unmount(
    const std::string& device
) const
{
    return runHelper(
        {
            "unmount",
            device
        }
    );
}

DiskOperationResult
DiskOperations::formatExt4(
    const std::string& device,
    const std::string& label
) const
{
    return runHelper(
        {
            "format-ext4",
            device,
            label
        }
    );
}

DiskOperationResult
DiskOperations::wipeSignatures(
    const std::string& device
) const
{
    return runHelper(
        {
            "wipefs",
            device
        }
    );
}

}
