#include "web/server/WebServer.h"

#include "core/logging/Logger.h"
#include "core/runtime/CoreRuntime.h"
#include "security/auth/SecurityManager.h"
#include "server/system/SystemMonitor.h"

#include <algorithm>
#include <arpa/inet.h>
#include <cctype>
#include <cerrno>
#include <cstring>
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

std::string jsonEscape(const std::string& value)
{
    std::string result;

    for (char c : value) {
        switch (c) {
            case '\\': result += "\\\\"; break;
            case '"':  result += "\\\""; break;
            case '\n': result += "\\n"; break;
            case '\r': result += "\\r"; break;
            case '\t': result += "\\t"; break;
            default:   result += c; break;
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
</body>
</html>
)HTML";

    return page.str();
}

}

WebServer::WebServer(
    CoreRuntime& runtime,
    SecurityManager& security
)
    : runtime_(runtime),
      security_(security)
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

        handleClient(client_fd);

        ::shutdown(
            client_fd,
            SHUT_RDWR
        );

        ::close(client_fd);
    }
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

    if (query != std::string::npos)
        path.resize(query);

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

    if (
        method == "GET" &&
        path == "/api/session"
    ) {
        const std::string response =
            "{"
            "\"username\":\"" +
            jsonEscape(
                session->username
            ) +
            "\","
            "\"role\":\"" +
            jsonEscape(
                SecurityManager::roleToString(
                    session->role
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
        method == "GET" &&
        path == "/api/system"
    ) {
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
        path == "/api/config"
    ) {
        if (
            !security_.isAdmin(
                session->role
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
        if (
            !security_.isAdmin(
                session->role
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

        const auto form =
            parseForm(body);

        auto& config =
            runtime_.config();

        const char* allowed_keys[] = {
            "core.name",
            "log.level",
            "runtime.tick_ms",
            "web.bind",
            "web.port"
        };

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

        sendRedirect(
            client_fd,
            "/"
        );

        return;
    }

    if (
        method == "GET" &&
        path == "/"
    ) {
        auto& config =
            runtime_.config();

        const auto core_name =
            htmlEscape(
                config.get(
                    "core.name",
                    "Home AI Core"
                )
            );

        const auto version =
            htmlEscape(
                config.get(
                    "core.version",
                    "unknown"
                )
            );

        const auto log_level =
            htmlEscape(
                config.get(
                    "log.level",
                    "info"
                )
            );

        const auto tick =
            htmlEscape(
                config.get(
                    "runtime.tick_ms",
                    "250"
                )
            );

        const auto web_bind =
            htmlEscape(
                config.get(
                    "web.bind",
                    "0.0.0.0"
                )
            );

        const auto web_port =
            htmlEscape(
                config.get(
                    "web.port",
                    "8080"
                )
            );

        const auto username =
            htmlEscape(
                session->username
            );

        const auto role =
            htmlEscape(
                SecurityManager::roleToString(
                    session->role
                )
            );

        std::ostringstream page;

        page << R"HTML(
<!doctype html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Home AI Core</title>
<style>
* {
    box-sizing: border-box;
}

body {
    margin: 0;
    font-family: system-ui, sans-serif;
    background: #111318;
    color: #e8eaf0;
}

header {
    padding: 18px 30px;
    background: #191c23;
    border-bottom: 1px solid #2c313c;
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 20px;
}

header h2 {
    margin: 0;
}

.userbar {
    display: flex;
    align-items: center;
    gap: 12px;
    color: #aeb4c0;
}

.userbar form {
    margin: 0;
}

main {
    max-width: 1000px;
    margin: 0 auto;
    padding: 25px;
}

.card {
    background: #191c23;
    border: 1px solid #2c313c;
    border-radius: 12px;
    padding: 22px;
    margin-bottom: 20px;
}

.status {
    color: #65d889;
    font-weight: 700;
}

.grid {
    display: grid;
    grid-template-columns:
        repeat(auto-fit, minmax(220px, 1fr));
    gap: 14px;
}

.metric {
    background: #12151a;
    border-radius: 8px;
    padding: 14px;
}

label {
    display: block;
    margin-top: 15px;
    margin-bottom: 6px;
}

input,
select {
    width: 100%;
    padding: 10px;
    background: #101217;
    border: 1px solid #363c49;
    border-radius: 7px;
    color: white;
}

button {
    margin-top: 20px;
    padding: 11px 20px;
    border: 0;
    border-radius: 7px;
    cursor: pointer;
    font-weight: 700;
}

.userbar button {
    margin: 0;
    padding: 8px 12px;
}

small {
    color: #999faa;
}
</style>
</head>
<body>
<header>
<h2>)HTML";

        page << core_name;

        page << R"HTML(</h2>
<div class="userbar">
<span>)HTML";

        page
            << username
            << " · "
            << role;

        page << R"HTML(</span>
<form method="POST" action="/logout">
<button type="submit">Выйти</button>
</form>
</div>
</header>

<main>

<div class="card">
<h3>Состояние ядра</h3>
<div class="grid">

<div class="metric">
Состояние<br>
<span class="status">RUNNING</span>
</div>

<div class="metric">
Версия<br>
<strong>)HTML";

        page << version;

        page << R"HTML(</strong>
</div>

<div class="metric">
Web Core<br>
<span class="status">RUNNING</span>
</div>

<div class="metric">
Security Core<br>
<span class="status">RUNNING</span>
</div>

<div class="metric">
CPU<br>
<strong id="cpu-value">...</strong>
</div>

<div class="metric">
RAM<br>
<strong id="ram-value">...</strong>
</div>

<div class="metric">
Disk<br>
<strong id="disk-value">...</strong>
</div>

<div class="metric">
Uptime<br>
<strong id="uptime-value">...</strong>
</div>

<div class="metric">
Load Average<br>
<strong id="load-value">...</strong>
</div>

</div>
</div>
)HTML";

        if (
            security_.isAdmin(
                session->role
            )
        ) {
            page << R"HTML(
<div class="card">
<h3>Настройки</h3>

<form method="POST" action="/api/config">

<label>Название ядра</label>
<input
    name="core.name"
    value=")HTML";

            page << core_name;

            page << R"HTML(">

<label>Уровень журналирования</label>
<select name="log.level">

<option value="debug")HTML";

            if (log_level == "debug")
                page << " selected";

            page << R"HTML(>Debug</option>
<option value="info")HTML";

            if (log_level == "info")
                page << " selected";

            page << R"HTML(>Info</option>
<option value="warning")HTML";

            if (log_level == "warning")
                page << " selected";

            page << R"HTML(>Warning</option>
<option value="error")HTML";

            if (log_level == "error")
                page << " selected";

            page << R"HTML(>Error</option>
