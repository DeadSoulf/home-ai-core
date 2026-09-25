#include "security/auth/SecurityManager.h"

#include <openssl/crypto.h>
#include <openssl/evp.h>
#include <openssl/rand.h>

#include <algorithm>
#include <array>
#include <cctype>
#include <chrono>
#include <filesystem>
#include <unordered_set>

namespace homeai {

namespace {

constexpr int password_iterations =
    310000;

constexpr std::int64_t session_seconds =
    8 * 60 * 60;

const std::array<unsigned char, 16>
    dummy_salt = {
        0x48, 0x6f, 0x6d, 0x65,
        0x41, 0x49, 0x2d, 0x53,
        0x65, 0x63, 0x75, 0x72,
        0x69, 0x74, 0x79, 0x21
    };

std::string sanitizeAudit(
    std::string value
)
{
    std::replace(
        value.begin(),
        value.end(),
        '\n',
        ' '
    );

    std::replace(
        value.begin(),
        value.end(),
        '\r',
        ' '
    );

    std::replace(
        value.begin(),
        value.end(),
        '\t',
        ' '
    );

    if (value.size() > 4096)
        value.resize(4096);

    return value;
}

std::unordered_set<std::string>
defaultsForRole(
    UserRole role
)
{
    const auto& catalog =
        SecurityManager::
            permissionCatalog();

    if (role == UserRole::Admin) {
        return {
            catalog.begin(),
            catalog.end()
        };
    }

    std::unordered_set<std::string>
        result = {
            "files.read",
            "cameras.view",
            "smart_home.view",
            "ai.use",
            "system.view",
            "storage.view",
            "network.view",
            "automation.view"
        };

    if (role == UserRole::Operator) {
        result.insert(
            "files.write"
        );

        result.insert(
            "cameras.manage"
        );

        result.insert(
            "smart_home.control"
        );

        result.insert(
            "automation.manage"
        );
    }

    return result;
}

}

bool SecurityManager::initialize(
    const std::string& database_file,
    const std::string& legacy_users_file,
    const std::string& legacy_audit_file
)
{
    std::string error;

    if (
        !database_.open(
            database_file,
            legacy_users_file,
            legacy_audit_file,
            error
        )
    ) {
        return false;
    }

    database_.deleteExpiredSessions(
        unixNow(),
        error
    );

    audit(
        "security.init",
        "system",
        "Security Core initialized"
    );

    return true;
}

bool SecurityManager::initialize(
    const std::string& users_file,
    const std::string& audit_file
)
{
    return initialize(
        users_file + ".sqlite3",
        users_file,
        audit_file
    );
}

bool SecurityManager::hasUsers() const
{
    std::string error;

    return database_.hasUsers(error);
}

bool SecurityManager::createUser(
    const std::string& username,
    const std::string& password,
    UserRole role,
    std::string& error
)
{
    if (!validUsername(username)) {
        error =
            "Username must be 3-32 characters";

        return false;
    }

    if (
        !validatePassword(
            password,
            error
        )
    ) {
        return false;
    }

    std::vector<unsigned char>
        salt(16);

    if (
        RAND_bytes(
            salt.data(),
            static_cast<int>(
                salt.size()
            )
        ) != 1
    ) {
        error =
            "Unable to generate salt";

        return false;
    }

    auto hash =
        derivePassword(
            password,
            salt,
            password_iterations
        );

    if (hash.empty()) {
        error =
            "Password hashing failed";

        return false;
    }

    const auto now =
        unixNow();

    DatabaseUser user;

    user.username =
        username;

    user.role =
        roleToString(role);

    user.enabled =
        true;

    user.iterations =
        password_iterations;

    user.salt =
        std::move(salt);

    user.password_hash =
        std::move(hash);

    user.created_at = now;
    user.updated_at = now;

    {
        std::lock_guard<std::mutex>
            lock(mutation_mutex_);

        std::string lookup_error;

        if (
            database_.findUserByName(
                username,
                lookup_error
            )
        ) {
            error =
                "User already exists";

            return false;
        }

        if (!lookup_error.empty()) {
            error = lookup_error;
            return false;
        }

        if (
            !database_.insertUser(
                user,
                error
            )
        ) {
            return false;
        }
    }

    audit(
        "user.create",
        username,
        "role=" +
            roleToString(role)
    );

    return true;
}

bool SecurityManager::updateUser(
    std::int64_t user_id,
    UserRole role,
    bool enabled,
    std::string& error
)
{
    std::lock_guard<std::mutex>
        lock(mutation_mutex_);

    auto user =
        database_.findUserById(
            user_id,
            error
        );

    if (!user) {
        if (error.empty())
            error = "User not found";

        return false;
    }

    const bool removes_admin =
        user->role == "admin"
        &&
        user->enabled
        &&
        (
            role != UserRole::Admin
            ||
            !enabled
        );

    if (removes_admin) {
        const bool last_admin =
            isLastEnabledAdmin(
                *user,
                error
            );

        if (!error.empty())
            return false;

        if (last_admin) {
            error =
                "The last enabled administrator cannot be disabled or demoted.";

            return false;
        }
    }

    if (
        !database_.updateUser(
            user_id,
            roleToString(role),
            enabled,
            unixNow(),
            error
        )
    ) {
        return false;
    }

    std::string ignored;

    database_.deleteSessionsForUser(
        user_id,
        ignored
    );

    audit(
        "user.update",
        user->username,
        "role=" +
            roleToString(role)
            +
            " enabled=" +
            (
                enabled
                ? "true"
                : "false"
            )
    );

    return true;
}

bool SecurityManager::deleteUser(
    std::int64_t user_id,
    std::string& error
)
{
    std::lock_guard<std::mutex>
        lock(mutation_mutex_);

    auto user =
        database_.findUserById(
            user_id,
            error
        );

    if (!user) {
        if (error.empty())
            error = "User not found";

        return false;
    }

    if (
        user->role == "admin"
        &&
        user->enabled
    ) {
        const bool last_admin =
            isLastEnabledAdmin(
                *user,
                error
            );

        if (!error.empty())
            return false;

        if (last_admin) {
            error =
                "The last enabled administrator cannot be deleted.";

            return false;
        }
    }

    if (
        !database_.deleteUser(
            user_id,
            error
        )
    ) {
        return false;
    }

    audit(
        "user.delete",
        user->username,
        "user deleted"
    );

    return true;
}

bool SecurityManager::setPassword(
    std::int64_t user_id,
    const std::string& password,
    std::string& error
)
{
    if (
        !validatePassword(
            password,
            error
        )
    ) {
        return false;
    }

    std::vector<unsigned char>
        salt(16);

    if (
        RAND_bytes(
            salt.data(),
            static_cast<int>(
                salt.size()
            )
        ) != 1
    ) {
        error =
            "Unable to generate salt";

        return false;
    }

    auto hash =
        derivePassword(
            password,
            salt,
            password_iterations
        );

    if (hash.empty()) {
        error =
            "Password hashing failed";

        return false;
    }

    std::lock_guard<std::mutex>
        lock(mutation_mutex_);

    auto user =
        database_.findUserById(
            user_id,
            error
        );

    if (!user) {
        if (error.empty())
            error = "User not found";

        return false;
    }

    if (
        !database_.updatePassword(
            user_id,
            password_iterations,
            salt,
            hash,
            unixNow(),
            error
        )
    ) {
        return false;
    }

    std::string ignored;

    database_.deleteSessionsForUser(
        user_id,
        ignored
    );

    audit(
        "user.password",
        user->username,
        "password changed and sessions revoked"
    );

    return true;
}

bool SecurityManager::setPermissionOverride(
    std::int64_t user_id,
    const std::string& permission,
    int decision,
    std::string& error
)
{
    if (!validPermission(permission)) {
        error =
            "Unknown permission";

        return false;
    }

    if (
        decision < -1
        ||
        decision > 1
    ) {
        error =
            "Invalid permission decision";

        return false;
    }

    std::lock_guard<std::mutex>
        lock(mutation_mutex_);

    auto user =
        database_.findUserById(
            user_id,
            error
        );

    if (!user) {
        if (error.empty())
            error = "User not found";

        return false;
    }

    if (
        permission ==
            "users.manage"
        &&
        decision == -1
        &&
        user->role == "admin"
        &&
        user->enabled
    ) {
        const bool last_admin =
            isLastEnabledAdmin(
                *user,
                error
            );

        if (!error.empty())
            return false;

        if (last_admin) {
            error =
                "users.manage cannot be denied for the last enabled administrator.";

            return false;
        }
    }

    bool success = false;

    if (decision == 0) {
        success =
            database_.
                clearPermissionOverride(
                    user_id,
                    permission,
                    error
                );
    }
    else {
        success =
            database_.
                setPermissionOverride(
                    user_id,
                    permission,
                    decision,
                    error
                );
    }

    if (!success)
        return false;

    audit(
        "user.permission",
        user->username,
        permission +
            "=" +
            std::to_string(
                decision
            )
    );

    return true;
}

std::vector<UserInfo>
SecurityManager::listUsers(
    std::string& error
) const
{
    std::vector<UserInfo>
        result;

    const auto users =
        database_.listUsers(
            error
        );

    if (!error.empty())
        return result;

    result.reserve(
        users.size()
    );

    for (const auto& user : users) {
        auto info =
            toUserInfo(
                user,
                error
            );

        if (!error.empty()) {
            result.clear();
            return result;
        }

        result.push_back(
            std::move(info)
        );
    }

    return result;
}

std::optional<UserInfo>
SecurityManager::userById(
    std::int64_t user_id,
    std::string& error
) const
{
    auto user =
        database_.findUserById(
            user_id,
            error
        );

    if (!user)
        return std::nullopt;

    return toUserInfo(
        *user,
        error
    );
}

std::optional<std::string>
SecurityManager::login(
    const std::string& username,
    const std::string& password,
    SessionInfo& session_info,
    std::string& error
)
{
    const auto steady_now =
        std::chrono::
            steady_clock::now();

    {
        std::lock_guard<std::mutex>
            lock(failure_mutex_);

        cleanupFailures(
            steady_now
        );

        auto& failure =
            failures_[username];

        if (
            steady_now <
            failure.locked_until
        ) {
            error =
                "Too many failed attempts";

            return std::nullopt;
        }
    }

    std::string database_error;

    auto user =
        database_.findUserByName(
            username,
            database_error
        );

    if (!database_error.empty()) {
        error =
            "Authentication database error";

        return std::nullopt;
    }

    std::vector<unsigned char>
        candidate;

    bool valid = false;

    if (user) {
        candidate =
            derivePassword(
                password,
                user->salt,
                user->iterations
            );

        valid =
            user->enabled
            &&
            candidate.size() ==
                user->password_hash.size()
            &&
            !candidate.empty()
            &&
            CRYPTO_memcmp(
                candidate.data(),
                user->password_hash.data(),
                user->password_hash.size()
            ) == 0;
    }
    else {
        std::vector<unsigned char>
            salt(
                dummy_salt.begin(),
                dummy_salt.end()
            );

        candidate =
            derivePassword(
                password,
                salt,
                password_iterations
            );

        valid = false;
    }

    if (!valid) {
        {
            std::lock_guard<std::mutex>
                lock(failure_mutex_);

            auto& failure =
                failures_[username];

            ++failure.failures;

            if (
                failure.failures >= 5
            ) {
                failure.failures = 0;

                failure.locked_until =
                    std::chrono::
                        steady_clock::now()
                    +
                    std::chrono::
                        seconds(60);
            }
        }

        audit(
            "login.failed",
            username,
            user && !user->enabled
                ? "account disabled"
                : "invalid credentials"
        );

        error =
            "Invalid username or password";

        return std::nullopt;
    }

    const auto role =
        roleFromString(
            user->role
        );

    if (!role) {
        error =
            "Invalid user role";

        return std::nullopt;
    }

    const auto token =
        randomHex(32);

    if (token.empty()) {
        error =
            "Unable to create session";

        return std::nullopt;
    }

    const auto hash =
        tokenHash(token);

    if (hash.empty()) {
        error =
            "Unable to protect session token";

        return std::nullopt;
    }

    const auto now =
        unixNow();

    database_.deleteExpiredSessions(
        now,
        database_error
    );

    if (
        !database_.createSession(
            hash,
            user->id,
            now,
            now + session_seconds,
            database_error
        )
    ) {
        error =
            "Unable to create session";

        return std::nullopt;
    }

    database_.updateLastLogin(
        user->id,
        now,
        database_error
    );

    {
        std::lock_guard<std::mutex>
            lock(failure_mutex_);

        failures_.erase(
            username
        );
    }

    session_info = {
        user->id,
        user->username,
        *role
    };

    audit(
        "login.success",
        username,
        "role=" +
            user->role
    );

    return token;
}

std::optional<SessionInfo>
SecurityManager::validateSession(
    const std::string& token
)
{
    if (token.empty())
        return std::nullopt;

    const auto hash =
        tokenHash(token);

    if (hash.empty())
        return std::nullopt;

    std::string error;

    auto session =
        database_.findSession(
            hash,
            error
        );

    if (
        !session
        ||
        !error.empty()
    ) {
        return std::nullopt;
    }

    const auto now =
        unixNow();

    if (
        !session->user_enabled
        ||
        now >= session->expires_at
    ) {
        database_.deleteSession(
            hash,
            error
        );

        return std::nullopt;
    }

    const auto role =
        roleFromString(
            session->role
        );

    if (!role) {
        database_.deleteSession(
            hash,
            error
        );

        return std::nullopt;
    }

    if (
        now -
            session->last_seen_at
        >= 300
    ) {
        database_.touchSession(
            hash,
            now,
            error
        );
    }

    return SessionInfo{
        session->user_id,
        session->username,
        *role
    };
}

void SecurityManager::logout(
    const std::string& token
)
{
    if (token.empty())
        return;

    const auto hash =
        tokenHash(token);

    if (hash.empty())
        return;

    std::string error;

    auto session =
        database_.findSession(
            hash,
            error
        );

    database_.deleteSession(
        hash,
        error
    );

    if (session) {
        audit(
            "logout",
            session->username,
            "session closed"
        );
    }
}

std::vector<SessionRecord>
SecurityManager::listSessions(
    std::optional<std::int64_t> user_id,
    std::string& error
) const
{
    std::vector<SessionRecord>
        result;

    const auto sessions =
        database_.listSessions(
            user_id,
            error
        );

    if (!error.empty())
        return result;

    result.reserve(
        sessions.size()
    );

    for (
        const auto& session :
        sessions
    ) {
        result.push_back(
            {
                session.id,
                session.user_id,
                session.username,
                session.created_at,
                session.expires_at,
                session.last_seen_at
            }
        );
    }

    return result;
}

bool SecurityManager::revokeSession(
    std::int64_t session_id,
    std::string& error
)
{
    return
        database_.
            deleteSessionById(
                session_id,
                error
            );
}

bool SecurityManager::revokeUserSessions(
    std::int64_t user_id,
    std::string& error
)
{
    return
        database_.
            deleteSessionsForUser(
                user_id,
                error
            );
}

std::vector<AuditEntry>
SecurityManager::listAudit(
    int limit,
    int offset,
    const std::string& username,
    const std::string& event,
    std::string& error
) const
{
    return database_.listAudit(
        limit,
        offset,
        username,
        event,
        error
    );
}

bool SecurityManager::hasPermission(
    const SessionInfo& session,
    const std::string& permission
) const
{
    if (!validPermission(permission))
        return false;

    std::string error;

    auto user =
        database_.findUserById(
            session.user_id,
            error
        );

    if (
        !user
        ||
        !error.empty()
        ||
        !user->enabled
    ) {
        return false;
    }

    auto role =
        roleFromString(
            user->role
        );

    if (!role)
        return false;

    const auto overrides =
        database_.
            permissionOverrides(
                user->id,
                error
            );

    if (!error.empty())
        return false;

    const auto override_it =
        overrides.find(
            permission
        );

    if (
        override_it !=
        overrides.end()
    ) {
        return
            override_it->second > 0;
    }

    const auto defaults =
        defaultsForRole(
            *role
        );

    return
        defaults.contains(
            permission
        );
}

bool SecurityManager::isAdmin(
    UserRole role
) const
{
    return
        role ==
        UserRole::Admin;
}

std::string SecurityManager::roleToString(
    UserRole role
)
{
    switch (role) {
        case UserRole::Admin:
            return "admin";
        case UserRole::Operator:
            return "operator";
        case UserRole::Viewer:
            return "viewer";
    }

    return "viewer";
}

std::optional<UserRole>
SecurityManager::roleFromString(
    const std::string& value
)
{
    if (value == "admin")
        return UserRole::Admin;

    if (value == "operator")
        return UserRole::Operator;

    if (value == "viewer")
        return UserRole::Viewer;

    return std::nullopt;
}

const std::vector<std::string>&
SecurityManager::permissionCatalog()
{
    static const std::vector<std::string>
        permissions = {
            "files.read",
            "files.write",
            "files.manage",
            "cameras.view",
            "cameras.manage",
            "smart_home.view",
            "smart_home.control",
            "smart_home.manage",
            "ai.use",
            "ai.manage",
            "users.view",
            "users.manage",
            "system.view",
            "system.manage",
            "storage.view",
            "storage.manage",
            "network.view",
            "network.manage",
            "hypervisor.view",
            "hypervisor.manage",
            "automation.view",
            "automation.manage"
        };

    return permissions;
}

std::vector<std::string>
SecurityManager::roleDefaultPermissions(
    UserRole role
)
{
    const auto values =
        defaultsForRole(role);

    std::vector<std::string>
        result(
            values.begin(),
            values.end()
        );

    std::sort(
        result.begin(),
        result.end()
    );

    return result;
}

void SecurityManager::audit(
    const std::string& event,
    const std::string& username,
    const std::string& details
)
{
    std::string error;

    database_.appendAudit(
        unixNow(),
        sanitizeAudit(event),
        sanitizeAudit(username),
        sanitizeAudit(details),
        error
    );
}

bool SecurityManager::validUsername(
    const std::string& username
)
{
    if (
        username.size() < 3
        ||
        username.size() > 32
    ) {
        return false;
    }

    return std::all_of(
        username.begin(),
        username.end(),
        [](unsigned char c) {
            return
                std::isalnum(c)
                ||
                c == '.'
                ||
                c == '_'
                ||
                c == '-';
        }
    );
}

bool SecurityManager::validPermission(
    const std::string& permission
)
{
    const auto& catalog =
        permissionCatalog();

    return
        std::find(
            catalog.begin(),
            catalog.end(),
            permission
        ) != catalog.end();
}

bool SecurityManager::validatePassword(
    const std::string& password,
    std::string& error
)
{
    if (password.size() < 12) {
        error =
            "Password must contain at least 12 characters";

        return false;
    }

    if (password.size() > 256) {
        error =
            "Password is too long";

        return false;
    }

    return true;
}

std::vector<unsigned char>
SecurityManager::derivePassword(
    const std::string& password,
    const std::vector<unsigned char>& salt,
    int iterations
)
{
    std::vector<unsigned char>
        result(32);

    const int success =
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
                result.size()
            ),
            result.data()
        );

    if (success != 1)
        return {};

    return result;
}

