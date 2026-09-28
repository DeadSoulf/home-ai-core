#include "core/modules/ModuleInstaller.h"

#include "core/logging/Logger.h"
#include "core/modules/ModuleManager.h"

#include <algorithm>
#include <fstream>
#include <system_error>

namespace homeai {

ModuleInstaller::ModuleInstaller(
    ModuleManager& modules,
    std::filesystem::path registry_path
)
    : modules_(modules),
      registry_path_(
          std::move(registry_path)
      )
{
}

bool ModuleInstaller::load(
    std::string& error
)
{
    std::lock_guard<std::mutex>
        lock(mutex_);

    std::error_code ec;

    if (
        !std::filesystem::exists(
            registry_path_,
            ec
        )
    ) {
        if (ec) {
            error =
                "Unable to inspect module registry: "
                + ec.message();
            return false;
        }

        return true;
    }

    std::ifstream input(registry_path_);

    if (!input) {
        error =
            "Unable to open module registry: "
            + registry_path_.string();
        return false;
    }

    std::string line;
    std::size_t line_number = 0;

    while (std::getline(input, line)) {
        ++line_number;

        if (
            !line.empty()
            &&
            line.back() == '\r'
        ) {
            line.pop_back();
        }

        if (
            line.empty()
            ||
            line.front() == '#'
        ) {
            continue;
        }

        const auto separator =
            line.find('=');

        if (
            separator == std::string::npos
            ||
            separator == 0
            ||
            separator + 1 >= line.size()
        ) {
            error =
                "Invalid module registry line "
                + std::to_string(line_number);
            return false;
        }

        const auto id =
            line.substr(0, separator);
        const auto value =
            line.substr(separator + 1);

        if (
            value != "0"
            &&
            value != "1"
        ) {
            error =
                "Invalid module registry state for "
                + id;
            return false;
        }

        const auto manifest =
            modules_.manifest(id);

        if (!manifest) {
            Logger::instance().warning(
                "Ignoring stale module registry entry: "
                + id
            );
            continue;
        }

        if (
            manifest->core_component
            ||
            !manifest->installable
        ) {
            Logger::instance().warning(
                "Ignoring immutable module registry entry: "
                + id
            );
            continue;
        }

        std::string state_error;

        if (
            !modules_.setInstalledState(
                id,
                value == "1",
                state_error
            )
        ) {
            error = state_error;
            return false;
        }
    }

    return true;
}

bool ModuleInstaller::install(
    const std::string& id,
    std::string& error
)
{
    std::lock_guard<std::mutex>
        lock(mutex_);

    const auto manifest =
        modules_.manifest(id);

    if (!manifest) {
        error =
            "Unknown project module: "
            + id;
        return false;
    }

    if (
        manifest->core_component
        ||
        !manifest->installable
    ) {
        error =
            "Module is not installable: "
            + id;
        return false;
    }

    if (modules_.isInstalled(id))
        return true;

    for (
        const auto& dependency :
        manifest->dependencies
    ) {
        if (
            !modules_.isInstalled(
                dependency
            )
        ) {
            error =
                "Dependency is not installed: "
                + dependency;
            return false;
        }
    }

    std::string state_error;

    if (
        !modules_.setInstalledState(
            id,
            true,
            state_error
        )
    ) {
        error = state_error;
        return false;
    }

    if (!save(error)) {
        std::string rollback_error;
        modules_.setInstalledState(
            id,
            false,
            rollback_error
        );
        return false;
    }

    Logger::instance().info(
        "Project module marked installed: "
        + id
    );

    return true;
}

bool ModuleInstaller::uninstall(
    const std::string& id,
    std::string& error
)
{
    std::lock_guard<std::mutex>
        lock(mutex_);

    const auto manifest =
        modules_.manifest(id);

    if (!manifest) {
        error =
            "Unknown project module: "
            + id;
        return false;
    }

    if (
        manifest->core_component
        ||
        !manifest->installable
    ) {
        error =
            "Module cannot be uninstalled: "
            + id;
        return false;
    }

    if (!modules_.isInstalled(id))
        return true;

    const auto catalog =
        modules_.catalogSnapshot();

    for (const auto& entry : catalog) {
        if (
            entry.manifest.id == id
            ||
            !entry.installed
        ) {
            continue;
        }

        if (
            std::find(
                entry.manifest.dependencies.begin(),
                entry.manifest.dependencies.end(),
                id
            )
            !=
            entry.manifest.dependencies.end()
        ) {
            error =
                "Installed module depends on "
                + id + ": "
                + entry.manifest.id;
            return false;
        }
    }

    std::string state_error;

    if (
        !modules_.setInstalledState(
            id,
            false,
            state_error
        )
    ) {
        error = state_error;
        return false;
    }

    if (!save(error)) {
        std::string rollback_error;
        modules_.setInstalledState(
            id,
            true,
            rollback_error
        );
        return false;
    }

    Logger::instance().info(
        "Project module marked uninstalled: "
        + id
    );

    return true;
}

const std::filesystem::path&
ModuleInstaller::registryPath() const
{
    return registry_path_;
}

bool ModuleInstaller::save(
    std::string& error
)
{
    std::error_code ec;

    const auto parent =
        registry_path_.parent_path();

    if (!parent.empty()) {
        std::filesystem::create_directories(
            parent,
            ec
        );

        if (ec) {
            error =
                "Unable to create module registry directory: "
                + ec.message();
            return false;
        }
    }

    auto temporary = registry_path_;
    temporary += ".tmp";

    std::ofstream output(
        temporary,
        std::ios::trunc
    );

    if (!output) {
        error =
            "Unable to write module registry: "
            + temporary.string();
        return false;
    }

    output
        << "# Home AI Core module installation state\n";

    const auto catalog =
        modules_.catalogSnapshot();

    for (const auto& entry : catalog) {
        if (
            entry.manifest.core_component
            ||
            !entry.manifest.installable
        ) {
            continue;
        }

        output
            << entry.manifest.id
            << "="
            << (
                entry.installed
                ? "1"
                : "0"
            )
            << "\n";
    }

    output.flush();

    if (!output) {
        error =
            "Unable to flush module registry.";
        output.close();
        std::filesystem::remove(
            temporary,
            ec
        );
        return false;
    }

    output.close();

    std::filesystem::permissions(
        temporary,
        std::filesystem::perms::owner_read
        |
        std::filesystem::perms::owner_write,
        std::filesystem::perm_options::replace,
        ec
    );

    if (ec) {
        error =
            "Unable to secure module registry: "
            + ec.message();
        std::filesystem::remove(
            temporary,
            ec
        );
        return false;
    }

    std::filesystem::rename(
        temporary,
        registry_path_,
        ec
    );

    if (ec) {
        error =
            "Unable to activate module registry: "
            + ec.message();
        std::filesystem::remove(
            temporary,
            ec
        );
        return false;
    }

    return true;
}

}
