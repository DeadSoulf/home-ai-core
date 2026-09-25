#include <algorithm>
#include <array>
#include <arpa/inet.h>
#include <cctype>
#include <filesystem>
#include <fstream>
#include <iostream>
#include <string>
#include <sys/stat.h>
#include <sys/wait.h>
#include <unistd.h>
#include <vector>

namespace {

struct CommandResult {
    int exit_code{-1};
    std::string output;
};

bool validInterfaceName(
    const std::string& name
)
{
    if (
        name.empty()
        ||
        name.size() >= 16
        ||
        name == "lo"
    ) {
        return false;
    }

    return std::all_of(
        name.begin(),
        name.end(),
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

bool validIpv4(
    const std::string& value
)
{
    in_addr address{};

    return
        !value.empty()
        &&
        inet_pton(
            AF_INET,
            value.c_str(),
            &address
        ) == 1;
}

bool validPrefix(
    const std::string& value
)
{
    try {
        std::size_t consumed = 0;

        const int prefix =
            std::stoi(
                value,
                &consumed,
                10
            );

        return
            consumed == value.size()
            &&
            prefix >= 1
            &&
            prefix <= 32;
    }
    catch (...) {
        return false;
    }
}

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

    if (::pipe(pipe_fd) != 0) {
        result.output =
            "pipe failed";

        return result;
    }

    const pid_t child =
        ::fork();

    if (child < 0) {
        ::close(pipe_fd[0]);
        ::close(pipe_fd[1]);

        result.output =
            "fork failed";

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

        if (
            result.output.size() >
            16384
        ) {
            result.output.erase(
                0,
                result.output.size() -
                    16384
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
        result.output +=
            "\nwaitpid failed";

        return result;
    }

    if (WIFEXITED(status)) {
        result.exit_code =
            WEXITSTATUS(status);
    }

    result.output =
        trimCopy(
            result.output
        );

    return result;
}

bool succeeded(
    const CommandResult& result
)
{
    return
        result.exit_code == 0;
}

std::string networkManagerConnection(
    const std::string& nmcli,
    const std::string& interface_name
)
{
    const auto result =
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
        !succeeded(result)
        ||
        result.output.empty()
        ||
        result.output == "--"
    ) {
        return {};
    }

    return
        result.output;
}

std::filesystem::path networkdPath(
    const std::string& interface_name
)
{
    return
        std::filesystem::path(
            "/etc/systemd/network"
        )
        /
        (
            "05-home-ai-"
            +
            interface_name
            +
            ".network"
        );
}

void removeManagedNetworkdProfile(
    const std::string& interface_name
)
{
    std::error_code error;

    std::filesystem::remove(
        networkdPath(
            interface_name
        ),
        error
    );
}

bool networkdActive()
{
    const auto systemctl =
        findExecutable(
            {
                "/usr/bin/systemctl",
                "/bin/systemctl"
            }
        );

    if (systemctl.empty())
        return false;

    const auto result =
        runCommand(
            systemctl,
            {
                "is-active",
                "--quiet",
                "systemd-networkd.service"
            }
        );

    return
        succeeded(
            result
        );
}

bool writeNetworkdProfile(
    const std::string& interface_name,
    bool dhcp,
    const std::string& address,
    const std::string& prefix,
    const std::string& gateway,
    const std::string& dns_primary,
    const std::string& dns_secondary,
    std::string& error
)
{
    std::error_code fs_error;

    const std::filesystem::path directory =
        "/etc/systemd/network";

    std::filesystem::create_directories(
        directory,
        fs_error
    );

    if (fs_error) {
        error =
            "Не удалось подготовить /etc/systemd/network.";

        return false;
    }

    const auto path =
        networkdPath(
            interface_name
        );

    const auto temp =
        path.string()
        +
        ".tmp";

    {
        std::ofstream file(
            temp,
            std::ios::trunc
        );

        if (!file.is_open()) {
            error =
                "Не удалось сохранить конфигурацию systemd-networkd.";

            return false;
        }

        file
            << "[Match]\n"
            << "Name="
            << interface_name
            << "\n\n"
            << "[Network]\n";

        if (dhcp) {
            file
                << "DHCP=ipv4\n";
        }
        else {
            file
                << "DHCP=no\n"
                << "Address="
                << address
                << "/"
                << prefix
                << "\n";

            if (gateway != "-") {
                file
                    << "Gateway="
                    << gateway
                    << "\n";
            }

            if (dns_primary != "-") {
                file
                    << "DNS="
                    << dns_primary
                    << "\n";
            }

            if (dns_secondary != "-") {
                file
                    << "DNS="
                    << dns_secondary
                    << "\n";
            }
        }
    }

    ::chmod(
        temp.c_str(),
        0644
    );

    std::filesystem::rename(
        temp,
        path,
        fs_error
    );

    if (fs_error) {
        std::filesystem::remove(
            temp
        );

        error =
            "Не удалось активировать конфигурацию systemd-networkd.";

        return false;
    }

    return true;
}

bool reloadNetworkd(
    const std::string& interface_name,
    std::string& error
)
{
    const auto networkctl =
        findExecutable(
            {
                "/usr/bin/networkctl",
                "/bin/networkctl"
            }
        );

    if (networkctl.empty()) {
        error =
            "networkctl не найден.";

        return false;
    }

    const auto reload =
        runCommand(
            networkctl,
            {
                "reload"
            }
        );

    if (!succeeded(reload)) {
        error =
            reload.output.empty()
            ? "Не удалось перезагрузить systemd-networkd."
            : reload.output;

        return false;
    }

    const auto reconfigure =
        runCommand(
            networkctl,
            {
                "reconfigure",
                interface_name
            }
        );

    if (!succeeded(reconfigure)) {
        error =
            reconfigure.output.empty()
            ? "Не удалось применить конфигурацию systemd-networkd."
            : reconfigure.output;

        return false;
    }

    return true;
}

int configureDhcp(
    const std::string& interface_name
)
{
    const auto ip =
        findExecutable(
            {
                "/usr/sbin/ip",
                "/usr/bin/ip",
                "/sbin/ip",
                "/bin/ip"
            }
        );

    if (!ip.empty()) {
        const auto link_up =
            runCommand(
                ip,
                {
                    "link",
                    "set",
                    "dev",
                    interface_name,
                    "up"
                }
            );

        if (!succeeded(link_up)) {
            std::cerr
                << "Не удалось включить сетевой интерфейс.\n"
                << link_up.output;

            return 1;
        }
    }

    const auto nmcli =
        findExecutable(
            {
                "/usr/bin/nmcli",
                "/bin/nmcli"
            }
        );

    if (!nmcli.empty()) {
        auto connection_name =
            networkManagerConnection(
                nmcli,
                interface_name
            );

        if (connection_name.empty()) {
            const auto connect =
                runCommand(
                    nmcli,
                    {
                        "device",
                        "connect",
                        interface_name
                    }
                );

            if (succeeded(connect)) {
                connection_name =
                    networkManagerConnection(
                        nmcli,
                        interface_name
                    );
            }
        }

        if (!connection_name.empty()) {
            removeManagedNetworkdProfile(
                interface_name
            );

            const auto configure =
                runCommand(
                    nmcli,
                    {
                        "connection",
                        "modify",
                        connection_name,
                        "ipv4.method",
                        "auto",
                        "ipv4.addresses",
                        "",
                        "ipv4.gateway",
                        "",
                        "ipv4.dns",
                        "",
                        "ipv4.ignore-auto-dns",
                        "no"
                    }
                );

            if (succeeded(configure)) {
                const auto activate =
                    runCommand(
                        nmcli,
                        {
                            "connection",
                            "up",
                            connection_name,
                            "ifname",
                            interface_name
                        }
                    );

                if (succeeded(activate)) {
                    std::cout
                        << "DHCP включён через NetworkManager. Конфигурация сохранена.";

                    return 0;
                }
            }
        }
    }

    if (networkdActive()) {
        std::string error;

        if (
            writeNetworkdProfile(
                interface_name,
                true,
                "",
                "",
                "-",
                "-",
                "-",
                error
            )
            &&
            reloadNetworkd(
                interface_name,
                error
            )
        ) {
            std::cout
                << "DHCP включён через systemd-networkd. Конфигурация сохранена.";

            return 0;
        }

        std::cerr
            << error
            << "\n";

        return 1;
    }

    const auto dhclient =
        findExecutable(
            {
                "/usr/sbin/dhclient",
                "/usr/bin/dhclient",
                "/sbin/dhclient"
            }
        );

    if (!dhclient.empty()) {
        runCommand(
            dhclient,
            {
                "-4",
                "-r",
                interface_name
            }
        );

        const auto lease =
            runCommand(
                dhclient,
                {
                    "-4",
                    "-1",
                    "-v",
                    interface_name
                }
            );

        if (succeeded(lease)) {
            std::cout
                << "IP-адрес получен через dhclient. Изменение действует до изменения системной сетевой конфигурации.";

            return 0;
        }

        std::cerr
            << "DHCP-сервер не выдал адрес через dhclient.\n"
            << lease.output;

        return 1;
    }

    const auto udhcpc =
        findExecutable(
            {
                "/usr/sbin/udhcpc",
                "/usr/bin/udhcpc",
                "/sbin/udhcpc",
                "/bin/udhcpc"
            }
        );

    if (!udhcpc.empty()) {
        const auto lease =
            runCommand(
                udhcpc,
                {
                    "-i",
                    interface_name,
                    "-q",
                    "-n",
                    "-t",
                    "5",
                    "-T",
                    "3"
                }
            );

        if (succeeded(lease)) {
            std::cout
                << "IP-адрес получен через udhcpc. Изменение действует до изменения системной сетевой конфигурации.";

            return 0;
        }

        std::cerr
            << "DHCP-сервер не выдал адрес через udhcpc.\n"
            << lease.output;

        return 1;
    }

    std::cerr
        << "Поддерживаемый DHCP backend не найден.";

    return 1;
}

int configureStatic(
    const std::string& interface_name,
    const std::string& address,
    const std::string& prefix,
    const std::string& gateway,
    const std::string& dns_primary,
    const std::string& dns_secondary
)
{
    const auto nmcli =
        findExecutable(
            {
                "/usr/bin/nmcli",
                "/bin/nmcli"
            }
        );

    if (!nmcli.empty()) {
        auto connection_name =
            networkManagerConnection(
                nmcli,
                interface_name
            );

        if (connection_name.empty()) {
            const auto connect =
                runCommand(
                    nmcli,
                    {
                        "device",
                        "connect",
                        interface_name
                    }
                );

            if (succeeded(connect)) {
                connection_name =
                    networkManagerConnection(
                        nmcli,
                        interface_name
                    );
            }
        }

        if (!connection_name.empty()) {
            std::string dns;

            if (dns_primary != "-")
                dns = dns_primary;

            if (dns_secondary != "-") {
                if (!dns.empty())
                    dns += ",";

                dns += dns_secondary;
            }

            removeManagedNetworkdProfile(
                interface_name
            );

            const auto configure =
                runCommand(
                    nmcli,
                    {
                        "connection",
                        "modify",
                        connection_name,
                        "ipv4.method",
                        "manual",
                        "ipv4.addresses",
                        address + "/" + prefix,
                        "ipv4.gateway",
                        gateway == "-"
                            ? ""
                            : gateway,
                        "ipv4.dns",
                        dns,
                        "ipv4.ignore-auto-dns",
                        "yes"
                    }
                );

            if (!succeeded(configure)) {
                std::cerr
                    << "NetworkManager не принял статическую конфигурацию.\n"
                    << configure.output;

                return 1;
            }

            const auto activate =
                runCommand(
                    nmcli,
                    {
                        "connection",
                        "up",
                        connection_name,
                        "ifname",
                        interface_name
                    }
                );

            if (!succeeded(activate)) {
                std::cerr
                    << "Не удалось активировать статический IP через NetworkManager.\n"
                    << activate.output;

                return 1;
            }

            std::cout
                << "Статический IPv4 применён через NetworkManager. Конфигурация сохранена.";

            return 0;
        }
    }

    if (networkdActive()) {
        std::string error;

        if (
            writeNetworkdProfile(
                interface_name,
                false,
                address,
                prefix,
                gateway,
                dns_primary,
                dns_secondary,
                error
            )
            &&
            reloadNetworkd(
                interface_name,
                error
            )
        ) {
            std::cout
                << "Статический IPv4 применён через systemd-networkd. Конфигурация сохранена.";

            return 0;
        }

        std::cerr
            << error
            << "\n";

        return 1;
    }

    const auto ip =
        findExecutable(
            {
                "/usr/sbin/ip",
                "/usr/bin/ip",
                "/sbin/ip",
                "/bin/ip"
            }
        );

    if (ip.empty()) {
        std::cerr
            << "Команда ip не найдена.";

        return 1;
    }

    const auto link_up =
        runCommand(
            ip,
            {
                "link",
                "set",
                "dev",
                interface_name,
                "up"
            }
        );

    if (!succeeded(link_up)) {
        std::cerr
            << "Не удалось включить сетевой интерфейс.\n"
            << link_up.output;

        return 1;
    }

    const auto flush =
        runCommand(
            ip,
            {
                "-4",
                "addr",
                "flush",
                "dev",
                interface_name,
                "scope",
                "global"
            }
        );

    if (!succeeded(flush)) {
        std::cerr
            << "Не удалось очистить старый IPv4-адрес.\n"
            << flush.output;

        return 1;
    }

    const auto add =
        runCommand(
            ip,
            {
                "-4",
                "addr",
                "add",
                address + "/" + prefix,
                "dev",
                interface_name
            }
        );

    if (!succeeded(add)) {
        std::cerr
            << "Не удалось назначить статический IPv4.\n"
            << add.output;

        return 1;
    }

    if (gateway != "-") {
        const auto route =
            runCommand(
                ip,
                {
                    "-4",
                    "route",
                    "replace",
                    "default",
                    "via",
                    gateway,
                    "dev",
                    interface_name
                }
            );

        if (!succeeded(route)) {
            std::cerr
                << "IPv4 назначен, но шлюз применить не удалось.\n"
                << route.output;

            return 1;
        }
    }

    const auto resolvectl =
        findExecutable(
            {
                "/usr/bin/resolvectl",
                "/bin/resolvectl"
            }
        );

    if (
        !resolvectl.empty()
        &&
        (
            dns_primary != "-"
            ||
            dns_secondary != "-"
        )
    ) {
        std::vector<std::string> arguments{
            "dns",
            interface_name
        };

        if (dns_primary != "-")
            arguments.push_back(
                dns_primary
            );

        if (dns_secondary != "-")
            arguments.push_back(
                dns_secondary
            );

        runCommand(
            resolvectl,
            arguments
        );
    }

    std::cout
        << "Статический IPv4 применён для текущего запуска. Для постоянной настройки нужен NetworkManager или systemd-networkd.";

    return 0;
}

}

int main(
    int argc,
    char** argv
)
{
    if (
        argc < 3
        ||
        argc > 8
    ) {
        std::cerr
            << "Usage:\n"
            << "  home-ai-network-helper dhcp <interface>\n"
            << "  home-ai-network-helper static <interface> <ip> <prefix> <gateway|-> <dns1|-> <dns2|->\n";

        return 2;
    }

    const std::string action =
        argv[1];

    const std::string interface_name =
        argv[2];

    if (
        !validInterfaceName(
            interface_name
        )
    ) {
        std::cerr
            << "Некорректный сетевой интерфейс.\n";

        return 2;
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
        std::cerr
            << "Сетевой интерфейс не найден.\n";

        return 2;
    }

    if (action == "dhcp") {
        if (argc != 3) {
            std::cerr
                << "Некорректные параметры DHCP.\n";

            return 2;
        }

        return
            configureDhcp(
                interface_name
            );
    }

    if (action == "static") {
        if (argc != 8) {
            std::cerr
                << "Некорректные параметры статического IPv4.\n";

            return 2;
        }

        const std::string address =
            argv[3];

        const std::string prefix =
            argv[4];

        const std::string gateway =
            argv[5];

        const std::string dns_primary =
            argv[6];

        const std::string dns_secondary =
            argv[7];

        if (
            !validIpv4(address)
            ||
            !validPrefix(prefix)
            ||
            (
                gateway != "-"
                &&
                !validIpv4(gateway)
            )
            ||
            (
                dns_primary != "-"
                &&
                !validIpv4(
                    dns_primary
                )
            )
            ||
            (
                dns_secondary != "-"
                &&
                !validIpv4(
                    dns_secondary
                )
            )
        ) {
            std::cerr
                << "Некорректная статическая IPv4-конфигурация.\n";

            return 2;
        }

        return configureStatic(
            interface_name,
            address,
            prefix,
            gateway,
            dns_primary,
            dns_secondary
        );
    }

    std::cerr
        << "Неизвестная сетевая операция.\n";

    return 2;
}
