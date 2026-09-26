#include "server/virtualization/HypervisorManager.h"

#include <dlfcn.h>
#include <fcntl.h>
#include <sys/stat.h>
#include <unistd.h>

#include <algorithm>
#include <array>
#include <charconv>
#include <cstdlib>
#include <filesystem>
#include <fstream>
#include <memory>
#include <sstream>
#include <string_view>
#include <utility>
#include <chrono>
#include <map>

namespace homeai {

namespace {

struct _virConnect;
struct _virDomain;

using virConnectPtr =
    _virConnect*;
using virDomainPtr =
    _virDomain*;

constexpr int vir_uuid_string_buflen = 37;

struct VirNodeInfo {
    char model[32];
    unsigned long memory;
    unsigned int cpus;
    unsigned int mhz;
    unsigned int nodes;
    unsigned int sockets;
    unsigned int cores;
    unsigned int threads;
};

struct VirDomainInfo {
    unsigned char state;
    unsigned long maxMem;
    unsigned long memory;
    unsigned short nrVirtCpu;
    unsigned long long cpuTime;
};

using VirInitialize =
    int (*)();

using VirConnectOpenReadOnly =
    virConnectPtr (*)(
        const char*
    );

using VirConnectClose =
    int (*)(
        virConnectPtr
    );

using VirConnectGetLibVersion =
    int (*)(
        virConnectPtr,
        unsigned long*
    );

using VirConnectGetVersion =
    int (*)(
        virConnectPtr,
        unsigned long*
    );

using VirConnectGetURI =
    char* (*)(
        virConnectPtr
    );

using VirNodeGetInfo =
    int (*)(
        virConnectPtr,
        VirNodeInfo*
    );

using VirConnectListAllDomains =
    int (*)(
        virConnectPtr,
        virDomainPtr**,
        unsigned int
    );

using VirDomainGetName =
    const char* (*)(
        virDomainPtr
    );

using VirDomainGetUUIDString =
    int (*)(
        virDomainPtr,
        char*
    );

using VirDomainGetInfo =
    int (*)(
        virDomainPtr,
        VirDomainInfo*
    );

using VirDomainGetAutostart =
    int (*)(
        virDomainPtr,
        int*
    );

using VirDomainIsActive =
    int (*)(
        virDomainPtr
    );

using VirDomainFree =
    int (*)(
        virDomainPtr
    );

bool fileExistsExecutable(
    const std::filesystem::path& path
)
{
    std::error_code error;

    const auto status =
        std::filesystem::status(
            path,
            error
        );

    if (
        error
        ||
        !std::filesystem::
            is_regular_file(
                status
            )
    ) {
        return false;
    }

    return
        ::access(
            path.c_str(),
            X_OK
        ) == 0;
}

bool qemuInstalled()
{
    static const std::array<
        const char*,
        7
    > candidates = {
        "/usr/bin/qemu-system-x86_64",
        "/usr/bin/qemu-system-aarch64",
        "/usr/bin/qemu-kvm",
        "/usr/libexec/qemu-kvm",
        "/usr/lib/qemu/qemu-system-x86_64",
        "/usr/libexec/qemu-system-x86_64",
        "/usr/local/bin/qemu-system-x86_64"
    };

    return
        std::any_of(
            candidates.begin(),
            candidates.end(),
            [](const char* path) {
                return
                    fileExistsExecutable(
                        path
                    );
            }
        );
}

bool cpuVirtualizationAvailable()
{
    std::ifstream input(
        "/proc/cpuinfo"
    );

    if (!input)
        return false;

    std::string line;

    while (
        std::getline(
            input,
            line
        )
    ) {
        if (
            line.rfind(
                "flags",
                0
            ) != 0
            &&
            line.rfind(
                "Features",
                0
            ) != 0
        ) {
            continue;
        }

        std::istringstream stream(
            line
        );

        std::string token;

        while (stream >> token) {
            if (
                token == "vmx"
                ||
                token == "svm"
            ) {
                return true;
            }
        }
    }

    return false;
}

bool parseUnsignedRange(std::string_view text, unsigned long long minimum,
    unsigned long long maximum, unsigned long long& value)
{
    if (text.empty()) return false;
    unsigned long long parsed = 0;
    const auto* begin = text.data();
    const auto* end = begin + text.size();
    const auto result = std::from_chars(begin, end, parsed);
    if (result.ec != std::errc{} || result.ptr != end ||
        parsed < minimum || parsed > maximum)
        return false;
    value = parsed;
    return true;
}

bool validVmName(std::string_view name)
{
    if (name.empty() || name.size() > 63) return false;
    const auto alnum = [](char value) {
        return (value >= 'a' && value <= 'z') ||
            (value >= 'A' && value <= 'Z') ||
            (value >= '0' && value <= '9');
    };
    if (!alnum(name.front())) return false;
    for (const char value : name) {
        if (!alnum(value) && value != '-' && value != '_' && value != '.')
            return false;
    }
    return true;
}

std::string xmlEscape(std::string_view value)
{
    std::string output;
    output.reserve(value.size() + 16);
    for (const char character : value) {
        switch (character) {
        case '&': output += "&amp;"; break;
        case '<': output += "&lt;"; break;
        case '>': output += "&gt;"; break;
        case '\'': output += "&apos;"; break;
        case '"': output += "&quot;"; break;
        default: output += character; break;
        }
    }
    return output;
}

}

struct HypervisorManager::Impl {
    mutable std::mutex mutex;

