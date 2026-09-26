#include "web/server/WebServer.h"
#include "core/runtime/CoreRuntime.h"
#include "core/modules/ModuleManager.h"
#include "security/auth/SecurityManager.h"
#include "server/cluster/ClusterManager.h"
#include "server/update/UpdateManager.h"

#include <arpa/inet.h>
#include <sys/socket.h>
#include <unistd.h>

#include <filesystem>
#include <fstream>
#include <iostream>
#include <stdexcept>
#include <string>

namespace {

void check(
    bool value,
    const char* message
)
{
    if (!value)
        throw std::runtime_error(message);
}

}

int main()
{
    namespace fs = std::filesystem;
    using namespace homeai;

    const auto root =
        fs::temp_directory_path()
        /
        (
            "homeai-cluster-api-"
            +
            std::to_string(
                ::getpid()
            )
        );

    fs::create_directories(root);

    try {
        std::ofstream(root / "config")
            << "core.name=Cluster Test\n";

        CoreRuntime runtime;
        check(
            runtime.initialize(
                (root / "config").string()
            ),
            "runtime init"
        );

        SecurityManager security;
        check(
            security.initialize(
                (root / "users").string(),
                (root / "audit").string()
            ),
            "security init"
        );

        std::string error;
        const std::string password =
            "Temporary-test-password!";

        check(
            security.createUser(
                "admin",
                password,
                UserRole::Admin,
                error
            ),
            "admin creation"
        );

        SessionInfo info;
        const auto admin =
            security.login(
                "admin",
                password,
                info,
                error
            );

        check(
            admin.has_value(),
            "login"
        );

        ClusterManager cluster;
        check(
            cluster.initialize(
                true,
                "controller-1",
                "Controller",
                "controller",
                "127.0.0.1",
                "",
                8080,
                "0123456789abcdef0123456789abcdef",
                5,
                20,
                error
            ),
            "cluster init"
        );

        UpdateManager updates;
        ModuleManager modules;

        WebServer server(
            runtime,
            security,
            updates,
            modules,
            GpuMonitor(),
            nullptr,
            nullptr,
            &cluster
        );

        int port = 25000;

        while (
            port < 26000
            &&
            !server.start(
                "127.0.0.1",
                static_cast<std::uint16_t>(
                    port
                )
            )
        ) {
            ++port;
        }

        check(
            port < 26000,
            "server start"
        );

        auto request =
            [&](const std::string& method,
                const std::string& path,
                const std::string& token,
                const std::string& body,
                const std::string& cluster_token) {
                const int fd =
                    ::socket(
                        AF_INET,
                        SOCK_STREAM,
                        0
                    );

                check(fd >= 0, "socket");

                sockaddr_in address{};
                address.sin_family =
                    AF_INET;
                address.sin_port =
                    htons(
                        static_cast<std::uint16_t>(
                            port
                        )
                    );

                ::inet_pton(
                    AF_INET,
                    "127.0.0.1",
                    &address.sin_addr
                );

                check(
                    ::connect(
                        fd,
                        reinterpret_cast<sockaddr*>(
                            &address
                        ),
                        sizeof(address)
                    ) == 0,
                    "connect"
                );

                std::string data =
                    method
                    + " "
                    + path
                    + " HTTP/1.1\r\n"
                      "Host: localhost\r\n";

                if (!token.empty()) {
                    data +=
                        "Cookie: homeai_session="
                        + token
                        + "\r\n";
                }

                if (!cluster_token.empty()) {
                    data +=
                        "X-HomeAI-Cluster-Token: "
                        + cluster_token
                        + "\r\n";
                }

                data +=
                    "Content-Type: application/x-www-form-urlencoded\r\n"
                    "Content-Length: "
                    + std::to_string(
                        body.size()
                    )
                    + "\r\n\r\n"
                    + body;

                check(
                    ::send(
                        fd,
                        data.data(),
                        data.size(),
                        MSG_NOSIGNAL
                    ) ==
                        static_cast<ssize_t>(
                            data.size()
                        ),
                    "send"
                );

                std::string response;
                char buffer[4096];
                ssize_t count = 0;

                while (
                    (
                        count =
                            ::recv(
                                fd,
                                buffer,
                                sizeof(buffer),
                                0
                            )
                    ) > 0
                ) {
                    response.append(
                        buffer,
                        static_cast<std::size_t>(
                            count
                        )
                    );
                }

                ::close(fd);
                return response;
            };

        check(
            request(
                "GET",
                "/api/cluster",
                "",
                "",
                ""
            ).find(
                "401 Unauthorized"
            ) != std::string::npos,
            "anonymous cluster inventory"
        );

        check(
            request(
                "POST",
                "/api/cluster/heartbeat",
                "",
                "node_id=worker-1&node_name=Worker&node_role=worker&address=10.0.0.2&cpu_percent=10&memory_percent=20&disk_percent=30",
                "wrong"
            ).find(
                "403 Forbidden"
            ) != std::string::npos,
            "heartbeat token"
        );

        check(
            request(
                "POST",
                "/api/cluster/heartbeat",
                "",
                "node_id=worker-1&node_name=Worker&node_role=worker&address=10.0.0.2&cpu_percent=10&memory_percent=20&disk_percent=30",
                "0123456789abcdef0123456789abcdef"
            ).find(
                "200 OK"
            ) != std::string::npos,
            "heartbeat accepted"
        );

        const auto inventory =
            request(
                "GET",
                "/api/cluster",
                *admin,
                "",
                ""
            );

        check(
            inventory.find(
                "200 OK"
            ) != std::string::npos
            &&
            inventory.find(
                "worker-1"
            ) != std::string::npos
            &&
            inventory.find(
                "\"placement\""
            ) != std::string::npos,
            "cluster inventory"
        );

        const auto placement =
            request(
                "GET",
                "/api/cluster/placement?workload=ai",
                *admin,
                "",
                ""
            );

        check(
            placement.find(
                "200 OK"
            ) != std::string::npos
            &&
            placement.find(
                "\"available\":true"
            ) != std::string::npos,
            "cluster placement"
        );

        server.stop();
        fs::remove_all(root);
    }
    catch (const std::exception& exception) {
        std::cerr
            << exception.what()
            << '\n';

        fs::remove_all(root);
        return 1;
    }

    std::cout
        << "Cluster API tests passed\n";
    return 0;
}
