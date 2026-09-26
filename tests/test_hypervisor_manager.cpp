#include "server/virtualization/HypervisorManager.h"

#include <iostream>
#include <string>

int main()
{
    using homeai::HypervisorManager;

    if (
        HypervisorManager::versionToString(
            1002003UL
        ) != "1.2.3"
        ||
        HypervisorManager::versionToString(
            10000000UL
        ) != "10.0.0"
    ) {
        std::cerr
            << "Hypervisor version formatting failed\n";

        return 1;
    }

    if (
        HypervisorManager::domainStateLabel(
            1
        ) != "running"
        ||
        HypervisorManager::domainStateLabel(
            3
        ) != "paused"
        ||
        HypervisorManager::domainStateLabel(
            5
        ) != "shutoff"
        ||
        HypervisorManager::domainStateLabel(
            6
        ) != "crashed"
        ||
        HypervisorManager::domainStateLabel(
            255
        ) != "unknown"
    ) {
        std::cerr
            << "Hypervisor domain-state mapping failed\n";

        return 1;
    }

    HypervisorManager manager;
    std::string error;

    if (!manager.initialize(error)) {
        std::cerr
            << "Hypervisor optional initialization failed: "
            << error
            << '\n';

        return 1;
    }

    // The test environment is intentionally not required to
    // provide KVM/libvirt. Snapshot must remain safe either way.
    auto snapshot =
        manager.snapshot(error);

    (void)snapshot;

    manager.shutdown();

    std::cout
        << "Hypervisor manager test passed\n";

    return 0;
}