    void* library{nullptr};

    VirInitialize virInitialize{nullptr};
    VirConnectOpenReadOnly
        virConnectOpenReadOnly{nullptr};
    VirConnectClose
        virConnectClose{nullptr};
    VirConnectGetLibVersion
        virConnectGetLibVersion{nullptr};
    VirConnectGetVersion
        virConnectGetVersion{nullptr};
    VirConnectGetURI
        virConnectGetURI{nullptr};
    VirNodeGetInfo
        virNodeGetInfo{nullptr};
    VirConnectListAllDomains
        virConnectListAllDomains{nullptr};
    VirDomainGetName
        virDomainGetName{nullptr};
    VirDomainGetUUIDString
        virDomainGetUUIDString{nullptr};
    VirDomainGetInfo
        virDomainGetInfo{nullptr};
    VirDomainGetAutostart
        virDomainGetAutostart{nullptr};
    VirDomainIsActive
        virDomainIsActive{nullptr};
    VirDomainFree
        virDomainFree{nullptr};

    bool initialized{false};
    VirConnectOpenReadOnly virConnectOpen{nullptr};
    virDomainPtr (*virDomainLookupByUUIDString)(virConnectPtr, const char*){nullptr};
    int (*virDomainCreate)(virDomainPtr){nullptr};
    int (*virDomainShutdown)(virDomainPtr){nullptr};
    int (*virDomainReboot)(virDomainPtr, unsigned int){nullptr};
    int (*virDomainDestroy)(virDomainPtr){nullptr};
    int (*virDomainSuspend)(virDomainPtr){nullptr};
    int (*virDomainResume)(virDomainPtr){nullptr};
    int (*virDomainSetAutostart)(virDomainPtr, int){nullptr};
    int (*virGetLastErrorCode)(){nullptr};
    const char* (*virGetLastErrorMessage)(){nullptr};
    bool lifecycle_loaded{false};
    bool extended_lifecycle_loaded{false};
    struct PendingAction {
        std::string action;
        std::chrono::steady_clock::time_point deadline;
    };
    std::map<std::string, PendingAction> pending;

    VmActionResult failure(const std::string& fallback) const
    {
        const int code = virGetLastErrorCode ? virGetLastErrorCode() : 0;
        const char* detail = virGetLastErrorMessage ? virGetLastErrorMessage() : nullptr;
        return {false, code == 42 ? "not_found" :
            (code == 29 || code == 45 || code == 79 || code == 88 || code == 94) ? "permission_denied" :
            code == 55 ? "invalid_state" : "backend_error",
            detail ? std::string(detail) : fallback, "unknown"};
    }
    bool libvirt_loaded{false};
    bool last_connected{false};
    std::string last_message;

    template<typename T>
    bool symbol(
        const char* name,
        T& target
    )
    {
        target =
            reinterpret_cast<T>(
                ::dlsym(
                    library,
                    name
                )
            );

        return target != nullptr;
    }

