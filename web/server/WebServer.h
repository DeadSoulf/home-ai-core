#pragma once

#include <atomic>
#include <cstdint>
#include <string>
#include <thread>

namespace homeai {

class CoreRuntime;

class WebServer {
public:
    explicit WebServer(CoreRuntime& runtime);
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

    CoreRuntime& runtime_;

    std::atomic<bool> running_{false};

    int server_fd_{-1};

    std::string bind_address_;
    std::uint16_t port_{0};

    std::thread server_thread_;
};

}
