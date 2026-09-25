#pragma once

#include <string>
#include <vector>

namespace homeai {

struct VpnProfileInfo {
    std::string name;
    bool active{false};
};

struct VpnActionResult {
    bool success{false};
    std::string code;
    std::string message;
};

struct VpnProfileConfigResult {
    bool success{false};
    std::string code;
    std::string message;
    std::string config;
};

class VpnService {
public:
    VpnService();
    ~VpnService();

    VpnService(
        const VpnService&
    ) = delete;

    VpnService& operator=(
        const VpnService&
    ) = delete;

    bool initialize(
        const std::string& config_directory
    );

    bool available() const;

    std::vector<VpnProfileInfo>
    profiles(
        std::string& error
    ) const;

    VpnProfileConfigResult loadProfile(
        const std::string& profile
    ) const;

    VpnActionResult saveProfile(
        const std::string& profile,
        const std::string& config
    ) const;

    VpnActionResult removeProfile(
        const std::string& profile
    ) const;

    VpnActionResult connect(
        const std::string& profile
    ) const;

    VpnActionResult disconnect(
        const std::string& profile
    ) const;

private:
    class Impl;
    Impl* impl_{nullptr};
};

}
