#pragma once

#include "server/system/SystemMonitor.h"

#include <atomic>
#include <condition_variable>
#include <cstdint>
#include <map>
#include <mutex>
#include <string>
#include <thread>
#include <vector>

namespace homeai {

struct ClusterNodeInfo {
    std::string id;
    std::string name;
    std::string role;
    std::string address;
    double cpu_percent{0.0};
    double memory_percent{0.0};
    double disk_percent{0.0};
    double score{0.0};
    std::uint64_t last_seen_unix{0};
    bool online{false};
    bool local{false};
};

struct ClusterSnapshot {
    bool enabled{false};
    bool configured{false};
    std::string role;
    std::string local_node_id;
    std::string local_node_name;
    std::string controller_host;
    std::uint16_t controller_port{0};
    int heartbeat_interval_seconds{0};
    int timeout_seconds{0};
    std::size_t online_nodes{0};
    std::string message;
    std::vector<ClusterNodeInfo> nodes;
};

struct ClusterPlacement {
    bool available{false};
    std::string workload;
    std::string node_id;
    std::string node_name;
    std::string reason;
    double score{0.0};
};

class ClusterManager {
public:
    ClusterManager();
    ~ClusterManager();

    ClusterManager(const ClusterManager&) = delete;
    ClusterManager& operator=(const ClusterManager&) = delete;

    bool initialize(
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
    );

    bool start(std::string& error);
    void stop();

    bool healthy() const;
    std::string healthMessage() const;

    ClusterSnapshot snapshot();
    ClusterPlacement selectNode(const std::string& workload);

    bool acceptHeartbeat(
        const std::string& token,
        const std::string& node_id,
        const std::string& node_name,
        const std::string& node_role,
        const std::string& address,
        double cpu_percent,
        double memory_percent,
        double disk_percent,
        std::string& error
    );

    bool enabled() const;
    bool isController() const;

    static bool validNodeId(const std::string& value);

private:
    void run();
    void refreshLocalNode();
    bool sendHeartbeat();
    bool tokenMatches(const std::string& token) const;

    static double loadScore(
        const ClusterNodeInfo& node,
        const std::string& workload
    );

    static std::uint64_t unixNow();

    mutable std::mutex mutex_;
    std::mutex wait_mutex_;
    std::condition_variable wait_cv_;

    std::atomic<bool> running_{false};
    std::thread thread_;

    bool initialized_{false};
    bool enabled_{false};
    bool configured_{false};

    std::string node_id_;
    std::string node_name_;
    std::string role_;
    std::string advertise_address_;
    std::string controller_host_;
    std::uint16_t controller_port_{0};
    std::string shared_token_;

    int heartbeat_interval_seconds_{5};
    int timeout_seconds_{20};

    SystemMonitor monitor_;

    std::map<std::string, ClusterNodeInfo> nodes_;

    bool heartbeat_success_{false};
    std::uint64_t last_heartbeat_success_unix_{0};
    std::uint64_t started_unix_{0};

    std::string last_message_;
};

}
