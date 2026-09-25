#include "web/server/WebServer.h"

#include "core/logging/Logger.h"
#include "core/runtime/CoreRuntime.h"

#include <algorithm>
#include <arpa/inet.h>
#include <cerrno>
#include <cstring>
#include <sstream>
#include <string>
#include <sys/socket.h>
#include <unistd.h>
#include <unordered_map>

namespace homeai {

namespace {

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

    for (std::size_t i = 0; i < value.size(); ++i) {
        if (value[i] == '+') {
            result += ' ';
        }
        else if (
            value[i] == '%' &&
            i + 2 < value.size()
        ) {
            const int high = hexValue(value[i + 1]);
            const int low  = hexValue(value[i + 2]);

            if (high >= 0 && low >= 0) {
                result += static_cast<char>(
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
    std::unordered_map<std::string, std::string> values;

    std::size_t start = 0;

    while (start < body.size()) {
        auto end = body.find('&', start);

        if (end == std::string::npos)
            end = body.size();

        auto part = body.substr(start, end - start);
        auto separator = part.find('=');

        if (separator != std::string::npos) {
            auto key = urlDecode(
                part.substr(0, separator)
            );

            auto value = urlDecode(
                part.substr(separator + 1)
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
        const auto result = ::send(
            socket_fd,
            data.data() + sent,
            data.size() - sent,
            MSG_NOSIGNAL
        );

        if (result <= 0)
            return;

        sent += static_cast<std::size_t>(result);
    }
}

void sendResponse(
    int client_fd,
    const std::string& status,
    const std::string& content_type,
    const std::string& body
)
{
    std::ostringstream response;

    response
        << "HTTP/1.1 " << status << "\r\n"
        << "Content-Type: " << content_type << "\r\n"
        << "Content-Length: " << body.size() << "\r\n"
        << "Connection: close\r\n"
        << "Cache-Control: no-store\r\n"
        << "\r\n"
        << body;

    sendAll(client_fd, response.str());
}

std::size_t readContentLength(
    const std::string& headers
)
{
    const std::string key = "Content-Length:";

    auto position = headers.find(key);

    if (position == std::string::npos)
        return 0;

    position += key.size();

    auto end = headers.find("\r\n", position);

    auto value = headers.substr(
        position,
        end - position
    );

    try {
        return static_cast<std::size_t>(
            std::stoul(value)
        );
    }
    catch (...) {
        return 0;
    }
}

}

WebServer::WebServer(CoreRuntime& runtime)
    : runtime_(runtime)
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

    server_fd_ = ::socket(
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
            reinterpret_cast<sockaddr*>(&address),
            sizeof(address)
        ) < 0
    ) {
        Logger::instance().error(
            "WebServer: bind failed: " +
            std::string(std::strerror(errno))
        );

        ::close(server_fd_);
        server_fd_ = -1;

        return false;
    }

    if (::listen(server_fd_, 32) < 0) {
        Logger::instance().error(
            "WebServer: listen failed"
        );

        ::close(server_fd_);
        server_fd_ = -1;

        return false;
    }

    running_ = true;

    server_thread_ = std::thread(
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

        const int client_fd = ::accept(
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

void WebServer::handleClient(int client_fd)
{
    std::string request;
    char buffer[4096];

    while (request.size() < 65536) {
        const auto received = ::recv(
            client_fd,
            buffer,
            sizeof(buffer),
            0
        );

        if (received <= 0)
            break;

        request.append(
            buffer,
            static_cast<std::size_t>(received)
        );

        const auto header_end =
            request.find("\r\n\r\n");

        if (header_end != std::string::npos) {
            const auto content_length =
                readContentLength(
                    request.substr(0, header_end)
                );

            const auto body_size =
                request.size() -
                (header_end + 4);

            if (body_size >= content_length)
                break;
        }
    }

    const auto first_line_end =
        request.find("\r\n");

    if (first_line_end == std::string::npos)
        return;

    const auto first_line =
        request.substr(0, first_line_end);

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

        const std::string body =
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
            body
        );

        return;
    }

    if (
        method == "GET" &&
        path == "/api/config"
    ) {
        const auto& config =
            runtime_.config();

        const std::string body =
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
            body
        );

        return;
    }

    if (
        method == "POST" &&
        path == "/api/config"
    ) {
        const auto header_end =
            request.find("\r\n\r\n");

        std::string body;

        if (header_end != std::string::npos)
            body = request.substr(
                header_end + 4
            );

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
            const auto it = form.find(key);

            if (it != form.end())
                config.set(
                    key,
                    it->second
                );
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

        const std::string response =
            "<!doctype html>"
            "<html>"
            "<head>"
            "<meta charset=\"utf-8\">"
            "<meta http-equiv=\"refresh\" "
            "content=\"2;url=/\">"
            "<title>Home AI Core</title>"
            "</head>"
            "<body>"
            "<h2>Configuration saved</h2>"
            "<p>Some settings require a core restart.</p>"
            "<p>Returning to dashboard...</p>"
            "</body>"
            "</html>";

        sendResponse(
            client_fd,
            "200 OK",
            "text/html; charset=utf-8",
            response
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

        std::ostringstream page;

        page << R"HTML(
<!doctype html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">

<title>Home AI Core</title>

<style>
body {
    margin: 0;
    font-family: system-ui, sans-serif;
    background: #111318;
    color: #e8eaf0;
}

header {
    padding: 22px 30px;
    background: #191c23;
    border-bottom: 1px solid #2c313c;
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

input, select {
    box-sizing: border-box;
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

</div>
</div>

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

<div class="card">
<h3>Будущие модули</h3>

<div class="grid">

<div class="metric">AI Brain</div>
<div class="metric">Smart Home</div>
<div class="metric">Memory</div>
<div class="metric">Hypervisor</div>
<div class="metric">Video Surveillance</div>
<div class="metric">Storage</div>

</div>

</div>

</main>

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
