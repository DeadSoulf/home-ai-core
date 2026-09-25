#include <algorithm>
#include <array>
#include <cctype>
#include <filesystem>
#include <iostream>
#include <string>
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

    return result;
}

bool succeeded(
    const CommandResult& result
)
{
    return
        result.exit_code == 0;
}

}

int main(
    int argc,
    char** argv
)
{
    if (
        argc != 3
        ||
        std::string(
            argv[1]
        ) != "dhcp"
    ) {
        std::cerr
            << "Usage: home-ai-network-helper dhcp <interface>\n";

        return 2;
    }

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

        const auto connection_name =
            trimCopy(
                connection.output
            );

        if (
            succeeded(connection)
            &&
            !connection_name.empty()
            &&
            connection_name != "--"
        ) {
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
                        ""
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
                        << "DHCP включён через NetworkManager. IP-адрес обновлён.";

                    return 0;
                }
            }
        }
        else {
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
                std::cout
                    << "DHCP-подключение создано через NetworkManager.";

                return 0;
            }
        }
    }

    const auto networkctl =
        findExecutable(
            {
                "/usr/bin/networkctl",
                "/bin/networkctl"
            }
        );

    if (!networkctl.empty()) {
        const auto renew =
            runCommand(
                networkctl,
                {
                    "renew",
                    interface_name
                }
            );

        if (succeeded(renew)) {
            std::cout
                << "DHCP-запрос отправлен через systemd-networkd.";

            return 0;
        }
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
                << "IP-адрес получен от DHCP-сервера через dhclient.";

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
                << "IP-адрес получен от DHCP-сервера через udhcpc.";

            return 0;
        }

        std::cerr
            << "DHCP-сервер не выдал адрес через udhcpc.\n"
            << lease.output;

        return 1;
    }

    std::cerr
        << "DHCP-клиент не найден. Нужен systemd-networkd с DHCP, dhclient или udhcpc.\n";

    return 1;
}
