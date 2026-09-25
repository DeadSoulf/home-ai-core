#pragma once

#include "security/auth/UserDatabase.h"

#include <chrono>
#include <cstdint>
#include <mutex>
#include <optional>
#include <string>
#include <unordered_map>
#include <vector>

namespace homeai {

enum class UserRole {
    Viewer,
    Operator,
    Admin
};

struct SessionInfo {
    std::int64_t user_id{0};
    std::string username;
    UserRole role{UserRole::Viewer};
};

struct UserInfo {
    std::int64_t id{0};
    std::string username;
    UserRole role{UserRole::Viewer};
    bool enabled{true};
    std::int64_t created_at{0};
    std::int64_t updated_at{0};
    std::int64_t last_login_at{0};

    std::unordered_map<
        std::string,
        int
    > permission_overrides;

    std::vector<std::string>
        effective_permissions;
};

struct SessionRecord {
    std::int64_t id{0};
    std::int64_t user_id{0};
    std::string username;
    std::int64_t created_at{0};
    std::int64_t expires_at{0};
    std::int64_t last_seen_at{0};
};

class SecurityManager {
public:
    bool initialize(
        const std::string& database_file,
        const std::string& legacy_users_file,
        const std::string& legacy_audit_file
    );

    bool initialize(
        const std::string& users_file,
        const std::string& audit_file
    );

    bool hasUsers() const;

    bool createUser(
        const std::string& username,
        const std::string& password,
        UserRole role,
        std::string& error
    );

    bool updateUser(
        std::int64_t user_id,
        UserRole role,
        bool enabled,
        std::string& error
    );

    bool deleteUser(
        std::int64_t user_id,
        std::string& error
    );

    bool setPassword(
        std::int64_t user_id,
        const std::string& password,
        std::string& error
    );

    bool setPermissionOverride(
        std::int64_t user_id,
        const std::string& permission,
        int decision,
        std::string& error
    );

    std::vector<UserInfo>
    listUsers(
        std::string& error
    ) const;

    std::optional<UserInfo>
    userById(
        std::int64_t user_id,
        std::string& error
    ) const;

    std::optional<std::string> login(
        const std::string& username,
        const std::string& password,
        SessionInfo& session_info,
        std::string& error
    );

    std::optional<SessionInfo> validateSession(
        const std::string& token
    );

    void logout(
        const std::string& token
    );

    std::vector<SessionRecord>
    listSessions(
        std::optional<std::int64_t> user_id,
        std::string& error
    ) const;

    bool revokeSession(
        std::int64_t session_id,
        std::string& error
    );

    bool revokeUserSessions(
        std::int64_t user_id,
        std::string& error
    );

    std::vector<AuditEntry>
    listAudit(
        int limit,
        int offset,
        const std::string& username,
        const std::string& event,
        std::string& error
    ) const;

    bool hasPermission(
        const SessionInfo& session,
        const std::string& permission
    ) const;

    bool isAdmin(
        UserRole role
    ) const;

    static std::string roleToString(
        UserRole role
    );

    static std::optional<UserRole>
    roleFromString(
        const std::string& value
    );

    static const std::vector<std::string>&
    permissionCatalog();

    static std::vector<std::string>
    roleDefaultPermissions(
        UserRole role
    );

    void audit(
        const std::string& event,
        const std::string& username,
        const std::string& details
    );

    const std::string&
    databaseFile() const
    {
        return database_.databaseFile();
    }

private:
    struct FailureState {
        int failures{0};
        std::chrono::steady_clock::
            time_point locked_until{};
    };

    static bool validUsername(
        const std::string& username
    );

    static bool validPermission(
        const std::string& permission
    );

    static bool validatePassword(
        const std::string& password,
        std::string& error
    );

    static std::vector<unsigned char>
    derivePassword(
        const std::string& password,
        const std::vector<unsigned char>& salt,
        int iterations
    );

    static std::string bytesToHex(
        const std::vector<unsigned char>& bytes
    );

    static std::string randomHex(
        std::size_t bytes
    );

    static std::string tokenHash(
        const std::string& token
    );

    static std::int64_t unixNow();

    UserInfo toUserInfo(
        const DatabaseUser& user,
        std::string& error
    ) const;

    bool isLastEnabledAdmin(
        const DatabaseUser& user,
        std::string& error
    ) const;

    void cleanupFailures(
        const std::chrono::steady_clock::
            time_point& now
    );

    mutable std::mutex
        failure_mutex_;

    mutable std::mutex
        mutation_mutex_;

    std::unordered_map<
        std::string,
        FailureState
    > failures_;

    UserDatabase database_;
};

}
