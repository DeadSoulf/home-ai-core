#pragma once

#include <cstddef>
#include <string>
#include <vector>

namespace homeai {

struct OnvifMediaProfile {
    std::string token;
    std::string name;
    std::string encoding;
    int width{0};
    int height{0};
    double fps{0.0};
    std::string rtsp_uri;
};

struct OnvifDeviceInformation {
    std::string manufacturer;
    std::string model;
    std::string firmware_version;
    std::string serial_number;
    std::string hardware_id;
};

struct OnvifMediaResult {
    bool success{false};
    std::string code;
    std::string message;
    std::string media_xaddr;
    OnvifDeviceInformation device_info;
    std::vector<OnvifMediaProfile> profiles;
    std::size_t recommended_index{0};
};

class OnvifMediaClient {
public:
    OnvifMediaResult profiles(
        const std::string& device_xaddr,
        const std::string& username,
        const std::string& password
    ) const;

    static OnvifDeviceInformation
    parseDeviceInformation(
        const std::string& xml
    );

    static std::string
    parseMediaXAddr(
        const std::string& xml
    );

    static std::vector<OnvifMediaProfile>
    parseProfiles(
        const std::string& xml
    );

    static std::string
    parseStreamUri(
        const std::string& xml
    );

    static std::string
    stripUriCredentials(
        const std::string& uri
    );
};

}
