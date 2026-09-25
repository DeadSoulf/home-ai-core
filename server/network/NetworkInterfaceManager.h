#pragma once

#include <cstdint>
#include <string>
#include <vector>

namespace homeai {

struct NetworkInterfaceInfo {
    std::string name;
    std::string mac_address;
    std::string oper_state;
    std::string ipv4_method{"unknown"};
    std::string gateway;

    std::vector<std::string>
        ipv4_addresses;

    std::vector<std::string>
        dns_servers;

    std::uint32_t mtu{0};

    bool up{false};
    bool carrier{false};
    bool loopback{false};
    bool default_route{false};
};

struct NetworkStaticConfig {
    std::string address;
    std::string netmask;
    std::string gateway;
    std::string dns_primary;
    std::string dns_secondary;
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

    NetworkActionResult setStaticIpv4(
        const std::string& interface_name,
        const NetworkStaticConfig& config
    ) const;

    static bool validInterfaceName(
        const std::string& interface_name
    );

    static bool validIpv4Address(
        const std::string& address
    );

    static int netmaskPrefix(
        const std::string& netmask
    );
};

}
