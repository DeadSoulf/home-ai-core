#pragma once

#include <filesystem>
#include <string>

namespace homeai {

class BuildActivator {
public:
    static std::string releaseDirectoryName(
        const std::string& commit_sha
    );

    static bool activate(
        const std::filesystem::path& repository_path,
        const std::filesystem::path& tested_build,
        std::string& error
    );

private:
    static bool removePrevious(
        const std::filesystem::path& repository_path,
        const std::filesystem::path& previous_build,
        std::string& error
    );

    static bool isManagedReleaseName(
        const std::string& name
    );
};

}
