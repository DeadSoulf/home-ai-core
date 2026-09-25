#pragma once

#include <string>

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
    std::string username;
    std::string role;
    bool admin{false};
};

bool isWebUiPath(
    const std::string& path
);

std::string renderWebUi(
    const WebUiContext& context
);

}
