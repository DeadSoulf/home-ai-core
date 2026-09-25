#pragma once

#include <cstdint>
#include <string>
#include <vector>

namespace homeai {

struct NetworkInterfaceInfo {
    std::string name;
    std::string mac_address;
    std::string oper_state;

    std::vector<std::string>
        ipv4_addresses;

    std::uint32_t mtu{0};

    bool up{false};
    bool carrier{false};
    bool loopback{false};
    bool default_route{false};
};

struct NetworkActionResult {
    bool success{false};
    std::string code;
    std::string message;
};

class NetworkInterfaceManager {
public:
    std::vector<NetworkInterfaceInfo>
    interfaces() const;

    bool helperInstalled() const;

    NetworkActionResult requestDhcp(
        const std::string& interface_name
    ) const;

    static bool validInterfaceName(
        const std::string& interface_name
    );
};

}
