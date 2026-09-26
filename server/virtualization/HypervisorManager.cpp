#include "server/virtualization/HypervisorManager.h"

#include <dlfcn.h>
#include <fcntl.h>
#include <sys/stat.h>
#include <unistd.h>

#include <algorithm>
#include <array>
#include <cstdlib>
#include <filesystem>
#include <fstream>
#include <memory>
#include <sstream>
#include <utility>

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
        return true;
    }

    void unload()
    {
        libvirt_loaded = false;
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

}
