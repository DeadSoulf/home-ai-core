#include "core/modules/ModuleInstaller.h"
#include "core/modules/ModuleManager.h"
#include "core/runtime/CoreRuntime.h"
#include "security/auth/SecurityManager.h"
#include "server/update/UpdateManager.h"
#include "web/server/WebServer.h"

#include <arpa/inet.h>
#include <filesystem>
#include <fstream>
#include <iostream>
#include <stdexcept>
#include <string>
#include <sys/socket.h>
#include <unistd.h>

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
            "homeai-module-installer-api-"
            +
            std::to_string(
                ::getpid()
            )
        );

    fs::remove_all(root);
    fs::create_directories(root);

    try {
        std::ofstream(root / "config")
            << "core.name=Module Installer Test\n"
            << "core.version=0.0.62\n";

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
                (root / "security.db").string(),
                (root / "users.db").string(),
                (root / "audit.log").string()
            ),
            "security init"
        );

        std::string error;

        check(
            security.createUser(
                "admin",
                "Admin-Password-123!",
                UserRole::Admin,
                error
            ),
            "admin create"
        );

        SessionInfo info;

        const auto token =
            security.login(
                "admin",
                "Admin-Password-123!",
                info,
                error
            );

        check(
            token.has_value(),
            "admin login"
        );

        UpdateManager updates;
        ModuleManager modules;

        check(
            modules.registerManifest(
                {
                    "network",
                    "Network Core",
                    "0.0.62",
                    "Network",
                    {},
                    {"network.view"},
                    {},
                    false,
                    true,
                    false
                },
                error
            ),
            "network manifest"
        );

        check(
            modules.registerManifest(
                {
                    "cluster",
                    "Cluster Core",
                    "0.0.62",
                    "Cluster",
                    {"network"},
                    {"cluster.view"},
                    {"cluster"},
                    false,
                    true,
                    true
                },
                error
            ),
            "cluster manifest"
        );

        ModuleInstaller installer(
            modules,
            root / "runtime/modules/installed.tsv"
        );

        check(
            installer.load(error),
            "installer load"
        );

        WebServer server(
            runtime,
            security,
            updates,
            modules,
            GpuMonitor(),
            nullptr,
            nullptr,
            nullptr,
            &installer
        );

        int port = 26000;

        while (
            port < 27000
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
            port < 27000,
            "server start"
        );

        auto request =
            [&](const std::string& method,
                const std::string& path,
                const std::string& body = "",
                bool csrf = true) {
                const int fd =
                    socket(
                        AF_INET,
                        SOCK_STREAM,
                        0
                    );

                check(
                    fd >= 0,
                    "socket"
                );

                sockaddr_in address{};
                address.sin_family =
                    AF_INET;
                address.sin_port =
                    htons(
                        static_cast<std::uint16_t>(
                            port
                        )
                    );

                inet_pton(
                    AF_INET,
                    "127.0.0.1",
                    &address.sin_addr
                );

                check(
                    connect(
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
                    + "Host: localhost\r\n"
                    + "Cookie: homeai_session="
                    + *token
                    + "\r\n"
                    + "Content-Type: application/x-www-form-urlencoded\r\n"
                    + "Content-Length: "
                    + std::to_string(
                        body.size()
                    )
                    + "\r\n";

                if (
                    csrf
                    &&
                    method != "GET"
                ) {
                    data +=
                        "X-HomeAI-Request: 1\r\n"
                        "X-HomeAI-CSRF: "
                        +
                        SecurityManager::
                            csrfTokenForSession(
                                *token
                            )
                        +
                        "\r\n";
                }

                data +=
                    "\r\n"
                    + body;

                check(
                    send(
                        fd,
                        data.data(),
                        data.size(),
                        MSG_NOSIGNAL
                    )
                    ==
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
                            recv(
                                fd,
                                buffer,
                                sizeof(buffer),
                                0
                            )
                    ) > 0
                ) {
                    response.append(
                        buffer,
                        count
                    );
                }

                close(fd);
                return response;
            };

        const auto catalog =
            request(
                "GET",
                "/api/module-catalog"
            );

        check(
            catalog.find(
                "\"id\":\"cluster\""
            ) != std::string::npos,
            "catalog cluster"
        );

        check(
            catalog.find(
                "\"installed\":true"
            ) != std::string::npos,
            "cluster initially installed"
        );

        const auto csrf_rejected =
            request(
                "POST",
                "/api/module-catalog/uninstall",
                "id=cluster",
                false
            );

        check(
            csrf_rejected.find(
                "403 Forbidden"
            ) != std::string::npos,
            "csrf guard"
        );

        const auto removed =
            request(
                "POST",
                "/api/module-catalog/uninstall",
                "id=cluster"
            );

        check(
            removed.find(
                "200 OK"
            ) != std::string::npos,
            "uninstall status"
        );

        check(
            removed.find(
                "\"restart_required\":true"
            ) != std::string::npos,
            "uninstall restart flag"
        );

        check(
            !modules.isInstalled(
                "cluster"
            ),
            "uninstall state"
        );

        const auto installed =
            request(
                "POST",
                "/api/module-catalog/install",
                "id=cluster"
            );

        check(
            installed.find(
                "200 OK"
            ) != std::string::npos,
            "install status"
        );

        check(
            modules.isInstalled(
                "cluster"
            ),
            "install state"
        );

        server.stop();

        std::cout
            << "Module Installer API test passed\n";
    }
    catch (const std::exception& ex) {
        std::cerr
            << "Module Installer API test failed: "
            << ex.what()
            << '\n';

        fs::remove_all(root);
        return 1;
    }

    fs::remove_all(root);
    return 0;
}
