#pragma once

#include <chrono>
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
    std::string username;
    UserRole role{UserRole::Viewer};
};

class SecurityManager {
public:
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

    std::optional<std::string> login(
        const std::string& username,
        const std::string& password,
        SessionInfo& session_info,
        std::string& error
    );

    std::optional<SessionInfo> validateSession(
        const std::string& token
    );

    void logout(const std::string& token);

    bool isAdmin(UserRole role) const;

    static std::string roleToString(UserRole role);

    void audit(
        const std::string& event,
        const std::string& username,
        const std::string& details
    );

private:
    struct User {
        std::string username;
        UserRole role{UserRole::Viewer};
        int iterations{310000};
        std::vector<unsigned char> salt;
        std::vector<unsigned char> hash;
    };

    struct Session {
        SessionInfo info;
        std::chrono::steady_clock::time_point expires_at;
    };

    struct FailureState {
        int failures{0};
        std::chrono::steady_clock::time_point locked_until{};
    };

    bool loadUsers();
    bool saveUsersUnlocked() const;

    static bool validUsername(
        const std::string& username
    );

    static std::vector<unsigned char> derivePassword(
        const std::string& password,
        const std::vector<unsigned char>& salt,
        int iterations
    );

    static std::string bytesToHex(
        const std::vector<unsigned char>& bytes
    );

    static bool hexToBytes(
        const std::string& hex,
        std::vector<unsigned char>& bytes
    );

    static std::string randomHex(
        std::size_t bytes
    );

    mutable std::mutex mutex_;
    mutable std::mutex audit_mutex_;

    std::unordered_map<std::string, User> users_;
    std::unordered_map<std::string, Session> sessions_;
    std::unordered_map<std::string, FailureState> failures_;

    std::string users_file_;
    std::string audit_file_;
};

}
