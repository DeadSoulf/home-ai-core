#pragma once

#include <string>
#include <vector>

namespace homeai {

struct LanCameraDevice {
    std::string address;
    std::string vendor_hint;
    int rtsp_port{0};
    std::vector<int> open_ports;
};

class LanCameraDiscovery {
public:
    std::vector<LanCameraDevice>
    discover(
        int timeout_ms,
        std::string& error
    ) const;

    static std::string vendorHint(
        const std::vector<int>& open_ports
    );

    static bool likelyCamera(
        const std::vector<int>& open_ports
    );

    static std::string suggestedRtspUrl(
        const std::string& address,
        const std::string& vendor_hint,
        int rtsp_port
    );
};

}
