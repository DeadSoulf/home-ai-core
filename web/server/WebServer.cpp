#include "web/server/WebServer.h"
#include "web/server/JsonUtils.h"
#include "web/ui/WebUi.h"
#include "web/ui/Localization.h"
#include "server/hardware/GpuMonitor.h"

#include "core/logging/Logger.h"
#include "core/modules/ModuleManager.h"
#include "core/runtime/CoreRuntime.h"
#include "security/auth/SecurityManager.h"
#include "server/system/SystemMonitor.h"
#include "server/network/NetworkInterfaceManager.h"
#include "server/network/VpnService.h"
#include "server/cameras/CameraManager.h"
#include "server/storage/StorageMonitor.h"
#include "server/storage/StoragePool.h"
#include "server/storage/DiskOperations.h"
#include "server/update/UpdateManager.h"

#include <algorithm>
#include <arpa/inet.h>
#include <cctype>
#include <cerrno>
#include <cstring>
#include <cstdint>
#include <optional>
#include <sstream>
#include <string>
#include <sys/socket.h>
#include <unistd.h>
#include <unordered_map>
#include <utility>
#include <vector>

namespace homeai {

namespace {

using Header = std::pair<std::string, std::string>;

std::string htmlEscape(const std::string& value)
{
    std::string result;

    for (char c : value) {
        switch (c) {
            case '&': result += "&amp;"; break;
            case '<': result += "&lt;"; break;
            case '>': result += "&gt;"; break;
            case '"': result += "&quot;"; break;
            case '\'': result += "&#39;"; break;
            default: result += c; break;
        }
    }

    return result;
}

std::string trimCopy(std::string value)
{
    auto first = std::find_if_not(
        value.begin(),
        value.end(),
        [](unsigned char c) {
            return std::isspace(c);
        }
    );

    auto last = std::find_if_not(
        value.rbegin(),
        value.rend(),
        [](unsigned char c) {
            return std::isspace(c);
        }
    ).base();

    if (first >= last)
        return {};

    return std::string(first, last);
}

std::optional<std::int64_t>
parseInt64(
    const std::string& value
)
{
    if (value.empty())
        return std::nullopt;

    try {
        std::size_t consumed = 0;

        const auto result =
            std::stoll(
                value,
                &consumed,
                10
            );

        if (consumed != value.size())
            return std::nullopt;

        return result;
    }
    catch (...) {
        return std::nullopt;
    }
}

std::string requiredPermissionForPage(
    const std::string& path
)
{
    if (
        path == "/"
        ||
        path == "/system"
    ) {
        return "system.view";
    }

    if (path == "/network")
        return "network.view";

    if (path == "/storage")
        return "storage.view";

    if (path == "/files")
        return "files.read";

    if (path == "/cameras")
        return "cameras.view";

    if (path == "/smart-home")
        return "smart_home.view";

    if (path == "/automation")
        return "automation.view";

    if (path == "/ai")
        return "ai.use";

    if (path == "/users")
        return "users.view";

    if (path == "/hypervisor")
        return "hypervisor.view";

    if (path == "/settings")
        return "system.manage";

    if (path == "/admin")
        return "ai.manage";

    return {};
}

std::string lowerCopy(std::string value)
{
    std::transform(
        value.begin(),
        value.end(),
        value.begin(),
        [](unsigned char c) {
            return static_cast<char>(
                std::tolower(c)
            );
        }
    );

    return value;
}

std::string headerValue(
    const std::string& headers,
    const std::string& name
)
{
    const auto target =
        lowerCopy(name);

    std::istringstream stream(headers);
    std::string line;

    while (std::getline(stream, line)) {
        if (
            !line.empty() &&
            line.back() == '\r'
        ) {
            line.pop_back();
        }

        const auto separator =
            line.find(':');

        if (separator == std::string::npos)
            continue;

        const auto key =
            lowerCopy(
                trimCopy(
                    line.substr(
                        0,
                        separator
                    )
                )
            );

        if (key != target)
            continue;

        return trimCopy(
            line.substr(
                separator + 1
            )
        );
    }

    return {};
}

std::string cookieValue(
    const std::string& cookie_header,
    const std::string& name
)
{
    std::size_t start = 0;

    while (start < cookie_header.size()) {
        auto end =
            cookie_header.find(
                ';',
                start
            );

        if (end == std::string::npos)
            end = cookie_header.size();

        auto item =
            trimCopy(
                cookie_header.substr(
                    start,
                    end - start
                )
            );

        const auto separator =
            item.find('=');

        if (separator != std::string::npos) {
            const auto key =
                trimCopy(
                    item.substr(
                        0,
                        separator
                    )
                );

            if (key == name) {
                return item.substr(
                    separator + 1
                );
            }
        }

        start = end + 1;
    }

    return {};
}

int hexValue(char c)
{
    if (c >= '0' && c <= '9')
        return c - '0';

    if (c >= 'a' && c <= 'f')
        return c - 'a' + 10;

    if (c >= 'A' && c <= 'F')
        return c - 'A' + 10;

    return -1;
}

std::string urlDecode(const std::string& value)
{
    std::string result;

    for (
        std::size_t i = 0;
        i < value.size();
        ++i
    ) {
        if (value[i] == '+') {
            result += ' ';
        }
        else if (
            value[i] == '%' &&
            i + 2 < value.size()
        ) {
            const int high =
                hexValue(value[i + 1]);

            const int low =
                hexValue(value[i + 2]);

            if (high >= 0 && low >= 0) {
                result +=
                    static_cast<char>(
                        (high << 4) | low
                    );

                i += 2;
            }
            else {
                result += value[i];
            }
        }
        else {
            result += value[i];
        }
    }

    return result;
}

std::unordered_map<std::string, std::string>
parseForm(const std::string& body)
{
    std::unordered_map<std::string, std::string>
        values;

    std::size_t start = 0;

    while (start < body.size()) {
        auto end =
            body.find('&', start);

        if (end == std::string::npos)
            end = body.size();

        const auto part =
            body.substr(
                start,
                end - start
            );

        const auto separator =
            part.find('=');

        if (separator != std::string::npos) {
            const auto key =
                urlDecode(
                    part.substr(
                        0,
                        separator
                    )
                );

            const auto value =
                urlDecode(
                    part.substr(
                        separator + 1
                    )
                );

            values[key] = value;
        }

        start = end + 1;
    }

    return values;
}

std::vector<std::string> splitCsv(
    const std::string& value
)
{
    std::vector<std::string> items;

    std::size_t start = 0;

    while (start <= value.size()) {
        auto end =
            value.find(
                ',',
                start
            );

        if (end == std::string::npos)
            end = value.size();

        auto item =
            trimCopy(
                value.substr(
                    start,
                    end - start
                )
            );

        if (
            !item.empty()
            &&
            std::find(
                items.begin(),
                items.end(),
                item
            ) == items.end()
        ) {
            items.push_back(
                std::move(item)
            );
        }

        if (end == value.size())
            break;

        start = end + 1;
    }

    return items;
}

std::string joinCsv(
    const std::vector<std::string>& items
)
{
    std::ostringstream stream;

    for (
        std::size_t i = 0;
        i < items.size();
        ++i
    ) {
        if (i > 0)
            stream << ",";

        stream << items[i];
    }

    return stream.str();
}

std::string addCsvValue(
    const std::string& current,
    const std::string& value
)
{
    auto items =
        splitCsv(current);

    if (
        std::find(
            items.begin(),
            items.end(),
            value
        ) == items.end()
    ) {
        items.push_back(value);
    }

    return joinCsv(items);
}

std::string removeCsvValue(
    const std::string& current,
    const std::string& value
)
{
    auto items =
        splitCsv(current);

    items.erase(
        std::remove(
            items.begin(),
            items.end(),
            value
        ),
        items.end()
    );

    return joinCsv(items);
}

const BlockDeviceInfo* findDeviceInfo(
    const std::vector<BlockDeviceInfo>& devices,
    const std::string& device
)
{
    for (const auto& item : devices) {
        if (item.device == device)
            return &item;
    }

    return nullptr;
}

void sendAll(
    int socket_fd,
    const std::string& data
)
{
    std::size_t sent = 0;

    while (sent < data.size()) {
        const auto result =
            ::send(
                socket_fd,
                data.data() + sent,
                data.size() - sent,
                MSG_NOSIGNAL
            );

        if (result <= 0)
            return;

        sent +=
            static_cast<std::size_t>(
                result
            );
    }
}

void sendResponse(
    int client_fd,
    const std::string& status,
    const std::string& content_type,
    const std::string& body,
    const std::vector<Header>& extra_headers = {}
)
{
    std::ostringstream response;

    response
        << "HTTP/1.1 "
        << status
        << "\r\n"
        << "Content-Type: "
        << content_type
        << "\r\n"
        << "Content-Length: "
        << body.size()
        << "\r\n"
        << "Connection: close\r\n"
        << "Cache-Control: no-store\r\n"
        << "X-Content-Type-Options: nosniff\r\n"
        << "X-Frame-Options: DENY\r\n"
        << "Referrer-Policy: no-referrer\r\n"
        << "Content-Security-Policy: "
           "default-src 'self'; "
           "style-src 'self' 'unsafe-inline'; "
           "script-src 'self' 'unsafe-inline'; "
           "img-src 'self' data:; "
           "frame-ancestors 'none'; "
           "base-uri 'none'; "
           "form-action 'self'\r\n";

    for (const auto& [name, value] : extra_headers) {
        response
            << name
            << ": "
            << value
            << "\r\n";
    }

    response
        << "\r\n"
        << body;

    sendAll(
        client_fd,
        response.str()
    );
}

void sendRedirect(
    int client_fd,
    const std::string& location,
    const std::vector<Header>& extra_headers = {}
)
{
    auto headers =
        extra_headers;

    headers.emplace_back(
        "Location",
        location
    );

    sendResponse(
        client_fd,
        "303 See Other",
        "text/plain; charset=utf-8",
        "",
        headers
    );
}

std::size_t readContentLength(
    const std::string& headers
)
{
    const auto value =
        headerValue(
            headers,
            "Content-Length"
        );

    if (value.empty())
        return 0;

    try {
        return static_cast<std::size_t>(
            std::stoul(value)
        );
    }
    catch (...) {
        return 0;
    }
}

bool isApiPath(
    const std::string& path
)
{
    return path.rfind(
        "/api/",
        0
    ) == 0;
}

std::string sessionCookie(
    const std::string& token
)
{
    return
        "homeai_session=" +
        token +
        "; Path=/; HttpOnly; SameSite=Strict; Max-Age=28800";
}

std::string clearSessionCookie()
{
    return
        "homeai_session=; "
        "Path=/; HttpOnly; SameSite=Strict; Max-Age=0";
}

std::string renderAuthPage(
    const std::string& title,
    const std::string& subtitle,
    const std::string& action,
    bool confirm_password,
    const std::string& error
)
{
    std::ostringstream page;

    page << R"HTML(
<!doctype html>
<html lang="ru">
<head>
<script src="/assets/i18n.js"></script>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Home AI Core</title>
<style>
* {
    box-sizing: border-box;
}

body {
    margin: 0;
    min-height: 100vh;
    display: grid;
    place-items: center;
    font-family: system-ui, sans-serif;
    background: #0d1015;
    color: #eef0f5;
}

.auth {
    width: min(420px, calc(100% - 32px));
    background: #191c23;
    border: 1px solid #2c313c;
    border-radius: 14px;
    padding: 28px;
}

h1 {
    margin-top: 0;
    font-size: 24px;
}

p {
    color: #aeb4c0;
}

label {
    display: block;
    margin-top: 16px;
    margin-bottom: 6px;
}

input {
    width: 100%;
    padding: 11px;
    border-radius: 8px;
    border: 1px solid #3a414f;
    background: #101319;
    color: white;
}

button {
    width: 100%;
    margin-top: 22px;
    padding: 12px;
    border: 0;
    border-radius: 8px;
    font-weight: 700;
    cursor: pointer;
}

.error {
    margin-top: 14px;
    padding: 10px;
    border-radius: 8px;
    background: #35191d;
    color: #ffb8c0;
}
</style>
</head>
<body>
<div class="auth">
<h1>)HTML";

    page << htmlEscape(title);

    page << "</h1><p>"
         << htmlEscape(subtitle)
         << "</p>";

    if (!error.empty()) {
        page
            << "<div class=\"error\">"
            << htmlEscape(error)
            << "</div>";
    }

    page
        << "<form method=\"POST\" action=\""
        << htmlEscape(action)
        << "\">"
        << R"HTML(
<label>Имя пользователя</label>
<input
    name="username"
    autocomplete="username"
    minlength="3"
    maxlength="32"
    required>

<label>Пароль</label>
<input
    type="password"
    name="password"
    autocomplete="current-password"
    minlength="12"
    maxlength="256"
    required>
)HTML";

    if (confirm_password) {
        page << R"HTML(
<label>Повторите пароль</label>
<input
    type="password"
    name="password_confirm"
    autocomplete="new-password"
    minlength="12"
    maxlength="256"
    required>
)HTML";
    }

    page << R"HTML(