    bool loadLibrary(
        std::string& error
    )
    {
        static const std::array<
            const char*,
            3
        > candidates = {
            "libvirt.so.0",
            "libvirt.so",
            "/usr/lib/x86_64-linux-gnu/libvirt.so.0"
        };

        for (
            const auto* candidate :
            candidates
        ) {
            library =
                ::dlopen(
                    candidate,
                    RTLD_NOW |
                    RTLD_LOCAL
                );

            if (library)
                break;
        }

        if (!library) {
            error =
                "libvirt is not installed. Hypervisor Core is running in detection-only mode.";

            return false;
        }

        const bool complete =
            symbol(
                "virInitialize",
                virInitialize
            )
            &&
            symbol(
                "virConnectOpenReadOnly",
                virConnectOpenReadOnly
            )
            &&
            symbol(
                "virConnectClose",
                virConnectClose
            )
            &&
            symbol(
                "virConnectGetLibVersion",
                virConnectGetLibVersion
            )
            &&
            symbol(
                "virConnectGetVersion",
                virConnectGetVersion
            )
            &&
            symbol(
                "virConnectGetURI",
                virConnectGetURI
            )
            &&
            symbol(
                "virNodeGetInfo",
                virNodeGetInfo
            )
            &&
            symbol(
                "virConnectListAllDomains",
                virConnectListAllDomains
            )
            &&
            symbol(
                "virDomainGetName",
                virDomainGetName
            )
            &&
            symbol(
                "virDomainGetUUIDString",
                virDomainGetUUIDString
            )
            &&
            symbol(
                "virDomainGetInfo",
                virDomainGetInfo
            )
            &&
            symbol(
                "virDomainGetAutostart",
                virDomainGetAutostart
            )
            &&
            symbol(
                "virDomainIsActive",
                virDomainIsActive
            )
            &&
            symbol(
                "virDomainFree",
                virDomainFree
            );

        if (!complete) {
            error =
                "libvirt runtime is missing required API symbols.";

            ::dlclose(
                library
            );

            library = nullptr;

            return false;
        }

        if (
            virInitialize
            &&
            virInitialize() < 0
        ) {
            error =
                "libvirt initialization failed.";

            ::dlclose(
                library
            );

            library = nullptr;

            return false;
        }

        libvirt_loaded = true;
        // Optional mutation symbols must never break read-only inventory.
        lifecycle_loaded =
            symbol("virConnectOpen", virConnectOpen) &&
            symbol("virDomainLookupByUUIDString", virDomainLookupByUUIDString) &&
            symbol("virDomainCreate", virDomainCreate) &&
            symbol("virDomainShutdown", virDomainShutdown) &&
            symbol("virDomainReboot", virDomainReboot) &&
            symbol("virDomainDestroy", virDomainDestroy) &&
            symbol("virGetLastErrorCode", virGetLastErrorCode) &&
            symbol("virGetLastErrorMessage", virGetLastErrorMessage);

        // Optional lifecycle extensions should not disable the already-supported
        // start/shutdown/reboot/force-off controls on older libvirt runtimes.
        extended_lifecycle_loaded =
            symbol("virDomainSuspend", virDomainSuspend) &&
            symbol("virDomainResume", virDomainResume) &&
            symbol("virDomainSetAutostart", virDomainSetAutostart);
        return true;
    }

    void unload()
    {
        libvirt_loaded = false;
        lifecycle_loaded = false;
        extended_lifecycle_loaded = false;
        pending.clear();
        last_connected = false;

        if (library) {
            ::dlclose(
                library
            );

            library = nullptr;
        }
    }

