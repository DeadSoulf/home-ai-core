#include "web/server/WebServer.h"
#include "core/runtime/CoreRuntime.h"
#include "core/modules/ModuleManager.h"
#include "security/auth/SecurityManager.h"
#include "server/update/UpdateManager.h"
#include <arpa/inet.h>
#include <sys/socket.h>
#include <unistd.h>
#include <filesystem>
#include <fstream>
#include <iostream>
#include <stdexcept>

void check(bool value, const char* message) { if (!value) throw std::runtime_error(message); }
int main() {
    namespace fs = std::filesystem;
    using namespace homeai;
    const auto root = fs::temp_directory_path() / ("homeai-api-" + std::to_string(getpid()));
    fs::create_directories(root / "pci/0000:01:00.0");
    std::ofstream(root / "pci/0000:01:00.0/class") << "0x030200";
    std::ofstream(root / "pci/0000:01:00.0/vendor") << "0x10de";
    std::ofstream(root / "pci/0000:01:00.0/device") << "0x1234";
    std::ofstream(root / "config") << "core.name=Preserve me\n";
    try {
        CoreRuntime runtime;
        runtime.initialize((root / "config").string());
        SecurityManager security;
        check(security.initialize((root / "users").string(), (root / "audit").string()), "security init");
        std::string error;
        const std::string password = "Temporary-test-password!";
        check(security.createUser("admin", password, UserRole::Admin, error), "admin creation");
        check(security.createUser("viewer", password, UserRole::Viewer, error), "viewer creation");
        SessionInfo info;
        auto admin = security.login("admin", password, info, error);
        auto viewer = security.login("viewer", password, info, error);
        check(admin && viewer, "login");
        UpdateManager updates;
        ModuleManager modules;
        WebServer server(runtime, security, updates, modules, GpuMonitor(root / "pci"));
        int port = 24000;
        while (port < 25000 && !server.start("127.0.0.1", port)) ++port;
        check(port < 25000, "server start");
        auto request = [&](std::string method, std::string path, std::string token = "", std::string body = "", bool header = true) {
            int fd = socket(AF_INET, SOCK_STREAM, 0);
            sockaddr_in address{};
            address.sin_family = AF_INET; address.sin_port = htons(port);
            inet_pton(AF_INET, "127.0.0.1", &address.sin_addr);
            check(connect(fd, reinterpret_cast<sockaddr*>(&address), sizeof(address)) == 0, "connect");
            std::string data = method + " " + path + " HTTP/1.1\r\nHost: localhost\r\nCookie: homeai_session=" + token +
                "\r\nContent-Type: application/x-www-form-urlencoded\r\nContent-Length: " + std::to_string(body.size()) +
                "\r\n" + (header ? "X-HomeAI-Request: 1\r\n" : "") + "\r\n" + body;
            check(send(fd, data.data(), data.size(), MSG_NOSIGNAL) == static_cast<ssize_t>(data.size()), "send");
            std::string response; char buffer[4096]; ssize_t n;
            while ((n = recv(fd, buffer, sizeof(buffer), 0)) > 0) response.append(buffer, n);
            close(fd); return response;
        };
        check(request("GET", "/api/admin/gpus").find("401 Unauthorized") != std::string::npos, "anonymous access");
        check(request("GET", "/api/admin/gpus", *viewer).find("403 Forbidden") != std::string::npos, "viewer inventory");
        check(request("GET", "/admin", *viewer).find("403 Forbidden") != std::string::npos, "viewer page");
        check(request("POST", "/api/admin/accelerator", *viewer, "pci_address=").find("403 Forbidden") != std::string::npos, "viewer mutation");
        check(request("GET", "/api/admin/gpus", *admin).find("0000:01:00.0") != std::string::npos, "inventory");
        check(request("POST", "/api/admin/accelerator", *admin, "pci_address=0000:01:00.0", false).find("403 Forbidden") != std::string::npos, "cross-origin form");
        check(request("POST", "/api/admin/accelerator", *admin, "pci_address=../../etc").find("400 Bad Request") != std::string::npos, "invalid ID");
        check(request("POST", "/api/admin/accelerator", *admin, "pci_address=0000:01:00.0").find("200 OK") != std::string::npos, "select GPU");
        ConfigManager loaded;
        check(loaded.load((root / "config").string()) && loaded.get("ai.accelerator.pci_address") == "0000:01:00.0" && loaded.get("core.name") == "Preserve me", "persistence");
        fs::remove_all(root / "pci/0000:01:00.0");
        check(request("GET", "/api/admin/gpus", *admin).find("\"selected_present\":false") != std::string::npos, "removed GPU");
        check(request("POST", "/api/admin/accelerator", *admin, "pci_address=0000:01:00.0").find("400 Bad Request") != std::string::npos, "stale selection");
        // An obstructed temporary file must not destroy the old configuration.
        fs::create_directory(root / "config.tmp");
        check(request("POST", "/api/admin/accelerator", *admin, "pci_address=").find("500 Internal Server Error") != std::string::npos, "save failure");
        check(runtime.config().get("ai.accelerator.pci_address") == "0000:01:00.0", "rollback memory");
        fs::remove(root / "config.tmp");
        check(request("POST", "/api/admin/accelerator", *admin, "pci_address=").find("200 OK") != std::string::npos, "clear missing GPU");
        check(request("GET", "/assets/i18n.js").find("language-selector") != std::string::npos, "public localization");
        const auto login = request("GET", "/login");
        check(login.find("/assets/i18n.js") != std::string::npos, "login localization");
        fs::create_directories("ui-fixtures");
        std::ofstream("ui-fixtures/login.html") << login.substr(login.find("\r\n\r\n") + 4);
        server.stop();
        fs::remove_all(root);
    } catch (const std::exception& error) {
        std::cerr << error.what() << '\n'; fs::remove_all(root); return 1;
    }
    std::cout << "Administration API test passed\n";
}
