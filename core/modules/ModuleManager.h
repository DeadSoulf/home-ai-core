#pragma once

#include "core/modules/Module.h"

#include <chrono>
#include <cstddef>
#include <memory>
#include <mutex>
#include <optional>
#include <string>
#include <unordered_map>
#include <vector>

namespace homeai {

enum class ModuleState {
    Registered,
    Initialized,
    Running,
    Stopped,
    Failed
};

struct ModuleStatus {
    std::string name;
    ModuleState state{
        ModuleState::Registered
    };

    ModuleHealth health{
        ModuleHealth::Unknown
    };

    std::string message;

    std::vector<std::string>
        dependencies;

    std::size_t restart_count{0};
    std::size_t watchdog_attempts{0};
};

struct ModuleManifest {
    std::string id;
    std::string display_name;
    std::string version;
    std::string description;

    std::vector<std::string>
        dependencies;

    std::vector<std::string>
        permissions;

    std::vector<std::string>
        runtime_modules;

    bool core_component{false};
    bool bundled{false};
    bool installable{false};
};

struct ModuleCatalogEntry {
    ModuleManifest manifest;
    bool installed{false};
    bool running{false};
    std::string state;
};

class ModuleManager {
public:
    bool registerManifest(
        ModuleManifest manifest,
        std::string& error
    );

    std::vector<ModuleCatalogEntry>
    catalogSnapshot() const;

    std::optional<ModuleManifest>
    manifest(
        const std::string& id
    ) const;

    bool isInstalled(
        const std::string& id
    ) const;

    bool setInstalledState(
        const std::string& id,
        bool installed,
        std::string& error
    );

    bool registerModule(
        std::unique_ptr<IModule> module,
        std::string& error
    );

    bool initializeAll(
        std::string& error
    );

    bool startAll(
        std::string& error
    );

    void stopAll();

    bool restartModule(
        const std::string& name,
        std::string& error
    );

    std::size_t watchdogPass(
        std::size_t max_attempts,
        std::chrono::seconds cooldown
    );

    std::vector<ModuleStatus>
    snapshot() const;

    bool hasModule(
        const std::string& name
    ) const;

    static std::string stateToString(
        ModuleState state
    );

private:
    struct Entry {
        std::unique_ptr<IModule> module;

        ModuleState state{
            ModuleState::Registered
        };

        std::string message;

        std::size_t restart_count{0};
        std::size_t watchdog_attempts{0};

        std::chrono::steady_clock::time_point
            last_watchdog_attempt{};
    };

    bool resolveOrder(
        std::vector<std::string>& order,
        std::string& error
    ) const;

    bool visit(
        const std::string& name,
        std::unordered_map<
            std::string,
            int
        >& marks,
        std::vector<std::string>& order,
        std::string& error
    ) const;

    mutable std::mutex mutex_;

    std::unordered_map<
        std::string,
        Entry
    > modules_;

    std::vector<std::string>
        registration_order_;

    std::vector<std::string>
        started_order_;

    std::unordered_map<
        std::string,
        ModuleManifest
    > manifests_;

    std::vector<std::string>
        manifest_order_;

    std::unordered_map<
        std::string,
        bool
    > installed_overrides_;
};

}
