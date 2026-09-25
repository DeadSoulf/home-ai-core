#pragma once
#include <filesystem>
#include <string>
#include <vector>
#include <utility>

namespace homeai {
struct GpuInfo {
    std::string pci_address, vendor_id, device_id, vendor, driver;
};
struct GpuSnapshot {
    bool available{false};
    std::vector<GpuInfo> devices;
};
// Read-only inventory. A display-class PCI device is not proof of AI compatibility.
class GpuMonitor {
public:
    explicit GpuMonitor(std::filesystem::path root = "/sys/bus/pci/devices") : root_(std::move(root)) {}
    GpuSnapshot snapshot() const;
    static bool validAddress(const std::string& address);
private:
    std::filesystem::path root_;
};
}
