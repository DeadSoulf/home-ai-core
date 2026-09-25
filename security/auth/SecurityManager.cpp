#include "security/auth/SecurityManager.h"

#include <openssl/crypto.h>
#include <openssl/evp.h>
#include <openssl/rand.h>

#include <algorithm>
#include <cctype>
#include <chrono>
#include <ctime>
#include <filesystem>
#include <fstream>
#include <iomanip>
#include <sstream>
#include <sys/stat.h>

namespace homeai {

namespace {

UserRole roleFromString(const std::string& value)
{
    if (value == "admin")
        return UserRole::Admin;

    if (value == "operator")
        return UserRole::Operator;

    return UserRole::Viewer;
}

std::string sanitizeAudit(std::string value)
{
    std::replace(value.begin(), value.end(), '\n', ' ');
    std::replace(value.begin(), value.end(), '\r', ' ');
    std::replace(value.begin(), value.end(), '\t', ' ');
    return value;
}

}

bool SecurityManager::initialize(
    const std::string& users_file,
    const std::string& audit_file
)
{
    users_file_ = users_file;
    audit_file_ = audit_file;

    try {
        const auto users_parent =
            std::filesystem::path(users_file_).parent_path();

        const auto audit_parent =
            std::filesystem::path(audit_file_).parent_path();

        if (!users_parent.empty())
            std::filesystem::create_directories(users_parent);

        if (!audit_parent.empty())
            std::filesystem::create_directories(audit_parent);
    }
    catch (...) {
        return false;
    }

    if (!loadUsers())
        return false;

    audit(
        "security.init",
        "system",
        "Security Core initialized"
    );

    return true;
}

bool SecurityManager::hasUsers() const
{
    std::lock_guard<std::mutex> lock(mutex_);
    return !users_.empty();
}

bool SecurityManager::createUser(
    const std::string& username,
    const std::string& password,
    UserRole role,
    std::string& error
)
{
    if (!validUsername(username)) {
        error = "Username must be 3-32 characters";
        return false;
    }

    if (password.size() < 12) {
        error = "Password must contain at least 12 characters";
        return false;
    }

    if (password.size() > 256) {
        error = "Password is too long";
        return false;
    }

    std::vector<unsigned char> salt(16);

    if (
        RAND_bytes(
            salt.data(),
            static_cast<int>(salt.size())
        ) != 1
    ) {
        error = "Unable to generate salt";
        return false;
    }

    constexpr int iterations = 310000;

    auto hash =
        derivePassword(
            password,
            salt,
            iterations
        );

    if (hash.empty()) {
        error = "Password hashing failed";
        return false;
    }

    {
        std::lock_guard<std::mutex> lock(mutex_);

        if (users_.contains(username)) {
            error = "User already exists";
            return false;
        }

        users_[username] = User{
            username,
            role,
            iterations,
            std::move(salt),
            std::move(hash)
        };

        if (!saveUsersUnlocked()) {
            users_.erase(username);
            error = "Unable to save users";
            return false;
        }
    }

    audit(
        "user.create",
        username,
        "role=" + roleToString(role)
    );

    return true;
}

std::optional<std::string>
SecurityManager::login(
    const std::string& username,
    const std::string& password,
    SessionInfo& session_info,
    std::string& error
)
{
    const auto now =
        std::chrono::steady_clock::now();

    User user;

    {
        std::lock_guard<std::mutex> lock(mutex_);

        auto& failure =
            failures_[username];

        if (now < failure.locked_until) {
            error = "Too many failed attempts";
            return std::nullopt;
        }

        const auto it =
            users_.find(username);

        if (it == users_.end()) {
            ++failure.failures;

            if (failure.failures >= 5) {
                failure.failures = 0;
                failure.locked_until =
                    now + std::chrono::seconds(60);
            }

            error = "Invalid username or password";
            return std::nullopt;
        }

        user = it->second;
    }

    const auto candidate =
        derivePassword(
            password,
            user.salt,
            user.iterations
        );

    const bool valid =
        candidate.size() == user.hash.size()
        &&
        !candidate.empty()
        &&
        CRYPTO_memcmp(
            candidate.data(),
            user.hash.data(),
            user.hash.size()
        ) == 0;

    if (!valid) {
        {
            std::lock_guard<std::mutex> lock(mutex_);

            auto& failure =
                failures_[username];

            ++failure.failures;

            if (failure.failures >= 5) {
                failure.failures = 0;
                failure.locked_until =
                    std::chrono::steady_clock::now()
                    + std::chrono::seconds(60);
            }
        }

        audit(
            "login.failed",
            username,
            "invalid credentials"
        );

        error = "Invalid username or password";
        return std::nullopt;
    }

    const auto token =
        randomHex(32);

    if (token.empty()) {
        error = "Unable to create session";
        return std::nullopt;
    }

    session_info = SessionInfo{
        user.username,
        user.role
    };

    {
        std::lock_guard<std::mutex> lock(mutex_);

        failures_.erase(username);

        sessions_[token] = Session{
            session_info,
            std::chrono::steady_clock::now()
                + std::chrono::hours(8)
        };
    }

    audit(
        "login.success",
        username,
        "role=" + roleToString(user.role)
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

    std::lock_guard<std::mutex> lock(mutex_);

    const auto it =
        sessions_.find(token);

    if (it == sessions_.end())
        return std::nullopt;

    if (
        std::chrono::steady_clock::now()
        >= it->second.expires_at
    ) {
        sessions_.erase(it);
        return std::nullopt;
    }

    return it->second.info;
}

void SecurityManager::logout(
    const std::string& token
)
{
    if (token.empty())
        return;

    std::string username;

    {
        std::lock_guard<std::mutex> lock(mutex_);

        const auto it =
            sessions_.find(token);

        if (it == sessions_.end())
            return;

        username =
            it->second.info.username;

        sessions_.erase(it);
    }

    audit(
        "logout",
        username,
        "session closed"
    );
}

bool SecurityManager::isAdmin(
    UserRole role
) const
{
    return role == UserRole::Admin;
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

void SecurityManager::audit(
    const std::string& event,
    const std::string& username,
    const std::string& details
)
{
    std::lock_guard<std::mutex> lock(audit_mutex_);

    std::ofstream file(
        audit_file_,
        std::ios::app
    );

    if (!file.is_open())
        return;

    const auto now =
        std::chrono::system_clock::now();

    const auto value =
        std::chrono::system_clock::to_time_t(now);

    std::tm tm{};

    localtime_r(
        &value,
        &tm
    );

    file
        << std::put_time(
            &tm,
            "%Y-%m-%d %H:%M:%S"
        )
        << '\t'
        << sanitizeAudit(event)
        << '\t'
        << sanitizeAudit(username)
        << '\t'
        << sanitizeAudit(details)
        << '\n';

    file.close();

    ::chmod(
        audit_file_.c_str(),
        S_IRUSR | S_IWUSR
    );
}

bool SecurityManager::loadUsers()
{
    std::lock_guard<std::mutex> lock(mutex_);

    users_.clear();

    if (
        !std::filesystem::exists(
            users_file_
        )
    ) {
        return true;
    }

    std::ifstream file(users_file_);

    if (!file.is_open())
        return false;

    std::string line;

    while (std::getline(file, line)) {
        if (line.empty())
            continue;

        std::istringstream stream(line);

        std::string username;
        std::string role;
        std::string iterations_text;
        std::string salt_hex;
        std::string hash_hex;

        if (
            !std::getline(stream, username, '\t')
            ||
            !std::getline(stream, role, '\t')
            ||
            !std::getline(stream, iterations_text, '\t')
            ||
            !std::getline(stream, salt_hex, '\t')
            ||
            !std::getline(stream, hash_hex, '\t')
        ) {
            continue;
        }

        int iterations = 0;

        try {
            iterations =
                std::stoi(iterations_text);
        }
        catch (...) {
            continue;
        }

        std::vector<unsigned char> salt;
        std::vector<unsigned char> hash;

        if (
            !hexToBytes(salt_hex, salt)
            ||
            !hexToBytes(hash_hex, hash)
            ||
            salt.empty()
            ||
            hash.empty()
            ||
            iterations < 100000
        ) {
            continue;
        }

        users_[username] =
            User{
                username,
                roleFromString(role),
                iterations,
                std::move(salt),
                std::move(hash)
            };
    }

    return true;
}

bool SecurityManager::saveUsersUnlocked() const
{
    const std::string temporary =
        users_file_ + ".tmp";

    std::ofstream file(
        temporary,
        std::ios::trunc
    );

    if (!file.is_open())
        return false;

    for (
        const auto& [username, user] :
        users_
    ) {
        file
            << username
            << '\t'
            << roleToString(user.role)
            << '\t'
            << user.iterations
            << '\t'
            << bytesToHex(user.salt)
            << '\t'
            << bytesToHex(user.hash)
            << '\n';
    }

    file.close();

    ::chmod(
        temporary.c_str(),
        S_IRUSR | S_IWUSR
    );

    std::error_code error;

    std::filesystem::rename(
        temporary,
        users_file_,
        error
    );

    if (error) {
        std::filesystem::remove(
            temporary
        );

        return false;
    }

    ::chmod(
        users_file_.c_str(),
        S_IRUSR | S_IWUSR
    );

    return true;
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

std::vector<unsigned char>
SecurityManager::derivePassword(
    const std::string& password,
    const std::vector<unsigned char>& salt,
    int iterations
)
{
    std::vector<unsigned char> result(32);

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
    static constexpr char digits[] =
        "0123456789abcdef";

    std::string result;

    result.reserve(bytes.size() * 2);

    for (const auto byte : bytes) {
        result.push_back(
            digits[(byte >> 4) & 0x0f]
        );

        result.push_back(
            digits[byte & 0x0f]
        );
    }

    return result;
}

bool SecurityManager::hexToBytes(
    const std::string& hex,
    std::vector<unsigned char>& bytes
)
{
    if (hex.size() % 2 != 0)
        return false;

    bytes.clear();
    bytes.reserve(hex.size() / 2);

    auto value =
        [](char c) -> int {
            if (c >= '0' && c <= '9')
                return c - '0';

            if (c >= 'a' && c <= 'f')
                return c - 'a' + 10;

            if (c >= 'A' && c <= 'F')
                return c - 'A' + 10;

            return -1;
        };

    for (
        std::size_t i = 0;
        i < hex.size();
        i += 2
    ) {
        const int high = value(hex[i]);
        const int low = value(hex[i + 1]);

        if (high < 0 || low < 0)
            return false;

        bytes.push_back(
            static_cast<unsigned char>(
                (high << 4) | low
            )
        );
    }

    return true;
}

std::string SecurityManager::randomHex(
    std::size_t bytes
)
{
    std::vector<unsigned char> data(bytes);

    if (
        RAND_bytes(
            data.data(),
            static_cast<int>(data.size())
        ) != 1
    ) {
        return {};
    }

    return bytesToHex(data);
}

}
