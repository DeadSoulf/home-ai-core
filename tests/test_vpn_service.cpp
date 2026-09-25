#include "server/network/VpnService.h"

#include <filesystem>
#include <iostream>
#include <string>
#include <sys/stat.h>
#include <unistd.h>

int main()
{
    using namespace homeai;

    const auto directory =
        std::filesystem::temp_directory_path()
        /
        (
            "home-ai-vpn-test-"
            +
            std::to_string(
                static_cast<long long>(
                    ::getpid()
                )
            )
        );

    std::error_code error;

    std::filesystem::remove_all(
        directory,
        error
    );

    VpnService vpn;

    if (
        !vpn.initialize(
            directory.string()
        )
    ) {
        std::cerr
            << "Unable to initialize VPN service test directory\n";

        return 1;
    }

    const std::string profile =
        "homeaitest_"
        +
        std::to_string(
            static_cast<long long>(
                ::getpid()
            )
        );

    const std::string config =
        "[Interface]\n"
        "PrivateKey = ABC+DEF=\n"
        "Address = 10.99.0.2/32\n"
        "\n"
        "[Peer]\n"
        "PublicKey = XYZ+123=\n"
        "AllowedIPs = 10.99.0.0/24\n"
        "Endpoint = 192.0.2.10:51820\n";

    const auto invalid_profile =
        vpn.saveProfile(
            "../bad",
            config
        );

    if (invalid_profile.success) {
        std::cerr
            << "Invalid WireGuard profile name was accepted\n";

        std::filesystem::remove_all(
            directory,
            error
        );

        return 1;
    }

    const auto invalid_config =
        vpn.saveProfile(
            profile,
            "[Peer]\nPublicKey = invalid\n"
        );

    if (invalid_config.success) {
        std::cerr
            << "Invalid WireGuard configuration was accepted\n";

        std::filesystem::remove_all(
            directory,
            error
        );

        return 1;
    }

    const auto saved =
        vpn.saveProfile(
            profile,
            config
        );

    if (!saved.success) {
        std::cerr
            << "Unable to save WireGuard profile: "
            << saved.message
            << '\n';

        std::filesystem::remove_all(
            directory,
            error
        );

        return 1;
    }

    const auto path =
        directory
        /
        (
            profile
            +
            ".conf"
        );

    struct stat file_stat {};

    if (
        ::stat(
            path.c_str(),
            &file_stat
        ) != 0
        ||
        (
            file_stat.st_mode
            &
            0777
        ) != 0600
    ) {
        std::cerr
            << "WireGuard profile permissions are not 0600\n";

        std::filesystem::remove_all(
            directory,
            error
        );

        return 1;
    }

    const auto loaded =
        vpn.loadProfile(
            profile
        );

    if (
        !loaded.success
        ||
        loaded.config != config
    ) {
        std::cerr
            << "WireGuard profile content did not round-trip\n";

        std::filesystem::remove_all(
            directory,
            error
        );

        return 1;
    }

    const auto removed =
        vpn.removeProfile(
            profile
        );

    if (!removed.success) {
        std::cerr
            << "Unable to remove WireGuard profile: "
            << removed.message
            << '\n';

        std::filesystem::remove_all(
            directory,
            error
        );

        return 1;
    }

    if (
        vpn.loadProfile(
            profile
        ).success
    ) {
        std::cerr
            << "Removed WireGuard profile is still readable\n";

        std::filesystem::remove_all(
            directory,
            error
        );

        return 1;
    }

    std::filesystem::remove_all(
        directory,
        error
    );

    std::cout
        << "VPN service test passed\n";

    return 0;
}
