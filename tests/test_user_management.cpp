#include "security/auth/SecurityManager.h"

#include <openssl/evp.h>

#include <filesystem>
#include <fstream>
#include <iostream>
#include <string>
#include <vector>

namespace {

std::string hex(
    const std::vector<unsigned char>& bytes
)
{
    static constexpr char digits[] =
        "0123456789abcdef";

    std::string result;

    for (const auto value : bytes) {
        result.push_back(
            digits[
                (value >> 4)
                &
                0x0f
            ]
        );

        result.push_back(
            digits[
                value
                &
                0x0f
            ]
        );
    }

    return result;
}

std::vector<unsigned char>
derive(
    const std::string& password,
    const std::vector<unsigned char>& salt,
    int iterations
)
{
    std::vector<unsigned char>
        hash(32);

    if (
        PKCS5_PBKDF2_HMAC(
            password.data(),
            static_cast<int>(
                password.size()
            ),
            salt.data(),
            static_cast<int>(
                salt.size()
            ),
            iterations,
            EVP_sha256(),
            static_cast<int>(
                hash.size()
            ),
            hash.data()
        ) != 1
    ) {
        return {};
    }

    return hash;
}

bool check(
    bool value,
    const std::string& message
)
{
    if (!value)
        std::cerr << message << '\n';

    return value;
}

}

