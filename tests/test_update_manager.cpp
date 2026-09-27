#include "server/update/UpdateManager.h"
#include "server/update/BuildActivator.h"

#include <filesystem>
#include <fstream>

#include <iostream>

int main()
{
    using homeai::UpdateManager;
    using homeai::UpdateState;

    if (
        UpdateManager::stateToString(
            UpdateState::Idle
        ) != "idle"
        ||
        UpdateManager::stateToString(
            UpdateState::UpdateAvailable
        ) != "update_available"
        ||
        UpdateManager::stateToString(
            UpdateState::ReadyToRestart
        ) != "ready_to_restart"
        ||
        UpdateManager::stateToString(
            UpdateState::Error
        ) != "error"
    ) {
        std::cerr
            << "Update state mapping failed\n";

        return 1;
    }

    homeai::UpdateManager manager;

    const auto status =
        manager.status();

    if (
        status.progress_percent != 0
        ||
        status.progress_stage != "idle"
    ) {
        std::cerr
            << "Update progress defaults are invalid\n";

        return 1;
    }


    const auto temporary =
        std::filesystem::temp_directory_path()
        /
        "home-ai-build-activation-test";

    std::filesystem::remove_all(
        temporary
    );

    std::filesystem::create_directories(
        temporary / "build"
    );

    {
        std::ofstream marker(
            temporary /
            "build" /
            "old.txt"
        );
        marker << "old";
    }

    const auto first_name =
        homeai::BuildActivator::
            releaseDirectoryName(
                "0123456789abcdef"
            );

    if (
        first_name !=
            "build-release-0123456789ab"
    ) {
        std::cerr
            << "Release build name is invalid\n";
        return 1;
    }

    const auto first_release =
        temporary /
        first_name;

    std::filesystem::create_directories(
        first_release
    );

    {
        std::ofstream binary(
            first_release /
            "home-ai-core"
        );
        binary << "first";
    }

    std::string activation_error;

    if (
        !homeai::BuildActivator::activate(
            temporary,
            first_release,
            activation_error
        )
    ) {
        std::cerr
            << "First build activation failed: "
            << activation_error
            << '\n';
        return 1;
    }

    if (
        !std::filesystem::is_symlink(
            temporary / "build"
        )
        ||
        !std::filesystem::exists(
            temporary /
            "build" /
            "home-ai-core"
        )
        ||
        !std::filesystem::exists(
            temporary /
            "build-prev" /
            "old.txt"
        )
    ) {
        std::cerr
            << "First build activation layout is invalid\n";
        return 1;
    }

    const auto second_name =
        homeai::BuildActivator::
            releaseDirectoryName(
                "fedcba9876543210"
            );

    const auto second_release =
        temporary /
        second_name;

    std::filesystem::create_directories(
        second_release
    );

    {
        std::ofstream binary(
            second_release /
            "home-ai-core"
        );
        binary << "second";
    }

    if (
        !homeai::BuildActivator::activate(
            temporary,
            second_release,
            activation_error
        )
    ) {
        std::cerr
            << "Second build activation failed: "
            << activation_error
            << '\n';
        return 1;
    }

    if (
        !std::filesystem::is_symlink(
            temporary / "build"
        )
        ||
        !std::filesystem::is_symlink(
            temporary / "build-prev"
        )
        ||
        std::filesystem::read_symlink(
            temporary / "build"
        ) !=
            second_release.filename()
        ||
        std::filesystem::read_symlink(
            temporary / "build-prev"
        ) !=
            first_release.filename()
        ||
        !std::filesystem::exists(
            temporary /
            "build" /
            "home-ai-core"
        )
        ||
        !std::filesystem::exists(
            temporary /
            "build-prev" /
            "home-ai-core"
        )
    ) {
        std::cerr
            << "Second build activation layout is invalid\n";
        return 1;
    }

    std::filesystem::remove_all(
        temporary
    );

    std::cout
        << "Update Manager test passed\n";

    return 0;
}
