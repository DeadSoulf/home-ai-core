#include "server/network/WireGuardManager.h"

#include <algorithm>
#include <array>
#include <cctype>
#include <filesystem>
#include <fstream>
#include <iterator>
#include <sys/stat.h>
#include <sys/wait.h>
#include <unistd.h>
#include <vector>

namespace homeai {

namespace {

std::string trimCopy(
    std::string value
)
{
    while (
        !value.empty()
        &&
        (
            value.back() == '\n'
            ||
            value.back() == '\r'
            ||
            value.back() == ' '
            ||
            value.back() == '\t'
        )
    ) {
        value.pop_back();
    }

    return value;
}

std::string findWgQuick()
{
    const char* paths[] = {
        "/usr/bin/wg-quick",
        "/usr/sbin/wg-quick"
    };

    for (const auto* path : paths) {
        if (
            ::access(
                path,
                X_OK
            ) == 0
        ) {
            return path;
        }
    }

    return {};
}

}

bool WireGuardManager::initialize(
    const std::string& config_directory
)
{
    if (config_directory.empty())
        return false;

    std::error_code error;

    std::filesystem::create_directories(
        config_directory,
        error
    );

    if (error)
        return false;

    ::chmod(
        config_directory.c_str(),
        0700
    );

    const auto canonical =
        std::filesystem::canonical(
            config_directory,
            error
        );

    if (error)
        return false;

    config_directory_ =
        canonical.string();

    return true;
}

bool WireGuardManager::available() const
{
    return !findWgQuick().empty();
}

bool WireGuardManager::validProfileName(
    const std::string& profile
)
{
    if (
        profile.empty()
        ||
        profile.size() > 32
    ) {
        return false;
    }

    return std::all_of(
        profile.begin(),
        profile.end(),
        [](unsigned char c) {
            return
                std::isalnum(c)
                ||
                c == '-'
                ||
                c == '_';
        }
    );
}

std::string WireGuardManager::profilePath(
    const std::string& profile
) const
{
    if (
        config_directory_.empty()
        ||
        !validProfileName(
            profile
        )
    ) {
        return {};
    }

    return (
        std::filesystem::path(
            config_directory_
        )
        /
        (
            profile +
            ".conf"
        )
    ).string();
}

std::vector<WireGuardProfile>
WireGuardManager::profiles(
    std::string& error
) const
{
    std::vector<WireGuardProfile>
        result;

    if (config_directory_.empty()) {
        error =
            "WireGuard manager is not initialized.";

        return result;
    }

    std::error_code fs_error;

    for (
        const auto& entry :
        std::filesystem::
            directory_iterator(
                config_directory_,
                fs_error
            )
    ) {
        if (fs_error) {
            error =
                fs_error.message();

            break;
        }

        if (
            !entry.is_regular_file()
            ||
            entry.path()
                .extension() !=
                ".conf"
        ) {
            continue;
        }

        const auto name =
            entry.path()
                .stem()
                .string();

        if (!validProfileName(name))
            continue;

        WireGuardProfile profile;

        profile.name =
            name;

        profile.active =
            std::filesystem::exists(
                std::filesystem::path(
                    "/sys/class/net"
                )
                /
                name
            );

        result.push_back(
            std::move(profile)
        );
    }

    std::sort(
        result.begin(),
        result.end(),
        [](const WireGuardProfile& a,
           const WireGuardProfile& b) {
            return a.name < b.name;
        }
    );

    return result;
}

WireGuardConfigResult
WireGuardManager::loadProfile(
    const std::string& profile
) const
{
    if (!validProfileName(profile)) {
        return {
            false,
            "invalid_profile",
            "Некорректное имя профиля.",
            {}
        };
    }

    const auto path =
        profilePath(profile);

    if (path.empty()) {
        return {
            false,
            "not_initialized",
            "WireGuard Manager не инициализирован.",
            {}
        };
    }

    std::error_code error;

    const auto status =
        std::filesystem::symlink_status(
            path,
            error
        );

    if (error) {
        return {
            false,
            "read_failed",
            "Не удалось прочитать профиль WireGuard.",
            {}
        };
    }

    if (
        !std::filesystem::is_regular_file(
            status
        )
    ) {
        return {
            false,
            "profile_not_found",
            "Профиль WireGuard не найден.",
            {}
        };
    }

    const auto size =
        std::filesystem::file_size(
            path,
            error
        );

    if (
        error
        ||
        size > 65536
    ) {
        return {
            false,
            "read_failed",
            "Конфигурация WireGuard слишком большая или недоступна.",
            {}
        };
    }

    std::ifstream file(
        path,
        std::ios::binary
    );

    if (!file.is_open()) {
        return {
            false,
            "read_failed",
            "Не удалось открыть конфигурацию WireGuard.",
            {}
        };
    }

    std::string config(
        (
            std::istreambuf_iterator<char>(
                file
            )
        ),
        std::istreambuf_iterator<char>()
    );

    if (file.bad()) {
        return {
            false,
            "read_failed",
            "Не удалось прочитать конфигурацию WireGuard.",
            {}
        };
    }

    return {
        true,
        "ok",
        "Профиль WireGuard загружен.",
        std::move(config)
    };
}

WireGuardResult
WireGuardManager::saveProfile(
    const std::string& profile,
    const std::string& config
) const
{
    if (!validProfileName(profile)) {
        return {
            false,
            "invalid_profile",
            "Некорректное имя профиля."
        };
    }

    if (
        config.empty()
        ||
        config.size() > 65536
        ||
        config.find('\0') !=
            std::string::npos
        ||
        config.find(
            "[Interface]"
        ) == std::string::npos
    ) {
        return {
            false,
            "invalid_config",
            "Конфигурация WireGuard некорректна."
        };
    }

    const auto path =
        profilePath(profile);

    if (path.empty()) {
        return {
            false,
            "not_initialized",
            "WireGuard Manager не инициализирован."
        };
    }

    const auto temp =
        path + ".tmp";

    {
        std::ofstream file(
            temp,
            std::ios::trunc
        );

        if (!file.is_open()) {
            return {
                false,
                "write_failed",
                "Не удалось сохранить конфигурацию WireGuard."
            };
        }

        file << config;
    }

    ::chmod(
        temp.c_str(),
        0600
    );

    std::error_code error;

    std::filesystem::rename(
        temp,
        path,
        error
    );

    if (error) {
        std::filesystem::remove(
            temp
        );

        return {
            false,
            "rename_failed",
            "Не удалось активировать конфигурацию WireGuard."
        };
    }

    ::chmod(
        path.c_str(),
        0600
    );

    return {
        true,
        "ok",
        "Профиль WireGuard сохранён."
    };
}

WireGuardResult
WireGuardManager::removeProfile(
    const std::string& profile
) const
{
    if (!validProfileName(profile)) {
        return {
            false,
            "invalid_profile",
            "Некорректное имя профиля."
        };
    }

    const auto path =
        profilePath(profile);

    if (path.empty()) {
        return {
            false,
            "not_initialized",
            "WireGuard Manager не инициализирован."
        };
    }

    if (
        std::filesystem::exists(
            std::filesystem::path(
                "/sys/class/net"
            )
            /
            profile
        )
    ) {
        return {
            false,
            "profile_active",
            "Сначала отключите WireGuard."
        };
    }

    std::error_code error;

    const bool removed =
        std::filesystem::remove(
            path,
            error
        );

    if (
        error
        ||
        !removed
    ) {
        return {
            false,
            "remove_failed",
            "Не удалось удалить профиль WireGuard."
        };
    }

    return {
        true,
        "ok",
        "Профиль WireGuard удалён."
    };
}

WireGuardManager::CommandResult
WireGuardManager::runWgQuick(
    const std::string& action,
    const std::string& profile
) const
{
    CommandResult result;

    if (
        action != "up"
        &&
        action != "down"
    ) {
        result.output =
            "Unsupported WireGuard action.";

        return result;
    }

    const auto executable =
        findWgQuick();

    if (executable.empty()) {
        result.output =
            "wg-quick not found. Install wireguard-tools.";

        return result;
    }

    const auto path =
        profilePath(profile);

    if (
        path.empty()
        ||
        !std::filesystem::exists(
            path
        )
    ) {
        result.output =
            "WireGuard profile not found.";

        return result;
    }

    int pipe_fd[2]{};

    if (::pipe(pipe_fd) != 0) {
        result.output =
            "Unable to create WireGuard process pipe.";

        return result;
    }

    const pid_t child =
        ::fork();

    if (child < 0) {
        ::close(pipe_fd[0]);
        ::close(pipe_fd[1]);

        result.output =
            "Unable to start wg-quick.";

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
            storage = {
                executable,
                action,
                path
            };

        std::vector<char*>
            argv;

        for (auto& item : storage) {
            argv.push_back(
                item.data()
            );
        }

        argv.push_back(nullptr);

        ::execv(
            executable.c_str(),
            argv.data()
        );

        _exit(127);
    }

    ::close(pipe_fd[1]);

    std::array<char, 2048>
        buffer{};

    std::string output;

    while (output.size() < 32768) {
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
        result.output =
            "Unable to obtain wg-quick result.";

        return result;
    }

    result.output =
        trimCopy(output);

    if (WIFEXITED(status)) {
        result.exit_code =
            WEXITSTATUS(status);
    }

    return result;
}

WireGuardResult
WireGuardManager::connect(
    const std::string& profile
) const
{
    if (!validProfileName(profile)) {
        return {
            false,
            "invalid_profile",
            "Некорректное имя профиля."
        };
    }

    const auto result =
        runWgQuick(
            "up",
            profile
        );

    return {
        result.exit_code == 0,
        result.exit_code == 0
            ? "ok"
            : "operation_failed",
        result.output.empty()
            ? (
                result.exit_code == 0
                ? "WireGuard подключён."
                : "Не удалось подключить WireGuard."
            )
            : result.output
    };
}

WireGuardResult
WireGuardManager::disconnect(
    const std::string& profile
) const
{
    if (!validProfileName(profile)) {
        return {
            false,
            "invalid_profile",
            "Некорректное имя профиля."
        };
    }

    const auto result =
        runWgQuick(
            "down",
            profile
        );

    return {
        result.exit_code == 0,
        result.exit_code == 0
            ? "ok"
            : "operation_failed",
        result.output.empty()
            ? (
                result.exit_code == 0
                ? "WireGuard отключён."
                : "Не удалось отключить WireGuard."
            )
            : result.output
    };
}

}
