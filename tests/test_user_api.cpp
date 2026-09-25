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
        throw std::runtime_error(
            message
        );
}

}

int main()
{
    namespace fs =
        std::filesystem;

    using namespace homeai;

    const auto root =
        fs::temp_directory_path()
        /
        (
            "home-ai-user-api-"
            +
            std::to_string(
                getpid()
            )
        );

    fs::remove_all(root);
    fs::create_directories(root);

    std::ofstream(
        root / "config"
    )
        << "core.name=Test\n"
        << "core.version=0.0.6\n";

    try {
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

        check(
            security.createUser(
                "viewer",
                "Viewer-Password-123!",
                UserRole::Viewer,
                error
            ),
            "viewer create"
        );

        const auto users =
            security.listUsers(error);

        std::int64_t admin_id = 0;
        std::int64_t viewer_id = 0;

        for (const auto& user : users) {
            if (user.username == "admin")
                admin_id = user.id;

            if (user.username == "viewer")
                viewer_id = user.id;
        }

        check(
            admin_id > 0
            &&
            viewer_id > 0,
            "user ids"
        );

        SessionInfo admin_info;
        SessionInfo viewer_info;

        const auto admin_token =
            security.login(
                "admin",
                "Admin-Password-123!",
                admin_info,
                error
            );

        const auto viewer_token =
            security.login(
                "viewer",
                "Viewer-Password-123!",
                viewer_info,
                error
            );

        check(
            admin_token.has_value()
            &&
            viewer_token.has_value(),
            "login"
        );

        UpdateManager updates;
        ModuleManager modules;

        WebServer server(
            runtime,
            security,
            updates,
            modules
        );

        int port = 25000;

        while (
            port < 26000
            &&
            !server.start(
                "127.0.0.1",
                static_cast<
                    std::uint16_t
                >(port)
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
                const std::string& body = "",
                bool protected_header = true) {
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
                        static_cast<
                            std::uint16_t
                        >(port)
                    );

                inet_pton(
                    AF_INET,
                    "127.0.0.1",
                    &address.sin_addr
                );

                check(
                    connect(
                        fd,
                        reinterpret_cast<
                            sockaddr*
                        >(&address),
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
                    + token
                    + "\r\n"
                    + "Content-Type: application/x-www-form-urlencoded\r\n"
                    + "Content-Length: "
                    + std::to_string(
                        body.size()
                    )
                    + "\r\n";

                if (protected_header) {
                    data +=
                        "X-HomeAI-Request: 1\r\n";
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

        check(
            request(
                "GET",
                "/api/users",
                *viewer_token
            ).find(
                "403 Forbidden"
            ) != std::string::npos,
            "viewer users access"
        );

        error.clear();

        check(
            security.
                setPermissionOverride(
                    viewer_id,
                    "users.view",
                    1,
                    error
                ),
            "grant users.view"
        );

        check(
            request(
                "GET",
                "/api/users",
                *viewer_token
            ).find(
                "\"username\":\"viewer\""
            ) != std::string::npos,
            "viewer read users"
        );

        check(
            request(
                "POST",
                "/api/users/create",
                *viewer_token,
                "username=blocked&password=Blocked-Password-123!&role=viewer"
            ).find(
                "403 Forbidden"
            ) != std::string::npos,
            "viewer manage users"
        );

        check(
            request(
                "POST",
                "/api/users/create",
                *admin_token,
                "username=operator&password=Operator-Password-123!&role=operator",
                false
            ).find(
                "403 Forbidden"
            ) != std::string::npos,
            "csrf header"
        );

        check(
            request(
                "POST",
                "/api/users/create",
                *admin_token,
                "username=operator&password=Operator-Password-123!&role=operator"
            ).find(
                "200 OK"
            ) != std::string::npos,
            "admin create user"
        );

        const auto updated_users =
            security.listUsers(error);

        std::int64_t operator_id =
            0;

        for (
            const auto& user :
            updated_users
        ) {
            if (
                user.username ==
                "operator"
            ) {
                operator_id =
                    user.id;
            }
        }

        check(
            operator_id > 0,
            "operator id"
        );

        check(
            request(
                "POST",
                "/api/users/update",
                *admin_token,
                "user_id="
                    + std::to_string(
                        admin_id
                    )
                    + "&role=viewer&enabled=1"
            ).find(
                "400 Bad Request"
            ) != std::string::npos,
            "last admin guard"
        );

        SessionInfo operator_info;

        const auto operator_token =
            security.login(
                "operator",
                "Operator-Password-123!",
                operator_info,
                error
            );

        check(
            operator_token.has_value(),
            "operator login"
        );

        const auto sessions_response =
            request(
                "GET",
                "/api/users/sessions",
                *admin_token
            );

        check(
            sessions_response.find(
                "\"username\":\"operator\""
            ) != std::string::npos,
            "session inventory"
        );

        const auto sessions =
            security.listSessions(
                operator_id,
                error
            );

        check(
            !sessions.empty(),
            "operator session missing"
        );

        check(
            request(
                "POST",
                "/api/users/session/revoke",
                *admin_token,
                "user_id="
                    + std::to_string(
                        operator_id
                    )
                    + "&session_id="
                    + std::to_string(
                        sessions.front().id
                    )
            ).find(
                "200 OK"
            ) != std::string::npos,
            "session revoke API"
        );

        check(
            !security.validateSession(
                *operator_token
            ),
            "revoked session remains valid"
        );

        check(
            request(
                "GET",
                "/api/users/audit?event=user.admin.create",
                *admin_token
            ).find(
                "user.admin.create"
            ) != std::string::npos,
            "audit API"
        );

        const auto viewer_page =
            request(
                "GET",
                "/users",
                *viewer_token
            );

        check(
            viewer_page.find(
                "id=\"users-list\""
            ) != std::string::npos,
            "users page"
        );

        check(
            viewer_page.find(
                "id=\"user-create-btn\""
            ) == std::string::npos,
            "viewer manage controls"
        );

        const auto admin_page =
            request(
                "GET",
                "/users",
                *admin_token
            );

        check(
            admin_page.find(
                "id=\"user-create-btn\""
            ) != std::string::npos,
            "admin manage controls"
        );

        server.stop();
        fs::remove_all(root);
    }
    catch (
        const std::exception& exception
    ) {
        std::cerr
            << exception.what()
            << '\n';

        fs::remove_all(root);

        return 1;
    }

    std::cout
        << "User administration API test passed\n";

    return 0;
}
