#include "security/auth/UserDatabase.h"

#include <sqlite3.h>

#include <algorithm>
#include <chrono>
#include <cctype>
#include <filesystem>
#include <fstream>
#include <sstream>
#include <sys/stat.h>

namespace homeai {

namespace {

constexpr int schema_version = 1;

std::string sqliteError(
    sqlite3* database,
    const std::string& prefix
)
{
    return
        prefix +
        ": " +
        (
            database
            ? sqlite3_errmsg(database)
            : "database is not open"
        );
}

class Statement {
public:
    Statement(
        sqlite3* database,
        const char* sql
    )
        : database_(database)
    {
        if (
            sqlite3_prepare_v2(
                database,
                sql,
                -1,
                &statement_,
                nullptr
            ) != SQLITE_OK
        ) {
            statement_ = nullptr;
        }
    }

    ~Statement()
    {
        if (statement_)
            sqlite3_finalize(statement_);
    }

    sqlite3_stmt* get() const
    {
        return statement_;
    }

    explicit operator bool() const
    {
        return statement_ != nullptr;
    }

private:
    sqlite3* database_{nullptr};
    sqlite3_stmt* statement_{nullptr};
};

bool bindText(
    sqlite3_stmt* statement,
    int index,
    const std::string& value
)
{
    return
        sqlite3_bind_text(
            statement,
            index,
            value.c_str(),
            static_cast<int>(
                value.size()
            ),
            SQLITE_TRANSIENT
        ) == SQLITE_OK;
}

bool bindBlob(
    sqlite3_stmt* statement,
    int index,
    const std::vector<unsigned char>& value
)
{
    return
        sqlite3_bind_blob(
            statement,
            index,
            value.data(),
            static_cast<int>(
                value.size()
            ),
            SQLITE_TRANSIENT
        ) == SQLITE_OK;
}

std::vector<unsigned char>
readBlob(
    sqlite3_stmt* statement,
    int column
)
{
    const auto* data =
        static_cast<const unsigned char*>(
            sqlite3_column_blob(
                statement,
                column
            )
        );

    const int size =
        sqlite3_column_bytes(
            statement,
            column
        );

    if (
        !data
        ||
        size <= 0
    ) {
        return {};
    }

    return std::vector<unsigned char>(
        data,
        data + size
    );
}

DatabaseUser readUser(
    sqlite3_stmt* statement,
    int offset = 0
)
{
    DatabaseUser user;

    user.id =
        sqlite3_column_int64(
            statement,
            offset + 0
        );

    const auto* username =
        sqlite3_column_text(
            statement,
            offset + 1
        );

    const auto* role =
        sqlite3_column_text(
            statement,
            offset + 2
        );

    user.username =
        username
        ? reinterpret_cast<
            const char*
          >(username)
        : "";

    user.role =
        role
        ? reinterpret_cast<
            const char*
          >(role)
        : "";

    user.enabled =
        sqlite3_column_int(
            statement,
            offset + 3
        ) != 0;

    user.iterations =
        sqlite3_column_int(
            statement,
            offset + 4
        );

    user.salt =
        readBlob(
            statement,
            offset + 5
        );

    user.password_hash =
        readBlob(
            statement,
            offset + 6
        );

    user.created_at =
        sqlite3_column_int64(
            statement,
            offset + 7
        );

    user.updated_at =
        sqlite3_column_int64(
            statement,
            offset + 8
        );

    if (
        sqlite3_column_type(
            statement,
            offset + 9
        ) != SQLITE_NULL
    ) {
        user.last_login_at =
            sqlite3_column_int64(
                statement,
                offset + 9
            );
    }

    return user;
}

bool validLegacyRole(
    const std::string& role
)
{
    return
        role == "viewer"
        ||
        role == "operator"
        ||
        role == "admin";
}

bool hexToBytes(
    const std::string& hex,
    std::vector<unsigned char>& bytes
)
{
    if (
        hex.empty()
        ||
        hex.size() % 2 != 0
    ) {
        return false;
    }

    auto nibble =
        [](char c) -> int {
            if (
                c >= '0'
                &&
                c <= '9'
            ) {
                return c - '0';
            }

            if (
                c >= 'a'
                &&
                c <= 'f'
            ) {
                return c - 'a' + 10;
            }

            if (
                c >= 'A'
                &&
                c <= 'F'
            ) {
                return c - 'A' + 10;
            }

            return -1;
        };

    bytes.clear();
    bytes.reserve(
        hex.size() / 2
    );

    for (
        std::size_t index = 0;
        index < hex.size();
        index += 2
    ) {
        const int high =
            nibble(hex[index]);

        const int low =
            nibble(
                hex[index + 1]
            );

        if (
            high < 0
            ||
            low < 0
        ) {
            bytes.clear();
            return false;
        }

        bytes.push_back(
            static_cast<unsigned char>(
                (high << 4) | low
            )
        );
    }

    return true;
}

bool validLegacyUsername(
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

}

UserDatabase::~UserDatabase()
{
    close();
}

bool UserDatabase::open(
    const std::string& database_file,
    const std::string& legacy_users_file,
    const std::string& legacy_audit_file,
    std::string& error
)
{
    close();

    database_file_ =
        database_file;

    try {
        const auto parent =
            std::filesystem::path(
                database_file_
            ).parent_path();

        if (!parent.empty()) {
            std::filesystem::
                create_directories(
                    parent
                );

            ::chmod(
                parent.c_str(),
                0700
            );
        }
    }
    catch (
        const std::exception& exception
    ) {
        error =
            exception.what();

        return false;
    }

    if (
        sqlite3_open_v2(
            database_file_.c_str(),
            &database_,
            SQLITE_OPEN_READWRITE
            |
            SQLITE_OPEN_CREATE
            |
            SQLITE_OPEN_FULLMUTEX,
            nullptr
        ) != SQLITE_OK
    ) {
        error =
            sqliteError(
                database_,
                "Unable to open user database"
            );

        close();
        return false;
    }

    ::chmod(
        database_file_.c_str(),
        0600
    );

    if (
        !configure(error)
        ||
        !integrityCheck(error)
        ||
        !migrateSchema(error)
        ||
        !migrateLegacyUsers(
            legacy_users_file,
            error
        )
        ||
        !migrateLegacyAudit(
            legacy_audit_file,
            error
        )
    ) {
        close();
        return false;
    }

    return true;
}

void UserDatabase::close()
{
    if (database_) {
        sqlite3_close(database_);
        database_ = nullptr;
    }

    database_file_.clear();
}

bool UserDatabase::configure(
    std::string& error
)
{
    if (
        !exec(
            "PRAGMA foreign_keys=ON;",
            error
        )
        ||
        !exec(
            "PRAGMA journal_mode=WAL;",
            error
        )
        ||
        !exec(
            "PRAGMA synchronous=NORMAL;",
            error
        )
    ) {
        return false;
    }

    sqlite3_busy_timeout(
        database_,
        5000
    );

    return true;
}

bool UserDatabase::integrityCheck(
    std::string& error
) const
{
    Statement statement(
        database_,
        "PRAGMA quick_check;"
    );

    if (!statement) {
        error =
            sqliteError(
                database_,
                "Unable to prepare database integrity check"
            );

        return false;
    }

    if (
        sqlite3_step(
            statement.get()
        ) != SQLITE_ROW
    ) {
        error =
            sqliteError(
                database_,
                "Unable to run database integrity check"
            );

        return false;
    }

    const auto* value =
        sqlite3_column_text(
            statement.get(),
            0
        );

    const std::string result =
        value
        ? reinterpret_cast<
            const char*
          >(value)
        : "";

    if (result != "ok") {
        error =
            "User database integrity check failed: "
            + result;

        return false;
    }

    return true;
}

bool UserDatabase::backupBeforeMigration(
    std::string& error
) const
{
    if (
        !std::filesystem::exists(
            database_file_
        )
    ) {
        return true;
    }

    sqlite3* backup_database =
        nullptr;

    const std::string backup_file =
        database_file_
        + ".pre-migration.bak";

    if (
        sqlite3_open_v2(
            backup_file.c_str(),
            &backup_database,
            SQLITE_OPEN_READWRITE
            |
            SQLITE_OPEN_CREATE,
            nullptr
        ) != SQLITE_OK
    ) {
        if (backup_database)
            sqlite3_close(
                backup_database
            );

        error =
            "Unable to create database backup.";

        return false;
    }

    sqlite3_backup* backup =
        sqlite3_backup_init(
            backup_database,
            "main",
            database_,
            "main"
        );

    if (!backup) {
        sqlite3_close(
            backup_database
        );

        error =
            "Unable to initialize database backup.";

        return false;
    }

    const int step =
        sqlite3_backup_step(
            backup,
            -1
        );

    const int finish =
        sqlite3_backup_finish(
            backup
        );

    sqlite3_close(
        backup_database
    );

    ::chmod(
        backup_file.c_str(),
        0600
    );

    if (
        (
            step != SQLITE_DONE
            &&
            step != SQLITE_OK
        )
        ||
        finish != SQLITE_OK
    ) {
        error =
            "Unable to complete database backup.";

        return false;
    }

    return true;
}

bool UserDatabase::migrateSchema(
    std::string& error
)
{
    Statement statement(
        database_,
        "PRAGMA user_version;"
    );

    if (!statement) {
        error =
            sqliteError(
                database_,
                "Unable to read database schema version"
            );

        return false;
    }

    if (
        sqlite3_step(
            statement.get()
        ) != SQLITE_ROW
    ) {
        error =
            "Unable to read database schema version.";

        return false;
    }

    const int current =
        sqlite3_column_int(
            statement.get(),
            0
        );

    if (
        current >
        schema_version
    ) {
        error =
            "User database was created by a newer Home AI Core version.";

        return false;
    }

    if (
        current ==
        schema_version
    ) {
        return true;
    }

    if (
        current > 0
        &&
        !backupBeforeMigration(
            error
        )
    ) {
        return false;
    }

    if (
        !exec(
            "BEGIN IMMEDIATE;",
            error
        )
    ) {
        return false;
    }

    const std::string schema =
        "CREATE TABLE IF NOT EXISTS users("
        "id INTEGER PRIMARY KEY AUTOINCREMENT,"
        "username TEXT NOT NULL UNIQUE,"
        "role TEXT NOT NULL CHECK(role IN ('viewer','operator','admin')),"
        "enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0,1)),"
        "iterations INTEGER NOT NULL CHECK(iterations>=100000),"
        "salt BLOB NOT NULL,"
        "password_hash BLOB NOT NULL,"
        "created_at INTEGER NOT NULL,"
        "updated_at INTEGER NOT NULL,"
        "last_login_at INTEGER"
        ");"
        "CREATE TABLE IF NOT EXISTS user_permissions("
        "user_id INTEGER NOT NULL,"
        "permission TEXT NOT NULL,"
        "decision INTEGER NOT NULL CHECK(decision IN (-1,1)),"
        "PRIMARY KEY(user_id,permission),"
        "FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE"
        ");"
        "CREATE TABLE IF NOT EXISTS sessions("
        "id INTEGER PRIMARY KEY AUTOINCREMENT,"
        "token_hash TEXT NOT NULL UNIQUE,"
        "user_id INTEGER NOT NULL,"
        "created_at INTEGER NOT NULL,"
        "expires_at INTEGER NOT NULL,"
        "last_seen_at INTEGER NOT NULL,"
        "FOREIGN KEY(user_id) REFERENCES users(id) ON DELETE CASCADE"
        ");"
        "CREATE INDEX IF NOT EXISTS sessions_user_idx ON sessions(user_id);"
        "CREATE INDEX IF NOT EXISTS sessions_expiry_idx ON sessions(expires_at);"
        "CREATE TABLE IF NOT EXISTS audit("
        "id INTEGER PRIMARY KEY AUTOINCREMENT,"
        "created_at INTEGER NOT NULL,"
        "event TEXT NOT NULL,"
        "username TEXT NOT NULL,"
        "details TEXT NOT NULL"
        ");"
        "CREATE INDEX IF NOT EXISTS audit_time_idx ON audit(created_at DESC);"
        "CREATE INDEX IF NOT EXISTS audit_user_idx ON audit(username);"
        "CREATE INDEX IF NOT EXISTS audit_event_idx ON audit(event);"
        "PRAGMA user_version=1;";

    if (
        !exec(
            schema,
            error
        )
        ||
        !exec(
            "COMMIT;",
            error
        )
    ) {
        std::string ignored;

        exec(
            "ROLLBACK;",
            ignored
        );

        return false;
    }

    return true;
}

bool UserDatabase::migrateLegacyUsers(
    const std::string& legacy_users_file,
    std::string& error
)
{
    if (
        legacy_users_file.empty()
        ||
        !std::filesystem::exists(
            legacy_users_file
        )
    ) {
        return true;
    }

    std::string count_error;

    if (hasUsers(count_error)) {
        return true;
    }

    if (!count_error.empty()) {
        error = count_error;
        return false;
    }

    std::ifstream file(
        legacy_users_file
    );

    if (!file.is_open()) {
        error =
            "Unable to open legacy users database.";

        return false;
    }

    std::vector<DatabaseUser>
        users;

    std::string line;

    while (
        std::getline(
            file,
            line
        )
    ) {
        if (line.empty())
            continue;

        std::istringstream stream(
            line
        );

        std::string username;
        std::string role;
        std::string iterations_text;
        std::string salt_hex;
        std::string hash_hex;
        std::string extra;

        if (
            !std::getline(
                stream,
                username,
                '\t'
            )
            ||
            !std::getline(
                stream,
                role,
                '\t'
            )
            ||
            !std::getline(
                stream,
                iterations_text,
                '\t'
            )
            ||
            !std::getline(
                stream,
                salt_hex,
                '\t'
            )
            ||
            !std::getline(
                stream,
                hash_hex,
                '\t'
            )
            ||
            std::getline(
                stream,
                extra,
                '\t'
            )
        ) {
            error =
                "Legacy users database is malformed; migration stopped.";

            return false;
        }

        if (
            !validLegacyUsername(
                username
            )
            ||
            !validLegacyRole(
                role
            )
        ) {
            error =
                "Legacy users database contains invalid user data.";

            return false;
        }

        int iterations = 0;

        try {
            iterations =
                std::stoi(
                    iterations_text
                );
        }
        catch (...) {
            error =
                "Legacy users database contains invalid password parameters.";

            return false;
        }

        DatabaseUser user;

        user.username =
            username;

        user.role =
            role;

        user.enabled =
            true;

        user.iterations =
            iterations;

        user.created_at =
            unixNow();

        user.updated_at =
            user.created_at;

        if (
            iterations < 100000
            ||
            !hexToBytes(
                salt_hex,
                user.salt
            )
            ||
            !hexToBytes(
                hash_hex,
                user.password_hash
            )
            ||
            user.salt.empty()
            ||
            user.password_hash.empty()
        ) {
            error =
                "Legacy users database contains invalid password hashes.";

            return false;
        }

        users.push_back(
            std::move(user)
        );
    }

    if (users.empty())
        return true;

    if (
        !exec(
            "BEGIN IMMEDIATE;",
            error
        )
    ) {
        return false;
    }

    for (const auto& user : users) {
        if (
            !insertUser(
                user,
                error
            )
        ) {
            std::string ignored;

            exec(
                "ROLLBACK;",
                ignored
            );

            return false;
        }
    }

    if (
        !exec(
            "COMMIT;",
            error
        )
    ) {
        std::string ignored;

        exec(
            "ROLLBACK;",
            ignored
        );

        return false;
    }

    std::error_code rename_error;

    std::filesystem::rename(
        legacy_users_file,
        legacy_users_file +
            ".migrated",
        rename_error
    );

    // Archiving the legacy file is best-effort. The SQLite
    // transaction is already committed, and a remaining legacy
    // file will not be imported again while users exist.
    (void)rename_error;

    return true;
}

bool UserDatabase::migrateLegacyAudit(
    const std::string& legacy_audit_file,
    std::string& error
)
{
    if (
        legacy_audit_file.empty()
        ||
        !std::filesystem::exists(
            legacy_audit_file
        )
    ) {
        return true;
    }

    Statement count(
        database_,
        "SELECT COUNT(*) FROM audit;"
    );

    if (
        !count
        ||
        sqlite3_step(
            count.get()
        ) != SQLITE_ROW
    ) {
        error =
            "Unable to inspect audit database.";

        return false;
    }

    if (
        sqlite3_column_int64(
            count.get(),
            0
        ) > 0
    ) {
        return true;
    }

    std::ifstream file(
        legacy_audit_file
    );

    if (!file.is_open()) {
        error =
            "Unable to open legacy audit log.";

        return false;
    }

    std::vector<AuditEntry>
        entries;

    std::string line;

    while (
        std::getline(
            file,
            line
        )
    ) {
        if (line.empty())
            continue;

        std::istringstream stream(
            line
        );

        std::string timestamp_text;
        AuditEntry entry;

        if (
            !std::getline(
                stream,
                timestamp_text,
                '\t'
            )
            ||
            !std::getline(
                stream,
                entry.event,
                '\t'
            )
            ||
            !std::getline(
                stream,
                entry.username,
                '\t'
            )
            ||
            !std::getline(
                stream,
                entry.details
            )
        ) {
            continue;
        }

        entry.created_at =
            unixNow();

        entries.push_back(
            std::move(entry)
        );
    }

    if (entries.empty())
        return true;

    if (
        !exec(
            "BEGIN IMMEDIATE;",
            error
        )
    ) {
        return false;
    }

    for (const auto& entry : entries) {
        if (
            !appendAudit(
                entry.created_at,
                entry.event,
                entry.username,
                entry.details,
                error
            )
        ) {
            std::string ignored;

            exec(
                "ROLLBACK;",
                ignored
            );

            return false;
        }
    }

    if (
        !exec(
            "COMMIT;",
            error
        )
    ) {
        return false;
    }

    std::error_code rename_error;

    std::filesystem::rename(
        legacy_audit_file,
        legacy_audit_file +
            ".migrated",
        rename_error
    );

    // As with users, archive failure must not invalidate an
    // already committed migration.
    (void)rename_error;

    return true;
}

bool UserDatabase::exec(
    const std::string& sql,
    std::string& error
) const
{
    char* message = nullptr;

    const int code =
        sqlite3_exec(
            database_,
            sql.c_str(),
            nullptr,
            nullptr,
            &message
        );

    if (code == SQLITE_OK)
        return true;

    error =
        message
        ? message
        : sqliteError(
            database_,
            "SQLite operation failed"
        );

    if (message)
        sqlite3_free(message);

    return false;
}

bool UserDatabase::hasUsers(
    std::string& error
) const
{
    Statement statement(
        database_,
        "SELECT 1 FROM users LIMIT 1;"
    );

    if (!statement) {
        error =
            sqliteError(
                database_,
                "Unable to inspect users"
            );

        return false;
    }

    const int code =
        sqlite3_step(
            statement.get()
        );

    if (code == SQLITE_ROW)
        return true;

    if (code == SQLITE_DONE)
        return false;

    error =
        sqliteError(
            database_,
            "Unable to inspect users"
        );

    return false;
}

std::optional<DatabaseUser>
UserDatabase::findUserByName(
    const std::string& username,
    std::string& error
) const
{
    Statement statement(
        database_,
        "SELECT id,username,role,enabled,iterations,salt,password_hash,"
        "created_at,updated_at,last_login_at "
        "FROM users WHERE username=?1;"
    );

    if (
        !statement
        ||
        !bindText(
            statement.get(),
            1,
            username
        )
    ) {
        error =
            sqliteError(
                database_,
                "Unable to prepare user query"
            );

        return std::nullopt;
    }

    const int code =
        sqlite3_step(
            statement.get()
        );

    if (code == SQLITE_ROW) {
        return readUser(
            statement.get()
        );
    }

    if (code != SQLITE_DONE) {
        error =
            sqliteError(
                database_,
                "Unable to query user"
            );
    }

    return std::nullopt;
}

std::optional<DatabaseUser>
UserDatabase::findUserById(
    std::int64_t user_id,
    std::string& error
) const
{
    Statement statement(
        database_,
        "SELECT id,username,role,enabled,iterations,salt,password_hash,"
        "created_at,updated_at,last_login_at "
        "FROM users WHERE id=?1;"
    );

    if (
        !statement
        ||
        sqlite3_bind_int64(
            statement.get(),
            1,
            user_id
        ) != SQLITE_OK
    ) {
        error =
            sqliteError(
                database_,
                "Unable to prepare user query"
            );

        return std::nullopt;
    }

    const int code =
        sqlite3_step(
            statement.get()
        );

    if (code == SQLITE_ROW)
        return readUser(
            statement.get()
        );

    if (code != SQLITE_DONE) {
        error =
            sqliteError(
                database_,
                "Unable to query user"
            );
    }

    return std::nullopt;
}

std::vector<DatabaseUser>
UserDatabase::listUsers(
    std::string& error
) const
{
    std::vector<DatabaseUser>
        result;

    Statement statement(
        database_,
        "SELECT id,username,role,enabled,iterations,salt,password_hash,"
        "created_at,updated_at,last_login_at "
        "FROM users ORDER BY username COLLATE NOCASE;"
    );

    if (!statement) {
        error =
            sqliteError(
                database_,
                "Unable to prepare users query"
            );

        return result;
    }

    while (true) {
        const int code =
            sqlite3_step(
                statement.get()
            );

        if (code == SQLITE_DONE)
            break;

        if (code != SQLITE_ROW) {
            error =
                sqliteError(
                    database_,
                    "Unable to list users"
                );

            result.clear();
            break;
        }

        result.push_back(
            readUser(
                statement.get()
            )
        );
    }

    return result;
}

bool UserDatabase::insertUser(
    const DatabaseUser& user,
    std::string& error
)
{
    Statement statement(
        database_,
        "INSERT INTO users("
        "username,role,enabled,iterations,salt,password_hash,"
        "created_at,updated_at,last_login_at"
        ") VALUES(?1,?2,?3,?4,?5,?6,?7,?8,NULL);"
    );

    if (
        !statement
        ||
        !bindText(
            statement.get(),
            1,
            user.username
        )
        ||
        !bindText(
            statement.get(),
            2,
            user.role
        )
        ||
        sqlite3_bind_int(
            statement.get(),
            3,
            user.enabled ? 1 : 0
        ) != SQLITE_OK
        ||
        sqlite3_bind_int(
            statement.get(),
            4,
            user.iterations
        ) != SQLITE_OK
        ||
        !bindBlob(
            statement.get(),
            5,
            user.salt
        )
        ||
        !bindBlob(
            statement.get(),
            6,
            user.password_hash
        )
        ||
        sqlite3_bind_int64(
            statement.get(),
            7,
            user.created_at
        ) != SQLITE_OK
        ||
        sqlite3_bind_int64(
            statement.get(),
            8,
            user.updated_at
        ) != SQLITE_OK
    ) {
        error =
            sqliteError(
                database_,
                "Unable to prepare user insert"
            );

        return false;
    }

    if (
        sqlite3_step(
            statement.get()
        ) != SQLITE_DONE
    ) {
        error =
            sqliteError(
                database_,
                "Unable to create user"
            );

        return false;
    }

    return true;
}

bool UserDatabase::updateUser(
    std::int64_t user_id,
    const std::string& role,
    bool enabled,
    std::int64_t updated_at,
    std::string& error
)
{
    Statement statement(
        database_,
        "UPDATE users "
        "SET role=?1,enabled=?2,updated_at=?3 "
        "WHERE id=?4;"
    );

    if (
        !statement
        ||
        !bindText(
            statement.get(),
            1,
            role
        )
        ||
        sqlite3_bind_int(
            statement.get(),
            2,
            enabled ? 1 : 0
        ) != SQLITE_OK
        ||
        sqlite3_bind_int64(
            statement.get(),
            3,
            updated_at
        ) != SQLITE_OK
        ||
        sqlite3_bind_int64(
            statement.get(),
            4,
            user_id
        ) != SQLITE_OK
    ) {
        error =
            sqliteError(
                database_,
                "Unable to prepare user update"
            );

        return false;
    }

    if (
        sqlite3_step(
            statement.get()
        ) != SQLITE_DONE
    ) {
        error =
            sqliteError(
                database_,
                "Unable to update user"
            );

        return false;
    }

    if (
        sqlite3_changes(
            database_
        ) != 1
    ) {
        error =
            "User not found.";

        return false;
    }

    return true;
}

bool UserDatabase::updatePassword(
    std::int64_t user_id,
    int iterations,
    const std::vector<unsigned char>& salt,
    const std::vector<unsigned char>& hash,
    std::int64_t updated_at,
    std::string& error
)
{
    Statement statement(
        database_,
        "UPDATE users "
        "SET iterations=?1,salt=?2,password_hash=?3,updated_at=?4 "
        "WHERE id=?5;"
    );

    if (
        !statement
        ||
        sqlite3_bind_int(
            statement.get(),
            1,
            iterations
        ) != SQLITE_OK
        ||
        !bindBlob(
            statement.get(),
            2,
            salt
        )
        ||
        !bindBlob(
            statement.get(),
            3,
            hash
        )
        ||
        sqlite3_bind_int64(
            statement.get(),
            4,
            updated_at
        ) != SQLITE_OK
        ||
        sqlite3_bind_int64(
            statement.get(),
            5,
            user_id
        ) != SQLITE_OK
    ) {
        error =
            sqliteError(
                database_,
                "Unable to prepare password update"
            );

        return false;
    }

    if (
        sqlite3_step(
            statement.get()
        ) != SQLITE_DONE
        ||
        sqlite3_changes(
            database_
        ) != 1
    ) {
        error =
            sqliteError(
                database_,
                "Unable to update password"
            );

        return false;
    }

    return true;
}

bool UserDatabase::deleteUser(
    std::int64_t user_id,
    std::string& error
)
{
    Statement statement(
        database_,
        "DELETE FROM users WHERE id=?1;"
    );

    if (
        !statement
        ||
        sqlite3_bind_int64(
            statement.get(),
            1,
            user_id
        ) != SQLITE_OK
    ) {
        error =
            sqliteError(
                database_,
                "Unable to prepare user deletion"
            );

        return false;
    }

    if (
        sqlite3_step(
            statement.get()
        ) != SQLITE_DONE
    ) {
        error =
            sqliteError(
                database_,
                "Unable to delete user"
            );

        return false;
    }

    if (
        sqlite3_changes(
            database_
        ) != 1
    ) {
        error =
            "User not found.";

        return false;
    }

    return true;
}

std::int64_t
UserDatabase::countEnabledAdmins(
    std::string& error
) const
{
    Statement statement(
        database_,
        "SELECT COUNT(*) FROM users "
        "WHERE role='admin' AND enabled=1;"
    );

    if (
        !statement
        ||
        sqlite3_step(
            statement.get()
        ) != SQLITE_ROW
    ) {
        error =
            sqliteError(
                database_,
                "Unable to count administrators"
            );

        return -1;
    }

    return
        sqlite3_column_int64(
            statement.get(),
            0
        );
}

bool UserDatabase::updateLastLogin(
    std::int64_t user_id,
    std::int64_t timestamp,
    std::string& error
)
{
    Statement statement(
        database_,
        "UPDATE users SET last_login_at=?1 WHERE id=?2;"
    );

    if (
        !statement
        ||
        sqlite3_bind_int64(
            statement.get(),
            1,
            timestamp
        ) != SQLITE_OK
        ||
        sqlite3_bind_int64(
            statement.get(),
            2,
            user_id
        ) != SQLITE_OK
        ||
        sqlite3_step(
            statement.get()
        ) != SQLITE_DONE
    ) {
        error =
            sqliteError(
                database_,
                "Unable to update last login"
            );

        return false;
    }

    return true;
}

std::unordered_map<std::string, int>
UserDatabase::permissionOverrides(
    std::int64_t user_id,
    std::string& error
) const
{
    std::unordered_map<std::string, int>
        result;

    Statement statement(
        database_,
        "SELECT permission,decision "
        "FROM user_permissions "
        "WHERE user_id=?1;"
    );

    if (
        !statement
        ||
        sqlite3_bind_int64(
            statement.get(),
            1,
            user_id
        ) != SQLITE_OK
    ) {
        error =
            sqliteError(
                database_,
                "Unable to prepare permission query"
            );

        return result;
    }

    while (true) {
        const int code =
            sqlite3_step(
                statement.get()
            );

        if (code == SQLITE_DONE)
            break;

        if (code != SQLITE_ROW) {
            error =
                sqliteError(
                    database_,
                    "Unable to read permissions"
                );

            result.clear();
            break;
        }

        const auto* permission =
            sqlite3_column_text(
                statement.get(),
                0
            );

        if (!permission)
            continue;

        result[
            reinterpret_cast<
                const char*
            >(permission)
        ] =
            sqlite3_column_int(
                statement.get(),
                1
            );
    }

    return result;
}

bool UserDatabase::setPermissionOverride(
    std::int64_t user_id,
    const std::string& permission,
    int decision,
    std::string& error
)
{
    if (
        decision != -1
        &&
        decision != 1
    ) {
        error =
            "Invalid permission decision.";

        return false;
    }

    Statement statement(
        database_,
        "INSERT INTO user_permissions(user_id,permission,decision) "
        "VALUES(?1,?2,?3) "
        "ON CONFLICT(user_id,permission) "
        "DO UPDATE SET decision=excluded.decision;"
    );

    if (
        !statement
        ||
        sqlite3_bind_int64(
            statement.get(),
            1,
            user_id
        ) != SQLITE_OK
        ||
        !bindText(
            statement.get(),
            2,
            permission
        )
        ||
        sqlite3_bind_int(
            statement.get(),
            3,
            decision
        ) != SQLITE_OK
        ||
        sqlite3_step(
            statement.get()
        ) != SQLITE_DONE
    ) {
        error =
            sqliteError(
                database_,
                "Unable to save permission override"
            );

        return false;
    }

    return true;
}

bool UserDatabase::clearPermissionOverride(
    std::int64_t user_id,
    const std::string& permission,
    std::string& error
)
{
    Statement statement(
        database_,
        "DELETE FROM user_permissions "
        "WHERE user_id=?1 AND permission=?2;"
    );

    if (
        !statement
        ||
        sqlite3_bind_int64(
            statement.get(),
            1,
            user_id
        ) != SQLITE_OK
        ||
        !bindText(
            statement.get(),
            2,
            permission
        )
        ||
        sqlite3_step(
            statement.get()
        ) != SQLITE_DONE
    ) {
        error =
            sqliteError(
                database_,
                "Unable to clear permission override"
            );

        return false;
    }

    return true;
}

bool UserDatabase::createSession(
    const std::string& token_hash,
    std::int64_t user_id,
    std::int64_t created_at,
    std::int64_t expires_at,
    std::string& error
)
{
    Statement statement(
        database_,
        "INSERT INTO sessions("
        "token_hash,user_id,created_at,expires_at,last_seen_at"
        ") VALUES(?1,?2,?3,?4,?3);"
    );

    if (
        !statement
        ||
        !bindText(
            statement.get(),
            1,
            token_hash
        )
        ||
        sqlite3_bind_int64(
            statement.get(),
            2,
            user_id
        ) != SQLITE_OK
        ||
        sqlite3_bind_int64(
            statement.get(),
            3,
            created_at
        ) != SQLITE_OK
        ||
        sqlite3_bind_int64(
            statement.get(),
            4,
            expires_at
        ) != SQLITE_OK
        ||
        sqlite3_step(
            statement.get()
        ) != SQLITE_DONE
    ) {
        error =
            sqliteError(
                database_,
                "Unable to create session"
            );

        return false;
    }

    return true;
}

std::optional<DatabaseSession>
UserDatabase::findSession(
    const std::string& token_hash,
    std::string& error
) const
{
    Statement statement(
        database_,
        "SELECT s.id,s.user_id,u.username,u.role,u.enabled,"
        "s.token_hash,s.created_at,s.expires_at,s.last_seen_at "
        "FROM sessions s "
        "JOIN users u ON u.id=s.user_id "
        "WHERE s.token_hash=?1;"
    );

    if (
        !statement
        ||
        !bindText(
            statement.get(),
            1,
            token_hash
        )
    ) {
        error =
            sqliteError(
                database_,
                "Unable to prepare session query"
            );

        return std::nullopt;
    }

    const int code =
        sqlite3_step(
            statement.get()
        );

    if (code == SQLITE_DONE)
        return std::nullopt;

    if (code != SQLITE_ROW) {
        error =
            sqliteError(
                database_,
                "Unable to query session"
            );

        return std::nullopt;
    }

    DatabaseSession session;

    session.id =
        sqlite3_column_int64(
            statement.get(),
            0
        );

    session.user_id =
        sqlite3_column_int64(
            statement.get(),
            1
        );

    const auto* username =
        sqlite3_column_text(
            statement.get(),
            2
        );

    const auto* role =
        sqlite3_column_text(
            statement.get(),
            3
        );

    const auto* token =
        sqlite3_column_text(
            statement.get(),
            5
        );

    session.username =
        username
        ? reinterpret_cast<
            const char*
          >(username)
        : "";

    session.role =
        role
        ? reinterpret_cast<
            const char*
          >(role)
        : "";

    session.user_enabled =
        sqlite3_column_int(
            statement.get(),
            4
        ) != 0;

    session.token_hash =
        token
        ? reinterpret_cast<
            const char*
          >(token)
        : "";

    session.created_at =
        sqlite3_column_int64(
            statement.get(),
            6
        );

    session.expires_at =
        sqlite3_column_int64(
            statement.get(),
            7
        );

    session.last_seen_at =
        sqlite3_column_int64(
            statement.get(),
            8
        );

    return session;
}

bool UserDatabase::touchSession(
    const std::string& token_hash,
    std::int64_t timestamp,
    std::string& error
)
{
    Statement statement(
        database_,
        "UPDATE sessions SET last_seen_at=?1 WHERE token_hash=?2;"
    );

    if (
        !statement
        ||
        sqlite3_bind_int64(
            statement.get(),
            1,
            timestamp
        ) != SQLITE_OK
        ||
        !bindText(
            statement.get(),
            2,
            token_hash
        )
        ||
        sqlite3_step(
            statement.get()
        ) != SQLITE_DONE
    ) {
        error =
            sqliteError(
                database_,
                "Unable to update session activity"
            );

        return false;
    }

    return true;
}

bool UserDatabase::deleteSession(
    const std::string& token_hash,
    std::string& error
)
{
    Statement statement(
        database_,
        "DELETE FROM sessions WHERE token_hash=?1;"
    );

    if (
        !statement
        ||
        !bindText(
            statement.get(),
            1,
            token_hash
        )
        ||
        sqlite3_step(
            statement.get()
        ) != SQLITE_DONE
    ) {
        error =
            sqliteError(
                database_,
                "Unable to delete session"
            );

        return false;
    }

    return true;
}

bool UserDatabase::deleteSessionById(
    std::int64_t session_id,
    std::string& error
)
{
    Statement statement(
        database_,
        "DELETE FROM sessions WHERE id=?1;"
    );

    if (
        !statement
        ||
        sqlite3_bind_int64(
            statement.get(),
            1,
            session_id
        ) != SQLITE_OK
        ||
        sqlite3_step(
            statement.get()
        ) != SQLITE_DONE
    ) {
        error =
            sqliteError(
                database_,
                "Unable to revoke session"
            );

        return false;
    }

    return true;
}

bool UserDatabase::deleteSessionsForUser(
    std::int64_t user_id,
    std::string& error
)
{
    Statement statement(
        database_,
        "DELETE FROM sessions WHERE user_id=?1;"
    );

    if (
        !statement
        ||
        sqlite3_bind_int64(
            statement.get(),
            1,
            user_id
        ) != SQLITE_OK
        ||
        sqlite3_step(
            statement.get()
        ) != SQLITE_DONE
    ) {
        error =
            sqliteError(
                database_,
                "Unable to revoke user sessions"
            );

        return false;
    }

    return true;
}

bool UserDatabase::deleteExpiredSessions(
    std::int64_t now,
    std::string& error
)
{
    Statement statement(
        database_,
        "DELETE FROM sessions WHERE expires_at<=?1;"
    );

    if (
        !statement
        ||
        sqlite3_bind_int64(
            statement.get(),
            1,
            now
        ) != SQLITE_OK
        ||
        sqlite3_step(
            statement.get()
        ) != SQLITE_DONE
    ) {
        error =
            sqliteError(
                database_,
                "Unable to clean expired sessions"
            );

        return false;
    }

    return true;
}

std::vector<DatabaseSession>
UserDatabase::listSessions(
    std::optional<std::int64_t> user_id,
    std::string& error
) const
{
    std::vector<DatabaseSession>
        result;

    const char* sql =
        user_id
        ? "SELECT s.id,s.user_id,u.username,u.role,u.enabled,"
          "s.token_hash,s.created_at,s.expires_at,s.last_seen_at "
          "FROM sessions s JOIN users u ON u.id=s.user_id "
          "WHERE s.user_id=?1 ORDER BY s.last_seen_at DESC;"
        : "SELECT s.id,s.user_id,u.username,u.role,u.enabled,"
          "s.token_hash,s.created_at,s.expires_at,s.last_seen_at "
          "FROM sessions s JOIN users u ON u.id=s.user_id "
          "ORDER BY s.last_seen_at DESC;";

    Statement statement(
        database_,
        sql
    );

    if (!statement) {
        error =
            sqliteError(
                database_,
                "Unable to prepare sessions query"
            );

        return result;
    }

    if (
        user_id
        &&
        sqlite3_bind_int64(
            statement.get(),
            1,
            *user_id
        ) != SQLITE_OK
    ) {
        error =
            sqliteError(
                database_,
                "Unable to filter sessions"
            );

        return result;
    }

    while (true) {
        const int code =
            sqlite3_step(
                statement.get()
            );

        if (code == SQLITE_DONE)
            break;

        if (code != SQLITE_ROW) {
            error =
                sqliteError(
                    database_,
                    "Unable to list sessions"
                );

            result.clear();
            break;
        }

        DatabaseSession session;

        session.id =
            sqlite3_column_int64(
                statement.get(),
                0
            );

        session.user_id =
            sqlite3_column_int64(
                statement.get(),
                1
            );

        const auto* username =
            sqlite3_column_text(
                statement.get(),
                2
            );

        const auto* role =
            sqlite3_column_text(
                statement.get(),
                3
            );

        const auto* token =
            sqlite3_column_text(
                statement.get(),
                5
            );

        session.username =
            username
            ? reinterpret_cast<
                const char*
              >(username)
            : "";

        session.role =
            role
            ? reinterpret_cast<
                const char*
              >(role)
            : "";

        session.user_enabled =
            sqlite3_column_int(
                statement.get(),
                4
            ) != 0;

        session.token_hash =
            token
            ? reinterpret_cast<
                const char*
              >(token)
            : "";

        session.created_at =
            sqlite3_column_int64(
                statement.get(),
                6
            );

        session.expires_at =
            sqlite3_column_int64(
                statement.get(),
                7
            );

        session.last_seen_at =
            sqlite3_column_int64(
                statement.get(),
                8
            );

        result.push_back(
            std::move(session)
        );
    }

    return result;
}

bool UserDatabase::appendAudit(
    std::int64_t timestamp,
    const std::string& event,
    const std::string& username,
    const std::string& details,
    std::string& error
)
{
    Statement statement(
        database_,
        "INSERT INTO audit(created_at,event,username,details) "
        "VALUES(?1,?2,?3,?4);"
    );

    if (
        !statement
        ||
        sqlite3_bind_int64(
            statement.get(),
            1,
            timestamp
        ) != SQLITE_OK
        ||
        !bindText(
            statement.get(),
            2,
            event
        )
        ||
        !bindText(
            statement.get(),
            3,
            username
        )
        ||
        !bindText(
            statement.get(),
            4,
            details
        )
        ||
        sqlite3_step(
            statement.get()
        ) != SQLITE_DONE
    ) {
        error =
            sqliteError(
                database_,
                "Unable to write audit event"
            );

        return false;
    }

    return true;
}

std::vector<AuditEntry>
UserDatabase::listAudit(
    int limit,
    int offset,
    const std::string& username,
    const std::string& event,
    std::string& error
) const
{
    std::vector<AuditEntry>
        result;

    limit =
        std::clamp(
            limit,
            1,
            200
        );

    offset =
        std::max(
            offset,
            0
        );

    Statement statement(
        database_,
        "SELECT id,created_at,event,username,details "
        "FROM audit "
        "WHERE (?1='' OR username=?1) "
        "AND (?2='' OR event=?2) "
        "ORDER BY id DESC "
        "LIMIT ?3 OFFSET ?4;"
    );

    if (
        !statement
        ||
        !bindText(
            statement.get(),
            1,
            username
        )
        ||
        !bindText(
            statement.get(),
            2,
            event
        )
        ||
        sqlite3_bind_int(
            statement.get(),
            3,
            limit
        ) != SQLITE_OK
        ||
        sqlite3_bind_int(
            statement.get(),
            4,
            offset
        ) != SQLITE_OK
    ) {
        error =
            sqliteError(
                database_,
                "Unable to prepare audit query"
            );

        return result;
    }

    while (true) {
        const int code =
            sqlite3_step(
                statement.get()
            );

        if (code == SQLITE_DONE)
            break;

        if (code != SQLITE_ROW) {
            error =
                sqliteError(
                    database_,
                    "Unable to read audit log"
                );

            result.clear();
            break;
        }

        AuditEntry entry;

        entry.id =
            sqlite3_column_int64(
                statement.get(),
                0
            );

        entry.created_at =
            sqlite3_column_int64(
                statement.get(),
                1
            );

        const auto* event_value =
            sqlite3_column_text(
                statement.get(),
                2
            );

        const auto* username_value =
            sqlite3_column_text(
                statement.get(),
                3
            );

        const auto* details_value =
            sqlite3_column_text(
                statement.get(),
                4
            );

        entry.event =
            event_value
            ? reinterpret_cast<
                const char*
              >(event_value)
            : "";

        entry.username =
            username_value
            ? reinterpret_cast<
                const char*
              >(username_value)
            : "";

        entry.details =
            details_value
            ? reinterpret_cast<
                const char*
              >(details_value)
            : "";

        result.push_back(
            std::move(entry)
        );
    }

    return result;
}

std::int64_t UserDatabase::unixNow()
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

}
