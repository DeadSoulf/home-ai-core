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

class WireGuardManager {
public:
    bool helperInstalled() const;

    std::vector<WireGuardProfile>
    profiles(std::string& error) const;

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

    CommandResult runHelper(
        const std::vector<std::string>& arguments
    ) const;
};

}
