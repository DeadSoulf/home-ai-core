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

    std::cout
        << "Update Manager test passed\n";

    return 0;
}
