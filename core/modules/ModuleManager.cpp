#include "core/modules/ModuleManager.h"

#include "core/logging/Logger.h"

#include <algorithm>

namespace homeai {

bool ModuleManager::registerModule(
    std::unique_ptr<IModule> module,
    std::string& error
)
{
    if (!module) {
        error =
            "Cannot register a null module.";

        return false;
    }

    const auto name =
        module->name();

    if (name.empty()) {
        error =
            "Module name cannot be empty.";

        return false;
    }

    std::lock_guard<std::mutex>
        lock(mutex_);

    if (
        modules_.contains(name)
    ) {
        error =
            "Duplicate module: " +
            name;

        return false;
    }

    Entry entry;

    entry.module =
        std::move(module);

    modules_.emplace(
        name,
        std::move(entry)
    );

    registration_order_
        .push_back(name);

    Logger::instance().info(
        "Module registered: " +
        name
    );

    return true;
}

bool ModuleManager::initializeAll(
    std::string& error
)
{
    std::vector<std::string>
        order;

    if (
        !resolveOrder(
            order,
            error
        )
    ) {
        return false;
    }

    for (const auto& name : order) {
        IModule* module = nullptr;

        {
            std::lock_guard<std::mutex>
                lock(mutex_);

            auto& entry =
                modules_.at(name);

            if (
                entry.state ==
                ModuleState::Initialized
                ||
                entry.state ==
                ModuleState::Running
            ) {
                continue;
            }

            module =
                entry.module.get();
        }

        std::string module_error;

        if (
            !module->initialize(
                module_error
            )
        ) {
            std::lock_guard<std::mutex>
                lock(mutex_);

            auto& entry =
                modules_.at(name);

            entry.state =
                ModuleState::Failed;

            entry.message =
                module_error.empty()
                ? "Initialization failed."
                : module_error;

            error =
                "Module initialization failed: "
                + name
                + ": "
                + entry.message;

            Logger::instance().error(
                error
            );

            return false;
        }

        {
            std::lock_guard<std::mutex>
                lock(mutex_);

            auto& entry =
                modules_.at(name);

            entry.state =
                ModuleState::Initialized;

            entry.message.clear();
        }

        Logger::instance().info(
            "Module initialized: " +
            name
        );
    }

    return true;
}

bool ModuleManager::startAll(
    std::string& error
)
{
    std::vector<std::string>
        order;

    if (
        !resolveOrder(
            order,
            error
        )
    ) {
        return false;
    }

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

            auto& entry =
                modules_.at(name);

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

            module =
                entry.module.get();
        }

        std::string module_error;

        if (
            !module->start(
                module_error
            )
        ) {
            {
                std::lock_guard<std::mutex>
                    lock(mutex_);

                auto& entry =
                    modules_.at(name);

                entry.state =
                    ModuleState::Failed;

                entry.message =
                    module_error.empty()
                    ? "Start failed."
                    : module_error;

                error =
                    "Module start failed: "
                    + name
                    + ": "
                    + entry.message;
            }

            Logger::instance().error(
                error
            );

            stopAll();

            return false;
        }

        {
            std::lock_guard<std::mutex>
                lock(mutex_);

            auto& entry =
                modules_.at(name);

            entry.state =
                ModuleState::Running;

            entry.message.clear();

            started_order_
                .push_back(name);
        }

        Logger::instance().info(
            "Module started: " +
            name
        );
    }

    return true;
}

void ModuleManager::stopAll()
{
    std::vector<std::string>
        order;

    {
        std::lock_guard<std::mutex>
            lock(mutex_);

        order =
            started_order_;

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

            if (
                it ==
                modules_.end()
            ) {
                continue;
            }

            module =
                it->second
                    .module.get();
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
            "Module stopped: " +
            name
        );
    }
}

std::vector<ModuleStatus>
ModuleManager::snapshot() const
{
    std::vector<ModuleStatus>
        result;

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

        status.name =
            name;

        status.state =
            entry.state;

        status.dependencies =
            entry.module
                ->dependencies();

        status.health =
            entry.module
                ->health();

        status.message =
            entry.message.empty()
            ? entry.module
                ->healthMessage()
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

    std::unordered_map<
        std::string,
        int
    > marks;

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

    if (
        mark_it != marks.end()
    ) {
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

    if (
        module_it ==
        modules_.end()
    ) {
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
                "Module "
                + name
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
