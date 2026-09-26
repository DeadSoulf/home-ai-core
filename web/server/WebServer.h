#pragma once

#include <atomic>
#include <condition_variable>
#include <cstddef>
#include <cstdint>
#include <mutex>
#include <string>
#include <thread>
#include "server/hardware/GpuMonitor.h"

namespace homeai {

class CoreRuntime;
class SecurityManager;
class UpdateManager;
class ModuleManager;
class CameraManager;
class HypervisorManager;

class WebServer {
public:
    WebServer(
        CoreRuntime& runtime,
        SecurityManager& security,
        UpdateManager& updates,
        ModuleManager& modules,
        GpuMonitor gpu_monitor = GpuMonitor(),
        CameraManager* cameras = nullptr,
        HypervisorManager* hypervisor = nullptr
    );

    ~WebServer();

    bool start(
        const std::string& bind_address,
        std::uint16_t port
    );

    void stop();

    bool isRunning() const;

private:
    void run();
    void handleClient(int client_fd);
    void handleClientWorker(int client_fd);

    CoreRuntime& runtime_;
    SecurityManager& security_;
    UpdateManager& updates_;
    ModuleManager& modules_;
    GpuMonitor gpu_monitor_;
    CameraManager* cameras_{nullptr};
    HypervisorManager* hypervisor_{nullptr};

    std::atomic<bool> running_{false};

    int server_fd_{-1};

    std::string bind_address_;
    std::uint16_t port_{0};

    std::thread server_thread_;

    std::mutex client_mutex_;
    std::condition_variable client_cv_;
    std::size_t active_clients_{0};
};

}
