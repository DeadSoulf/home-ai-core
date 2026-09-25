#pragma once

#include "core/modules/Module.h"

#include <memory>
#include <mutex>
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
};

class ModuleManager {
public:
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
};

}