<button type="submit">Продолжить</button>
</form>
</div>
<footer>Copyright © TexNik</footer>
</body>
</html>
)HTML";

    return page.str();
}

}

WebServer::WebServer(
    CoreRuntime& runtime,
    SecurityManager& security,
    UpdateManager& updates,
    ModuleManager& modules,
    GpuMonitor gpu_monitor,
    CameraManager* cameras
)
    : runtime_(runtime),
      security_(security),
      updates_(updates),
      modules_(modules),
      gpu_monitor_(std::move(gpu_monitor)),
      cameras_(cameras)
{
}

WebServer::~WebServer()
{
    stop();
}

bool WebServer::start(
    const std::string& bind_address,
    std::uint16_t port
)
{
    if (running_)
        return true;

    bind_address_ = bind_address;
    port_ = port;

    server_fd_ =
        ::socket(
            AF_INET,
            SOCK_STREAM,
            0
        );

    if (server_fd_ < 0) {
        Logger::instance().error(
            "WebServer: socket creation failed"
        );

        return false;
    }

    int option = 1;

    setsockopt(
        server_fd_,
        SOL_SOCKET,
        SO_REUSEADDR,
        &option,
        sizeof(option)
    );

    sockaddr_in address{};
    address.sin_family = AF_INET;
    address.sin_port = htons(port_);

    if (
        inet_pton(
            AF_INET,
            bind_address_.c_str(),
            &address.sin_addr
        ) != 1
    ) {
        Logger::instance().error(
            "WebServer: invalid bind address: " +
            bind_address_
        );

        ::close(server_fd_);
        server_fd_ = -1;
        return false;
    }

    if (
        ::bind(
            server_fd_,
            reinterpret_cast<sockaddr*>(
                &address
            ),
            sizeof(address)
        ) < 0
    ) {
        Logger::instance().error(
            "WebServer: bind failed: " +
            std::string(
                std::strerror(errno)
            )
        );

        ::close(server_fd_);
        server_fd_ = -1;
        return false;
    }

    if (
        ::listen(
            server_fd_,
            32
        ) < 0
    ) {
        Logger::instance().error(
            "WebServer: listen failed"
        );

        ::close(server_fd_);
        server_fd_ = -1;
        return false;
    }

    running_ = true;

    server_thread_ =
        std::thread(
            &WebServer::run,
            this
        );

    Logger::instance().info(
        "Web interface started on http://" +
        bind_address_ +
        ":" +
        std::to_string(port_)
    );

    return true;
}

void WebServer::stop()
{
    if (!running_)
        return;

    running_ = false;

    if (server_fd_ >= 0) {
        ::shutdown(
            server_fd_,
            SHUT_RDWR
        );

        ::close(server_fd_);
        server_fd_ = -1;
    }

    if (server_thread_.joinable())
        server_thread_.join();

    {
        std::unique_lock<std::mutex>
            lock(client_mutex_);

        client_cv_.wait(
            lock,
            [this]() {
                return
                    active_clients_ == 0;
            }
        );
    }

    Logger::instance().info(
        "Web interface stopped"
    );
}

bool WebServer::isRunning() const
{
    return running_;
}

void WebServer::run()
{
    while (running_) {
        sockaddr_in client_address{};
        socklen_t client_size =
            sizeof(client_address);

        const int client_fd =
            ::accept(
                server_fd_,
                reinterpret_cast<sockaddr*>(
                    &client_address
                ),
                &client_size
            );

        if (client_fd < 0) {
            if (running_) {
                Logger::instance().warning(
                    "WebServer: accept failed"
                );
            }

            continue;
        }

        timeval client_timeout{};
        client_timeout.tv_sec = 15;

        ::setsockopt(
            client_fd,
            SOL_SOCKET,
            SO_RCVTIMEO,
            &client_timeout,
            sizeof(client_timeout)
        );

        ::setsockopt(
            client_fd,
            SOL_SOCKET,
            SO_SNDTIMEO,
            &client_timeout,
            sizeof(client_timeout)
        );

        bool client_slot = false;

        {
            std::lock_guard<std::mutex>
                lock(client_mutex_);

            if (
                active_clients_ <
                64
            ) {
                ++active_clients_;
                client_slot = true;
            }
        }

        if (!client_slot) {
            sendResponse(
                client_fd,
                "503 Service Unavailable",
                "text/plain; charset=utf-8",
                "Server busy"
            );

            ::shutdown(
                client_fd,
                SHUT_RDWR
            );

            ::close(client_fd);
            continue;
        }

        try {
            std::thread(
                &WebServer::handleClientWorker,
                this,
                client_fd
            ).detach();
        }
        catch (...) {
            ::shutdown(
                client_fd,
                SHUT_RDWR
            );

            ::close(client_fd);

            {
                std::lock_guard<std::mutex>
                    lock(client_mutex_);

                if (active_clients_ > 0)
                    --active_clients_;
            }

            client_cv_.notify_all();

            Logger::instance().error(
                "WebServer: unable to start client worker"
            );
        }
    }
}

void WebServer::handleClientWorker(
    int client_fd
)
{
    try {
        handleClient(
            client_fd
        );
    }
    catch (...) {
        Logger::instance().error(
            "WebServer: client worker failed"
        );
    }

    ::shutdown(
        client_fd,
        SHUT_RDWR
    );

    ::close(client_fd);

    {
        std::lock_guard<std::mutex>
            lock(client_mutex_);

        if (active_clients_ > 0)
            --active_clients_;
    }

    client_cv_.notify_all();
}

