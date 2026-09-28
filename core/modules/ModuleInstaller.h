#pragma once

#include <filesystem>
#include <mutex>
#include <string>

namespace homeai {

class ModuleManager;

class ModuleInstaller {
public:
    ModuleInstaller(
        ModuleManager& modules,
        std::filesystem::path registry_path
    );

    bool load(
        std::string& error
    );

    bool install(
        const std::string& id,
        std::string& error
    );

    bool uninstall(
        const std::string& id,
        std::string& error
    );

    const std::filesystem::path&
    registryPath() const;

private:
    bool save(
        std::string& error
    );

    ModuleManager& modules_;
    std::filesystem::path registry_path_;
    mutable std::mutex mutex_;
};

}
