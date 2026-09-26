#include "web/server/WebServer.h"
#include "core/runtime/CoreRuntime.h"
#include "core/modules/ModuleManager.h"
#include "security/auth/SecurityManager.h"
#include "server/update/UpdateManager.h"
#include "server/virtualization/HypervisorManager.h"
#include <dlfcn.h>
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
    const auto root = fs::temp_directory_path() / ("homeai-hypervisor-api-" + std::to_string(getpid()));
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
        for (const auto& user : security.listUsers(error)) {
            if (user.username == "viewer")
                check(security.setPermissionOverride(user.id, "hypervisor.view", 1, error), "view-only permission");
        }
        SessionInfo info;
        auto admin = security.login("admin", password, info, error);
        auto viewer = security.login("viewer", password, info, error);
        check(admin && viewer, "login");
        UpdateManager updates;
        ModuleManager modules;
        HypervisorManager hypervisor;
        hypervisor.initialize(error);
        WebServer server(runtime, security, updates, modules, GpuMonitor(root / "pci"), nullptr, &hypervisor);
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
        const std::string uuid = "11111111-2222-3333-4444-555555555555";
        const std::string action_path = "/api/hypervisor/action";
        auto form = [&](std::string action, std::string state) {
            return "uuid=" + uuid + "&confirmation=" + uuid + "&action=" + action + "&expected_state=" + state;
        };
        const std::string storage_preview_path = "/api/hypervisor/storage/preview";
        check(request("POST", storage_preview_path, "", "size_gib=32").find("401 Unauthorized") != std::string::npos,
            "anonymous disk placement preview");
        check(request("POST", storage_preview_path, *viewer, "size_gib=32").find("403 Forbidden") != std::string::npos,
            "viewer disk placement preview");
        check(request("POST", storage_preview_path, *admin, "size_gib=32", false).find("403 Forbidden") != std::string::npos,
            "disk placement request header");
        check(request("POST", storage_preview_path, *admin, "size_gib=0").find("invalid_size") != std::string::npos,
            "disk placement size validation");
        check(request("POST", storage_preview_path, *admin, "size_gib=32").find("no_vm_storage") != std::string::npos,
            "disk placement requires VM pool");

        const std::string preview_path = "/api/hypervisor/create/preview";
        const std::string preview_form = "name=preview-vm&vcpus=2&memory_mib=4096&architecture=x86_64&machine_type=q35";
        check(request("POST", preview_path, "", preview_form).find("401 Unauthorized") != std::string::npos, "anonymous create preview");
        check(request("POST", preview_path, *viewer, preview_form).find("403 Forbidden") != std::string::npos, "viewer create preview");
        check(request("POST", preview_path, *admin, preview_form, false).find("403 Forbidden") != std::string::npos, "preview request header");
        const auto preview_response = request("POST", preview_path, *admin, preview_form);
        check(preview_response.find("200 OK") != std::string::npos &&
            preview_response.find("preview_ready") != std::string::npos &&
            preview_response.find("preview-vm") != std::string::npos &&
            preview_response.find("<domain type='kvm'>") != std::string::npos, "valid create preview");
        check(request("POST", preview_path, *admin,
            "name=%3Cbad%3E&vcpus=2&memory_mib=4096&architecture=x86_64&machine_type=q35")
            .find("invalid_name") != std::string::npos, "preview rejects unsafe name");

        const std::string create_path = "/api/hypervisor/create";
        const std::string create_form = preview_form + "&confirmation=preview-vm";
        check(request("POST", create_path, "", create_form).find("401 Unauthorized") != std::string::npos, "anonymous create");
        check(request("POST", create_path, *viewer, create_form).find("403 Forbidden") != std::string::npos, "viewer create");
        check(request("POST", create_path, *admin, create_form, false).find("403 Forbidden") != std::string::npos, "create request header");
        check(request("POST", create_path, *admin, preview_form + "&confirmation=wrong")
            .find("confirmation_required") != std::string::npos, "create confirmation");
        check(request("POST", create_path, *admin,
            "name=too-large&vcpus=9&memory_mib=4096&architecture=x86_64&machine_type=q35&confirmation=too-large")
            .find("409 Conflict") != std::string::npos, "create host limit");
        const auto create_response = request("POST", create_path, *admin, create_form);
        check(create_response.find("201 Created") != std::string::npos &&
            create_response.find("\"code\":\"created\"") != std::string::npos &&
            create_response.find("aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee") != std::string::npos,
            "persistent create");
        const auto duplicate_response = request("POST", create_path, *admin, create_form);
        check(duplicate_response.find("409 Conflict") != std::string::npos &&
            duplicate_response.find("duplicate_name") != std::string::npos,
            "duplicate create");

        check(request("GET", "/api/hypervisor").find("401 Unauthorized") != std::string::npos, "anonymous inventory");
        check(request("POST", action_path, "", form("start", "shutoff")).find("401 Unauthorized") != std::string::npos, "anonymous mutation");
        check(request("POST", action_path, *viewer, form("start", "shutoff")).find("403 Forbidden") != std::string::npos, "viewer mutation");
        check(request("POST", action_path, *admin, form("start", "shutoff"), false).find("403 Forbidden") != std::string::npos, "missing CSRF header");
        check(request("POST", action_path, *admin, "uuid=" + uuid + "&action=start&expected_state=shutoff").find("confirmation_required") != std::string::npos, "confirmation required");
        check(request("POST", action_path, *admin, "uuid=../bad").find("400 Bad Request") != std::string::npos, "invalid uuid");
        auto inventory = request("GET", "/api/hypervisor", *admin);
        const auto view_only = request("GET", "/api/hypervisor", *viewer);
        check(view_only.find("200 OK") != std::string::npos &&
            view_only.find("\"allowed_actions\":[]") != std::string::npos, "view-only inventory disables actions");
        check(inventory.find("allowed_actions") != std::string::npos &&
            inventory.find("capabilities") != std::string::npos &&
            inventory.find("\"create\":true") != std::string::npos &&
            inventory.find("preview-vm") != std::string::npos, "inventory contract");
        check(request("POST", action_path, *admin, form("start", "running")).find("409 Conflict") != std::string::npos, "stale state");
        check(request("POST", action_path, *admin, form("start", "shutoff")).find("202 Accepted") != std::string::npos, "start");
        check(request("POST", action_path, *admin, form("pause", "running")).find("202 Accepted") != std::string::npos, "pause");
        check(request("POST", action_path, *admin, form("resume", "paused")).find("202 Accepted") != std::string::npos, "resume");
        check(request("POST", action_path, *admin, form("autostart-on", "running")).find("202 Accepted") != std::string::npos, "autostart on");
        check(request("GET", "/api/hypervisor", *admin).find("\"autostart\":true") != std::string::npos, "autostart inventory on");
        check(request("POST", action_path, *admin, form("autostart-off", "running")).find("202 Accepted") != std::string::npos, "autostart off");
        check(request("POST", action_path, *admin, form("shutdown", "running")).find("202 Accepted") != std::string::npos, "shutdown accepted");
        check(request("POST", action_path, *admin, form("reboot", "running")).find("operation_pending") != std::string::npos, "duplicate request");
        check(request("POST", action_path, *admin, form("force-off", "running")).find("202 Accepted") != std::string::npos, "force off");
        check(request("POST", action_path, *admin, form("delete", "shutoff")).find("unsupported_action") != std::string::npos, "future mutation blocked");
        void* fake = dlopen("libvirt.so.0", RTLD_NOW);
        auto failure = reinterpret_cast<void(*)(int)>(dlsym(fake, "fakeFailure"));
        check(failure != nullptr, "isolated fake library");
        failure(5);
        check(request("POST", create_path, *admin,
            "name=backend-fail&vcpus=2&memory_mib=4096&architecture=x86_64&machine_type=q35&confirmation=backend-fail")
            .find("502 Bad Gateway") != std::string::npos, "create backend failure");
        failure(1);
        check(request("POST", action_path, *admin, form("start", "shutoff")).find("403 Forbidden") != std::string::npos, "libvirt denied");
        failure(2);
        check(request("POST", action_path, *admin, form("start", "shutoff")).find("502 Bad Gateway") != std::string::npos, "libvirt error");
        failure(4);
        check(request("POST", action_path, *admin, form("start", "shutoff")).find("404 Not Found") != std::string::npos, "missing domain");
        failure(0);
        dlclose(fake);
        const auto audit = request("GET", "/api/users/audit", *admin);
        check(audit.find("hypervisor.action") != std::string::npos &&
            audit.find("hypervisor.create") != std::string::npos, "audit trail");
        hypervisor.shutdown();
        check(request("POST", action_path, *admin, form("start", "shutoff")).find("503 Service Unavailable") != std::string::npos, "stopped module");
        server.stop();
        fs::remove_all(root);
    } catch (const std::exception& error) {
        std::cerr << error.what() << '\n'; fs::remove_all(root); return 1;
    }
    std::cout << "Hypervisor API tests passed\n";
}
