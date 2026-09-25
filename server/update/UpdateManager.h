#pragma once

#include <atomic>
#include <chrono>
#include <condition_variable>
#include <mutex>
#include <string>
#include <thread>
#include <vector>

namespace homeai {

enum class UpdateState {
    Idle,
    Checking,
    UpToDate,
    UpdateAvailable,
    Updating,
    Building,
    Testing,
    ReadyToRestart,
    Error
};

struct UpdateStatus {
    UpdateState state{UpdateState::Idle};
    std::string state_text;
    std::string local_sha;
    std::string remote_sha;
    std::string branch;
    std::string message;
    std::string last_output;

    bool update_available{false};
    bool busy{false};
    bool restart_required{false};
};

class UpdateManager {
public:
    UpdateManager() = default;
    ~UpdateManager();

    bool initialize(
        const std::string& repository_path,
        const std::string& remote,
        const std::string& branch,
        int check_interval_seconds = 60
    );

    void start();
    void stop();

    UpdateStatus status() const;

    void requestCheck();

    bool requestUpdate(
        std::string& error
    );

    bool requestRestart(
        std::string& error
    );

    bool consumeRestartRequest();

    std::string restartBinaryPath() const;

    static std::string stateToString(
        UpdateState state
    );

private:
    struct CommandResult {
        int exit_code{-1};
        std::string output;
    };

    void workerLoop();
    void performCheck();
    void performUpdate();

    CommandResult runCommand(
        const std::string& executable,
        const std::vector<std::string>& arguments
    ) const;

    bool worktreeClean(
        std::string& details
    ) const;

    std::string gitHead() const;
    std::string remoteHead() const;

    void setState(
        UpdateState state,
        const std::string& message,
        const std::string& output = ""
    );

    void rollbackSource(
        const std::string& old_head
    );

    mutable std::mutex mutex_;
    std::condition_variable condition_;

    std::atomic<bool> running_{false};

    bool check_requested_{false};
    bool update_requested_{false};
    bool restart_requested_{false};

    std::string repository_path_;
    std::string remote_{"origin"};
    std::string branch_{"develop"};

    int check_interval_seconds_{60};

    UpdateStatus status_;

    std::thread worker_;
};

}