std::string SecurityManager::bytesToHex(
    const std::vector<unsigned char>& bytes
)
{
    static constexpr char
        digits[] =
            "0123456789abcdef";

    std::string result;

    result.reserve(
        bytes.size() * 2
    );

    for (const auto byte : bytes) {
        result.push_back(
            digits[
                (byte >> 4)
                &
                0x0f
            ]
        );

        result.push_back(
            digits[
                byte
                &
                0x0f
            ]
        );
    }

    return result;
}

std::string SecurityManager::randomHex(
    std::size_t bytes
)
{
    std::vector<unsigned char>
        data(bytes);

    if (
        RAND_bytes(
            data.data(),
            static_cast<int>(
                data.size()
            )
        ) != 1
    ) {
        return {};
    }

    return bytesToHex(data);
}

std::string SecurityManager::tokenHash(
    const std::string& token
)
{
    std::array<unsigned char, 32>
        digest{};

    unsigned int length = 0;

    if (
        EVP_Digest(
            token.data(),
            token.size(),
            digest.data(),
            &length,
            EVP_sha256(),
            nullptr
        ) != 1
        ||
        length != digest.size()
    ) {
        return {};
    }

    return bytesToHex(
        std::vector<unsigned char>(
            digest.begin(),
            digest.end()
        )
    );
}

