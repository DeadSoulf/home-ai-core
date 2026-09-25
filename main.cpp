#include <atomic>
#include <chrono>
#include <csignal>
#include <iostream>
#include <thread>

static std::atomic<bool> running{true};

void signal_handler(int signal)
{
    std::cout << "\n[CORE] Received signal: " << signal << '\n';
    running = false;
}

int main()
{
    std::signal(SIGINT, signal_handler);
    std::signal(SIGTERM, signal_handler);

    std::cout << "=====================================\n";
    std::cout << "       Home AI Core 0.0.1\n";
    std::cout << "=====================================\n";
    std::cout << "[CORE] Starting...\n";
    std::cout << "[CORE] Runtime initialized\n";
    std::cout << "[CORE] Status: RUNNING\n";

    while (running)
    {
        std::this_thread::sleep_for(std::chrono::seconds(1));
    }

    std::cout << "[CORE] Shutting down...\n";
    std::cout << "[CORE] Shutdown complete\n";

    return 0;
}
