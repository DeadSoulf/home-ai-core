#pragma once

#include <string>
#include <vector>

namespace homeai {

struct HikvisionSadpDevice {
    std::string address;
    std::string mac;
    std::string model;
    std::string serial_number;
    std::string software_version;
    int command_port{0};
    int http_port{0};
    bool activated{false};
};

class HikvisionSadpDiscovery {
public:
    std::vector<HikvisionSadpDevice>
    discover(
        int timeout_ms,
        std::string& error
    ) const;

    static HikvisionSadpDevice
    parseResponse(
        const std::string& xml
    );
};

}
