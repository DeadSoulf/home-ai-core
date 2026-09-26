#pragma once

#include <string>
#include <vector>

namespace homeai {

struct WebUiContext {
    std::string page;
    std::string core_name;
    std::string version;
    std::string log_level;
    std::string tick_ms;
    std::string web_bind;
    std::string web_port;
    std::string storage_video_mounts;
    std::string storage_personal_mounts;
    std::string storage_video_policy;
    std::string storage_files_policy;
    std::string storage_video_reserve_percent;
    std::string storage_files_reserve_percent;
    std::string storage_video_reserve_gb;
    std::string storage_files_reserve_gb;
    std::string storage_video_pinned_mount;
    std::string storage_files_pinned_mount;
    std::string files_root;
    std::string cluster_enabled;
    std::string cluster_role;
    std::string cluster_node_id;
    std::string cluster_node_name;
    std::string cluster_advertise_address;
    std::string cluster_controller_host;
    std::string cluster_controller_port;
    std::string cluster_token_configured;
    std::string cluster_heartbeat_interval;
    std::string cluster_timeout;
    std::string username;
    std::string role;
    std::vector<std::string> permissions;
    bool admin{false};
};

bool isWebUiPath(
    const std::string& path
);

std::string renderWebUi(
    const WebUiContext& context
);

}
