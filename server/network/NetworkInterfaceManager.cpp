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
#include <utility>
#include <vector>

namespace homeai {

namespace {

constexpr const char* helper_path =
    "/usr/local/libexec/home-ai-network-helper";

constexpr const char* sudo_path =
    "/usr/bin/sudo";

struct CommandResult {
    int exit_code{-1};
    std::string output;
};

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

CommandResult runCommand(
    const std::string& executable,
    const std::vector<std::string>& arguments
)
{
    CommandResult result;

    int pipe_fd[2]{};

    if (::pipe(pipe_fd) != 0)
        return result;

    const pid_t child =
        ::fork();

    if (child < 0) {
        ::close(pipe_fd[0]);
        ::close(pipe_fd[1]);

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

        storage.push_back(
            executable
        );

        for (const auto& argument : arguments)
            storage.push_back(argument);

        std::vector<char*> argv;

        for (auto& item : storage)
            argv.push_back(item.data());

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

    while (true) {
        const auto count =
            ::read(
                pipe_fd[0],
                buffer.data(),
                buffer.size()
            );

        if (count <= 0)
            break;

        result.output.append(
            buffer.data(),
            static_cast<std::size_t>(
                count
            )
        );

        if (result.output.size() > 16384) {
            result.output.erase(
                0,
                result.output.size() - 16384
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
        ) >= 0
        &&
        WIFEXITED(status)
    ) {
        result.exit_code =
            WEXITSTATUS(status);
    }

    result.output =
        trimCopy(
            result.output
        );

    return result;
}

std::string gatewayHexToIpv4(
    const std::string& value
)
{
    if (value.size() != 8)
        return {};

    try {
        const auto raw =
            static_cast<std::uint32_t>(
                std::stoul(
                    value,
                    nullptr,
                    16
                )
            );

        std::array<unsigned int, 4>
            octets{
                raw & 0xffU,
                (raw >> 8U) & 0xffU,
                (raw >> 16U) & 0xffU,
                (raw >> 24U) & 0xffU
            };

        return
            std::to_string(octets[0])
            + "."
            + std::to_string(octets[1])
            + "."
            + std::to_string(octets[2])
            + "."
            + std::to_string(octets[3]);
    }
    catch (...) {
        return {};
    }
}

std::map<std::string, std::string>
defaultGateways()
{
    std::map<std::string, std::string>
        gateways;

    std::ifstream file(
        "/proc/net/route"
    );

    if (!file.is_open())
        return gateways;

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

        const auto address =
            gatewayHexToIpv4(
                gateway
            );

        if (!address.empty()) {
            gateways[
                interface_name
            ] = address;
        }
    }

    return gateways;
}

std::vector<std::string>
dnsServers()
{
    std::vector<std::string> result;

    const std::filesystem::path paths[] = {
        "/run/systemd/resolve/resolv.conf",
        "/etc/resolv.conf"
    };

    for (const auto& path : paths) {
        std::ifstream file(path);

        if (!file.is_open())
            continue;

        std::string line;

        while (
            std::getline(
                file,
                line
            )
        ) {
            std::istringstream stream(line);

            std::string key;
            std::string value;

            if (
                !(stream >> key >> value)
                ||
                key != "nameserver"
            ) {
                continue;
            }

            in_addr address{};

            if (
                inet_pton(
                    AF_INET,
                    value.c_str(),
                    &address
                ) != 1
            ) {
                continue;
            }

            const auto host =
                ntohl(
                    address.s_addr
                );

            if (
                (
                    host
                    &
                    0xff000000U
                ) ==
                0x7f000000U
            ) {
                continue;
            }

            if (
                std::find(
                    result.begin(),
                    result.end(),
                    value
                ) == result.end()
            ) {
                result.push_back(
                    value
                );
            }

            if (result.size() >= 2)
                return result;
        }
    }

    return result;
}

std::string networkManagerMethod(
    const std::string& interface_name
)
{
    const auto nmcli =
        findExecutable(
            {
                "/usr/bin/nmcli",
                "/bin/nmcli"
            }
        );

    if (nmcli.empty())
        return {};

    const auto connection =
        runCommand(
            nmcli,
            {
                "-g",
                "GENERAL.CONNECTION",
                "device",
                "show",
                interface_name
            }
        );

    if (
        connection.exit_code != 0
        ||
        connection.output.empty()
        ||
        connection.output == "--"
    ) {
        return {};
    }

    const auto method =
        runCommand(
            nmcli,
            {
                "-g",
                "ipv4.method",
                "connection",
                "show",
                connection.output
            }
        );

    if (method.exit_code != 0)
        return {};

    if (method.output == "auto")
        return "dhcp";

    if (method.output == "manual")
        return "static";

    return {};
}

std::string ifupdownMethod(
    const std::string& interface_name
)
{
    std::vector<std::filesystem::path>
        files{
            "/etc/network/interfaces"
        };

    std::error_code error;

    const std::filesystem::path directory =
        "/etc/network/interfaces.d";

    if (
        std::filesystem::exists(
            directory,
            error
        )
        &&
        !error
    ) {
        for (
            const auto& entry :
            std::filesystem::
                directory_iterator(
                    directory,
                    error
                )
        ) {
            if (error)
                break;

            std::error_code type_error;

            if (
                !entry.is_regular_file(
                    type_error
                )
                ||
                type_error
            ) {
                continue;
            }

            const auto filename =
                entry.path()
                    .filename()
                    .string();

            if (
                filename.ends_with(
                    ".home-ai.bak"
                )
                ||
                filename.ends_with(
                    ".home-ai.tmp"
                )
            ) {
                continue;
            }

            files.push_back(
                entry.path()
            );
        }
    }

    for (const auto& path : files) {
        std::ifstream file(path);

        if (!file.is_open())
            continue;

        std::string line;

        while (
            std::getline(
                file,
                line
            )
        ) {
            std::istringstream stream(line);

            std::string keyword;
            std::string name;
            std::string family;
            std::string method;

            if (
                !(stream
                    >> keyword
                    >> name
                    >> family
                    >> method)
            ) {
                continue;
            }

            if (
                keyword != "iface"
                ||
                name != interface_name
                ||
                family != "inet"
            ) {
                continue;
            }

            if (method == "dhcp")
                return "dhcp";

            if (method == "static")
                return "static";
        }
    }

    return {};
}

std::string managedNetworkdMethod(
    const std::string& interface_name
)
{
    const auto path =
        std::filesystem::path(
            "/etc/systemd/network"
        )
        /
        (
            "00-home-ai-"
            +
            interface_name
            +
            ".network"
        );

    std::ifstream file(path);

    if (!file.is_open())
        return {};

    std::string line;

    while (
        std::getline(
            file,
            line
        )
    ) {
        const auto trimmed =
            trimCopy(line);

        if (
            trimmed == "DHCP=ipv4"
            ||
            trimmed == "DHCP=yes"
        ) {
            return "dhcp";
        }

        if (
            trimmed.rfind(
                "Address=",
                0
            ) == 0
        ) {
            return "static";
        }
    }

    return {};
}

std::string detectedIpv4Method(
    const std::string& interface_name
)
{
    auto method =
        networkManagerMethod(
            interface_name
        );

    if (!method.empty())
        return method;

    method =
        ifupdownMethod(
            interface_name
        );

    if (!method.empty())
        return method;

    method =
        managedNetworkdMethod(
            interface_name
        );

    if (!method.empty())
        return method;

    const auto ifindex =
        readTextFile(
            std::filesystem::path(
                "/sys/class/net"
            )
            /
            interface_name
            /
            "ifindex"
        );

    if (
        !ifindex.empty()
        &&
        std::filesystem::exists(
            std::filesystem::path(
                "/run/systemd/netif/leases"
            )
            /
            ifindex
        )
    ) {
        return "dhcp";
    }

    return "unknown";
}

NetworkActionResult runHelper(
    const std::vector<std::string>& arguments,
    const std::string& failure_code,
    const std::string& failure_message
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

    std::vector<std::string>
        command_arguments;

    if (::geteuid() != 0) {
        command_arguments.push_back(
            "-n"
        );

        command_arguments.push_back(
            helper_path
        );
    }

    for (const auto& argument : arguments) {
        command_arguments.push_back(
            argument
        );
    }

    const auto command =
        ::geteuid() == 0
        ? runCommand(
            helper_path,
            arguments
        )
        : runCommand(
            sudo_path,
            command_arguments
        );

    result.success =
        command.exit_code == 0;

    result.code =
        result.success
        ? "ok"
        : failure_code;

    result.message =
        command.output.empty()
        ? (
            result.success
            ? "Сетевая конфигурация применена."
            : failure_message
        )
        : command.output;

    return result;
}

bool sameSubnet(
    const std::string& address,
    const std::string& gateway,
    int prefix
)
{
    in_addr ip_address{};
    in_addr gateway_address{};

    if (
        inet_pton(
            AF_INET,
            address.c_str(),
            &ip_address
        ) != 1
        ||
        inet_pton(
            AF_INET,
            gateway.c_str(),
            &gateway_address
        ) != 1
    ) {
        return false;
    }

    const auto ip =
        ntohl(
            ip_address.s_addr
        );

    const auto gw =
        ntohl(
            gateway_address.s_addr
        );

    const std::uint32_t mask =
        prefix == 0
        ? 0U
        : 0xffffffffU
            << (
                32 -
                static_cast<unsigned int>(
                    prefix
                )
            );

    return
        (ip & mask) ==
        (gw & mask);
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

bool
NetworkInterfaceManager::validIpv4Address(
    const std::string& address
)
{
    if (address.empty())
        return false;

    in_addr value{};

    return
        inet_pton(
            AF_INET,
            address.c_str(),
            &value
        ) == 1;
}

int
NetworkInterfaceManager::netmaskPrefix(
    const std::string& netmask
)
{
    if (netmask.empty())
        return -1;

    if (
        netmask.find('.') ==
        std::string::npos
    ) {
        try {
            std::size_t consumed = 0;

            const int prefix =
                std::stoi(
                    netmask,
                    &consumed,
                    10
                );

            if (
                consumed ==
                    netmask.size()
                &&
                prefix >= 1
                &&
                prefix <= 32
            ) {
                return prefix;
            }
        }
        catch (...) {
        }

        return -1;
    }

    in_addr value{};

    if (
        inet_pton(
            AF_INET,
            netmask.c_str(),
            &value
        ) != 1
    ) {
        return -1;
    }

    const std::uint32_t mask =
        ntohl(
            value.s_addr
        );

    int prefix = 0;
    bool zero_seen = false;

    for (int bit = 31; bit >= 0; --bit) {
        const bool one =
            (
                mask
                &
                (
                    1U
                    <<
                    static_cast<unsigned int>(
                        bit
                    )
                )
            ) != 0;

        if (one) {
            if (zero_seen)
                return -1;

            ++prefix;
        }
        else {
            zero_seen = true;
        }
    }

    return
        prefix >= 1
        ? prefix
        : -1;
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

    const auto gateways =
        defaultGateways();

    const auto dns =
        dnsServers();

    std::vector<NetworkInterfaceInfo>
        result;

    for (auto& [name, info] : by_name) {
        const auto gateway_it =
            gateways.find(name);

        if (
            gateway_it !=
            gateways.end()
        ) {
            info.gateway =
                gateway_it->second;

            info.default_route =
                true;
        }

        info.dns_servers =
            dns;

        if (!info.loopback) {
            info.ipv4_method =
                detectedIpv4Method(
                    name
                );
        }

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

    return runHelper(
        {
            "dhcp",
            interface_name
        },
        "dhcp_failed",
        "Не удалось получить IP по DHCP."
    );
}

NetworkActionResult
NetworkInterfaceManager::setStaticIpv4(
    const std::string& interface_name,
    const NetworkStaticConfig& config
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

    if (!validIpv4Address(config.address)) {
        return {
            false,
            "invalid_address",
            "Некорректный IPv4-адрес."
        };
    }

    const int prefix =
        netmaskPrefix(
            config.netmask
        );

    if (prefix < 1) {
        return {
            false,
            "invalid_netmask",
            "Некорректная маска сети."
        };
    }

    if (
        !config.gateway.empty()
        &&
        !validIpv4Address(
            config.gateway
        )
    ) {
        return {
            false,
            "invalid_gateway",
            "Некорректный шлюз."
        };
    }

    if (
        !config.gateway.empty()
        &&
        !sameSubnet(
            config.address,
            config.gateway,
            prefix
        )
    ) {
        return {
            false,
            "gateway_outside_subnet",
            "Шлюз должен находиться в той же IPv4-подсети."
        };
    }

    if (
        !config.dns_primary.empty()
        &&
        !validIpv4Address(
            config.dns_primary
        )
    ) {
        return {
            false,
            "invalid_dns",
            "Некорректный основной DNS."
        };
    }

    if (
        !config.dns_secondary.empty()
        &&
        !validIpv4Address(
            config.dns_secondary
        )
    ) {
        return {
            false,
            "invalid_dns",
            "Некорректный дополнительный DNS."
        };
    }

    return runHelper(
        {
            "static",
            interface_name,
            config.address,
            std::to_string(prefix),
            config.gateway.empty()
                ? "-"
                : config.gateway,
            config.dns_primary.empty()
                ? "-"
                : config.dns_primary,
            config.dns_secondary.empty()
                ? "-"
                : config.dns_secondary
        },
        "static_failed",
        "Не удалось применить статический IPv4."
    );
}

}
