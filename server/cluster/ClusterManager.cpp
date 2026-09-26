#include "server/cluster/ClusterManager.h"

#include <cerrno>
#include <cctype>
#include <cmath>
#include <fcntl.h>
#include <iomanip>
#include <limits>
#include <netdb.h>
#include <sstream>
#include <sys/select.h>
#include <sys/socket.h>
#include <unistd.h>

#include <algorithm>
#include <chrono>

namespace homeai {

namespace {

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

std::string defaultNodeId()
{
    char hostname[256]{};

    if (
        ::gethostname(
            hostname,
            sizeof(hostname) - 1
        ) != 0
    ) {
        return "node";
    }

    std::string result = hostname;

    for (auto& c : result) {
        const auto value =
            static_cast<unsigned char>(c);

        if (
            !std::isalnum(value)
            &&
            c != '.'
            &&
            c != '_'
            &&
            c != '-'
        ) {
            c = '-';
        }
    }

    if (result.empty())
        return "node";

    if (result.size() > 64)
        result.resize(64);

    return result;
}

std::string formEncode(
    const std::string& value
)
{
    static const char* hex =
        "0123456789ABCDEF";

    std::string result;

    for (unsigned char c : value) {
        if (
            std::isalnum(c)
            ||
            c == '-'
            ||
            c == '_'
            ||
            c == '.'
            ||
            c == '~'
        ) {
            result.push_back(
                static_cast<char>(c)
            );
            continue;
        }

        result.push_back('%');
        result.push_back(
            hex[(c >> 4) & 0x0f]
        );
        result.push_back(
            hex[c & 0x0f]
        );
    }

    return result;
}

std::string percentString(double value)
{
    std::ostringstream stream;

    stream
        << std::fixed
        << std::setprecision(2)
        << value;

    return stream.str();
}

bool connectTcp(
    const std::string& host,
    std::uint16_t port,
    int& fd
)
{
    fd = -1;

    struct addrinfo hints {};
    hints.ai_family = AF_UNSPEC;
    hints.ai_socktype = SOCK_STREAM;

    struct addrinfo* addresses = nullptr;

    const auto service =
        std::to_string(port);

    if (
        ::getaddrinfo(
            host.c_str(),
            service.c_str(),
            &hints,
            &addresses
        ) != 0
    ) {
        return false;
    }

    for (
        auto* address = addresses;
        address;
        address = address->ai_next
    ) {
        const int current =
            ::socket(
                address->ai_family,
                address->ai_socktype,
                address->ai_protocol
            );

        if (current < 0)
            continue;

        const int flags =
            ::fcntl(
                current,
                F_GETFL,
                0
            );

        if (flags >= 0) {
            ::fcntl(
                current,
                F_SETFL,
                flags | O_NONBLOCK
            );
        }

        int result =
            ::connect(
                current,
                address->ai_addr,
                address->ai_addrlen
            );

        if (
            result < 0
            &&
            errno == EINPROGRESS
        ) {
            fd_set write_set;
            FD_ZERO(&write_set);
            FD_SET(current, &write_set);

            struct timeval timeout {};
            timeout.tv_sec = 2;

            result =
                ::select(
                    current + 1,
                    nullptr,
                    &write_set,
                    nullptr,
                    &timeout
                );

            if (result > 0) {
                int socket_error = 0;
                socklen_t length =
                    sizeof(socket_error);

                if (
                    ::getsockopt(
                        current,
                        SOL_SOCKET,
                        SO_ERROR,
                        &socket_error,
                        &length
                    ) != 0
                    ||
                    socket_error != 0
                ) {
                    result = -1;
                }
                else {
                    result = 0;
                }
            }
            else {
                result = -1;
            }
        }

        if (flags >= 0) {
            ::fcntl(
                current,
                F_SETFL,
                flags
            );
        }

        if (result == 0) {
            fd = current;
            break;
        }

        ::close(current);
    }

    ::freeaddrinfo(addresses);

    return fd >= 0;
}

bool postHeartbeat(
    const std::string& host,
    std::uint16_t port,
    const std::string& token,
    const std::string& body
)
{
    int fd = -1;

    if (!connectTcp(host, port, fd))
        return false;

    struct timeval timeout {};
    timeout.tv_sec = 2;

    ::setsockopt(
        fd,
        SOL_SOCKET,
        SO_RCVTIMEO,
        &timeout,
        sizeof(timeout)
    );

    ::setsockopt(
        fd,
        SOL_SOCKET,
        SO_SNDTIMEO,
        &timeout,
        sizeof(timeout)
    );

    std::ostringstream request;

    request
        << "POST /api/cluster/heartbeat HTTP/1.1\r\n"
        << "Host: " << host << "\r\n"
        << "Content-Type: application/x-www-form-urlencoded\r\n"
        << "X-HomeAI-Cluster-Token: " << token << "\r\n"
        << "Content-Length: " << body.size() << "\r\n"
        << "Connection: close\r\n\r\n"
        << body;

    const auto payload = request.str();
    std::size_t sent = 0;

    while (sent < payload.size()) {
        const auto result =
            ::send(
                fd,
                payload.data() + sent,
                payload.size() - sent,
                MSG_NOSIGNAL
            );

        if (result <= 0) {
            ::close(fd);
            return false;
        }

        sent +=
            static_cast<std::size_t>(result);
    }

    char buffer[256]{};

    const auto received =
        ::recv(
            fd,
            buffer,
            sizeof(buffer) - 1,
            0
        );

    ::close(fd);

    if (received <= 0)
        return false;

    const std::string response(
        buffer,
        static_cast<std::size_t>(received)
    );

    return
        response.rfind(
            "HTTP/1.1 200",
            0
        ) == 0
        ||
        response.rfind(
            "HTTP/1.0 200",
            0
        ) == 0;
}

bool validPercent(double value)
{
    return
        std::isfinite(value)
        &&
        value >= 0.0
        &&
        value <= 100.0;
}

}

ClusterManager::ClusterManager() = default;

ClusterManager::~ClusterManager()
{
    stop();
}

bool ClusterManager::initialize(
    bool enabled,
    std::string node_id,
    std::string node_name,
    std::string role,
    std::string advertise_address,
    std::string controller_host,
    std::uint16_t controller_port,
    std::string shared_token,
    int heartbeat_interval_seconds,
    int timeout_seconds,
    std::string& error
)
{
    stop();

    std::lock_guard<std::mutex>
        lock(mutex_);

    initialized_ = true;
    enabled_ = enabled;

    role =
        lowerCopy(
            std::move(role)
        );

    if (!enabled_) {
        role_ = "standalone";
    }
    else {
        role_ = role;
    }

    node_id_ =
        node_id.empty()
        ? defaultNodeId()
        : std::move(node_id);

    node_name_ =
        node_name.empty()
        ? node_id_
        : std::move(node_name);

    advertise_address_ =
        std::move(advertise_address);

    controller_host_ =
        std::move(controller_host);

    controller_port_ =
        controller_port;

    shared_token_ =
        std::move(shared_token);

    heartbeat_interval_seconds_ =
        std::clamp(
            heartbeat_interval_seconds,
            2,
            60
        );

    timeout_seconds_ =
        std::clamp(
            timeout_seconds,
            heartbeat_interval_seconds_ * 2,
            300
        );

    configured_ = true;

    if (
        enabled_
        &&
        role_ != "controller"
        &&
        role_ != "worker"
    ) {
        configured_ = false;
        last_message_ =
            "Cluster role must be controller or worker.";
    }
    else if (
        enabled_
        &&
        !validNodeId(node_id_)
    ) {
        configured_ = false;
        last_message_ =
            "Cluster node ID contains unsupported characters.";
    }
    else if (
        enabled_
        &&
        shared_token_.size() < 16
    ) {
        configured_ = false;
        last_message_ =
            "Cluster shared token must contain at least 16 characters.";
    }
    else if (
        enabled_
        &&
        role_ == "worker"
        &&
        controller_host_.empty()
    ) {
        configured_ = false;
        last_message_ =
            "Cluster worker requires controller host.";
    }
    else if (!enabled_) {
        last_message_ =
            "Cluster mode is disabled.";
    }
    else if (role_ == "controller") {
        last_message_ =
            "Cluster controller is ready.";
    }
    else {
        last_message_ =
            "Cluster worker is ready to connect.";
    }

    nodes_.clear();
    heartbeat_success_ = false;
    last_heartbeat_success_unix_ = 0;
    started_unix_ = 0;

    error.clear();
    return true;
}

bool ClusterManager::start(
    std::string& error
)
{
    {
        std::lock_guard<std::mutex>
            lock(mutex_);

        if (!initialized_) {
            error =
                "Cluster Core is not initialized.";
            return false;
        }

        if (!enabled_) {
            error.clear();
            return true;
        }

        if (!configured_) {
            error.clear();
            return true;
        }

        if (running_) {
            error.clear();
            return true;
        }

        started_unix_ =
            unixNow();
    }

    running_ = true;

    thread_ =
        std::thread(
            &ClusterManager::run,
            this
        );

    error.clear();
    return true;
}

void ClusterManager::stop()
{
    running_ = false;
    wait_cv_.notify_all();

    if (thread_.joinable())
        thread_.join();

    std::lock_guard<std::mutex>
        lock(mutex_);

    if (
        initialized_
        &&
        enabled_
    ) {
        last_message_ =
            "Cluster Core stopped.";
    }
}

bool ClusterManager::healthy() const
{
    std::lock_guard<std::mutex>
        lock(mutex_);

    if (!initialized_)
        return false;

    if (!enabled_)
        return true;

    if (!configured_)
        return false;

    if (role_ == "controller")
        return true;

    if (role_ != "worker")
        return false;

    if (!running_)
        return false;

    const auto now =
        unixNow();

    if (
        heartbeat_success_
        &&
        now <=
            last_heartbeat_success_unix_
            +
            static_cast<std::uint64_t>(
                timeout_seconds_
            )
    ) {
        return true;
    }

    return
        started_unix_ > 0
        &&
        now <=
            started_unix_
            +
            static_cast<std::uint64_t>(
                timeout_seconds_
            );
}

std::string ClusterManager::healthMessage() const
{
    std::lock_guard<std::mutex>
        lock(mutex_);

    return last_message_;
}

ClusterSnapshot ClusterManager::snapshot()
{
    refreshLocalNode();

    ClusterSnapshot result;

    std::lock_guard<std::mutex>
        lock(mutex_);

    result.enabled = enabled_;
    result.configured = configured_;
    result.role = role_;
    result.local_node_id = node_id_;
    result.local_node_name = node_name_;
    result.controller_host = controller_host_;
    result.controller_port = controller_port_;
    result.heartbeat_interval_seconds =
        heartbeat_interval_seconds_;
    result.timeout_seconds =
        timeout_seconds_;
    result.message = last_message_;

    const auto now = unixNow();

    for (
        const auto& [id, stored] :
        nodes_
    ) {
        (void)id;

        auto node = stored;

        node.online =
            node.local
            ||
            (
                node.last_seen_unix > 0
                &&
                now <=
                    node.last_seen_unix
                    +
                    static_cast<std::uint64_t>(
                        timeout_seconds_
                    )
            );

        node.score =
            loadScore(
                node,
                "generic"
            );

        if (node.online)
            ++result.online_nodes;

        result.nodes.push_back(
            std::move(node)
        );
    }

    return result;
}

ClusterPlacement ClusterManager::selectNode(
    const std::string& workload
)
{
    refreshLocalNode();

    ClusterPlacement result;
    result.workload =
        workload.empty()
        ? "generic"
        : lowerCopy(workload);

    std::lock_guard<std::mutex>
        lock(mutex_);

    if (!enabled_) {
        result.reason =
            "Cluster mode is disabled.";
        return result;
    }

    if (!configured_) {
        result.reason =
            last_message_;
        return result;
    }

    if (role_ != "controller") {
        result.reason =
            "Placement decisions are made by the cluster controller.";
        return result;
    }

    const auto now = unixNow();

    double best_score =
        std::numeric_limits<double>::
            infinity();

    for (
        const auto& [id, node] :
        nodes_
    ) {
        (void)id;

        const bool online =
            node.local
            ||
            (
                node.last_seen_unix > 0
                &&
                now <=
                    node.last_seen_unix
                    +
                    static_cast<std::uint64_t>(
                        timeout_seconds_
                    )
            );

        if (!online)
            continue;

        if (
            node.cpu_percent >= 95.0
            ||
            node.memory_percent >= 95.0
            ||
            node.disk_percent >= 98.0
        ) {
            continue;
        }

        const auto score =
            loadScore(
                node,
                result.workload
            );

        if (
            score < best_score
            ||
            (
                std::abs(
                    score - best_score
                ) < 0.0001
                &&
                (
                    result.node_id.empty()
                    ||
                    node.id < result.node_id
                )
            )
        ) {
            best_score = score;
            result.available = true;
            result.node_id = node.id;
            result.node_name = node.name;
            result.score = score;
        }
    }

    if (result.available) {
        result.reason =
            "Selected the online node with the lowest weighted load.";
    }
    else {
        result.reason =
            "No online cluster node has enough free capacity.";
    }

    return result;
}

bool ClusterManager::acceptHeartbeat(
    const std::string& token,
    const std::string& node_id,
    const std::string& node_name,
    const std::string& node_role,
    const std::string& address,
    double cpu_percent,
    double memory_percent,
    double disk_percent,
    std::string& error
)
{
    std::lock_guard<std::mutex>
        lock(mutex_);

    if (
        !enabled_
        ||
        role_ != "controller"
        ||
        !configured_
    ) {
        error =
            "cluster_not_controller";
        return false;
    }

    if (!tokenMatches(token)) {
        error =
            "unauthorized";
        return false;
    }

    if (
        !validNodeId(node_id)
        ||
        node_id == node_id_
    ) {
        error =
            "invalid_node_id";
        return false;
    }

    const auto role =
        lowerCopy(node_role);

    if (
        role != "worker"
        &&
        role != "controller"
    ) {
        error =
            "invalid_node_role";
        return false;
    }

    if (
        node_name.size() > 128
        ||
        address.size() > 256
        ||
        !validPercent(cpu_percent)
        ||
        !validPercent(memory_percent)
        ||
        !validPercent(disk_percent)
    ) {
        error =
            "invalid_heartbeat";
        return false;
    }

    ClusterNodeInfo node;
    node.id = node_id;
    node.name =
        node_name.empty()
        ? node_id
        : node_name;
    node.role = role;
    node.address = address;
    node.cpu_percent = cpu_percent;
    node.memory_percent = memory_percent;
    node.disk_percent = disk_percent;
    node.last_seen_unix = unixNow();
    node.online = true;
    node.local = false;
    node.score =
        loadScore(
            node,
            "generic"
        );

    nodes_[node.id] =
        std::move(node);

    last_message_ =
        "Cluster controller is receiving node heartbeats.";

    error.clear();
    return true;
}

bool ClusterManager::enabled() const
{
    std::lock_guard<std::mutex>
        lock(mutex_);
    return enabled_;
}

bool ClusterManager::isController() const
{
    std::lock_guard<std::mutex>
        lock(mutex_);

    return
        enabled_
        &&
        configured_
        &&
        role_ == "controller";
}

bool ClusterManager::validNodeId(
    const std::string& value
)
{
    if (
        value.empty()
        ||
        value.size() > 64
    ) {
        return false;
    }

    return
        std::all_of(
            value.begin(),
            value.end(),
            [](unsigned char c) {
                return
                    std::isalnum(c)
                    ||
                    c == '.'
                    ||
                    c == '_'
                    ||
                    c == '-';
            }
        );
}

void ClusterManager::run()
{
    while (running_) {
        refreshLocalNode();

        bool send_to_controller = false;

        {
            std::lock_guard<std::mutex>
                lock(mutex_);

            send_to_controller =
                enabled_
                &&
                configured_
                &&
                role_ == "worker";
        }

        if (send_to_controller) {
            const bool sent =
                sendHeartbeat();

            std::lock_guard<std::mutex>
                lock(mutex_);

            if (sent) {
                heartbeat_success_ = true;
                last_heartbeat_success_unix_ =
                    unixNow();
                last_message_ =
                    "Cluster worker is connected to the controller.";
            }
            else {
                last_message_ =
                    "Cluster worker cannot reach the controller.";
            }
        }
        else {
            std::lock_guard<std::mutex>
                lock(mutex_);

            if (role_ == "controller") {
                std::size_t online = 0;
                const auto now = unixNow();

                for (
                    const auto& [id, node] :
                    nodes_
                ) {
                    (void)id;

                    if (
                        node.local
                        ||
                        (
                            node.last_seen_unix > 0
                            &&
                            now <=
                                node.last_seen_unix
                                +
                                static_cast<std::uint64_t>(
                                    timeout_seconds_
                                )
                        )
                    ) {
                        ++online;
                    }
                }

                last_message_ =
                    "Cluster controller active. Online nodes: "
                    +
                    std::to_string(online)
                    +
                    ".";
            }
        }

        std::unique_lock<std::mutex>
            wait_lock(wait_mutex_);

        wait_cv_.wait_for(
            wait_lock,
            std::chrono::seconds(
                heartbeat_interval_seconds_
            ),
            [this]() {
                return !running_.load();
            }
        );
    }
}

void ClusterManager::refreshLocalNode()
{
    {
        std::lock_guard<std::mutex>
            lock(mutex_);

        if (!initialized_)
            return;
    }

    const auto stats =
        monitor_.snapshot();

    std::lock_guard<std::mutex>
        lock(mutex_);

    ClusterNodeInfo node;
    node.id = node_id_;
    node.name = node_name_;
    node.role = role_;
    node.address = advertise_address_;
    node.cpu_percent =
        std::clamp(
            stats.cpu_percent,
            0.0,
            100.0
        );
    node.memory_percent =
        std::clamp(
            stats.memory_percent,
            0.0,
            100.0
        );
    node.disk_percent =
        std::clamp(
            stats.disk_percent,
            0.0,
            100.0
        );
    node.last_seen_unix = unixNow();
    node.online = true;
    node.local = true;
    node.score =
        loadScore(
            node,
            "generic"
        );

    nodes_[node.id] =
        std::move(node);
}

bool ClusterManager::sendHeartbeat()
{
    ClusterNodeInfo local;
    std::string host;
    std::string token;
    std::uint16_t port = 0;

    {
        std::lock_guard<std::mutex>
            lock(mutex_);

        const auto it =
            nodes_.find(node_id_);

        if (it == nodes_.end())
            return false;

        local = it->second;
        host = controller_host_;
        port = controller_port_;
        token = shared_token_;
    }

    const std::string body =
        "node_id="
        + formEncode(local.id)
        + "&node_name="
        + formEncode(local.name)
        + "&node_role="
        + formEncode(local.role)
        + "&address="
        + formEncode(local.address)
        + "&cpu_percent="
        + formEncode(
            percentString(
                local.cpu_percent
            )
        )
        + "&memory_percent="
        + formEncode(
            percentString(
                local.memory_percent
            )
        )
        + "&disk_percent="
        + formEncode(
            percentString(
                local.disk_percent
            )
        );

    return
        postHeartbeat(
            host,
            port,
            token,
            body
        );
}

bool ClusterManager::tokenMatches(
    const std::string& token
) const
{
    if (
        shared_token_.empty()
        ||
        token.size() !=
            shared_token_.size()
    ) {
        return false;
    }

    unsigned char difference = 0;

    for (
        std::size_t index = 0;
        index < token.size();
        ++index
    ) {
        difference |=
            static_cast<unsigned char>(
                token[index]
                ^
                shared_token_[index]
            );
    }

    return difference == 0;
}

double ClusterManager::loadScore(
    const ClusterNodeInfo& node,
    const std::string& workload
)
{
    double cpu_weight = 0.45;
    double memory_weight = 0.40;
    double disk_weight = 0.15;

    const auto type =
        lowerCopy(workload);

    if (type == "ai") {
        cpu_weight = 0.45;
        memory_weight = 0.45;
        disk_weight = 0.10;
    }
    else if (
        type == "camera"
        ||
        type == "cameras"
    ) {
        cpu_weight = 0.40;
        memory_weight = 0.25;
        disk_weight = 0.35;
    }
    else if (
        type == "vm"
        ||
        type == "virtual-machine"
    ) {
        cpu_weight = 0.40;
        memory_weight = 0.50;
        disk_weight = 0.10;
    }

    return
        node.cpu_percent * cpu_weight
        +
        node.memory_percent * memory_weight
        +
        node.disk_percent * disk_weight;
}

std::uint64_t ClusterManager::unixNow()
{
    return
        static_cast<std::uint64_t>(
            std::chrono::duration_cast<
                std::chrono::seconds
            >(
                std::chrono::
                    system_clock::now()
                    .time_since_epoch()
            ).count()
        );
}

}
