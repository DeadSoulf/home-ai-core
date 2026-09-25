#include "server/network/NetworkInterfaceManager.h"

#include <algorithm>
#include <array>
#include <arpa/inet.h>
#include <cctype>
#include <filesystem>
#include <fstream>
#include <ifaddrs.h>
#include <map>
#include <net/if.h>
#include <sstream>
#include <string>
#include <sys/socket.h>
#include <sys/wait.h>
#include <unistd.h>
#include <vector>

namespace homeai {

namespace {

constexpr const char* helper_path =
    "/usr/local/libexec/home-ai-network-helper";

constexpr const char* sudo_path =
    "/usr/bin/sudo";

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

    std::size_t start = 0;

    while (
        start < value.size()
        &&
        (
            value[start] == ' '
            ||
            value[start] == '\t'
            ||
            value[start] == '\n'
            ||
            value[start] == '\r'
        )
    ) {
        ++start;
    }

    return value.substr(start);
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

std::uint32_t readUnsigned(
    const std::filesystem::path& path
)
{
    const auto value =
        readTextFile(path);

    if (value.empty())
        return 0;

    try {
        return static_cast<std::uint32_t>(
            std::stoul(value)
        );
    }
    catch (...) {
        return 0;
    }
}

int prefixLength(
    const sockaddr* netmask
)
{
    if (
        netmask == nullptr
        ||
        netmask->sa_family != AF_INET
    ) {
        return 0;
    }

    const auto* address =
        reinterpret_cast<
            const sockaddr_in*
        >(
            netmask
        );

    std::uint32_t mask =
        ntohl(
            address->sin_addr.s_addr
        );

    int bits = 0;

    while (mask != 0) {
        bits +=
            static_cast<int>(
                mask & 1U
            );

        mask >>= 1U;
    }

    return bits;
}

std::string ipv4Address(
    const sockaddr* address,
    const sockaddr* netmask
)
{
    if (
        address == nullptr
        ||
        address->sa_family != AF_INET
    ) {
        return {};
    }

    const auto* ipv4 =
        reinterpret_cast<
            const sockaddr_in*
        >(
            address
        );

    std::array<char, INET_ADDRSTRLEN>
        buffer{};

    if (
        inet_ntop(
            AF_INET,
            &ipv4->sin_addr,
            buffer.data(),
            buffer.size()
        ) == nullptr
    ) {
        return {};
    }

    return
        std::string(
            buffer.data()
        )
        +
        "/"
        +
        std::to_string(
            prefixLength(
                netmask
            )
        );
}

std::string defaultRouteInterface()
{
    std::ifstream file(
        "/proc/net/route"
    );

    if (!file.is_open())
        return {};

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

        std::string interface_name;
        std::string destination;
        std::string gateway;
        std::string flags;

        if (
            !(stream
                >> interface_name
                >> destination
                >> gateway
                >> flags)
        ) {
            continue;
        }

        if (destination != "00000000")
            continue;

        try {
            const auto flag_value =
                std::stoul(
                    flags,
                    nullptr,
                    16
                );

            if (
                (
                    flag_value
                    &
                    0x1U
                ) == 0
            ) {
                continue;
            }
        }
        catch (...) {
            continue;
        }

        return interface_name;
    }

    return {};
}

NetworkActionResult runHelper(
    const std::string& interface_name
)
{
    NetworkActionResult result;

    if (
        ::access(
            helper_path,
            X_OK
        ) != 0
    ) {
        result.code =
            "helper_not_installed";

        result.message =
            "Network Helper не установлен.";

        return result;
    }

    int pipe_fd[2]{};

    if (::pipe(pipe_fd) != 0) {
        result.code =
            "pipe_failed";

        result.message =
            "Не удалось создать канал сетевой операции.";

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
            "Не удалось запустить сетевую операцию.";

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

        const bool already_root =
            ::geteuid() == 0;

        if (already_root) {
            ::execl(
                helper_path,
                helper_path,
                "dhcp",
                interface_name.c_str(),
                static_cast<char*>(nullptr)
            );
        }
        else {
            ::execl(
                sudo_path,
                sudo_path,
                "-n",
                helper_path,
                "dhcp",
                interface_name.c_str(),
                static_cast<char*>(nullptr)
            );
        }

        _exit(127);
    }

    ::close(pipe_fd[1]);

    std::string output;

    std::array<char, 2048>
        buffer{};

    while (true) {
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

        if (output.size() > 16384) {
            output.erase(
                0,
                output.size() - 16384
            );
        }
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
            "Не удалось получить результат сетевой операции.";

        return result;
    }

    output =
        trimCopy(
            output
        );

    result.success =
        WIFEXITED(status)
        &&
        WEXITSTATUS(status) == 0;

    result.code =
        result.success
        ? "ok"
        : "dhcp_failed";

    result.message =
        output.empty()
        ? (
            result.success
            ? "DHCP-запрос выполнен."
            : "Не удалось получить IP по DHCP."
        )
        : output;

    return result;
}

}

