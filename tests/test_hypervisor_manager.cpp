#include "server/virtualization/HypervisorManager.h"

#include <iostream>
#include <string>

int main()
{
    using homeai::HypervisorManager;

    const std::string uuid = "11111111-2222-3333-4444-555555555555";
    if (!HypervisorManager::validUuid(uuid) || HypervisorManager::validUuid("../vm") ||
        HypervisorManager::validUuid(uuid + "0") ||
        HypervisorManager::allowedActions("unknown").size() != 0 ||
        HypervisorManager::allowedActions("shutoff") != std::vector<std::string>{"start"} ||
        HypervisorManager::allowedActions("paused") != std::vector<std::string>{"resume", "force-off"})
        return 1;
    homeai::VmCreateDraft draft;
    draft.name = "home-ai-test";
    draft.vcpus = "2";
    draft.memory_mib = "4096";
    draft.architecture = "x86_64";
    draft.machine_type = "q35";
    const auto preview = HypervisorManager::previewCreate(draft);
    if (!preview.success || preview.code != "preview_ready" || preview.vcpus != 2 ||
        preview.memory_bytes != 4096ULL * 1024ULL * 1024ULL ||
        preview.xml.find("<domain type='kvm'>") == std::string::npos ||
        preview.xml.find("<name>home-ai-test</name>") == std::string::npos ||
        preview.xml.find("machine='q35'") == std::string::npos)
        return 1;
    draft.name = "<bad>";
    if (HypervisorManager::previewCreate(draft).code != "invalid_name") return 1;
    draft.name = "valid"; draft.vcpus = "0";
    if (HypervisorManager::previewCreate(draft).code != "invalid_vcpus") return 1;
    draft.vcpus = "2"; draft.memory_mib = "255";
    if (HypervisorManager::previewCreate(draft).code != "invalid_memory") return 1;
    draft.memory_mib = "4096"; draft.architecture = "aarch64"; draft.machine_type = "q35";
    if (HypervisorManager::previewCreate(draft).code != "invalid_machine_type") return 1;

    HypervisorManager unavailable;
    if (unavailable.performAction(uuid, "start", "shutoff", "").code != "confirmation_required" ||
        unavailable.performAction(uuid, "delete", "shutoff", uuid).code != "unsupported_action" ||
        unavailable.performAction(uuid, "start", "", uuid).code != "expected_state_required" ||
        unavailable.performAction(uuid, "start", "shutoff", uuid).code != "unavailable")
        return 1;

    const auto capabilities =
        HypervisorManager::capabilities();

    bool disk_preview = false;
    bool disks = false;

    for (const auto& capability : capabilities) {
        if (capability.name == "disk_preview")
            disk_preview = capability.implemented;

        if (capability.name == "disks")
            disks = capability.implemented;
    }

    if (!disk_preview || disks)
        return 1;

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

    if (
        !snapshot.host.libvirt_available
        &&
        !manager.healthy()
    ) {
        std::cerr
            << "Missing optional libvirt must not degrade Hypervisor Core\n";

        return 1;
    }

    if (
        snapshot.host.libvirt_available
        &&
        snapshot.host.libvirt_connected
        &&
        !manager.healthy()
    ) {
        std::cerr
            << "Connected libvirt must report healthy Hypervisor Core\n";

        return 1;
    }

    if (
        snapshot.host.libvirt_available
        &&
        !snapshot.host.libvirt_connected
        &&
        manager.healthy()
    ) {
        std::cerr
            << "Installed but unreachable libvirt must degrade Hypervisor Core\n";

        return 1;
    }

    manager.shutdown();

    std::cout
        << "Hypervisor manager test passed\n";

    return 0;
}
