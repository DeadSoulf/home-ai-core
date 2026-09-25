#include "server/hardware/GpuMonitor.h"
#include <filesystem>
#include <fstream>
#include <iostream>
#include <unistd.h>

int main() {
    namespace fs = std::filesystem;
    const auto root = fs::temp_directory_path() / ("homeai-gpu-" + std::to_string(getpid()));
    fs::create_directories(root);
    auto add = [&](const std::string& address, const std::string& cls, const std::string& vendor) {
        fs::create_directories(root / address);
        std::ofstream(root / address / "class") << cls;
        std::ofstream(root / address / "vendor") << vendor;
        std::ofstream(root / address / "device") << "0x1234";
    };
    add("0000:01:00.0", "0x030000", "0x10de");
    add("0000:02:00.0", "0x030200", "0x1002");
    add("0000:03:00.0", "0x020000", "0x8086");
    add("0000:04:00.0", "garbage", "0x8086");
    add("0000:05:00.0", "0x030000", "bad");
    fs::create_symlink("/sys/bus/pci/drivers/nvidia", root / "0000:01:00.0" / "driver");
    homeai::GpuMonitor monitor(root);
    const auto found = monitor.snapshot();
    bool ok = found.available && found.devices.size() == 2;
    if (ok) ok = found.devices[0].vendor == "NVIDIA" && found.devices[0].driver == "nvidia" && found.devices[1].driver.empty();
    fs::remove_all(root / "0000:01:00.0");
    ok = ok && monitor.snapshot().devices.size() == 1;
    fs::remove_all(root);
    ok = ok && !monitor.snapshot().available && !homeai::GpuMonitor::validAddress("../../etc/passwd") &&
         !homeai::GpuMonitor::validAddress("0000:01:00.0\nai.gpu=evil");
    if (!ok) { std::cerr << "GPU inventory test failed\n"; return 1; }
    std::cout << "GPU inventory test passed\n";
}
