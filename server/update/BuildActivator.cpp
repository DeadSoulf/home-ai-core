#include "server/update/BuildActivator.h"

#include <algorithm>
#include <cctype>
#include <system_error>

namespace homeai {

std::string BuildActivator::releaseDirectoryName(
    const std::string& commit_sha
)
{
    std::string suffix;

    suffix.reserve(12);

    for (char c : commit_sha) {
        const auto value =
            static_cast<unsigned char>(c);

        if (!std::isxdigit(value))
            return {};

        suffix.push_back(
            static_cast<char>(
                std::tolower(value)
            )
        );

        if (suffix.size() == 12)
            break;
    }

    if (suffix.size() < 7)
        return {};

    return
        "build-release-" +
        suffix;
}

bool BuildActivator::isManagedReleaseName(
    const std::string& name
)
{
    constexpr const char* prefix =
        "build-release-";

    if (
        name.rfind(prefix, 0) != 0
        ||
        name.size() <=
            std::char_traits<char>::length(
                prefix
            )
    ) {
        return false;
    }

    return std::all_of(
        name.begin() +
            static_cast<std::ptrdiff_t>(
                std::char_traits<char>::length(
                    prefix
                )
            ),
        name.end(),
        [](unsigned char c) {
            return std::isxdigit(c);
        }
    );
}

bool BuildActivator::removePrevious(
    const std::filesystem::path& repository_path,
    const std::filesystem::path& previous_build,
    std::string& error
)
{
    std::error_code status_error;

    const auto status =
        std::filesystem::symlink_status(
            previous_build,
            status_error
        );

    if (status_error) {
        if (
            status_error ==
            std::errc::no_such_file_or_directory
        ) {
            return true;
        }

        error =
            "Unable to inspect previous build: "
            + status_error.message();

        return false;
    }

    if (
        status.type() ==
        std::filesystem::file_type::not_found
    ) {
        return true;
    }

    std::filesystem::path managed_target;

    if (
        status.type() ==
        std::filesystem::file_type::symlink
    ) {
        std::error_code link_error;

        const auto target =
            std::filesystem::read_symlink(
                previous_build,
                link_error
            );

        if (link_error) {
            error =
                "Unable to inspect previous build link: "
                + link_error.message();

            return false;
        }

        if (
            !target.is_absolute()
            &&
            target.parent_path().empty()
            &&
            isManagedReleaseName(
                target.filename().string()
            )
        ) {
            managed_target =
                repository_path /
                target;
        }

        std::error_code remove_link_error;

        std::filesystem::remove(
            previous_build,
            remove_link_error
        );

        if (remove_link_error) {
            error =
                "Unable to remove previous build link: "
                + remove_link_error.message();

            return false;
        }
    }
    else {
        std::error_code remove_error;

        std::filesystem::remove_all(
            previous_build,
            remove_error
        );

        if (remove_error) {
            error =
                "Unable to remove previous build: "
                + remove_error.message();

            return false;
        }
    }

    if (!managed_target.empty()) {
        std::error_code remove_target_error;

        std::filesystem::remove_all(
            managed_target,
            remove_target_error
        );

        if (remove_target_error) {
            error =
                "Unable to remove previous release directory: "
                + remove_target_error.message();

            return false;
        }
    }

    return true;
}

bool BuildActivator::activate(
    const std::filesystem::path& repository_path,
    const std::filesystem::path& tested_build,
    std::string& error
)
{
    const auto canonical_repository =
        std::filesystem::weakly_canonical(
            repository_path
        );

    const auto canonical_tested =
        std::filesystem::weakly_canonical(
            tested_build
        );

    if (
        canonical_tested.parent_path() !=
            canonical_repository
        ||
        !isManagedReleaseName(
            canonical_tested.filename().string()
        )
        ||
        !std::filesystem::is_directory(
            canonical_tested
        )
        ||
        !std::filesystem::is_regular_file(
            canonical_tested /
            "home-ai-core"
        )
    ) {
        error =
            "Tested build is not a valid managed release directory.";

        return false;
    }

    const auto active_build =
        canonical_repository /
        "build";

    const auto previous_build =
        canonical_repository /
        "build-prev";

    if (
        !removePrevious(
            canonical_repository,
            previous_build,
            error
        )
    ) {
        return false;
    }

    std::error_code active_status_error;

    auto active_status =
        std::filesystem::symlink_status(
            active_build,
            active_status_error
        );

    if (active_status_error) {
        if (
            active_status_error ==
            std::errc::no_such_file_or_directory
        ) {
            active_status =
                std::filesystem::file_status(
                    std::filesystem::file_type::not_found
                );
        }
        else {
            error =
                "Unable to inspect active build: "
                + active_status_error.message();

            return false;
        }
    }

    const bool had_active =
        active_status.type() !=
            std::filesystem::file_type::not_found;

    if (had_active) {
        std::error_code move_error;

        std::filesystem::rename(
            active_build,
            previous_build,
            move_error
        );

        if (move_error) {
            error =
                "Unable to preserve active build: "
                + move_error.message();

            return false;
        }
    }

    std::error_code link_error;

    std::filesystem::create_directory_symlink(
        canonical_tested.filename(),
        active_build,
        link_error
    );

    if (link_error) {
        if (had_active) {
            std::error_code restore_error;

            std::filesystem::rename(
                previous_build,
                active_build,
                restore_error
            );
        }

        error =
            "Unable to activate tested build: "
            + link_error.message();

        return false;
    }

    error.clear();
    return true;
}

}
