#pragma once

#include <string>
#include <vector>

namespace homeai {

struct OnvifDevice {
    std::string endpoint_reference;
    std::string xaddr;
    std::string types;
    std::string scopes;
    std::string remote_address;
};

class OnvifDiscovery {
public:
    std::vector<OnvifDevice>
    discover(
        int timeout_ms,
        std::string& error
    ) const;

    static std::vector<OnvifDevice>
    parseResponse(
        const std::string& xml,
        const std::string& remote_address
    );

    static std::string scopeValue(
        const std::string& scopes,
        const std::string& category
    );
};

}