bool
NetworkInterfaceManager::validInterfaceName(
    const std::string& interface_name
)
{
    if (
        interface_name.empty()
        ||
        interface_name.size() >=
            IFNAMSIZ
        ||
        interface_name == "lo"
    ) {
        return false;
    }

    return std::all_of(
        interface_name.begin(),
        interface_name.end(),
        [](unsigned char c) {
            return
                std::isalnum(c)
                ||
                c == '_'
                ||
                c == '-'
                ||
                c == '.';
        }
    );
}

std::vector<NetworkInterfaceInfo>
NetworkInterfaceManager::interfaces() const
{
    std::map<
        std::string,
        NetworkInterfaceInfo
    > by_name;

    ifaddrs* addresses = nullptr;

    if (
        getifaddrs(
            &addresses
        ) == 0
    ) {
        for (
            auto* entry = addresses;
            entry != nullptr;
            entry = entry->ifa_next
        ) {
            if (
                entry->ifa_name ==
                    nullptr
                ||
                entry->ifa_addr ==
                    nullptr
            ) {
                continue;
            }

            auto& info =
                by_name[
                    entry->ifa_name
                ];

            info.name =
                entry->ifa_name;

            info.up =
                (
                    entry->ifa_flags
                    &
                    IFF_UP
                ) != 0;

            info.loopback =
                (
                    entry->ifa_flags
                    &
                    IFF_LOOPBACK
                ) != 0;

            if (
                entry->ifa_addr->sa_family
                ==
                AF_INET
            ) {
                const auto value =
                    ipv4Address(
                        entry->ifa_addr,
                        entry->ifa_netmask
                    );

                if (
                    !value.empty()
                    &&
                    std::find(
                        info.ipv4_addresses.begin(),
                        info.ipv4_addresses.end(),
                        value
                    ) ==
                        info.ipv4_addresses.end()
                ) {
                    info.ipv4_addresses.
                        push_back(
                            value
                        );
                }
            }
        }

        freeifaddrs(
            addresses
        );
    }

    std::error_code error;

    const std::filesystem::path root =
        "/sys/class/net";

    if (
        std::filesystem::exists(
            root,
            error
        )
        &&
        !error
    ) {
        for (
            const auto& entry :
            std::filesystem::
                directory_iterator(
                    root,
                    error
                )
        ) {
            if (error)
                break;

            const auto name =
                entry.path()
                    .filename()
                    .string();

            auto& info =
                by_name[name];

            info.name = name;

            info.mac_address =
                readTextFile(
                    entry.path() /
                    "address"
                );

            info.oper_state =
                readTextFile(
                    entry.path() /
                    "operstate"
                );

            info.mtu =
                readUnsigned(
                    entry.path() /
                    "mtu"
                );

            const auto carrier =
                readTextFile(
                    entry.path() /
                    "carrier"
                );

            info.carrier =
                carrier == "1";

            if (name == "lo")
                info.loopback = true;
        }
    }

    const auto default_route =
        defaultRouteInterface();

    std::vector<NetworkInterfaceInfo>
        result;

    for (auto& [name, info] : by_name) {
        info.default_route =
            name == default_route;

        std::sort(
            info.ipv4_addresses.begin(),
            info.ipv4_addresses.end()
        );

        result.push_back(
            std::move(info)
        );
    }

    std::sort(
        result.begin(),
        result.end(),
        [](const NetworkInterfaceInfo& a,
           const NetworkInterfaceInfo& b) {
            if (
                a.loopback !=
                b.loopback
            ) {
                return
                    !a.loopback;
            }

            if (
                a.default_route !=
                b.default_route
            ) {
                return
                    a.default_route;
            }

            return
                a.name <
                b.name;
        }
    );

    return result;
}

bool
NetworkInterfaceManager::helperInstalled()
    const
{
    return
        ::access(
            helper_path,
            X_OK
        ) == 0;
}

NetworkActionResult
NetworkInterfaceManager::requestDhcp(
    const std::string& interface_name
) const
{
    if (
        !validInterfaceName(
            interface_name
        )
    ) {
        return {
            false,
            "invalid_interface",
            "Некорректный сетевой интерфейс."
        };
    }

    if (
        !std::filesystem::exists(
            std::filesystem::path(
                "/sys/class/net"
            )
            /
            interface_name
        )
    ) {
        return {
            false,
            "interface_not_found",
            "Сетевой интерфейс не найден."
        };
    }

    return
        runHelper(
            interface_name
        );
}

}