std::int64_t SecurityManager::unixNow()
{
    return
        std::chrono::duration_cast<
            std::chrono::seconds
        >(
            std::chrono::
                system_clock::now()
                .time_since_epoch()
        ).count();
}

UserInfo SecurityManager::toUserInfo(
    const DatabaseUser& user,
    std::string& error
) const
{
    UserInfo result;

    const auto role =
        roleFromString(
            user.role
        );

    if (!role) {
        error =
            "Invalid role stored for user "
            + user.username;

        return result;
    }

    result.id =
        user.id;

    result.username =
        user.username;

    result.role =
        *role;

    result.enabled =
        user.enabled;

    result.created_at =
        user.created_at;

    result.updated_at =
        user.updated_at;

    result.last_login_at =
        user.last_login_at;

    result.permission_overrides =
        database_.
            permissionOverrides(
                user.id,
                error
            );

    if (!error.empty())
        return result;

    const auto defaults =
        defaultsForRole(
            *role
        );

    for (
        const auto& permission :
        permissionCatalog()
    ) {
        const auto override_it =
            result.permission_overrides.
                find(permission);

        const bool allowed =
            override_it !=
                result.permission_overrides.
                    end()
            ? override_it->second > 0
            : defaults.contains(
                permission
            );

        if (allowed) {
            result.effective_permissions.
                push_back(permission);
        }
    }

    return result;
}

bool SecurityManager::isLastEnabledAdmin(
    const DatabaseUser& user,
    std::string& error
) const
{
    if (
        user.role != "admin"
        ||
        !user.enabled
    ) {
        return false;
    }

    const auto overrides =
        database_.
            permissionOverrides(
                user.id,
                error
            );

    if (!error.empty())
        return false;

    const auto permission =
        overrides.find(
            "users.manage"
        );

    if (
        permission !=
            overrides.end()
        &&
        permission->second < 0
    ) {
        return false;
    }

    const auto count =
        database_.
            countManagingAdmins(
                error
            );

    if (count < 0)
        return false;

    return count <= 1;
}

void SecurityManager::cleanupFailures(
    const std::chrono::steady_clock::
        time_point& now
)
{
    for (
        auto iterator =
            failures_.begin();
        iterator !=
            failures_.end();
    ) {
        if (
            iterator->second.failures == 0
            &&
            now >=
                iterator->second.locked_until
        ) {
            iterator =
                failures_.erase(
                    iterator
                );
        }
        else {
            ++iterator;
        }
    }

    if (
        failures_.size() > 4096
    ) {
        failures_.clear();
    }
}

}
