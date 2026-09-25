#pragma once

#include <string>
#include <vector>

namespace homeai {

struct WireGuardProfile {
    std::string name;
    bool active{false};
};

struct WireGuardResult {
    bool success{false};
    std::string code;
    std::string message;
};

struct WireGuardConfigResult {
    bool success{false};
    std::string code;
    std::string message;
    std::string config;
};

class WireGuardManager {
public:
    bool initialize(
        const std::string& config_directory
    );

    bool available() const;

    std::vector<WireGuardProfile>
    profiles(
        std::string& error
    ) const;

    WireGuardConfigResult loadProfile(
        const std::string& profile
    ) const;

    WireGuardResult saveProfile(
        const std::string& profile,
        const std::string& config
    ) const;

    WireGuardResult removeProfile(
        const std::string& profile
    ) const;

    WireGuardResult connect(
        const std::string& profile
    ) const;

    WireGuardResult disconnect(
        const std::string& profile
    ) const;

    static bool validProfileName(
        const std::string& profile
    );

private:
    struct CommandResult {
        int exit_code{-1};
        std::string output;
    };

    CommandResult runWgQuick(
        const std::string& action,
        const std::string& profile
    ) const;

    std::string profilePath(
        const std::string& profile
    ) const;

    std::string config_directory_;
};

}