void WebServer::handleClient(
    int client_fd
)
{
    std::string request;
    char buffer[4096];

    while (request.size() < 65536) {
        const auto received =
            ::recv(
                client_fd,
                buffer,
                sizeof(buffer),
                0
            );

        if (received <= 0)
            break;

        request.append(
            buffer,
            static_cast<std::size_t>(
                received
            )
        );

        const auto header_end =
            request.find(
                "\r\n\r\n"
            );

        if (header_end != std::string::npos) {
            const auto headers =
                request.substr(
                    0,
                    header_end
                );

            const auto content_length =
                readContentLength(headers);

            const auto body_size =
                request.size() -
                (header_end + 4);

            if (body_size >= content_length)
                break;
        }
    }

    const auto header_end =
        request.find(
            "\r\n\r\n"
        );

    if (header_end == std::string::npos)
        return;

    const auto headers =
        request.substr(
            0,
            header_end
        );

    const auto body =
        request.substr(
            header_end + 4
        );

    const auto first_line_end =
        headers.find("\r\n");

    if (
        first_line_end ==
        std::string::npos
    ) {
        return;
    }

    const auto first_line =
        headers.substr(
            0,
            first_line_end
        );

    std::istringstream request_line(
        first_line
    );

    std::string method;
    std::string path;
    std::string http_version;

    request_line
        >> method
        >> path
        >> http_version;

    const auto query =
        path.find('?');

    std::string query_string;

    if (query != std::string::npos) {
        query_string =
            path.substr(
                query + 1
            );

        path.resize(query);
    }

    if (method == "GET" && path == "/assets/i18n.js") {
        sendResponse(client_fd, "200 OK", "application/javascript; charset=utf-8", localizationScript());
        return;
    }

    const auto cookie_header =
        headerValue(
            headers,
            "Cookie"
        );

    const auto session_token =
        cookieValue(
            cookie_header,
            "homeai_session"
        );

    auto session =
        security_.validateSession(
            session_token
        );

    const bool users_exist =
        security_.hasUsers();

    if (!users_exist) {
        if (
            method == "GET" &&
            path == "/setup"
        ) {
            sendResponse(
                client_fd,
                "200 OK",
                "text/html; charset=utf-8",
                renderAuthPage(
                    "Первоначальная настройка",
                    "Создайте первого администратора Home AI Core.",
                    "/setup",
                    true,
                    ""
                )
            );

            return;
        }

        if (
            method == "POST" &&
            path == "/setup"
        ) {
            const auto form =
                parseForm(body);

            const auto username =
                form.contains("username")
                ? form.at("username")
                : "";

            const auto password =
                form.contains("password")
                ? form.at("password")
                : "";

            const auto confirmation =
                form.contains("password_confirm")
                ? form.at("password_confirm")
                : "";

            if (password != confirmation) {
                sendResponse(
                    client_fd,
                    "400 Bad Request",
                    "text/html; charset=utf-8",
                    renderAuthPage(
                        "Первоначальная настройка",
                        "Создайте первого администратора Home AI Core.",
                        "/setup",
                        true,
                        "Пароли не совпадают."
                    )
                );

                return;
            }

            std::string error;

            if (
                !security_.createUser(
                    username,
                    password,
                    UserRole::Admin,
                    error
                )
            ) {
                sendResponse(
                    client_fd,
                    "400 Bad Request",
                    "text/html; charset=utf-8",
                    renderAuthPage(
                        "Первоначальная настройка",
                        "Создайте первого администратора Home AI Core.",
                        "/setup",
                        true,
                        error
                    )
                );

                return;
            }

            SessionInfo info;

            const auto token =
                security_.login(
                    username,
                    password,
                    info,
                    error
                );

            if (!token) {
                sendRedirect(
                    client_fd,
                    "/login"
                );

                return;
            }

            sendRedirect(
                client_fd,
                "/",
                {
                    {
                        "Set-Cookie",
                        sessionCookie(*token)
                    }
                }
            );

            return;
        }

        if (isApiPath(path)) {
            sendResponse(
                client_fd,
                "503 Service Unavailable",
                "application/json; charset=utf-8",
                "{\"error\":\"setup_required\"}"
            );

            return;
        }

        sendRedirect(
            client_fd,
            "/setup"
        );

        return;
    }

    if (
        path == "/setup"
    ) {
        sendRedirect(
            client_fd,
            session
                ? "/"
                : "/login"
        );

        return;
    }

    if (
        method == "GET" &&
        path == "/login"
    ) {
        if (session) {
            sendRedirect(
                client_fd,
                "/"
            );

            return;
        }

        sendResponse(
            client_fd,
            "200 OK",
            "text/html; charset=utf-8",
            renderAuthPage(
                "Вход в Home AI Core",
                "Введите данные локального пользователя.",
                "/login",
                false,
                ""
            )
        );

        return;
    }

    if (
        method == "POST" &&
        path == "/login"
    ) {
        const auto form =
            parseForm(body);

        const auto username =
            form.contains("username")
            ? form.at("username")
            : "";

        const auto password =
            form.contains("password")
            ? form.at("password")
            : "";

        SessionInfo info;
        std::string error;

        const auto token =
            security_.login(
                username,
                password,
                info,
                error
            );

        if (!token) {
            sendResponse(
                client_fd,
                "401 Unauthorized",
                "text/html; charset=utf-8",
                renderAuthPage(
                    "Вход в Home AI Core",
                    "Введите данные локального пользователя.",
                    "/login",
                    false,
                    error
                )
            );

            return;
        }

        sendRedirect(
            client_fd,
            "/",
            {
                {
                    "Set-Cookie",
                    sessionCookie(*token)
                }
            }
        );

        return;
    }

    if (
        method == "POST" &&
        path == "/logout"
    ) {
        security_.logout(
            session_token
        );

        sendRedirect(
            client_fd,
            "/login",
            {
                {
                    "Set-Cookie",
                    clearSessionCookie()
                }
            }
        );

        return;
    }

    if (!session) {
        if (isApiPath(path)) {
            sendResponse(
                client_fd,
                "401 Unauthorized",
                "application/json; charset=utf-8",
                "{\"error\":\"authentication_required\"}"
            );

            return;
        }

        sendRedirect(
            client_fd,
            "/login"
        );

        return;
    }

    const auto page_permission =
        requiredPermissionForPage(
            path
        );

    if (
        !page_permission.empty()
        &&
        !security_.hasPermission(
            *session,
            page_permission
        )
    ) {
        sendResponse(
            client_fd,
            "403 Forbidden",
            isApiPath(path)
                ? "application/json; charset=utf-8"
                : "text/plain; charset=utf-8",
            isApiPath(path)
                ? "{\"error\":\"permission_denied\"}"
                : "403 Forbidden"
        );

        return;
    }

    if (
        (
            path == "/api/admin/gpus"
            ||
            path == "/api/admin/accelerator"
        )
        &&
        !security_.hasPermission(
            *session,
            "ai.manage"
        )
    ) {
        sendResponse(
            client_fd,
            "403 Forbidden",
            "application/json; charset=utf-8",
            "{\"error\":\"permission_denied\"}"
        );

        return;
    }
    if (method == "GET" && path == "/api/admin/gpus") {
        const auto inventory = gpu_monitor_.snapshot();
        const auto selected = runtime_.config().get("ai.accelerator.pci_address");
        bool present = false;
        std::ostringstream json;
        json << "{\"available\":" << (inventory.available ? "true" : "false")
             << ",\"selected\":\"" << jsonEscape(selected) << "\",\"devices\":[";
        bool first = true;
        for (const auto& gpu : inventory.devices) {
            if (!first) json << ',';
            first = false;
            present = present || selected == gpu.pci_address;
            json << "{\"pci_address\":\"" << jsonEscape(gpu.pci_address)
                 << "\",\"vendor\":\"" << jsonEscape(gpu.vendor)
                 << "\",\"vendor_id\":\"" << jsonEscape(gpu.vendor_id)
                 << "\",\"device_id\":\"" << jsonEscape(gpu.device_id)
                 << "\",\"driver\":\"" << jsonEscape(gpu.driver) << "\"}";
        }
        json << "],\"selected_present\":" << (present ? "true" : "false") << '}';
        sendResponse(client_fd, "200 OK", "application/json; charset=utf-8", json.str());
        return;
    }
    if (method == "POST" && path == "/api/admin/accelerator") {
        // Custom header prevents cross-origin form submissions; no CORS is enabled.
        if (headerValue(headers, "X-HomeAI-Request") != "1") {
            sendResponse(client_fd, "403 Forbidden", "application/json; charset=utf-8", "{\"error\":\"request_header_required\"}");
            return;
        }
        const auto form = parseForm(body);
        if (!form.contains("pci_address")) {
            sendResponse(client_fd, "400 Bad Request", "application/json; charset=utf-8", "{\"error\":\"invalid_gpu\"}");
            return;
        }
        const auto selected = form.at("pci_address");
        if (!selected.empty()) {
            const auto inventory = gpu_monitor_.snapshot();
            const bool found = inventory.available && GpuMonitor::validAddress(selected) &&
                std::any_of(inventory.devices.begin(), inventory.devices.end(), [&](const auto& gpu) { return gpu.pci_address == selected; });
            if (!found) {
                sendResponse(client_fd, "400 Bad Request", "application/json; charset=utf-8", "{\"error\":\"gpu_not_found\"}");
                return;
            }
        }
        if (!runtime_.config().setAndSave("ai.accelerator.pci_address", selected)) {
            sendResponse(client_fd, "500 Internal Server Error", "application/json; charset=utf-8", "{\"error\":\"config_save_failed\"}");
            return;
        }
        security_.audit("ai.accelerator.select", session->username, selected.empty() ? "none" : selected);
        sendResponse(client_fd, "200 OK", "application/json; charset=utf-8", "{\"success\":true}");
        return;
    }

    if (
        method == "GET" &&
        path == "/api/session"
    ) {
        std::ostringstream json;

        json
            << "{"
            << "\"user_id\":"
            << session->user_id
            << ",\"username\":\""
            << jsonEscape(
                session->username
            )
            << "\",\"role\":\""
            << jsonEscape(
                SecurityManager::roleToString(
                    session->role
                )
            )
            << "\",\"permissions\":[";

        bool first = true;

        for (
            const auto& permission :
            SecurityManager::
                permissionCatalog()
        ) {
            if (
                !security_.hasPermission(
                    *session,
                    permission
                )
            ) {
                continue;
            }

            if (!first)
                json << ",";

            first = false;

            json
                << "\""
                << jsonEscape(
                    permission
                )
                << "\"";
        }

        json << "]}";

        sendResponse(
            client_fd,
            "200 OK",
            "application/json; charset=utf-8",
            json.str()
        );

        return;
    }

    if (
        method == "GET"
        &&
        path == "/api/users"
    ) {
        if (
            !security_.hasPermission(
                *session,
                "users.view"
            )
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"error\":\"permission_denied\"}"
            );

            return;
        }

        std::string error;

        const auto users =
            security_.listUsers(
                error
            );

        if (!error.empty()) {
            sendResponse(
                client_fd,
                "500 Internal Server Error",
                "application/json; charset=utf-8",
                "{\"error\":\"user_database_error\",\"message\":\"" +
                jsonEscape(error) +
                "\"}"
            );

            return;
        }

        std::ostringstream json;

        json << "{\"permission_catalog\":[";

        bool first_permission = true;

        for (
            const auto& permission :
            SecurityManager::
                permissionCatalog()
        ) {
            if (!first_permission)
                json << ",";

            first_permission = false;

            json
                << "\""
                << jsonEscape(permission)
                << "\"";
        }

        json << "],\"users\":[";

        bool first_user = true;

        for (const auto& user : users) {
            if (!first_user)
                json << ",";

            first_user = false;

            json
                << "{"
                << "\"id\":"
                << user.id
                << ",\"username\":\""
                << jsonEscape(
                    user.username
                )
                << "\",\"role\":\""
                << jsonEscape(
                    SecurityManager::
                        roleToString(
                            user.role
                        )
                )
                << "\",\"enabled\":"
                << (
                    user.enabled
                    ? "true"
                    : "false"
                )
                << ",\"created_at\":"
                << user.created_at
                << ",\"updated_at\":"
                << user.updated_at
                << ",\"last_login_at\":"
                << user.last_login_at
                << ",\"permission_overrides\":{";

            bool first_override = true;

            for (
                const auto& [
                    permission,
                    decision
                ] :
                user.permission_overrides
            ) {
                if (!first_override)
                    json << ",";

                first_override = false;

                json
                    << "\""
                    << jsonEscape(
                        permission
                    )
                    << "\":"
                    << decision;
            }

            json
                << "},\"effective_permissions\":[";

            bool first_effective = true;

            for (
                const auto& permission :
                user.effective_permissions
            ) {
                if (!first_effective)
                    json << ",";

                first_effective = false;

                json
                    << "\""
                    << jsonEscape(
                        permission
                    )
                    << "\"";
            }

            json << "]}";
        }

        json << "]}";

        sendResponse(
            client_fd,
            "200 OK",
            "application/json; charset=utf-8",
            json.str()
        );

        return;
    }

    if (
        method == "GET"
        &&
        path == "/api/users/sessions"
    ) {
        if (
            !security_.hasPermission(
                *session,
                "users.manage"
            )
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"error\":\"permission_denied\"}"
            );

            return;
        }

        const auto query_values =
            parseForm(
                query_string
            );

        std::optional<std::int64_t>
            user_id;

        if (
            query_values.contains(
                "user_id"
            )
        ) {
            user_id =
                parseInt64(
                    query_values.at(
                        "user_id"
                    )
                );

            if (!user_id) {
                sendResponse(
                    client_fd,
                    "400 Bad Request",
                    "application/json; charset=utf-8",
                    "{\"error\":\"invalid_user_id\"}"
                );

                return;
            }
        }

        std::string error;

        const auto sessions =
            security_.listSessions(
                user_id,
                error
            );

        if (!error.empty()) {
            sendResponse(
                client_fd,
                "500 Internal Server Error",
                "application/json; charset=utf-8",
                "{\"error\":\"session_database_error\"}"
            );

            return;
        }

        std::ostringstream json;

        json << "{\"sessions\":[";

        bool first = true;

        for (
            const auto& item :
            sessions
        ) {
            if (!first)
                json << ",";

            first = false;

            json
                << "{"
                << "\"id\":"
                << item.id
                << ",\"user_id\":"
                << item.user_id
                << ",\"username\":\""
                << jsonEscape(
                    item.username
                )
                << "\",\"created_at\":"
                << item.created_at
                << ",\"expires_at\":"
                << item.expires_at
                << ",\"last_seen_at\":"
                << item.last_seen_at
                << "}";
        }

        json << "]}";

        sendResponse(
            client_fd,
            "200 OK",
            "application/json; charset=utf-8",
            json.str()
        );

        return;
    }

    if (
        method == "GET"
        &&
        path == "/api/users/audit"
    ) {
        if (
            !security_.hasPermission(
                *session,
                "users.manage"
            )
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"error\":\"permission_denied\"}"
            );

            return;
        }

        const auto values =
            parseForm(
                query_string
            );

        int limit = 50;
        int offset = 0;

        if (
            values.contains("limit")
        ) {
            const auto parsed =
                parseInt64(
                    values.at("limit")
                );

            if (parsed)
                limit =
                    static_cast<int>(
                        *parsed
                    );
        }

        if (
            values.contains("offset")
        ) {
            const auto parsed =
                parseInt64(
                    values.at("offset")
                );

            if (parsed)
                offset =
                    static_cast<int>(
                        *parsed
                    );
        }

        const auto username =
            values.contains(
                "username"
            )
            ? values.at(
                "username"
            )
            : "";

        const auto event =
            values.contains(
                "event"
            )
            ? values.at(
                "event"
            )
            : "";

        std::string error;

        const auto entries =
            security_.listAudit(
                limit,
                offset,
                username,
                event,
                error
            );

        if (!error.empty()) {
            sendResponse(
                client_fd,
                "500 Internal Server Error",
                "application/json; charset=utf-8",
                "{\"error\":\"audit_database_error\"}"
            );

            return;
        }

        std::ostringstream json;

        json << "{\"entries\":[";

        bool first = true;

        for (
            const auto& entry :
            entries
        ) {
            if (!first)
                json << ",";

            first = false;

            json
                << "{"
                << "\"id\":"
                << entry.id
                << ",\"created_at\":"
                << entry.created_at
                << ",\"event\":\""
                << jsonEscape(
                    entry.event
                )
                << "\",\"username\":\""
                << jsonEscape(
                    entry.username
                )
                << "\",\"details\":\""
                << jsonEscape(
                    entry.details
                )
                << "\"}";
        }

        json << "]}";

        sendResponse(
            client_fd,
            "200 OK",
            "application/json; charset=utf-8",
            json.str()
        );

        return;
    }

    if (
        method == "POST"
        &&
        path.rfind(
            "/api/users/",
            0
        ) == 0
    ) {
        if (
            !security_.hasPermission(
                *session,
                "users.manage"
            )
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"error\":\"permission_denied\"}"
            );

            return;
        }

        if (
            headerValue(
                headers,
                "X-HomeAI-Request"
            ) != "1"
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"error\":\"request_header_required\"}"
            );

            return;
        }

        const auto form =
            parseForm(body);

        auto sendUserResult =
            [&](bool success,
                const std::string& message,
                const std::string& code = "") {
                sendResponse(
                    client_fd,
                    success
                        ? "200 OK"
                        : "400 Bad Request",
                    "application/json; charset=utf-8",
                    "{\"success\":" +
                    std::string(
                        success
                        ? "true"
                        : "false"
                    )
                    +
                    ",\"code\":\"" +
                    jsonEscape(code)
                    +
                    "\",\"message\":\"" +
                    jsonEscape(message)
                    +
                    "\"}"
                );
            };

        if (
            path == "/api/users/create"
        ) {
            const auto username =
                form.contains(
                    "username"
                )
                ? form.at(
                    "username"
                )
                : "";

            const auto password =
                form.contains(
                    "password"
                )
                ? form.at(
                    "password"
                )
                : "";

            const auto role_text =
                form.contains(
                    "role"
                )
                ? form.at("role")
                : "viewer";

            const auto role =
                SecurityManager::
                    roleFromString(
                        role_text
                    );

            if (!role) {
                sendUserResult(
                    false,
                    "Invalid role",
                    "invalid_role"
                );

                return;
            }

            std::string error;

            const bool success =
                security_.createUser(
                    username,
                    password,
                    *role,
                    error
                );

            if (success) {
                security_.audit(
                    "user.admin.create",
                    session->username,
                    "created=" +
                        username
                        +
                        " role=" +
                        role_text
                );
            }

            sendUserResult(
                success,
                success
                    ? "User created"
                    : error,
                success
                    ? "ok"
                    : "create_failed"
            );

            return;
        }

        const auto user_id =
            form.contains(
                "user_id"
            )
            ? parseInt64(
                form.at(
                    "user_id"
                )
            )
            : std::nullopt;

        if (!user_id) {
            sendUserResult(
                false,
                "Invalid user ID",
                "invalid_user_id"
            );

            return;
        }

        if (
            path == "/api/users/update"
        ) {
            const auto role =
                SecurityManager::
                    roleFromString(
                        form.contains(
                            "role"
                        )
                        ? form.at(
                            "role"
                        )
                        : ""
                    );

            if (!role) {
                sendUserResult(
                    false,
                    "Invalid role",
                    "invalid_role"
                );

                return;
            }

            const bool enabled =
                form.contains(
                    "enabled"
                )
                &&
                (
                    form.at(
                        "enabled"
                    ) == "1"
                    ||
                    form.at(
                        "enabled"
                    ) == "true"
                );

            std::string error;

            const bool success =
                security_.updateUser(
                    *user_id,
                    *role,
                    enabled,
                    error
                );

            if (success) {
                security_.audit(
                    "user.admin.update",
                    session->username,
                    "user_id=" +
                        std::to_string(
                            *user_id
                        )
                );
            }

            sendUserResult(
                success,
                success
                    ? "User updated"
                    : error,
                success
                    ? "ok"
                    : "update_failed"
            );

            return;
        }

        if (
            path == "/api/users/password"
        ) {
            const auto password =
                form.contains(
                    "password"
                )
                ? form.at(
                    "password"
                )
                : "";

            std::string error;

            const bool success =
                security_.setPassword(
                    *user_id,
                    password,
                    error
                );

            if (success) {
                security_.audit(
                    "user.admin.password",
                    session->username,
                    "user_id=" +
                        std::to_string(
                            *user_id
                        )
                );
            }

            sendUserResult(
                success,
                success
                    ? "Password changed"
                    : error,
                success
                    ? "ok"
                    : "password_failed"
            );

            return;
        }

        if (
            path == "/api/users/delete"
        ) {
            std::string error;

            const bool success =
                security_.deleteUser(
                    *user_id,
                    error
                );

            if (success) {
                security_.audit(
                    "user.admin.delete",
                    session->username,
                    "user_id=" +
                        std::to_string(
                            *user_id
                        )
                );
            }

            sendUserResult(
                success,
                success
                    ? "User deleted"
                    : error,
                success
                    ? "ok"
                    : "delete_failed"
            );

            return;
        }

        if (
            path == "/api/users/permission"
        ) {
            const auto permission =
                form.contains(
                    "permission"
                )
                ? form.at(
                    "permission"
                )
                : "";

            const auto decision_value =
                form.contains(
                    "decision"
                )
                ? parseInt64(
                    form.at(
                        "decision"
                    )
                )
                : std::nullopt;

            if (
                !decision_value
                ||
                *decision_value < -1
                ||
                *decision_value > 1
            ) {
                sendUserResult(
                    false,
                    "Invalid permission decision",
                    "invalid_permission"
                );

                return;
            }

            std::string error;

            const bool success =
                security_.
                    setPermissionOverride(
                        *user_id,
                        permission,
                        static_cast<int>(
                            *decision_value
                        ),
                        error
                    );

            if (success) {
                security_.audit(
                    "user.admin.permission",
                    session->username,
                    "user_id=" +
                        std::to_string(
                            *user_id
                        )
                        +
                        " permission=" +
                        permission
                );
            }

            sendUserResult(
                success,
                success
                    ? "Permission updated"
                    : error,
                success
                    ? "ok"
                    : "permission_failed"
            );

            return;
        }

        if (
            path == "/api/users/session/revoke"
        ) {
            const auto session_id =
                form.contains(
                    "session_id"
                )
                ? parseInt64(
                    form.at(
                        "session_id"
                    )
                )
                : std::nullopt;

            if (!session_id) {
                sendUserResult(
                    false,
                    "Invalid session ID",
                    "invalid_session_id"
                );

                return;
            }

            std::string error;

            const bool success =
                security_.revokeSession(
                    *session_id,
                    error
                );

            if (success) {
                security_.audit(
                    "session.admin.revoke",
                    session->username,
                    "session_id=" +
                        std::to_string(
                            *session_id
                        )
                );
            }

            sendUserResult(
                success,
                success
                    ? "Session revoked"
                    : error,
                success
                    ? "ok"
                    : "session_failed"
            );

            return;
        }

        if (
            path ==
                "/api/users/sessions/revoke"
        ) {
            std::string error;

            const bool success =
                security_.revokeUserSessions(
                    *user_id,
                    error
                );

            if (success) {
                security_.audit(
                    "session.admin.revoke_all",
                    session->username,
                    "user_id=" +
                        std::to_string(
                            *user_id
                        )
                );
            }

            sendUserResult(
                success,
                success
                    ? "User sessions revoked"
                    : error,
                success
                    ? "ok"
                    : "session_failed"
            );

            return;
        }

        sendUserResult(
            false,
            "Unknown user operation",
            "unsupported_action"
        );

        return;
    }

    if (
        method == "GET" &&
        path == "/api/status"
    ) {
        const auto name =
            runtime_.config().get(
                "core.name",
                "Home AI Core"
            );

        const auto version =
            runtime_.config().get(
                "core.version",
                "unknown"
            );

        const std::string response =
            "{"
            "\"name\":\"" +
            jsonEscape(name) +
            "\","
            "\"version\":\"" +
            jsonEscape(version) +
            "\","
            "\"status\":\"running\","
            "\"web\":\"running\""
            "}";

        sendResponse(
            client_fd,
            "200 OK",
            "application/json; charset=utf-8",
            response
        );

        return;
    }

    if (
        method == "GET"
        &&
        path == "/api/cameras"
    ) {
        if (
            !security_.hasPermission(
                *session,
                "cameras.view"
            )
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"success\":false,\"error\":\"permission_denied\"}"
            );

            return;
        }

        if (
            !cameras_
            ||
            !cameras_->healthy()
        ) {
            sendResponse(
                client_fd,
                "503 Service Unavailable",
                "application/json; charset=utf-8",
                "{\"success\":false,\"error\":\"camera_core_unavailable\"}"
            );

            return;
        }

        std::string error;
        const auto items =
            cameras_->cameras(error);

        if (!error.empty()) {
            sendResponse(
                client_fd,
                "500 Internal Server Error",
                "application/json; charset=utf-8",
                "{\"success\":false,\"message\":\"" +
                jsonEscape(error) +
                "\"}"
            );

            return;
        }

        std::ostringstream json;
        json << "{\"success\":true,\"cameras\":[";

        bool first = true;

        for (const auto& camera : items) {
            if (!first)
                json << ",";

            first = false;

            json
                << "{"
                << "\"id\":"
                << camera.id
                << ",\"name\":\""
                << jsonEscape(camera.name)
                << "\",\"rtsp_url\":\""
                << jsonEscape(camera.rtsp_url)
                << "\",\"onvif_xaddr\":\""
                << jsonEscape(camera.onvif_xaddr)
                << "\",\"manufacturer\":\""
                << jsonEscape(camera.manufacturer)
                << "\",\"model\":\""
                << jsonEscape(camera.model)
                << "\",\"firmware_version\":\""
                << jsonEscape(camera.firmware_version)
                << "\",\"serial_number\":\""
                << jsonEscape(camera.serial_number)
                << "\",\"hardware_id\":\""
                << jsonEscape(camera.hardware_id)
                << "\",\"ptz_xaddr\":\""
                << jsonEscape(camera.ptz_xaddr)
                << "\",\"ptz_profile_token\":\""
                << jsonEscape(camera.ptz_profile_token)
                << "\",\"ptz_supported\":"
                << (
                    !camera.ptz_xaddr.empty()
                    &&
                    !camera.ptz_profile_token.empty()
                    ? "true"
                    : "false"
                )
                << ",\"username\":\""
                << jsonEscape(camera.username)
                << "\",\"has_password\":"
                << (
                    camera.has_password
                    ? "true"
                    : "false"
                )
                << ",\"enabled\":"
                << (
                    camera.enabled
                    ? "true"
                    : "false"
                )
                << ",\"status\":\""
                << jsonEscape(camera.status)
                << "\",\"last_error\":\""
                << jsonEscape(camera.last_error)
                << "\",\"last_seen_at\":"
                << camera.last_seen_at
                << ",\"created_at\":"
                << camera.created_at
                << ",\"updated_at\":"
                << camera.updated_at
                << "}";
        }

        json << "]}";

        sendResponse(
            client_fd,
            "200 OK",
            "application/json; charset=utf-8",
            json.str()
        );

        return;
    }

    if (
        method == "GET"
        &&
        path == "/api/cameras/snapshot"
    ) {
        if (
            !security_.hasPermission(
                *session,
                "cameras.view"
            )
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"success\":false,\"error\":\"permission_denied\"}"
            );

            return;
        }

        if (
            !cameras_
            ||
            !cameras_->healthy()
        ) {
            sendResponse(
                client_fd,
                "503 Service Unavailable",
                "application/json; charset=utf-8",
                "{\"success\":false,\"error\":\"camera_core_unavailable\"}"
            );

            return;
        }

        const auto values =
            parseForm(
                query_string
            );

        std::int64_t id = 0;

        if (
            values.contains("id")
        ) {
            const auto parsed =
                parseInt64(
                    values.at("id")
                );

            if (
                parsed
                &&
                *parsed > 0
            ) {
                id = *parsed;
            }
        }

        if (id <= 0) {
            sendResponse(
                client_fd,
                "400 Bad Request",
                "application/json; charset=utf-8",
                "{\"success\":false,\"message\":\"Некорректный ID камеры.\"}"
            );

            return;
        }

        auto result =
            cameras_->snapshot(id);

        if (!result.success) {
            sendResponse(
                client_fd,
                result.code ==
                    "camera_unavailable"
                    ? "404 Not Found"
                    : "400 Bad Request",
                "application/json; charset=utf-8",
                "{\"success\":false,\"code\":\"" +
                jsonEscape(result.code)
                +
                "\",\"message\":\"" +
                jsonEscape(result.message)
                +
                "\"}"
            );

            return;
        }

        sendResponse(
            client_fd,
            "200 OK",
            "image/jpeg",
            result.jpeg
        );

        return;
    }

    if (
        method == "POST"
        &&
        (
            path == "/api/cameras/save"
            ||
            path == "/api/cameras/delete"
            ||
            path == "/api/cameras/probe"
            ||
            path == "/api/cameras/media-probe"
            ||
            path == "/api/cameras/discover"
            ||
            path == "/api/cameras/onvif-streams"
            ||
            path == "/api/cameras/ptz"
        )
    ) {
        if (
            !security_.hasPermission(
                *session,
                "cameras.manage"
            )
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"success\":false,\"message\":\"Требуются права управления камерами.\"}"
            );

            return;
        }

        if (
            headerValue(
                headers,
                "X-HomeAI-Request"
            ) != "1"
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"success\":false,\"error\":\"request_header_required\"}"
            );

            return;
        }

        if (
            !cameras_
            ||
            !cameras_->healthy()
        ) {
            sendResponse(
                client_fd,
                "503 Service Unavailable",
                "application/json; charset=utf-8",
                "{\"success\":false,\"error\":\"camera_core_unavailable\"}"
            );

            return;
        }

        const auto form =
            parseForm(body);

        if (
            path ==
                "/api/cameras/discover"
        ) {
            int timeout_ms = 1800;

            if (
                form.contains(
                    "timeout_ms"
                )
            ) {
                try {
                    timeout_ms =
                        std::stoi(
                            form.at(
                                "timeout_ms"
                            )
                        );
                }
                catch (...) {
                    timeout_ms = 1800;
                }
            }

            std::string error;

            const auto devices =
                cameras_->discoverCameras(
                    timeout_ms,
                    error
                );

            if (!error.empty()) {
                sendResponse(
                    client_fd,
                    "500 Internal Server Error",
                    "application/json; charset=utf-8",
                    "{\"success\":false,\"message\":\"" +
                    jsonEscape(error)
                    +
                    "\"}"
                );

                return;
            }

            security_.audit(
                "camera.discover",
                session->username,
                "devices=" +
                std::to_string(
                    devices.size()
                )
            );

            std::ostringstream json;
            json
                << "{\"success\":true,\"devices\":[";

            bool first = true;

            for (
                const auto& device :
                devices
            ) {
                if (!first)
                    json << ",";

                first = false;

                json
                    << "{"
                    << "\"remote_address\":\""
                    << jsonEscape(
                        device.address
                    )
                    << "\",\"xaddr\":\""
                    << jsonEscape(
                        device.onvif_xaddr
                    )
                    << "\",\"vendor_hint\":\""
                    << jsonEscape(
                        device.vendor_hint
                    )
                    << "\",\"suggested_rtsp_url\":\""
                    << jsonEscape(
                        device.suggested_rtsp_url
                    )
                    << "\",\"rtsp_port\":"
                    << device.rtsp_port
                    << ",\"onvif\":"
                    << (
                        device.onvif
                        ? "true"
                        : "false"
                    )
                    << "}";
            }

            json << "]}";

            sendResponse(
                client_fd,
                "200 OK",
                "application/json; charset=utf-8",
                json.str()
            );

            return;
        }

        if (
            path ==
                "/api/cameras/onvif-streams"
        ) {
            const auto xaddr =
                form.contains(
                    "onvif_xaddr"
                )
                ? form.at(
                    "onvif_xaddr"
                )
                : "";

            const auto username =
                form.contains(
                    "username"
                )
                ? form.at(
                    "username"
                )
                : "";

            const auto password =
                form.contains(
                    "password"
                )
                ? form.at(
                    "password"
                )
                : "";

            const auto result =
                cameras_->
                    discoverOnvifStreams(
                        xaddr,
                        username,
                        password
                    );

            security_.audit(
                "camera.onvif.streams",
                session->username,
                "profiles=" +
                std::to_string(
                    result.profiles.size()
                )
                +
                " result=" +
                result.code
            );

            std::ostringstream json;

            json
                << "{\"success\":"
                << (
                    result.success
                    ? "true"
                    : "false"
                )
                << ",\"code\":\""
                << jsonEscape(
                    result.code
                )
                << "\",\"message\":\""
                << jsonEscape(
                    result.message
                )
                << "\",\"media_xaddr\":\""
                << jsonEscape(
                    result.media_xaddr
                )
                << "\",\"device_info\":{"
                << "\"manufacturer\":\""
                << jsonEscape(
                    result.device_info.manufacturer
                )
                << "\",\"model\":\""
                << jsonEscape(
                    result.device_info.model
                )
                << "\",\"firmware_version\":\""
                << jsonEscape(
                    result.device_info.firmware_version
                )
                << "\",\"serial_number\":\""
                << jsonEscape(
                    result.device_info.serial_number
                )
                << "\",\"hardware_id\":\""
                << jsonEscape(
                    result.device_info.hardware_id
                )
                << "\"},\"ptz_xaddr\":\""
                << jsonEscape(
                    result.ptz_xaddr
                )
                << "\",\"ptz_profile_token\":\""
                << jsonEscape(
                    result.ptz_profile_token
                )
                << "\",\"ptz_supported\":"
                << (
                    result.ptz_supported
                    ? "true"
                    : "false"
                )
                << ",\"recommended_index\":"
                << result.recommended_index
                << ",\"profiles\":[";

            bool first = true;

            for (
                const auto& profile :
                result.profiles
            ) {
                if (!first)
                    json << ",";

                first = false;

                json
                    << "{"
                    << "\"token\":\""
                    << jsonEscape(
                        profile.token
                    )
                    << "\",\"name\":\""
                    << jsonEscape(
                        profile.name
                    )
                    << "\",\"encoding\":\""
                    << jsonEscape(
                        profile.encoding
                    )
                    << "\",\"width\":"
                    << profile.width
                    << ",\"height\":"
                    << profile.height
                    << ",\"fps\":"
                    << profile.fps
                    << ",\"rtsp_uri\":\""
                    << jsonEscape(
                        profile.rtsp_uri
                    )
                    << "\"}";
            }

            json << "]}";

            sendResponse(
                client_fd,
                result.success
                    ? "200 OK"
                    : "400 Bad Request",
                "application/json; charset=utf-8",
                json.str()
            );

            return;
        }

        auto parse_id =
            [&]() -> std::int64_t {
                const auto it =
                    form.find("id");

                if (
                    it == form.end()
                    ||
                    it->second.empty()
                ) {
                    return 0;
                }

                const auto parsed =
                    parseInt64(
                        it->second
                    );

                return
                    (
                        parsed
                        &&
                        *parsed > 0
                    )
                    ? *parsed
                    : 0;
            };

        if (
            path ==
                "/api/cameras/ptz"
        ) {
            const auto id =
                parse_id();

            if (id <= 0) {
                sendResponse(
                    client_fd,
                    "400 Bad Request",
                    "application/json; charset=utf-8",
                    "{\"success\":false,\"message\":\"Некорректный ID камеры.\"}"
                );

                return;
            }

            const auto action =
                form.contains("action")
                ? form.at("action")
                : "";

            double speed = 0.55;

            if (
                form.contains("speed")
            ) {
                try {
                    speed =
                        std::stod(
                            form.at("speed")
                        );
                }
                catch (...) {
                    speed = 0.55;
                }
            }

            const auto result =
                cameras_->ptz(
                    id,
                    action,
                    speed
                );

            security_.audit(
                "camera.ptz",
                session->username,
                "camera_id=" +
                std::to_string(id)
                +
                " action=" +
                action
                +
                " result=" +
                result.code
            );

            sendResponse(
                client_fd,
                result.success
                    ? "200 OK"
                    : "400 Bad Request",
                "application/json; charset=utf-8",
                "{\"success\":" +
                std::string(
                    result.success
                    ? "true"
                    : "false"
                )
                +
                ",\"code\":\"" +
                jsonEscape(result.code)
                +
                "\",\"message\":\"" +
                jsonEscape(result.message)
                +
                "\"}"
            );

            return;
        }

        if (
            path ==
                "/api/cameras/media-probe"
        ) {
            const auto id =
                parse_id();

            if (id <= 0) {
                sendResponse(
                    client_fd,
                    "400 Bad Request",
                    "application/json; charset=utf-8",
                    "{\"success\":false,\"message\":\"Некорректный ID камеры.\"}"
                );

                return;
            }

            auto result =
                cameras_->mediaProbe(id);

            security_.audit(
                "camera.media.probe",
                session->username,
                "camera_id=" +
                std::to_string(id)
                +
                " result=" +
                result.code
            );

            std::ostringstream json;
            json
                << "{\"success\":"
                << (
                    result.success
                    ? "true"
                    : "false"
                )
                << ",\"code\":\""
                << jsonEscape(
                    result.code
                )
                << "\",\"message\":\""
                << jsonEscape(
                    result.message
                )
                << "\",\"video_codec\":\""
                << jsonEscape(
                    result.video_codec
                )
                << "\",\"audio_codec\":\""
                << jsonEscape(
                    result.audio_codec
                )
                << "\",\"width\":"
                << result.width
                << ",\"height\":"
                << result.height
                << ",\"fps\":"
                << result.fps
                << "}";

            sendResponse(
                client_fd,
                result.success
                    ? "200 OK"
                    : "400 Bad Request",
                "application/json; charset=utf-8",
                json.str()
            );

            return;
        }

        CameraResult result;

        if (
            path == "/api/cameras/save"
        ) {
            CameraInput input;

            input.name =
                form.contains("name")
                ? form.at("name")
                : "";

            input.rtsp_url =
                form.contains("rtsp_url")
                ? form.at("rtsp_url")
                : "";

            input.onvif_xaddr =
                form.contains(
                    "onvif_xaddr"
                )
                ? form.at(
                    "onvif_xaddr"
                )
                : "";

            input.manufacturer =
                form.contains(
                    "manufacturer"
                )
                ? form.at(
                    "manufacturer"
                )
                : "";

            input.model =
                form.contains("model")
                ? form.at("model")
                : "";

            input.firmware_version =
                form.contains(
                    "firmware_version"
                )
                ? form.at(
                    "firmware_version"
                )
                : "";

            input.serial_number =
                form.contains(
                    "serial_number"
                )
                ? form.at(
                    "serial_number"
                )
                : "";

            input.hardware_id =
                form.contains(
                    "hardware_id"
                )
                ? form.at(
                    "hardware_id"
                )
                : "";

            input.ptz_xaddr =
                form.contains(
                    "ptz_xaddr"
                )
                ? form.at(
                    "ptz_xaddr"
                )
                : "";

            input.ptz_profile_token =
                form.contains(
                    "ptz_profile_token"
                )
                ? form.at(
                    "ptz_profile_token"
                )
                : "";

            input.username =
                form.contains("username")
                ? form.at("username")
                : "";

            input.password =
                form.contains("password")
                ? form.at("password")
                : "";

            input.enabled =
                form.contains("enabled")
                &&
                (
                    form.at("enabled") ==
                        "1"
                    ||
                    form.at("enabled") ==
                        "true"
                    ||
                    form.at("enabled") ==
                        "on"
                );

            const auto id = parse_id();

            input.update_password =
                id == 0
                ||
                (
                    form.contains(
                        "update_password"
                    )
                    &&
                    form.at(
                        "update_password"
                    ) == "1"
                );

            result =
                id == 0
                ? cameras_->create(input)
                : cameras_->update(
                    id,
                    input
                );

            if (result.success) {
                security_.audit(
                    id == 0
                        ? "camera.create"
                        : "camera.update",
                    session->username,
                    "camera_id=" +
                    std::to_string(
                        result.id
                    )
                    +
                    " name=" +
                    input.name
                );
            }
        }
        else {
            const auto id =
                parse_id();

            if (id <= 0) {
                result = {
                    false,
                    "invalid_id",
                    "Некорректный ID камеры.",
                    0
                };
            }
            else if (
                path ==
                    "/api/cameras/delete"
            ) {
                result =
                    cameras_->remove(id);

                if (result.success) {
                    security_.audit(
                        "camera.delete",
                        session->username,
                        "camera_id=" +
                        std::to_string(id)
                    );
                }
            }
            else {
                result =
                    cameras_->probe(id);

                security_.audit(
                    "camera.probe",
                    session->username,
                    "camera_id=" +
                    std::to_string(id)
                    +
                    " result=" +
                    result.code
                );
            }
        }

        sendResponse(
            client_fd,
            result.success
                ? "200 OK"
                : (
                    result.code ==
                        "not_found"
                    ? "404 Not Found"
                    : "400 Bad Request"
                ),
            "application/json; charset=utf-8",
            "{\"success\":" +
            std::string(
                result.success
                    ? "true"
                    : "false"
            )
            +
            ",\"code\":\"" +
            jsonEscape(result.code)
            +
            "\",\"message\":\"" +
            jsonEscape(result.message)
            +
            "\",\"id\":" +
            std::to_string(
                result.id
            )
            +
            "}"
        );

        return;
    }

    if (
        method == "GET" &&
        path == "/api/system"
    ) {
        if (
            !security_.hasPermission(
                *session,
                "system.view"
            )
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"error\":\"permission_denied\"}"
            );

            return;
        }

        static SystemMonitor monitor;

        const auto stats =
            monitor.snapshot();

        std::ostringstream json;

        json
            << "{"
            << "\"cpu_percent\":"
            << stats.cpu_percent
            << ",\"memory_percent\":"
            << stats.memory_percent
            << ",\"disk_percent\":"
            << stats.disk_percent
            << ",\"memory_total_bytes\":"
            << stats.memory_total_bytes
            << ",\"memory_available_bytes\":"
            << stats.memory_available_bytes
            << ",\"disk_total_bytes\":"
            << stats.disk_total_bytes
            << ",\"disk_free_bytes\":"
            << stats.disk_free_bytes
            << ",\"uptime_seconds\":"
            << stats.uptime_seconds
            << ",\"load_1\":"
            << stats.load_1
            << ",\"load_5\":"
            << stats.load_5
            << ",\"load_15\":"
            << stats.load_15
            << "}";

        sendResponse(
            client_fd,
            "200 OK",
            "application/json; charset=utf-8",
            json.str()
        );

        return;
    }

    if (
        method == "GET" &&
        path == "/api/storage"
    ) {
        if (
            !security_.hasPermission(
                *session,
                "storage.view"
            )
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"error\":\"permission_denied\"}"
            );

            return;
        }

        static StorageMonitor monitor;

        auto& storage_config =
            runtime_.config();

        const auto volumes =
            monitor.snapshot(
                storage_config.get(
                    "storage.video_mounts",
                    ""
                ),
                storage_config.get(
                    "storage.personal_mounts",
                    ""
                )
            );

        const auto makePoolOptions =
            [&](const std::string& prefix) {
                StoragePoolOptions options;

                options.policy =
                    StoragePoolSelector::
                        policyFromString(
                            storage_config.get(
                                prefix + "_policy",
                                "most_free"
                            )
                        );

                options.reserve_percent =
                    storage_config.getInt(
                        prefix +
                            "_reserve_percent",
                        10
                    );

                const auto reserve_gb =
                    std::max(
                        0,
                        storage_config.getInt(
                            prefix +
                                "_reserve_gb",
                            0
                        )
                    );

                options.reserve_bytes =
                    static_cast<std::uint64_t>(
                        reserve_gb
                    )
                    *
                    1024ULL
                    *
                    1024ULL
                    *
                    1024ULL;

                options.preferred_mount =
                    storage_config.get(
                        prefix +
                            "_pinned_mount",
                        ""
                    );

                return options;
            };

        const auto video_options =
            makePoolOptions(
                "storage.video"
            );

        const auto files_options =
            makePoolOptions(
                "storage.files"
            );

        const auto video_target =
            StoragePoolSelector::select(
                volumes,
                "video",
                video_options
            );

        const auto files_target =
            StoragePoolSelector::select(
                volumes,
                "personal",
                files_options
            );

        const auto video_summary =
            StoragePoolSelector::summarize(
                volumes,
                "video"
            );

        const auto files_summary =
            StoragePoolSelector::summarize(
                volumes,
                "personal"
            );

        std::ostringstream json;

        json
            << "{"
            << "\"video_policy\":\""
            << jsonEscape(
                StoragePoolSelector::
                    policyToString(
                        video_options.policy
                    )
            )
            << "\","
            << "\"files_policy\":\""
            << jsonEscape(
                StoragePoolSelector::
                    policyToString(
                        files_options.policy
                    )
            )
            << "\","
            << "\"video_target\":\""
            << jsonEscape(
                video_target
                ? video_target->mount_point
                : ""
            )
            << "\","
            << "\"files_target\":\""
            << jsonEscape(
                files_target
                ? files_target->mount_point
                : ""
            )
            << "\","
            << "\"video_summary\":{"
            << "\"assigned_volumes\":"
            << video_summary.assigned_volumes
            << ",\"online_volumes\":"
            << video_summary.online_volumes
            << ",\"total_bytes\":"
            << video_summary.total_bytes
            << ",\"free_bytes\":"
            << video_summary.free_bytes
            << ",\"free_bytes_complete\":"
            << (
                video_summary.free_bytes_complete
                ? "true"
                : "false"
            )
            << "},"
            << "\"files_summary\":{"
            << "\"assigned_volumes\":"
            << files_summary.assigned_volumes
            << ",\"online_volumes\":"
            << files_summary.online_volumes
            << ",\"total_bytes\":"
            << files_summary.total_bytes
            << ",\"free_bytes\":"
            << files_summary.free_bytes
            << ",\"free_bytes_complete\":"
            << (
                files_summary.free_bytes_complete
                ? "true"
                : "false"
            )
            << "},"
            << "\"volumes\":[";

        bool first = true;

        for (const auto& volume : volumes) {
            if (!first)
                json << ",";

            first = false;

            json
                << "{"
                << "\"source\":\""
                << jsonEscape(volume.source)
                << "\","
                << "\"mount_point\":\""
                << jsonEscape(volume.mount_point)
                << "\","
                << "\"filesystem\":\""
                << jsonEscape(volume.filesystem)
                << "\","
                << "\"uuid\":\""
                << jsonEscape(volume.uuid)
                << "\","
                << "\"role\":\""
                << jsonEscape(volume.role)
                << "\","
                << "\"status\":\""
                << jsonEscape(volume.status)
                << "\","
                << "\"device_size_bytes\":"
                << volume.device_size_bytes
                << ",\"capacity_available\":"
                << (
                    volume.capacity_available
                    ? "true"
                    : "false"
                )
                << ",\"total_bytes\":"
                << volume.total_bytes
                << ",\"used_bytes\":"
                << volume.used_bytes
                << ",\"free_bytes\":"
                << volume.free_bytes
                << ",\"used_percent\":"
                << volume.used_percent
                << ",\"read_only\":"
                << (
                    volume.read_only
                    ? "true"
                    : "false"
                )
                << "}";
        }

        json << "]}";

        sendResponse(
            client_fd,
            "200 OK",
            "application/json; charset=utf-8",
            json.str()
        );

        return;
    }

    if (
        method == "GET" &&
        path == "/api/storage/devices"
    ) {
        if (
            !security_.hasPermission(
                *session,
                "storage.view"
            )
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"error\":\"permission_denied\"}"
            );

            return;
        }

        static StorageMonitor monitor;

        const auto devices =
            monitor.blockDevices();

        DiskOperations operations;

        std::ostringstream json;

        json
            << "{\"helper_installed\":"
            << (
                operations.helperInstalled()
                ? "true"
                : "false"
            )
            << ",\"devices\":[";

        bool first = true;

        for (const auto& device : devices) {
            if (!first)
                json << ",";

            first = false;

            json
                << "{"
                << "\"name\":\""
                << jsonEscape(device.name)
                << "\","
                << "\"device\":\""
                << jsonEscape(device.device)
                << "\","
                << "\"parent\":\""
                << jsonEscape(device.parent)
                << "\","
                << "\"type\":\""
                << jsonEscape(device.type)
                << "\","
                << "\"model\":\""
                << jsonEscape(device.model)
                << "\","
                << "\"vendor\":\""
                << jsonEscape(device.vendor)
                << "\","
                << "\"serial\":\""
                << jsonEscape(device.serial)
                << "\","
                << "\"uuid\":\""
                << jsonEscape(device.uuid)
                << "\","
                << "\"mount_point\":\""
                << jsonEscape(device.mount_point)
                << "\","
                << "\"size_bytes\":"
                << device.size_bytes
                << ",\"removable\":"
                << (
                    device.removable
                    ? "true"
                    : "false"
                )
                << ",\"mounted\":"
                << (
                    device.mounted
                    ? "true"
                    : "false"
                )
                << ",\"in_use\":"
                << (
                    device.in_use
                    ? "true"
                    : "false"
                )
                << ",\"has_partitions\":"
                << (
                    device.has_partitions
                    ? "true"
                    : "false"
                )
                << ",\"candidate\":"
                << (
                    device.candidate
                    ? "true"
                    : "false"
                )
                << "}";
        }

        json << "]}";

        sendResponse(
            client_fd,
            "200 OK",
            "application/json; charset=utf-8",
            json.str()
        );

        return;
    }

    if (
        method == "POST" &&
        path == "/api/storage/action"
    ) {
        if (
            !security_.hasPermission(
                *session,
                "storage.manage"
            )
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"success\":false,\"code\":\"admin_required\",\"message\":\"Требуются права администратора.\"}"
            );

            return;
        }

        const auto form =
            parseForm(body);

        const auto action =
            form.contains("action")
            ? form.at("action")
            : "";

        const auto device =
            form.contains("device")
            ? form.at("device")
            : "";

        const auto confirm =
            form.contains("confirm")
            ? form.at("confirm")
            : "";

        const auto label =
            form.contains("label")
            ? form.at("label")
            : "homeai-data";

        const bool use_video =
            form.contains("video")
            &&
            form.at("video") == "1";

        const bool use_personal =
            form.contains("personal")
            &&
            form.at("personal") == "1";

        StorageMonitor monitor;

        const auto devices =
            monitor.blockDevices();

        const auto* info =
            findDeviceInfo(
                devices,
                device
            );

        if (info == nullptr) {
            sendResponse(
                client_fd,
                "400 Bad Request",
                "application/json; charset=utf-8",
                "{\"success\":false,\"code\":\"device_not_found\",\"message\":\"Устройство не найдено.\"}"
            );

            return;
        }

        auto& config =
            runtime_.config();

        auto sendActionResult =
            [&](const DiskOperationResult& result) {
                const std::string response =
                    "{"
                    "\"success\":" +
                    std::string(
                        result.success
                        ? "true"
                        : "false"
                    ) +
                    ",\"code\":\"" +
                    jsonEscape(
                        result.code
                    ) +
                    "\","
                    "\"message\":\"" +
                    jsonEscape(
                        result.message
                    ) +
                    "\""
                    "}";

                sendResponse(
                    client_fd,
                    result.success
                        ? "200 OK"
                        : "400 Bad Request",
                    "application/json; charset=utf-8",
                    response
                );
            };

        if (
            action == "apply-roles"
        ) {
            std::string mount_point =
                info->mount_point;

            DiskOperations role_operations;

            bool mounted_by_request =
                false;

            if (!info->mounted) {
                if (
                    !use_video
                    &&
                    !use_personal
                ) {
                    sendActionResult(
                        {
                            false,
                            "role_required",
                            "Выберите хотя бы одно назначение для нового диска."
                        }
                    );

                    return;
                }

                const std::string mount_role =
                    use_video
                    ? "video"
                    : "personal";

                const auto mount_result =
                    role_operations.mount(
                        device,
                        mount_role,
                        false
                    );

                if (!mount_result.success) {
                    sendActionResult(
                        mount_result
                    );

                    return;
                }

                mount_point =
                    DiskOperations::
                        defaultMountPoint(
                            device,
                            mount_role
                        );

                mounted_by_request =
                    true;
            }

            if (mount_point.empty()) {
                if (mounted_by_request) {
                    role_operations.unmount(
                        device
                    );
                }
                sendActionResult(
                    {
                        false,
                        "mount_point_missing",
                        "Не удалось определить точку монтирования диска."
                    }
                );

                return;
            }

            const auto video_key =
                "storage.video_mounts";

            const auto personal_key =
                "storage.personal_mounts";

            const auto previous_video =
                config.get(
                    video_key,
                    ""
                );

            const auto previous_personal =
                config.get(
                    personal_key,
                    ""
                );

            config.set(
                video_key,
                use_video
                ? addCsvValue(
                    config.get(
                        video_key,
                        ""
                    ),
                    mount_point
                )
                : removeCsvValue(
                    config.get(
                        video_key,
                        ""
                    ),
                    mount_point
                )
            );

            config.set(
                personal_key,
                use_personal
                ? addCsvValue(
                    config.get(
                        personal_key,
                        ""
                    ),
                    mount_point
                )
                : removeCsvValue(
                    config.get(
                        personal_key,
                        ""
                    ),
                    mount_point
                )
            );

            if (!config.save()) {
                config.set(
                    video_key,
                    previous_video
                );

                config.set(
                    personal_key,
                    previous_personal
                );

                if (mounted_by_request) {
                    role_operations.unmount(
                        device
                    );
                }

                sendActionResult(
                    {
                        false,
                        "config_save_failed",
                        "Не удалось сохранить назначение диска."
                    }
                );

                return;
            }

            security_.audit(
                "storage.roles",
                session->username,
                "device=" +
                device +
                " uuid=" +
                info->uuid +
                " mount=" +
                mount_point +
                " video=" +
                (
                    use_video
                    ? "1"
                    : "0"
                ) +
                " files=" +
                (
                    use_personal
                    ? "1"
                    : "0"
                )
            );

            sendActionResult(
                {
                    true,
                    "ok",
                    use_video || use_personal
                    ? "Назначение диска обновлено."
                    : "Назначение Home AI Core снято."
                }
            );

            return;
        }

        if (
            action == "assign-video"
            ||
            action == "assign-personal"
            ||
            action == "unassign"
        ) {
            if (
                !info->mounted
                ||
                info->mount_point.empty()
            ) {
                sendActionResult(
                    {
                        false,
                        "not_mounted",
                        "Сначала смонтируйте диск."
                    }
                );

                return;
            }

            const auto video_key =
                "storage.video_mounts";

            const auto personal_key =
                "storage.personal_mounts";

            if (
                action == "assign-video"
            ) {
                config.set(
                    video_key,
                    addCsvValue(
                        config.get(
                            video_key,
                            ""
                        ),
                        info->mount_point
                    )
                );

                config.set(
                    personal_key,
                    removeCsvValue(
                        config.get(
                            personal_key,
                            ""
                        ),
                        info->mount_point
                    )
                );
            }
            else if (
                action ==
                "assign-personal"
            ) {
                config.set(
                    personal_key,
                    addCsvValue(
                        config.get(
                            personal_key,
                            ""
                        ),
                        info->mount_point
                    )
                );

                config.set(
                    video_key,
                    removeCsvValue(
                        config.get(
                            video_key,
                            ""
                        ),
                        info->mount_point
                    )
                );
            }
            else {
                config.set(
                    video_key,
                    removeCsvValue(
                        config.get(
                            video_key,
                            ""
                        ),
                        info->mount_point
                    )
                );

                config.set(
                    personal_key,
                    removeCsvValue(
                        config.get(
                            personal_key,
                            ""
                        ),
                        info->mount_point
                    )
                );
            }

            if (!config.save()) {
                sendActionResult(
                    {
                        false,
                        "config_save_failed",
                        "Не удалось сохранить назначение диска."
                    }
                );

                return;
            }

            security_.audit(
                "storage.role",
                session->username,
                action +
                " device=" +
                device +
                " mount=" +
                info->mount_point
            );

            sendActionResult(
                {
                    true,
                    "ok",
                    "Назначение диска обновлено."
                }
            );

            return;
        }

        DiskOperations operations;

        DiskOperationResult result;

        if (
            action == "mount-video"
        ) {
            result =
                operations.mount(
                    device,
                    "video",
                    false
                );

            if (result.success) {
                const auto mount_point =
                    DiskOperations::
                    defaultMountPoint(
                        device,
                        "video"
                    );

                config.set(
                    "storage.video_mounts",
                    addCsvValue(
                        config.get(
                            "storage.video_mounts",
                            ""
                        ),
                        mount_point
                    )
                );

                config.save();
            }
        }
        else if (
            action == "mount-personal"
        ) {
            result =
                operations.mount(
                    device,
                    "personal",
                    false
                );

            if (result.success) {
                const auto mount_point =
                    DiskOperations::
                    defaultMountPoint(
                        device,
                        "personal"
                    );

                config.set(
                    "storage.personal_mounts",
                    addCsvValue(
                        config.get(
                            "storage.personal_mounts",
                            ""
                        ),
                        mount_point
                    )
                );

                config.save();
            }
        }
        else if (
            action == "unmount"
        ) {
            result =
                operations.unmount(
                    device
                );
        }
        else if (
            action == "format-ext4"
        ) {
            if (confirm != device) {
                sendActionResult(
                    {
                        false,
                        "confirmation_required",
                        "Для форматирования нужно подтвердить точное имя устройства."
                    }
                );

                return;
            }

            result =
                operations.formatExt4(
                    device,
                    label
                );
        }
        else if (
            action == "wipefs"
        ) {
            if (confirm != device) {
                sendActionResult(
                    {
                        false,
                        "confirmation_required",
                        "Для удаления сигнатур нужно подтвердить точное имя устройства."
                    }
                );

                return;
            }

            result =
                operations.wipeSignatures(
                    device
                );
        }
        else {
            sendActionResult(
                {
                    false,
                    "unsupported_action",
                    "Неизвестная операция с диском."
                }
            );

            return;
        }

        security_.audit(
            "storage.action",
            session->username,
            action +
            " device=" +
            device +
            " result=" +
            result.code
        );

        sendActionResult(result);

        return;
    }

    if (
        method == "GET"
        &&
        path == "/api/network/interfaces"
    ) {
        if (
            !security_.hasPermission(
                *session,
                "network.view"
            )
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"error\":\"permission_denied\"}"
            );

            return;
        }

        NetworkInterfaceManager network;

        const auto interfaces =
            network.interfaces();

        std::ostringstream json;

        json
            << "{\"helper_installed\":"
            << (
                network.helperInstalled()
                ? "true"
                : "false"
            )
            << ",\"interfaces\":[";

        bool first = true;

        for (const auto& interface_info : interfaces) {
            if (!first)
                json << ",";

            first = false;

            json
                << "{"
                << "\"name\":\""
                << jsonEscape(
                    interface_info.name
                )
                << "\","
                << "\"mac_address\":\""
                << jsonEscape(
                    interface_info.mac_address
                )
                << "\","
                << "\"oper_state\":\""
                << jsonEscape(
                    interface_info.oper_state
                )
                << "\","
                << "\"ipv4_method\":\""
                << jsonEscape(
                    interface_info.ipv4_method
                )
                << "\","
                << "\"gateway\":\""
                << jsonEscape(
                    interface_info.gateway
                )
                << "\","
                << "\"mtu\":"
                << interface_info.mtu
                << ",\"up\":"
                << (
                    interface_info.up
                    ? "true"
                    : "false"
                )
                << ",\"carrier\":"
                << (
                    interface_info.carrier
                    ? "true"
                    : "false"
                )
                << ",\"loopback\":"
                << (
                    interface_info.loopback
                    ? "true"
                    : "false"
                )
                << ",\"default_route\":"
                << (
                    interface_info.default_route
                    ? "true"
                    : "false"
                )
                << ",\"ipv4_addresses\":[";

            bool first_address = true;

            for (
                const auto& address :
                interface_info.ipv4_addresses
            ) {
                if (!first_address)
                    json << ",";

                first_address = false;

                json
                    << "\""
                    << jsonEscape(
                        address
                    )
                    << "\"";
            }

            json
                << "],\"dns_servers\":[";

            bool first_dns = true;

            for (
                const auto& dns :
                interface_info.dns_servers
            ) {
                if (!first_dns)
                    json << ",";

                first_dns = false;

                json
                    << "\""
                    << jsonEscape(
                        dns
                    )
                    << "\"";
            }

            json << "]}";
        }

        json << "]}";

        sendResponse(
            client_fd,
            "200 OK",
            "application/json; charset=utf-8",
            json.str()
        );

        return;
    }

    if (
        method == "POST"
        &&
        path == "/api/network/ipv4"
    ) {
        if (
            !security_.hasPermission(
                *session,
                "network.manage"
            )
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"success\":false,\"message\":\"Требуются права управления сетью.\"}"
            );

            return;
        }

        if (
            headerValue(
                headers,
                "X-HomeAI-Request"
            ) != "1"
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"success\":false,\"message\":\"Включена проверка запросов.\"}"
            );

            return;
        }

        const auto form =
            parseForm(body);

        const auto interface_name =
            form.contains("interface")
            ? form.at("interface")
            : "";

        const auto mode =
            form.contains("mode")
            ? form.at("mode")
            : "";

        NetworkInterfaceManager network;

        NetworkActionResult result;

        if (mode == "dhcp") {
            result =
                network.requestDhcp(
                    interface_name
                );
        }
        else if (mode == "static") {
            NetworkStaticConfig config;

            config.address =
                form.contains("address")
                ? form.at("address")
                : "";

            config.netmask =
                form.contains("netmask")
                ? form.at("netmask")
                : "";

            config.gateway =
                form.contains("gateway")
                ? form.at("gateway")
                : "";

            config.dns_primary =
                form.contains("dns_primary")
                ? form.at("dns_primary")
                : "";

            config.dns_secondary =
                form.contains("dns_secondary")
                ? form.at("dns_secondary")
                : "";

            result =
                network.setStaticIpv4(
                    interface_name,
                    config
                );
        }
        else {
            result = {
                false,
                "invalid_mode",
                "Выберите DHCP или статический IPv4."
            };
        }

        security_.audit(
            "network.ipv4.configure",
            session->username,
            "interface=" +
                interface_name +
                " mode=" +
                mode +
                " result=" +
                result.code
        );

        sendResponse(
            client_fd,
            result.success
                ? "200 OK"
                : "400 Bad Request",
            "application/json; charset=utf-8",
            "{\"success\":" +
            std::string(
                result.success
                ? "true"
                : "false"
            ) +
            ",\"code\":\"" +
            jsonEscape(
                result.code
            ) +
            "\",\"message\":\"" +
            jsonEscape(
                result.message
            ) +
            "\"}"
        );

        return;
    }

    if (
        method == "POST"
        &&
        path == "/api/network/dhcp"
    ) {
        if (
            !security_.hasPermission(
                *session,
                "network.manage"
            )
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"success\":false,\"message\":\"Требуются права управления сетью.\"}"
            );

            return;
        }

        if (
            headerValue(
                headers,
                "X-HomeAI-Request"
            ) != "1"
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"success\":false,\"message\":\"Включена проверка запросов.\"}"
            );

            return;
        }

        const auto form =
            parseForm(body);

        const auto interface_name =
            form.contains("interface")
            ? form.at("interface")
            : "";

        NetworkInterfaceManager network;

        const auto result =
            network.requestDhcp(
                interface_name
            );

        security_.audit(
            "network.dhcp.request",
            session->username,
            "interface=" +
                interface_name +
                " result=" +
                result.code
        );

        sendResponse(
            client_fd,
            result.success
                ? "200 OK"
                : "400 Bad Request",
            "application/json; charset=utf-8",
            "{\"success\":" +
            std::string(
                result.success
                ? "true"
                : "false"
            ) +
            ",\"code\":\"" +
            jsonEscape(
                result.code
            ) +
            "\",\"message\":\"" +
            jsonEscape(
                result.message
            ) +
            "\"}"
        );

        return;
    }

    if (
        method == "GET" &&
        path == "/api/network/vpn"
    ) {
        if (
            !security_.hasPermission(
                *session,
                "network.view"
            )
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"error\":\"permission_denied\"}"
            );

            return;
        }

        VpnService vpn;
        const bool initialized =
            vpn.initialize(
                "runtime/wireguard"
            );

        std::string error;
        const auto profiles =
            initialized
            ? vpn.profiles(error)
            : std::vector<VpnProfileInfo>{};

        std::ostringstream json;
        json
            << "{\"available\":"
            << (vpn.available() ? "true" : "false")
            << ",\"profiles\":[";

        bool first = true;

        for (const auto& profile : profiles) {
            if (!first)
                json << ",";

            first = false;

            json
                << "{\"name\":\""
                << jsonEscape(profile.name)
                << "\",\"active\":"
                << (profile.active ? "true" : "false")
                << "}";
        }

        json
            << "],\"error\":\""
            << jsonEscape(error)
            << "\"}";

        sendResponse(
            client_fd,
            "200 OK",
            "application/json; charset=utf-8",
            json.str()
        );

        return;
    }

    if (
        method == "GET" &&
        path == "/api/network/vpn/profile"
    ) {
        if (
            !security_.hasPermission(
                *session,
                "network.manage"
            )
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"success\":false,\"error\":\"permission_denied\"}"
            );

            return;
        }

        const auto query_values =
            parseForm(
                query_string
            );

        const auto profile =
            query_values.contains(
                "profile"
            )
            ? query_values.at(
                "profile"
            )
            : "";

        VpnService vpn;

        if (
            !vpn.initialize(
                "runtime/wireguard"
            )
        ) {
            sendResponse(
                client_fd,
                "500 Internal Server Error",
                "application/json; charset=utf-8",
                "{\"success\":false,\"message\":\"VPN недоступен.\"}"
            );

            return;
        }

        const auto result =
            vpn.loadProfile(
                profile
            );

        security_.audit(
            "network.vpn.profile.read",
            session->username,
            "result=" +
                result.code
        );

        sendResponse(
            client_fd,
            result.success
                ? "200 OK"
                : (
                    result.code ==
                        "profile_not_found"
                    ? "404 Not Found"
                    : "400 Bad Request"
                ),
            "application/json; charset=utf-8",
            "{\"success\":" +
            std::string(
                result.success
                    ? "true"
                    : "false"
            ) +
            ",\"code\":\"" +
            jsonEscape(
                result.code
            ) +
            "\",\"message\":\"" +
            jsonEscape(
                result.message
            ) +
            "\",\"profile\":\"" +
            jsonEscape(
                profile
            ) +
            "\",\"config\":\"" +
            jsonEscape(
                result.config
            ) +
            "\"}"
        );

        return;
    }

    if (
        method == "POST" &&
        path == "/api/network/vpn/profile"
    ) {
        if (
            !security_.hasPermission(
                *session,
                "network.manage"
            )
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"success\":false,\"message\":\"Требуются права администратора.\"}"
            );

            return;
        }

        if (
            headerValue(
                headers,
                "X-HomeAI-Request"
            ) != "1"
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"success\":false,\"error\":\"request_header_required\"}"
            );

            return;
        }

        const auto form =
            parseForm(body);

        const auto action =
            form.contains("action")
            ? form.at("action")
            : "";

        const auto profile =
            form.contains("profile")
            ? form.at("profile")
            : "";

        const auto config =
            form.contains("config")
            ? form.at("config")
            : "";

        VpnService vpn;

        if (
            !vpn.initialize(
                "runtime/wireguard"
            )
        ) {
            sendResponse(
                client_fd,
                "500 Internal Server Error",
                "application/json; charset=utf-8",
                "{\"success\":false,\"message\":\"VPN недоступен.\"}"
            );

            return;
        }

        VpnActionResult result;

        if (action == "save") {
            result =
                vpn.saveProfile(
                    profile,
                    config
                );
        }
        else if (
            action == "remove"
        ) {
            result =
                vpn.removeProfile(
                    profile
                );
        }
        else {
            result = {
                false,
                "unsupported_action",
                "Неизвестная операция с профилем WireGuard."
            };
        }

        std::string audit_details =
            "action=" +
            action +
            " result=" +
            result.code;

        if (result.success) {
            audit_details +=
                " profile=" +
                profile;
        }

        security_.audit(
            "network.vpn.profile",
            session->username,
            audit_details
        );

        sendResponse(
            client_fd,
            result.success
                ? "200 OK"
                : "400 Bad Request",
            "application/json; charset=utf-8",
            "{\"success\":" +
            std::string(
                result.success
                    ? "true"
                    : "false"
            ) +
            ",\"code\":\"" +
            jsonEscape(
                result.code
            ) +
            "\",\"message\":\"" +
            jsonEscape(
                result.message
            ) +
            "\"}"
        );

        return;
    }

    if (
        method == "POST" &&
        path == "/api/network/vpn/action"
    ) {
        if (
            !security_.hasPermission(
                *session,
                "network.manage"
            )
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"success\":false,\"message\":\"Требуются права администратора.\"}"
            );

            return;
        }

        const auto form =
            parseForm(body);

        const auto action =
            form.contains("action")
            ? form.at("action")
            : "";

        const auto profile =
            form.contains("profile")
            ? form.at("profile")
            : "";

        VpnService vpn;

        if (
            !vpn.initialize(
                "runtime/wireguard"
            )
        ) {
            sendResponse(
                client_fd,
                "500 Internal Server Error",
                "application/json; charset=utf-8",
                "{\"success\":false,\"message\":\"VPN недоступен.\"}"
            );

            return;
        }

        VpnActionResult result;

        if (action == "connect") {
            result =
                vpn.connect(profile);
        }
        else if (
            action == "disconnect"
        ) {
            result =
                vpn.disconnect(profile);
        }
        else {
            result = {
                false,
                "unsupported_action",
                "Неизвестная VPN-операция."
            };
        }

        security_.audit(
            "network.vpn.action",
            session->username,
            "action=" +
            action +
            " profile=" +
            profile +
            " result=" +
            result.code
        );

        sendResponse(
            client_fd,
            result.success
                ? "200 OK"
                : "400 Bad Request",
            "application/json; charset=utf-8",
            "{\"success\":" +
            std::string(
                result.success
                ? "true"
                : "false"
            ) +
            ",\"message\":\"" +
            jsonEscape(result.message) +
            "\"}"
        );

        return;
    }

    if (
        method == "GET" &&
        path == "/api/modules"
    ) {
        if (
            !security_.hasPermission(
                *session,
                "system.view"
            )
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"error\":\"permission_denied\"}"
            );

            return;
        }

        const auto modules =
            modules_.snapshot();

        std::ostringstream json;

        json << "{\"modules\":[";

        bool first = true;

        for (const auto& module : modules) {
            if (!first)
                json << ",";

            first = false;

            json
                << "{"
                << "\"name\":\""
                << jsonEscape(
                    module.name
                )
                << "\","
                << "\"state\":\""
                << jsonEscape(
                    ModuleManager::stateToString(
                        module.state
                    )
                )
                << "\","
                << "\"health\":\""
                << jsonEscape(
                    moduleHealthToString(
                        module.health
                    )
                )
                << "\","
                << "\"message\":\""
                << jsonEscape(
                    module.message
                )
                << "\","
                << "\"dependencies\":[";

            bool first_dependency = true;

            for (
                const auto& dependency :
                module.dependencies
            ) {
                if (!first_dependency)
                    json << ",";

                first_dependency = false;

                json
                    << "\""
                    << jsonEscape(
                        dependency
                    )
                    << "\"";
            }

            json << "]}";
        }

        json << "]}";

        sendResponse(
            client_fd,
            "200 OK",
            "application/json; charset=utf-8",
            json.str()
        );

        return;
    }

    if (
        method == "GET" &&
        path == "/api/update/status"
    ) {
        if (
            !security_.hasPermission(
                *session,
                "system.view"
            )
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"error\":\"permission_denied\"}"
            );

            return;
        }

        const auto status =
            updates_.status();

        const std::string response =
            "{"
            "\"state\":\"" +
            jsonEscape(
                status.state_text
            ) +
            "\","
            "\"local_sha\":\"" +
            jsonEscape(
                status.local_sha
            ) +
            "\","
            "\"remote_sha\":\"" +
            jsonEscape(
                status.remote_sha
            ) +
            "\","
            "\"branch\":\"" +
            jsonEscape(
                status.branch
            ) +
            "\","
            "\"message\":\"" +
            jsonEscape(
                status.message
            ) +
            "\","
            "\"last_output\":\"" +
            jsonEscape(
                status.last_output
            ) +
            "\","
            "\"progress_stage\":\"" +
            jsonEscape(
                status.progress_stage
            ) +
            "\","
            "\"progress_percent\":" +
            std::to_string(
                status.progress_percent
            ) +
            ","
            "\"progress_current\":" +
            std::to_string(
                status.progress_current
            ) +
            ","
            "\"progress_total\":" +
            std::to_string(
                status.progress_total
            ) +
            ","
            "\"update_available\":" +
            std::string(
                status.update_available
                ? "true"
                : "false"
            ) +
            ","
            "\"busy\":" +
            std::string(
                status.busy
                ? "true"
                : "false"
            ) +
            ","
            "\"restart_required\":" +
            std::string(
                status.restart_required
                ? "true"
                : "false"
            ) +
            "}";

        sendResponse(
            client_fd,
            "200 OK",
            "application/json; charset=utf-8",
            response
        );

        return;
    }

    if (
        method == "POST" &&
        path == "/api/update/check"
    ) {
        if (
            !security_.hasPermission(
                *session,
                "system.manage"
            )
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"success\":false,\"message\":\"Требуются права администратора.\"}"
            );

            return;
        }

        updates_.requestCheck();

        security_.audit(
            "update.check",
            session->username,
            "manual update check"
        );

        sendResponse(
            client_fd,
            "202 Accepted",
            "application/json; charset=utf-8",
            "{\"success\":true,\"message\":\"Проверка обновлений запущена.\"}"
        );

        return;
    }

    if (
        method == "POST" &&
        path == "/api/update/apply"
    ) {
        if (
            !security_.hasPermission(
                *session,
                "system.manage"
            )
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"success\":false,\"message\":\"Требуются права администратора.\"}"
            );

            return;
        }

        const auto form =
            parseForm(body);

        const auto confirm =
            form.contains("confirm")
            ? form.at("confirm")
            : "";

        if (confirm != "UPDATE") {
            sendResponse(
                client_fd,
                "400 Bad Request",
                "application/json; charset=utf-8",
                "{\"success\":false,\"message\":\"Для обновления требуется подтверждение UPDATE.\"}"
            );

            return;
        }

        std::string error;

        if (
            !updates_.requestUpdate(
                error
            )
        ) {
            sendResponse(
                client_fd,
                "409 Conflict",
                "application/json; charset=utf-8",
                "{\"success\":false,\"message\":\"" +
                jsonEscape(error) +
                "\"}"
            );

            return;
        }

        security_.audit(
            "update.apply",
            session->username,
            "update requested"
        );

        sendResponse(
            client_fd,
            "202 Accepted",
            "application/json; charset=utf-8",
            "{\"success\":true,\"message\":\"Обновление запущено.\"}"
        );

        return;
    }

    if (
        method == "POST" &&
        path == "/api/update/restart"
    ) {
        if (
            !security_.hasPermission(
                *session,
                "system.manage"
            )
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"success\":false,\"message\":\"Требуются права администратора.\"}"
            );

            return;
        }

        std::string error;

        if (
            !updates_.requestRestart(
                error
            )
        ) {
            sendResponse(
                client_fd,
                "409 Conflict",
                "application/json; charset=utf-8",
                "{\"success\":false,\"message\":\"" +
                jsonEscape(error) +
                "\"}"
            );

            return;
        }

        security_.audit(
            "update.restart",
            session->username,
            "restart requested"
        );

        sendResponse(
            client_fd,
            "202 Accepted",
            "application/json; charset=utf-8",
            "{\"success\":true,\"message\":\"Перезапуск запрошен.\"}"
        );

        return;
    }

    if (
        method == "GET" &&
        path == "/api/config"
    ) {
        if (
            !security_.hasPermission(
                *session,
                "system.manage"
            )
        ) {
            sendResponse(
                client_fd,
                "403 Forbidden",
                "application/json; charset=utf-8",
                "{\"error\":\"admin_required\"}"
            );

            return;
        }

        const auto& config =
            runtime_.config();

        const std::string response =
            "{"
            "\"core.name\":\"" +
            jsonEscape(
                config.get(
                    "core.name",
                    "Home AI Core"
                )
            ) +
            "\","
            "\"core.version\":\"" +
            jsonEscape(
                config.get(
                    "core.version",
                    "unknown"
                )
            ) +
            "\","
            "\"log.level\":\"" +
            jsonEscape(
                config.get(
                    "log.level",
                    "info"
                )
            ) +
            "\","
            "\"runtime.tick_ms\":\"" +
            jsonEscape(
                config.get(
                    "runtime.tick_ms",
                    "250"
                )
            ) +
            "\","
            "\"web.bind\":\"" +
            jsonEscape(
                config.get(
                    "web.bind",
                    "0.0.0.0"
                )
            ) +
            "\","
            "\"web.port\":\"" +
            jsonEscape(
                config.get(
                    "web.port",
                    "8080"
                )
            ) +
            "\","
            "\"storage.video_mounts\":\"" +
            jsonEscape(
                config.get(
                    "storage.video_mounts",
                    ""
                )
            ) +
            "\","
            "\"storage.personal_mounts\":\"" +
            jsonEscape(
                config.get(
                    "storage.personal_mounts",
                    ""
                )
            ) +
            "\","
            "\"storage.video_policy\":\"" +
            jsonEscape(
                config.get(
                    "storage.video_policy",
                    "most_free"
                )
            ) +
            "\","
            "\"storage.files_policy\":\"" +
            jsonEscape(
                config.get(
                    "storage.files_policy",
                    "most_free"
                )
            ) +
            "\","
            "\"storage.video_reserve_percent\":\"" +
            jsonEscape(
                config.get(
                    "storage.video_reserve_percent",
                    "10"
                )
            ) +
            "\","
            "\"storage.files_reserve_percent\":\"" +
            jsonEscape(
                config.get(
                    "storage.files_reserve_percent",
                    "10"
                )
            ) +
            "\","
            "\"storage.video_reserve_gb\":\"" +
            jsonEscape(
                config.get(
                    "storage.video_reserve_gb",
                    "0"
                )
            ) +
            "\","
            "\"storage.files_reserve_gb\":\"" +
            jsonEscape(
                config.get(
                    "storage.files_reserve_gb",
                    "0"
                )
            ) +
            "\","
            "\"storage.video_pinned_mount\":\"" +
            jsonEscape(
                config.get(
                    "storage.video_pinned_mount",
                    ""
                )
            ) +
            "\","
            "\"storage.files_pinned_mount\":\"" +
            jsonEscape(
                config.get(
                    "storage.files_pinned_mount",
                    ""
                )
            ) +
            "\","
            "\"files.root\":\"" +
            jsonEscape(
                config.get(
                    "files.root",
                    "/mnt/home-ai/files"
                )
            ) +
            "\""
            "}";

        sendResponse(
            client_fd,
            "200 OK",
            "application/json; charset=utf-8",
            response
        );

        return;
    }

    if (
        method == "POST" &&
        path == "/api/config"
    ) {
        const auto form =
            parseForm(body);

        const auto allowed =
            [&](const std::string& key) {
                if (
                    key == "core.name"
                    ||
                    key == "log.level"
                    ||
                    key == "runtime.tick_ms"
                ) {
                    return
                        security_.hasPermission(
                            *session,
                            "system.manage"
                        );
                }

                if (
                    key == "web.bind"
                    ||
                    key == "web.port"
                ) {
                    return
                        security_.hasPermission(
                            *session,
                            "network.manage"
                        );
                }

                if (
                    key == "storage.video_mounts"
                    ||
                    key == "storage.personal_mounts"
                    ||
                    key == "storage.video_policy"
                    ||
                    key == "storage.files_policy"
                    ||
                    key == "storage.video_reserve_percent"
                    ||
                    key == "storage.files_reserve_percent"
                    ||
                    key == "storage.video_reserve_gb"
                    ||
                    key == "storage.files_reserve_gb"
                    ||
                    key == "storage.video_pinned_mount"
                    ||
                    key == "storage.files_pinned_mount"
                ) {
                    return
                        security_.hasPermission(
                            *session,
                            "storage.manage"
                        );
                }

                if (key == "files.root") {
                    return
                        security_.hasPermission(
                            *session,
                            "files.manage"
                        );
                }

                return false;
            };

        const char* allowed_keys[] = {
            "core.name",
            "log.level",
            "runtime.tick_ms",
            "web.bind",
            "web.port",
            "storage.video_mounts",
            "storage.personal_mounts",
            "storage.video_policy",
            "storage.files_policy",
            "storage.video_reserve_percent",
            "storage.files_reserve_percent",
            "storage.video_reserve_gb",
            "storage.files_reserve_gb",
            "storage.video_pinned_mount",
            "storage.files_pinned_mount",
            "files.root"
        };

        for (const auto* key : allowed_keys) {
            if (
                form.contains(key)
                &&
                !allowed(key)
            ) {
                sendResponse(
                    client_fd,
                    "403 Forbidden",
                    "application/json; charset=utf-8",
                    "{\"error\":\"permission_denied\"}"
                );

                return;
            }
        }

        auto& config =
            runtime_.config();

        for (const auto* key : allowed_keys) {
            const auto it =
                form.find(key);

            if (it != form.end()) {
                config.set(
                    key,
                    it->second
                );
            }
        }

        if (!config.save()) {
            sendResponse(
                client_fd,
                "500 Internal Server Error",
                "text/plain; charset=utf-8",
                "Failed to save configuration"
            );

            return;
        }

        security_.audit(
            "config.update",
            session->username,
            "Web configuration updated"
        );

        std::string return_to =
            "/";

        const auto return_it =
            form.find(
                "return_to"
            );

        if (
            return_it != form.end()
            &&
            isWebUiPath(
                return_it->second
            )
        ) {
            return_to =
                return_it->second;
        }

        sendRedirect(
            client_fd,
            return_to
        );

        return;
    }

    if (
        method == "GET"
        &&
        isWebUiPath(path)
    ) {
        auto& config =
            runtime_.config();

        WebUiContext context;

        context.page =
            path;

        context.core_name =
            config.get(
                "core.name",
                "Home AI Core"
            );

        context.version =
            config.get(
                "core.version",
                "unknown"
            );

        context.log_level =
            config.get(
                "log.level",
                "info"
            );

        context.tick_ms =
            config.get(
                "runtime.tick_ms",
                "250"
            );

        context.web_bind =
            config.get(
                "web.bind",
                "0.0.0.0"
            );

        context.web_port =
            config.get(
                "web.port",
                "8080"
            );

        context.storage_video_mounts =
            config.get(
                "storage.video_mounts",
                ""
            );

        context.storage_personal_mounts =
            config.get(
                "storage.personal_mounts",
                ""
            );

        context.storage_video_policy =
            config.get(
                "storage.video_policy",
                "most_free"
            );

        context.storage_files_policy =
            config.get(
                "storage.files_policy",
                "most_free"
            );

        context.storage_video_reserve_percent =
            config.get(
                "storage.video_reserve_percent",
                "10"
            );

        context.storage_files_reserve_percent =
            config.get(
                "storage.files_reserve_percent",
                "10"
            );

        context.storage_video_reserve_gb =
            config.get(
                "storage.video_reserve_gb",
                "0"
            );

        context.storage_files_reserve_gb =
            config.get(
                "storage.files_reserve_gb",
                "0"
            );

        context.storage_video_pinned_mount =
            config.get(
                "storage.video_pinned_mount",
                ""
            );

        context.storage_files_pinned_mount =
            config.get(
                "storage.files_pinned_mount",
                ""
            );

        context.files_root =
            config.get(
                "files.root",
                "/mnt/home-ai/files"
            );

        context.username =
            session->username;

        context.role =
            SecurityManager::roleToString(
                session->role
            );

        context.admin =
            security_.isAdmin(
                session->role
            );

        for (
            const auto& permission :
            SecurityManager::
                permissionCatalog()
        ) {
            if (
                security_.hasPermission(
                    *session,
                    permission
                )
            ) {
                context.permissions.
                    push_back(
                        permission
                    );
            }
        }

        sendResponse(
            client_fd,
            "200 OK",
            "text/html; charset=utf-8",
            renderWebUi(context)
        );

        return;
    }

    sendResponse(
        client_fd,
        "404 Not Found",
        "text/plain; charset=utf-8",
        "404 Not Found"
    );
}

}