    HypervisorSnapshot
    collect(
        std::string& error
    )
    {
        HypervisorSnapshot result;
        error.clear();

        last_connected = false;

        result.host.kvm_present =
            std::filesystem::exists(
                "/dev/kvm"
            );

        result.host.kvm_accessible =
            ::access(
                "/dev/kvm",
                R_OK |
                W_OK
            ) == 0;

        result.host.
            hardware_virtualization =
                cpuVirtualizationAvailable();

        result.host.qemu_available =
            qemuInstalled();

        result.host.libvirt_available =
            libvirt_loaded;

        if (!libvirt_loaded) {
            result.host.message =
                "libvirt is not installed. Hypervisor Core is running in detection-only mode.";

            return result;
        }

        virConnectPtr connection =
            virConnectOpenReadOnly(
                "qemu:///system"
            );

        if (!connection) {
            result.host.message =
                "Unable to connect to qemu:///system. Check libvirt daemon/socket permissions.";

            error =
                result.host.message;

            return result;
        }

        result.host.libvirt_connected =
            true;

        last_connected = true;

        unsigned long version = 0;

        if (
            virConnectGetLibVersion(
                connection,
                &version
            ) == 0
        ) {
            result.host.libvirt_version =
                HypervisorManager::
                    versionToString(
                        version
                    );
        }

        version = 0;

        if (
            virConnectGetVersion(
                connection,
                &version
            ) == 0
        ) {
            result.host.hypervisor_version =
                HypervisorManager::
                    versionToString(
                        version
                    );
        }

        if (virConnectGetURI) {
            char* uri =
                virConnectGetURI(
                    connection
                );

            if (uri) {
                result.host.connection_uri =
                    uri;

                std::free(uri);
            }
        }

        VirNodeInfo node{};

        if (
            virNodeGetInfo(
                connection,
                &node
            ) == 0
        ) {
            result.host.cpu_model =
                node.model;

            result.host.memory_bytes =
                static_cast<
                    std::uint64_t
                >(
                    node.memory
                )
                *
                1024ULL;

            result.host.cpus =
                node.cpus;
            result.host.mhz =
                node.mhz;
            result.host.nodes =
                node.nodes;
            result.host.sockets =
                node.sockets;
            result.host.cores =
                node.cores;
            result.host.threads =
                node.threads;
        }

        virDomainPtr* domains =
            nullptr;

        const int count =
            virConnectListAllDomains(
                connection,
                &domains,
                0
            );

        if (count < 0) {
            last_connected = false;
            error =
                "Unable to list libvirt virtual machines.";

            result.host.message =
                error;

            virConnectClose(
                connection
            );

            return result;
        }

        result.machines.reserve(
            static_cast<
                std::size_t
            >(
                count
            )
        );

        for (
            int index = 0;
            index < count;
            ++index
        ) {
            virDomainPtr domain =
                domains[index];

            if (!domain)
                continue;

            VirtualMachineInfo machine;

            const char* name =
                virDomainGetName(
                    domain
                );

            if (name)
                machine.name = name;

            std::array<
                char,
                vir_uuid_string_buflen
            > uuid{};

            if (
                virDomainGetUUIDString(
                    domain,
                    uuid.data()
                ) == 0
            ) {
                machine.uuid =
                    uuid.data();
            }

            VirDomainInfo info{};

            if (
                virDomainGetInfo(
                    domain,
                    &info
                ) == 0
            ) {
                machine.state =
                    HypervisorManager::
                        domainStateLabel(
                            info.state
                        );

                const int active =
                    virDomainIsActive(
                        domain
                    );

                machine.active =
                    active > 0;

                machine.vcpus =
                    info.nrVirtCpu;

                machine.memory_bytes =
                    static_cast<
                        std::uint64_t
                    >(
                        info.memory
                    )
                    *
                    1024ULL;

                machine.max_memory_bytes =
                    static_cast<
                        std::uint64_t
                    >(
                        info.maxMem
                    )
                    *
                    1024ULL;

                machine.cpu_time_ns =
                    info.cpuTime;
                if (active >= 0 && lifecycle_loaded && HypervisorManager::validUuid(machine.uuid)) {
                    machine.allowed_actions = HypervisorManager::allowedActions(machine.state);
                    if (!extended_lifecycle_loaded) {
                        std::erase(machine.allowed_actions, "pause");
                        std::erase(machine.allowed_actions, "resume");
                    }
                }
                if (active < 0) machine.error = "Unable to read VM activity.";
                if (machine.state == "shutoff") pending.erase(machine.uuid);
                const auto request = pending.find(machine.uuid);
                if (request != pending.end() && request->second.deadline > std::chrono::steady_clock::now()) {
                    machine.pending_action = request->second.action;
                    std::erase_if(machine.allowed_actions, [](const auto& action) { return action != "force-off"; });
                }
            } else {
                machine.error = "Unable to read VM state.";
            }

            int autostart = 0;

            if (
                virDomainGetAutostart(
                    domain,
                    &autostart
                ) == 0
            ) {
                machine.autostart =
                    autostart != 0;

                if (
                    lifecycle_loaded
                    && extended_lifecycle_loaded
                    && HypervisorManager::validUuid(machine.uuid)
                    && machine.pending_action.empty()
                    && machine.error.empty()
                ) {
                    machine.allowed_actions.push_back(
                        machine.autostart
                            ? "autostart-off"
                            : "autostart-on"
                    );
                }
            }

            result.machines.push_back(
                std::move(
                    machine
                )
            );

            virDomainFree(
                domain
            );
        }

        if (domains)
            std::free(domains);

        virConnectClose(
            connection
        );

        result.host.message =
            "KVM/QEMU/libvirt inventory is available.";

        return result;
    }
};

HypervisorManager::HypervisorManager()
    : impl_(
        new Impl()
    )
{
}

HypervisorManager::~HypervisorManager()
{
    shutdown();

    delete impl_;
    impl_ = nullptr;
}

bool HypervisorManager::initialize(
    std::string& error
)
{
    std::lock_guard<std::mutex>
        lock(
            impl_->mutex
        );

    if (impl_->initialized)
        return true;

    std::string load_error;

    impl_->loadLibrary(
        load_error
    );

    impl_->initialized = true;

    std::string snapshot_error;

    const auto current =
        impl_->collect(
            snapshot_error
        );

    if (
        current.host.libvirt_connected
    ) {
        impl_->last_message =
            current.host.message;
    }
    else if (!snapshot_error.empty()) {
        impl_->last_message =
            snapshot_error;
    }
    else if (!load_error.empty()) {
        impl_->last_message =
            load_error;
    }
    else {
        impl_->last_message =
            current.host.message;
    }

    // Hypervisor support is optional. Core startup must not fail
    // on hosts without KVM/libvirt.
    error.clear();
    return true;
}

void HypervisorManager::shutdown()
{
    if (!impl_)
        return;

    std::lock_guard<std::mutex>
        lock(
            impl_->mutex
        );

    if (!impl_->initialized)
        return;

    impl_->unload();
    impl_->initialized = false;
    impl_->last_message =
        "Hypervisor Core stopped.";
}

HypervisorSnapshot
HypervisorManager::snapshot(
    std::string& error
)
{
    std::lock_guard<std::mutex>
        lock(
            impl_->mutex
        );

    if (!impl_->initialized) {
        error =
            "Hypervisor Core is not initialized.";

        return {};
    }

    auto result =
        impl_->collect(
            error
        );

    impl_->last_message =
        !error.empty()
        ? error
        : result.host.message;

    return result;
}

bool HypervisorManager::healthy() const
{
    if (!impl_)
        return false;

    std::lock_guard<std::mutex>
        lock(
            impl_->mutex
        );

    if (!impl_->initialized)
        return false;

    // libvirt is an optional runtime dependency. Its complete
    // absence means virtualization management is not configured
    // yet, but Hypervisor Core itself remains healthy and can
    // still report KVM/QEMU capability. If libvirt is present,
    // failure to connect to qemu:///system is actionable.
    if (!impl_->libvirt_loaded)
        return true;

    return impl_->last_connected;
}

std::string
HypervisorManager::healthMessage() const
{
    if (!impl_)
        return
            "Hypervisor Core is unavailable.";

    std::lock_guard<std::mutex>
        lock(
            impl_->mutex
        );

    return impl_->last_message;
}

std::string
HypervisorManager::versionToString(
    unsigned long version
)
{
    const auto major =
        version / 1000000UL;

    const auto minor =
        (
            version /
            1000UL
        )
        %
        1000UL;

    const auto release =
        version %
        1000UL;

    return
        std::to_string(
            major
        )
        + "."
        + std::to_string(
            minor
        )
        + "."
        + std::to_string(
            release
        );
}

std::string
HypervisorManager::domainStateLabel(
    unsigned char state
)
{
    switch (state) {
    case 0:
        return "no-state";
    case 1:
        return "running";
    case 2:
        return "blocked";
    case 3:
        return "paused";
    case 4:
        return "shutdown";
    case 5:
        return "shutoff";
    case 6:
        return "crashed";
    case 7:
        return "suspended";
    default:
        return "unknown";
    }
}

bool HypervisorManager::validUuid(const std::string& uuid)
{
    if (uuid.size() != 36) return false;
    for (std::size_t i = 0; i < uuid.size(); ++i) {
        const char c = uuid[i];
        if (i == 8 || i == 13 || i == 18 || i == 23) {
            if (c != '-') return false;
        } else if (!((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f'))) {
            return false;
        }
    }
    return true;
}

std::vector<std::string> HypervisorManager::allowedActions(const std::string& state)
{
    if (state == "shutoff") return {"start"};
    if (state == "running" || state == "blocked")
        return {"shutdown", "reboot", "pause", "force-off"};
    if (state == "paused") return {"resume", "force-off"};
    if (state == "shutdown" || state == "crashed" || state == "suspended")
        return {"force-off"};
    return {};
}

VmCreatePreviewResult HypervisorManager::previewCreate(const VmCreateDraft& draft)
{
    VmCreatePreviewResult result;
    if (!validVmName(draft.name)) {
        result.code = "invalid_name";
        result.message = "VM name must be 1-63 ASCII characters, start with a letter or digit, and contain only letters, digits, '.', '_' or '-'.";
        return result;
    }
    unsigned long long vcpus = 0;
    if (!parseUnsignedRange(draft.vcpus, 1, 256, vcpus)) {
        result.code = "invalid_vcpus";
        result.message = "vCPU count must be an integer from 1 to 256.";
        return result;
    }
    unsigned long long memory_mib = 0;
    if (!parseUnsignedRange(draft.memory_mib, 256, 1048576, memory_mib)) {
        result.code = "invalid_memory";
        result.message = "Memory must be an integer from 256 to 1048576 MiB.";
        return result;
    }
    const std::string architecture = draft.architecture.empty() ? "x86_64" : draft.architecture;
    if (architecture != "x86_64" && architecture != "aarch64") {
        result.code = "invalid_architecture";
        result.message = "Architecture must be x86_64 or aarch64.";
        return result;
    }
    const std::string machine = draft.machine_type.empty() ? "auto" : draft.machine_type;
    const bool machine_valid = machine == "auto" ||
        (architecture == "x86_64" && (machine == "q35" || machine == "pc")) ||
        (architecture == "aarch64" && machine == "virt");
    if (!machine_valid) {
        result.code = "invalid_machine_type";
        result.message = "Machine type is incompatible with the selected architecture.";
        return result;
    }

    std::ostringstream xml;
    xml << "<domain type='kvm'>\n"
        << "  <name>" << xmlEscape(draft.name) << "</name>\n"
        << "  <memory unit='MiB'>" << memory_mib << "</memory>\n"
        << "  <currentMemory unit='MiB'>" << memory_mib << "</currentMemory>\n"
        << "  <vcpu placement='static'>" << vcpus << "</vcpu>\n"
        << "  <os>\n"
        << "    <type arch='" << architecture << "'";
    if (machine != "auto") xml << " machine='" << machine << "'";
    xml << ">hvm</type>\n"
        << "    <boot dev='hd'/>\n"
        << "  </os>\n"
        << "  <clock offset='utc'/>\n"
        << "  <on_poweroff>destroy</on_poweroff>\n"
        << "  <on_reboot>restart</on_reboot>\n"
        << "  <on_crash>destroy</on_crash>\n"
        << "</domain>\n";

    result.success = true;
    result.code = "preview_ready";
    result.message = "Validated VM definition preview. No VM, disk or network was created.";
    result.name = draft.name;
    result.architecture = architecture;
    result.machine_type = machine;
    result.vcpus = static_cast<unsigned int>(vcpus);
    result.memory_bytes = static_cast<std::uint64_t>(memory_mib) * 1024ULL * 1024ULL;
    result.xml = xml.str();
    return result;
}

std::vector<HypervisorCapability> HypervisorManager::capabilities()
{
    return {{"lifecycle", true}, {"pause_resume", true}, {"autostart", true},
        {"create_preview", true}, {"create", false}, {"edit", false},
        {"delete", false}, {"snapshots", false}, {"disks", false},
        {"networks", false}, {"console", false}};
}

VmActionResult HypervisorManager::performAction(const std::string& uuid,
    const std::string& action, const std::string& expected_state,
    const std::string& confirmation)
{
    if (!validUuid(uuid)) return {false, "invalid_uuid", "A canonical VM UUID is required."};
    const bool extended_action =
        action == "pause"
        || action == "resume"
        || action == "autostart-on"
        || action == "autostart-off";
    if (
        action != "start"
        && action != "shutdown"
        && action != "reboot"
        && action != "force-off"
        && !extended_action
    )
        return {false, "unsupported_action", "This VM operation is not implemented."};
    if (confirmation != uuid) return {false, "confirmation_required", "Confirm the target VM UUID."};
    if (expected_state.empty()) return {false, "expected_state_required", "Refresh the VM state before confirming."};

    std::lock_guard<std::mutex> lock(impl_->mutex);
    if (!impl_->initialized || !impl_->lifecycle_loaded)
        return {false, "unavailable", "VM lifecycle support is unavailable."};
    if (extended_action && !impl_->extended_lifecycle_loaded)
        return {false, "unavailable", "Extended VM lifecycle support is unavailable."};
    auto* connection = impl_->virConnectOpen("qemu:///system");
    if (!connection) {
        auto result = impl_->failure("Unable to open a writable libvirt connection.");
        if (result.code == "backend_error") result.code = "unavailable";
        return result;
    }
    auto close = [this](auto* value) { impl_->virConnectClose(value); };
    std::unique_ptr<_virConnect, decltype(close)> connection_guard(connection, close);
    auto* domain = impl_->virDomainLookupByUUIDString(connection, uuid.c_str());
    if (!domain) return impl_->failure("Unable to find VM.");
    auto release = [this](auto* value) { impl_->virDomainFree(value); };
    std::unique_ptr<_virDomain, decltype(release)> domain_guard(domain, release);
    VirDomainInfo info{};
    if (impl_->virDomainGetInfo(domain, &info) < 0) return impl_->failure("Unable to read VM state.");
    const auto state = domainStateLabel(info.state);
    if (state != expected_state) return {false, "state_changed", "VM state changed; refresh and confirm again.", state};
    const auto actions = allowedActions(state);
    const bool autostart_action =
        action == "autostart-on"
        || action == "autostart-off";
    if (
        !autostart_action
        && std::find(actions.begin(), actions.end(), action) == actions.end()
    )
        return {false, "invalid_state", "Operation is not allowed in the current VM state.", state};
    const auto now = std::chrono::steady_clock::now();
    for (auto it = impl_->pending.begin(); it != impl_->pending.end();) {
        if (it->second.deadline <= now) it = impl_->pending.erase(it);
        else ++it;
    }
    // A guest can ignore graceful requests. Suppress duplicate submissions for
    // 30 seconds, but permit an explicitly confirmed force-off immediately.
    if (state == "shutoff") impl_->pending.erase(uuid);
    if (action != "force-off" && impl_->pending.contains(uuid))
        return {false, "operation_pending", "A recent request is still pending. Refresh and wait before retrying.", state};
    int rc = -1;
    if (action == "start") rc = impl_->virDomainCreate(domain);
    else if (action == "shutdown") rc = impl_->virDomainShutdown(domain);
    else if (action == "reboot") rc = impl_->virDomainReboot(domain, 0);
    else if (action == "pause") rc = impl_->virDomainSuspend(domain);
    else if (action == "resume") rc = impl_->virDomainResume(domain);
    else if (action == "autostart-on") rc = impl_->virDomainSetAutostart(domain, 1);
    else if (action == "autostart-off") rc = impl_->virDomainSetAutostart(domain, 0);
    else rc = impl_->virDomainDestroy(domain);
    if (rc < 0) return impl_->failure("libvirt rejected the VM operation.");
    if (action == "shutdown" || action == "reboot")
        impl_->pending[uuid] = {action, now + std::chrono::seconds(30)};
    else impl_->pending.erase(uuid);
    // Never claim that a guest has shut down or rebooted just because it
    // accepted the request. This is only an immediate observed state.
    const auto observed = impl_->virDomainGetInfo(domain, &info) == 0
        ? domainStateLabel(info.state) : "unknown";
    return {true, "accepted", "VM request accepted. Refresh to observe the current state.", observed};
}

}
