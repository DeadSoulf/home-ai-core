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
    std::string files_root;
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