</select>

<label>Runtime Tick, ms</label>
<input
    type="number"
    min="10"
    max="10000"
    name="runtime.tick_ms"
    value=")HTML";

            page << tick;

            page << R"HTML(">

<label>Web bind address</label>
<input
    name="web.bind"
    value=")HTML";

            page << web_bind;

            page << R"HTML(">

<label>Web port</label>
<input
    type="number"
    min="1"
    max="65535"
    name="web.port"
    value=")HTML";

            page << web_port;

            page << R"HTML(">

<button type="submit">
Сохранить настройки
</button>

</form>

<p>
<small>
Изменение адреса, порта и некоторых runtime-параметров
вступает в силу после перезапуска ядра.
</small>
</p>
</div>
)HTML";
        }

        page << R"HTML(
<div class="card">
<h3>Модули</h3>

<div class="grid">
<div class="metric">AI Brain — PLANNED</div>
<div class="metric">Smart Home — PLANNED</div>
<div class="metric">Memory — PLANNED</div>
<div class="metric">Hypervisor — PLANNED</div>
<div class="metric">Video Surveillance — PLANNED</div>
<div class="metric">Storage — PLANNED</div>
</div>
</div>

</main>

<script>
function formatUptime(seconds) {
    seconds = Number(seconds);

    const days =
        Math.floor(seconds / 86400);

    const hours =
        Math.floor(
            (seconds % 86400) / 3600
        );

    const minutes =
        Math.floor(
            (seconds % 3600) / 60
        );

    if (days > 0) {
        return (
            days + "d " +
            hours + "h " +
            minutes + "m"
        );
    }

    return (
        hours + "h " +
        minutes + "m"
    );
}

async function updateSystemStats() {
    try {
        const response =
            await fetch(
                "/api/system",
                {
                    method: "GET",
                    cache: "no-store"
                }
            );

        if (response.status === 401) {
            window.location = "/login";
            return;
        }

        if (!response.ok)
            return;

        const data =
            await response.json();

        document.getElementById(
            "cpu-value"
        ).textContent =
            Number(
                data.cpu_percent
            ).toFixed(1) + "%";

        document.getElementById(
            "ram-value"
        ).textContent =
            Number(
                data.memory_percent
            ).toFixed(1) + "%";

        document.getElementById(
            "disk-value"
        ).textContent =
            Number(
                data.disk_percent
            ).toFixed(1) + "%";

        document.getElementById(
            "uptime-value"
        ).textContent =
            formatUptime(
                data.uptime_seconds
            );

        document.getElementById(
            "load-value"
        ).textContent =
            Number(data.load_1).toFixed(2)
            + " / "
            + Number(data.load_5).toFixed(2)
            + " / "
            + Number(data.load_15).toFixed(2);
    }
    catch (error) {
        console.error(
            "System monitor error:",
            error
        );
    }
}

document.addEventListener(
    "DOMContentLoaded",
    function() {
        updateSystemStats();

        setInterval(
            updateSystemStats,
            2000
        );
    }
);
</script>

</body>
</html>
)HTML";

        sendResponse(
            client_fd,
            "200 OK",
            "text/html; charset=utf-8",
            page.str()
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
