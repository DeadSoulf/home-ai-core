#include "server/update/UpdateManager.h"

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

    std::cout
        << "Update Manager test passed\n";

    return 0;
}
