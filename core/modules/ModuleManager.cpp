#include "core/modules/ModuleManager.h"

#include "core/logging/Logger.h"

#include <algorithm>
#include <unordered_set>

namespace homeai {

bool ModuleManager::registerModule(
    std::unique_ptr<IModule> module,
    std::string& error
)
{
    if (!module) {
        error = "Cannot register a null module.";
        return false;
    }

    const auto name = module->name();

    if (name.empty()) {
        error = "Module name cannot be empty.";
        return false;
    }

    std::lock_guard<std::mutex> lock(mutex_);

    if (modules_.contains(name)) {
        error = "Duplicate module: " + name;
        return false;
    }

    Entry entry;
    entry.module = std::move(module);

    modules_.emplace(name, std::move(entry));
    registration_order_.push_back(name);

    Logger::instance().info(
        "Module registered: " + name
    );

    return true;
}

bool ModuleManager::initializeAll(
    std::string& error
)
{
    std::vector<std::string> order;

    if (!resolveOrder(order, error))
        return false;

    for (const auto& name : order) {
        IModule* module = nullptr;

        {
            std::lock_guard<std::mutex>
                lock(mutex_);

            auto& entry = modules_.at(name);

            if (
                entry.state ==
                    ModuleState::Initialized
                ||
                entry.state ==
                    ModuleState::Running
            ) {
                continue;
            }

            module = entry.module.get();
        }

        std::string module_error;

        if (!module->initialize(module_error)) {
            std::lock_guard<std::mutex>
                lock(mutex_);

            auto& entry = modules_.at(name);
            entry.state = ModuleState::Failed;
            entry.message =
                module_error.empty()
                ? "Initialization failed."
                : module_error;

            error =
                "Module initialization failed: "
                + name + ": " + entry.message;

            Logger::instance().error(error);
            return false;
        }

        {
            std::lock_guard<std::mutex>
                lock(mutex_);

            auto& entry = modules_.at(name);
            entry.state =
                ModuleState::Initialized;
            entry.message.clear();
        }

        Logger::instance().info(
            "Module initialized: " + name
        );
    }

    return true;
}

bool ModuleManager::startAll(
    std::string& error
)
{
    std::vector<std::string> order;

    if (!resolveOrder(order, error))
        return false;

    {
        std::lock_guard<std::mutex>
            lock(mutex_);
        started_order_.clear();
    }

    for (const auto& name : order) {
        IModule* module = nullptr;

        {
            std::lock_guard<std::mutex>
                lock(mutex_);

            auto& entry = modules_.at(name);

            if (
                entry.state !=
                    ModuleState::Initialized
                &&
                entry.state !=
                    ModuleState::Stopped
            ) {
                if (
                    entry.state ==
                    ModuleState::Running
                ) {
                    continue;
                }

                error =
                    "Module is not initialized: "
                    + name;
                return false;
            }

            module = entry.module.get();
        }

        std::string module_error;

        if (!module->start(module_error)) {
            {
                std::lock_guard<std::mutex>
                    lock(mutex_);

                auto& entry = modules_.at(name);
                entry.state =
                    ModuleState::Failed;
                entry.message =
                    module_error.empty()
                    ? "Start failed."
                    : module_error;

                error =
                    "Module start failed: "
                    + name + ": "
                    + entry.message;
            }

            Logger::instance().error(error);
            stopAll();
            return false;
        }

        {
            std::lock_guard<std::mutex>
                lock(mutex_);

            auto& entry = modules_.at(name);
            entry.state =
                ModuleState::Running;
            entry.message.clear();
            started_order_.push_back(name);
        }

        Logger::instance().info(
            "Module started: " + name
        );
    }

    return true;
}

void ModuleManager::stopAll()
{
    std::vector<std::string> order;

    {
        std::lock_guard<std::mutex>
            lock(mutex_);

        order = started_order_;
        started_order_.clear();
    }

    std::reverse(
        order.begin(),
        order.end()
    );

    for (const auto& name : order) {
        IModule* module = nullptr;

        {
            std::lock_guard<std::mutex>
                lock(mutex_);

            const auto it =
                modules_.find(name);

            if (it == modules_.end())
                continue;

            module =
                it->second.module.get();
        }

        module->stop();

        {
            std::lock_guard<std::mutex>
                lock(mutex_);

            auto& entry =
                modules_.at(name);

            if (
                entry.state !=
                ModuleState::Failed
            ) {
                entry.state =
                    ModuleState::Stopped;
                entry.message.clear();
            }
        }

        Logger::instance().info(
            "Module stopped: " + name
        );
    }
}

bool ModuleManager::restartModule(
    const std::string& name,
    std::string& error
)
{
    std::vector<std::string> order;

    if (!resolveOrder(order, error))
        return false;

    std::vector<std::string>
        restart_order;

    {
        std::lock_guard<std::mutex>
            lock(mutex_);

        const auto target =
            modules_.find(name);

        if (target == modules_.end()) {
            error = "Unknown module: " + name;
            return false;
        }

        if (
            target->second.state !=
            ModuleState::Running
        ) {
            error =
                "Module is not running: "
                + name;
            return false;
        }

        std::unordered_set<std::string>
            affected;

        for (const auto& current : order) {
            bool include =
                current == name;

            if (!include) {
                const auto& dependencies =
                    modules_.at(current)
                        .module
                        ->dependencies();

                for (
                    const auto& dependency :
                    dependencies
                ) {
                    if (
                        affected.contains(
                            dependency
                        )
                    ) {
                        include = true;
                        break;
                    }
                }
            }

            if (!include)
                continue;

            affected.insert(current);

            if (
                modules_.at(current).state ==
                ModuleState::Running
            ) {
                restart_order.push_back(
                    current
                );
            }
        }

        started_order_.erase(
            std::remove_if(
                started_order_.begin(),
                started_order_.end(),
                [&](const std::string& current) {
                    return
                        std::find(
                            restart_order.begin(),
                            restart_order.end(),
                            current
                        ) != restart_order.end();
                }
            ),
            started_order_.end()
        );
    }

    for (
        auto it = restart_order.rbegin();
        it != restart_order.rend();
        ++it
    ) {
        IModule* module = nullptr;

        {
            std::lock_guard<std::mutex>
                lock(mutex_);
            module =
                modules_.at(*it).module.get();
        }

        module->stop();

        {
            std::lock_guard<std::mutex>
                lock(mutex_);

            auto& entry =
                modules_.at(*it);
            entry.state =
                ModuleState::Stopped;
            entry.message =
                "Restart in progress.";
        }

        Logger::instance().warning(
            "Module stopped for restart: "
            + *it
        );
    }

    std::vector<std::string>
        restarted;

    for (const auto& current : restart_order) {
        IModule* module = nullptr;

        {
            std::lock_guard<std::mutex>
                lock(mutex_);

            auto& entry =
                modules_.at(current);

            for (
                const auto& dependency :
                entry.module->dependencies()
            ) {
                const auto dependency_it =
                    modules_.find(dependency);

                if (
                    dependency_it ==
                        modules_.end()
                    ||
                    dependency_it->second.state !=
                        ModuleState::Running
                ) {
                    entry.state =
                        ModuleState::Failed;
                    entry.message =
                        "Dependency is not running: "
                        + dependency;

                    error =
                        "Module restart failed: "
                        + current + ": "
                        + entry.message;

                    Logger::instance().error(
                        error
                    );
                    return false;
                }
            }

            module = entry.module.get();
        }

        std::string module_error;

        if (!module->start(module_error)) {
            {
                std::lock_guard<std::mutex>
                    lock(mutex_);

                auto& entry =
                    modules_.at(current);
                entry.state =
                    ModuleState::Failed;
                entry.message =
                    module_error.empty()
                    ? "Restart failed."
                    : module_error;
            }

            for (
                auto it = restarted.rbegin();
                it != restarted.rend();
                ++it
            ) {
                IModule* started_module =
                    nullptr;

                {
                    std::lock_guard<std::mutex>
                        lock(mutex_);
                    started_module =
                        modules_.at(*it)
                            .module.get();
                }

                started_module->stop();

                {
                    std::lock_guard<std::mutex>
                        lock(mutex_);

                    auto& entry =
                        modules_.at(*it);
                    entry.state =
                        ModuleState::Stopped;
                    entry.message =
                        "Stopped after dependent restart failure.";

                    started_order_.erase(
                        std::remove(
                            started_order_.begin(),
                            started_order_.end(),
                            *it
                        ),
                        started_order_.end()
                    );
                }
            }

            error =
                "Module restart failed: "
                + current + ": "
                + (
                    module_error.empty()
                    ? std::string("Restart failed.")
                    : module_error
                );

            Logger::instance().error(error);
            return false;
        }

        {
            std::lock_guard<std::mutex>
                lock(mutex_);

            auto& entry =
                modules_.at(current);
            entry.state =
                ModuleState::Running;
            entry.message.clear();
            ++entry.restart_count;
            started_order_.push_back(
                current
            );
        }

        restarted.push_back(current);

        Logger::instance().info(
            "Module restarted: " + current
        );
    }

    return true;
}

std::size_t ModuleManager::watchdogPass(
    std::size_t max_attempts,
    std::chrono::seconds cooldown
)
{
    if (max_attempts == 0)
        return 0;

    std::vector<std::string>
        candidates;

    const auto now =
        std::chrono::steady_clock::now();

    {
        std::lock_guard<std::mutex>
            lock(mutex_);

        for (
            const auto& name :
            registration_order_
        ) {
            auto& entry =
                modules_.at(name);

            if (
                entry.state !=
                ModuleState::Running
            ) {
                continue;
            }

            const auto health =
                entry.module->health();

            if (
                health ==
                ModuleHealth::Healthy
            ) {
                entry.watchdog_attempts = 0;
                continue;
            }

            if (
                health !=
                ModuleHealth::Unhealthy
            ) {
                continue;
            }

            if (
                entry.watchdog_attempts >=
                max_attempts
            ) {
                continue;
            }

            if (
                entry.last_watchdog_attempt !=
                    std::chrono::steady_clock::
                        time_point{}
                &&
                now -
                    entry.last_watchdog_attempt <
                    cooldown
            ) {
                continue;
            }

            candidates.push_back(name);
        }
    }

    std::size_t attempts = 0;

    for (const auto& name : candidates) {
        const auto attempt_time =
            std::chrono::steady_clock::now();

        {
            std::lock_guard<std::mutex>
                lock(mutex_);

            auto& entry =
                modules_.at(name);

            if (
                entry.state !=
                    ModuleState::Running
                ||
                entry.module->health() !=
                    ModuleHealth::Unhealthy
                ||
                entry.watchdog_attempts >=
                    max_attempts
            ) {
                continue;
            }

            if (
                entry.last_watchdog_attempt !=
                    std::chrono::steady_clock::
                        time_point{}
                &&
                attempt_time -
                    entry.last_watchdog_attempt <
                    cooldown
            ) {
                continue;
            }

            ++entry.watchdog_attempts;
            entry.last_watchdog_attempt =
                attempt_time;
        }

        ++attempts;

        Logger::instance().warning(
            "Watchdog restarting unhealthy module: "
            + name
        );

        std::string error;

        if (!restartModule(name, error)) {
            Logger::instance().error(
                "Watchdog recovery failed for "
                + name + ": " + error
            );
        }
    }

    return attempts;
}

std::vector<ModuleStatus>
ModuleManager::snapshot() const
{
    std::vector<ModuleStatus> result;

    std::lock_guard<std::mutex>
        lock(mutex_);

    result.reserve(
        registration_order_.size()
    );

    for (
        const auto& name :
        registration_order_
    ) {
        const auto& entry =
            modules_.at(name);

        ModuleStatus status;
        status.name = name;
        status.state = entry.state;
        status.dependencies =
            entry.module->dependencies();
        status.restart_count =
            entry.restart_count;
        status.watchdog_attempts =
            entry.watchdog_attempts;
        status.health =
            entry.module->health();

        status.message =
            entry.message.empty()
            ? entry.module->healthMessage()
            : entry.message;

        result.push_back(
            std::move(status)
        );
    }

    return result;
}

bool ModuleManager::hasModule(
    const std::string& name
) const
{
    std::lock_guard<std::mutex>
        lock(mutex_);
    return modules_.contains(name);
}

std::string ModuleManager::stateToString(
    ModuleState state
)
{
    switch (state) {
        case ModuleState::Registered:
            return "registered";
        case ModuleState::Initialized:
            return "initialized";
        case ModuleState::Running:
            return "running";
        case ModuleState::Stopped:
            return "stopped";
        case ModuleState::Failed:
            return "failed";
    }

    return "unknown";
}

bool ModuleManager::resolveOrder(
    std::vector<std::string>& order,
    std::string& error
) const
{
    std::lock_guard<std::mutex>
        lock(mutex_);

    std::unordered_map<std::string, int>
        marks;

    order.clear();

    for (
        const auto& name :
        registration_order_
    ) {
        if (
            !visit(
                name,
                marks,
                order,
                error
            )
        ) {
            return false;
        }
    }

    return true;
}

bool ModuleManager::visit(
    const std::string& name,
    std::unordered_map<
        std::string,
        int
    >& marks,
    std::vector<std::string>& order,
    std::string& error
) const
{
    const auto mark_it =
        marks.find(name);

    if (mark_it != marks.end()) {
        if (mark_it->second == 2)
            return true;

        if (mark_it->second == 1) {
            error =
                "Module dependency cycle at: "
                + name;
            return false;
        }
    }

    const auto module_it =
        modules_.find(name);

    if (module_it == modules_.end()) {
        error =
            "Missing module dependency: "
            + name;
        return false;
    }

    marks[name] = 1;

    for (
        const auto& dependency :
        module_it->second
            .module
            ->dependencies()
    ) {
        if (
            !modules_.contains(
                dependency
            )
        ) {
            error =
                "Module " + name
                + " requires missing dependency "
                + dependency;
            return false;
        }

        if (
            !visit(
                dependency,
                marks,
                order,
                error
            )
        ) {
            return false;
        }
    }

    marks[name] = 2;
    order.push_back(name);
    return true;
}

}
