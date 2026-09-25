#pragma once

#include <cstdint>
#include <optional>
#include <string>
#include <unordered_map>
#include <vector>

struct sqlite3;

namespace homeai {

struct DatabaseUser {
    std::int64_t id{0};
    std::string username;
    std::string role;
    bool enabled{true};
    int iterations{310000};
    std::vector<unsigned char> salt;
    std::vector<unsigned char> password_hash;
    std::int64_t created_at{0};
    std::int64_t updated_at{0};
    std::int64_t last_login_at{0};
};

struct DatabaseSession {
    std::int64_t id{0};
    std::int64_t user_id{0};
    std::string username;
    std::string role;
    bool user_enabled{true};
    std::string token_hash;
    std::int64_t created_at{0};
    std::int64_t expires_at{0};
    std::int64_t last_seen_at{0};
};

struct AuditEntry {
    std::int64_t id{0};
    std::int64_t created_at{0};
    std::string event;
    std::string username;
    std::string details;
};

class UserDatabase {
public:
    UserDatabase() = default;
    ~UserDatabase();

    UserDatabase(
        const UserDatabase&
    ) = delete;

    UserDatabase& operator=(
        const UserDatabase&
    ) = delete;

    bool open(
        const std::string& database_file,
        const std::string& legacy_users_file,
        const std::string& legacy_audit_file,
        std::string& error
    );

    void close();

    bool hasUsers(
        std::string& error
    ) const;

    std::optional<DatabaseUser>
    findUserByName(
        const std::string& username,
        std::string& error
    ) const;

    std::optional<DatabaseUser>
    findUserById(
        std::int64_t user_id,
        std::string& error
    ) const;

    std::vector<DatabaseUser>
    listUsers(
        std::string& error
    ) const;

    bool insertUser(
        const DatabaseUser& user,
        std::string& error
    );

    bool updateUser(
        std::int64_t user_id,
        const std::string& role,
        bool enabled,
        std::int64_t updated_at,
        std::string& error
    );

    bool updatePassword(
        std::int64_t user_id,
        int iterations,
        const std::vector<unsigned char>& salt,
        const std::vector<unsigned char>& hash,
        std::int64_t updated_at,
        std::string& error
    );

    bool deleteUser(
        std::int64_t user_id,
        std::string& error
    );

    std::int64_t countEnabledAdmins(
        std::string& error
    ) const;

    bool updateLastLogin(
        std::int64_t user_id,
        std::int64_t timestamp,
        std::string& error
    );

    std::unordered_map<std::string, int>
    permissionOverrides(
        std::int64_t user_id,
        std::string& error
    ) const;

    bool setPermissionOverride(
        std::int64_t user_id,
        const std::string& permission,
        int decision,
        std::string& error
    );

    bool clearPermissionOverride(
        std::int64_t user_id,
        const std::string& permission,
        std::string& error
    );

    bool createSession(
        const std::string& token_hash,
        std::int64_t user_id,
        std::int64_t created_at,
        std::int64_t expires_at,
        std::string& error
    );

    std::optional<DatabaseSession>
    findSession(
        const std::string& token_hash,
        std::string& error
    ) const;

    bool touchSession(
        const std::string& token_hash,
        std::int64_t timestamp,
        std::string& error
    );

    bool deleteSession(
        const std::string& token_hash,
        std::string& error
    );

    bool deleteSessionById(
        std::int64_t session_id,
        std::string& error
    );

    bool deleteSessionsForUser(
        std::int64_t user_id,
        std::string& error
    );

    bool deleteExpiredSessions(
        std::int64_t now,
        std::string& error
    );

    std::vector<DatabaseSession>
    listSessions(
        std::optional<std::int64_t> user_id,
        std::string& error
    ) const;

    bool appendAudit(
        std::int64_t timestamp,
        const std::string& event,
        const std::string& username,
        const std::string& details,
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

    const std::string&
    databaseFile() const
    {
        return database_file_;
    }

private:
    bool configure(
        std::string& error
    );

    bool migrateSchema(
        std::string& error
    );

    bool migrateLegacyUsers(
        const std::string& legacy_users_file,
        std::string& error
    );

    bool migrateLegacyAudit(
        const std::string& legacy_audit_file,
        std::string& error
    );

    bool integrityCheck(
        std::string& error
    ) const;

    bool backupBeforeMigration(
        std::string& error
    ) const;

    bool exec(
        const std::string& sql,
        std::string& error
    ) const;

    static std::int64_t unixNow();

    sqlite3* database_{nullptr};
    std::string database_file_;
};

}
