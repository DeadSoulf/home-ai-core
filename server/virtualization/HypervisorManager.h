#pragma once

#include <cstdint>
#include <mutex>
#include <string>
#include <vector>

namespace homeai {

struct HypervisorHostInfo {
    bool kvm_present{false};
    bool kvm_accessible{false};
    bool hardware_virtualization{false};
    bool qemu_available{false};
    bool libvirt_available{false};
    bool libvirt_connected{false};

    std::string connection_uri;
    std::string libvirt_version;
    std::string hypervisor_version;
    std::string cpu_model;

    std::uint64_t memory_bytes{0};
    unsigned int cpus{0};
    unsigned int mhz{0};
    unsigned int nodes{0};
    unsigned int sockets{0};
    unsigned int cores{0};
    unsigned int threads{0};

    std::string message;
};

struct VirtualMachineInfo {
    std::string name;
    std::string uuid;
    std::string state;
    bool active{false};
    bool autostart{false};

    unsigned int vcpus{0};
    std::uint64_t memory_bytes{0};
    std::uint64_t max_memory_bytes{0};
    std::uint64_t cpu_time_ns{0};
};

struct HypervisorSnapshot {
    HypervisorHostInfo host;
    std::vector<VirtualMachineInfo> machines;
};

class HypervisorManager {
public:
    HypervisorManager();
    ~HypervisorManager();

    HypervisorManager(
        const HypervisorManager&
    ) = delete;

    HypervisorManager& operator=(
        const HypervisorManager&
    ) = delete;

    bool initialize(
        std::string& error
    );

    void shutdown();

    HypervisorSnapshot snapshot(
        std::string& error
    );

    bool healthy() const;

    std::string healthMessage() const;

    static std::string versionToString(
        unsigned long version
    );

    static std::string domainStateLabel(
        unsigned char state
    );

private:
    struct Impl;
    Impl* impl_;
};

}
