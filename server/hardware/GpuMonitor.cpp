#include "server/hardware/GpuMonitor.h"
#include <algorithm>
#include <cctype>
#include <fstream>
#include <regex>

namespace homeai {
namespace {
std::string readHex(const std::filesystem::path& path, std::size_t digits) {
    std::ifstream file(path);
    std::string value;
    file >> value;
    if (value.size() != digits + 2 || value.substr(0, 2) != "0x" ||
        !std::all_of(value.begin() + 2, value.end(), [](unsigned char c) { return std::isxdigit(c); }))
        return {};
    return value;
}
}
bool GpuMonitor::validAddress(const std::string& address) {
    static const std::regex pattern("[0-9a-fA-F]{4}:[0-9a-fA-F]{2}:[0-9a-fA-F]{2}\\.[0-7]");
    return std::regex_match(address, pattern);
}
GpuSnapshot GpuMonitor::snapshot() const {
    GpuSnapshot result;
    std::error_code error;
    std::filesystem::directory_iterator it(root_, error), end;
    if (error) return result;
    result.available = true;
    for (; it != end; it.increment(error)) {
        if (error) { result.available = false; break; }
        const auto path = it->path();
        const auto address = path.filename().string();
        if (!validAddress(address)) continue;
        const auto pci_class = readHex(path / "class", 6);
        if (pci_class.substr(0, 4) != "0x03") continue;
        GpuInfo gpu;
        gpu.pci_address = address;
        gpu.vendor_id = readHex(path / "vendor", 4);
        gpu.device_id = readHex(path / "device", 4);
        if (gpu.vendor_id.empty() || gpu.device_id.empty()) continue;
        gpu.vendor = gpu.vendor_id == "0x10de" ? "NVIDIA" :
                     gpu.vendor_id == "0x1002" ? "AMD" :
                     gpu.vendor_id == "0x8086" ? "Intel" : "PCI";
        std::error_code link_error;
        auto driver = std::filesystem::read_symlink(path / "driver", link_error);
        if (!link_error) gpu.driver = driver.filename().string();
        result.devices.push_back(std::move(gpu));
    }
    if (error) result.available = false;
    std::sort(result.devices.begin(), result.devices.end(), [](const auto& a, const auto& b) {
        return a.pci_address < b.pci_address;
    });
    return result;
}
}