int main()
{
    namespace fs =
        std::filesystem;

    const auto root =
        fs::temp_directory_path()
        /
        "home-ai-user-management-test";

    fs::remove_all(root);
    fs::create_directories(root);

    const auto database =
        root / "security.db";

    const auto legacy_users =
        root / "users.db";

    const auto legacy_audit =
        root / "audit.log";

    const std::string
        legacy_password =
            "Legacy-Password-123!";

    const int iterations =
        310000;

    const std::vector<unsigned char>
        salt = {
            0x00, 0x11, 0x22, 0x33,
            0x44, 0x55, 0x66, 0x77,
            0x88, 0x99, 0xaa, 0xbb,
            0xcc, 0xdd, 0xee, 0xff
        };

    const auto hash =
        derive(
            legacy_password,
            salt,
            iterations
        );

    if (
        !check(
            !hash.empty(),
            "Unable to prepare legacy password"
        )
    ) {
        return 1;
    }

    {
        std::ofstream file(
            legacy_users
        );

        file
            << "legacy-admin"
            << '\t'
            << "admin"
            << '\t'
            << iterations
            << '\t'
            << hex(salt)
            << '\t'
            << hex(hash)
            << '\n';
    }

    {
        std::ofstream file(
            legacy_audit
        );

        file
            << "2026-01-01 00:00:00"
            << '\t'
            << "legacy.event"
            << '\t'
            << "legacy-admin"
            << '\t'
            << "legacy audit"
            << '\n';
    }

    homeai::SecurityManager security;

    if (
        !check(
            security.initialize(
                database.string(),
                legacy_users.string(),
                legacy_audit.string()
            ),
            "Security database initialization failed"
        )
    ) {
        return 1;
    }

    if (
        !check(
            fs::exists(database),
            "Central SQLite database was not created"
        )
        ||
        !check(
            fs::exists(
                legacy_users.string()
                + ".migrated"
            ),
            "Legacy users database was not archived"
        )
        ||
        !check(
            fs::exists(
                legacy_audit.string()
                + ".migrated"
            ),
            "Legacy audit log was not archived"
        )
    ) {
        return 1;
    }

    std::string error;
    homeai::SessionInfo
        legacy_session;

    const auto legacy_token =
        security.login(
            "legacy-admin",
            legacy_password,
            legacy_session,
            error
        );

    if (
        !check(
            legacy_token.has_value(),
            "Migrated password did not authenticate"
        )
        ||
        !check(
            security.hasPermission(
                legacy_session,
                "users.manage"
            ),
            "Admin role does not include users.manage"
        )
    ) {
        return 1;
    }

    if (
        !check(
            security.createUser(
                "viewer-user",
                "Viewer-Password-123!",
                homeai::UserRole::Viewer,
                error
            ),
            "Unable to create viewer user: "
                + error
        )
    ) {
        return 1;
    }

    auto users =
        security.listUsers(error);

    if (
        !check(
            error.empty()
            &&
            users.size() == 2,
            "User list is invalid"
        )
    ) {
        return 1;
    }

    std::int64_t legacy_id = 0;
    std::int64_t viewer_id = 0;

    for (const auto& user : users) {
        if (
            user.username ==
            "legacy-admin"
        ) {
            legacy_id = user.id;
        }

        if (
            user.username ==
            "viewer-user"
        ) {
            viewer_id = user.id;
        }
    }

    if (
        !check(
            legacy_id > 0
            &&
            viewer_id > 0,
            "User IDs were not assigned"
        )
    ) {
        return 1;
    }

    error.clear();

    if (
        !check(
            !security.
                setPermissionOverride(
                    legacy_id,
                    "users.manage",
                    -1,
                    error
                ),
            "Last administrator users.manage deny was accepted"
        )
    ) {
        return 1;
    }

    error.clear();

    if (
        !check(
            security.
                setPermissionOverride(
                    viewer_id,
                    "files.write",
                    1,
                    error
                ),
            "Viewer permission override failed: "
                + error
        )
    ) {
        return 1;
    }

    homeai::SessionInfo
        viewer_session;

    const auto viewer_token =
        security.login(
            "viewer-user",
            "Viewer-Password-123!",
            viewer_session,
            error
        );

    if (
        !check(
            viewer_token.has_value(),
            "Viewer login failed"
        )
        ||
        !check(
            security.hasPermission(
                viewer_session,
                "files.write"
            ),
            "Allow permission override was not applied"
        )
        ||
        !check(
            !security.hasPermission(
                viewer_session,
                "users.manage"
            ),
            "Viewer unexpectedly received users.manage"
        )
    ) {
        return 1;
    }

    error.clear();

    if (
        !check(
            !security.updateUser(
                legacy_id,
                homeai::UserRole::Operator,
                true,
                error
            ),
            "Last administrator was demoted"
        )
    ) {
        return 1;
    }

    error.clear();

    if (
        !check(
            security.updateUser(
                viewer_id,
                homeai::UserRole::Admin,
                true,
                error
            ),
            "Unable to promote second administrator: "
                + error
        )
        ||
        !check(
            security.updateUser(
                legacy_id,
                homeai::UserRole::Operator,
                true,
                error
            ),
            "Unable to demote administrator after replacement: "
                + error
        )
    ) {
        return 1;
    }

    homeai::SessionInfo
        admin_session;

    const auto admin_token =
        security.login(
            "viewer-user",
            "Viewer-Password-123!",
            admin_session,
            error
        );

    if (
        !check(
            admin_token.has_value(),
            "Promoted administrator login failed"
        )
    ) {
        return 1;
    }

    auto sessions =
        security.listSessions(
            std::nullopt,
            error
        );

    if (
        !check(
            error.empty()
            &&
            !sessions.empty(),
            "Session inventory is empty"
        )
    ) {
        return 1;
    }

    homeai::SecurityManager
        reloaded;

    if (
        !check(
            reloaded.initialize(
                database.string(),
                legacy_users.string(),
                legacy_audit.string()
            ),
            "Reloaded Security Manager failed"
        )
        ||
        !check(
            reloaded.validateSession(
                *admin_token
            ).has_value(),
            "Persistent session did not survive manager reload"
        )
    ) {
        return 1;
    }

    const auto audit =
        reloaded.listAudit(
            200,
            0,
            "",
            "",
            error
        );

    bool legacy_audit_found =
        false;

    for (const auto& entry : audit) {
        if (
            entry.event ==
            "legacy.event"
        ) {
            legacy_audit_found =
                true;
        }
    }

    if (
        !check(
            error.empty()
            &&
            legacy_audit_found,
            "Legacy audit event was not migrated"
        )
    ) {
        return 1;
    }

    error.clear();

    if (
        !check(
            !reloaded.deleteUser(
                viewer_id,
                error
            ),
            "Last enabled administrator was deleted"
        )
    ) {
        return 1;
    }

    error.clear();

    if (
        !check(
            reloaded.setPassword(
                legacy_id,
                "New-Legacy-Password-123!",
                error
            ),
            "Password reset failed: "
                + error
        )
    ) {
        return 1;
    }

    homeai::SessionInfo
        old_password_info;

    if (
        !check(
            !reloaded.login(
                "legacy-admin",
                legacy_password,
                old_password_info,
                error
            ),
            "Old password remained valid"
        )
    ) {
        return 1;
    }

    homeai::SessionInfo
        new_password_info;

    error.clear();

    if (
        !check(
            reloaded.login(
                "legacy-admin",
                "New-Legacy-Password-123!",
                new_password_info,
                error
            ).has_value(),
            "New password was not accepted"
        )
    ) {
        return 1;
    }

    fs::remove_all(root);

    std::cout
        << "User management database test passed\n";

    return 0;
}
