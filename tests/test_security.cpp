#include "security/auth/SecurityManager.h"

#include <filesystem>
#include <iostream>

int main()
{
    const std::string directory =
        "/tmp/home-ai-security-test";

    std::filesystem::remove_all(
        directory
    );

    homeai::SecurityManager security;

    if (
        !security.initialize(
            directory + "/users.db",
            directory + "/audit.log"
        )
    ) {
        std::cerr
            << "Security initialization failed\n";
        return 1;
    }

    std::string error;

    if (
        !security.createUser(
            "admin",
            "HomeAI-Test-Password-123!",
            homeai::UserRole::Admin,
            error
        )
    ) {
        std::cerr
            << "User creation failed: "
            << error
            << '\n';
        return 1;
    }

    homeai::SessionInfo info;

    auto token =
        security.login(
            "admin",
            "HomeAI-Test-Password-123!",
            info,
            error
        );

    if (!token) {
        std::cerr
            << "Login failed: "
            << error
            << '\n';
        return 1;
    }

    auto session =
        security.validateSession(*token);

    if (
        !session
        ||
        session->username != "admin"
        ||
        session->role !=
            homeai::UserRole::Admin
    ) {
        std::cerr
            << "Session validation failed\n";
        return 1;
    }

    security.logout(*token);

    if (
        security.validateSession(*token)
    ) {
        std::cerr
            << "Logout failed\n";
        return 1;
    }

    std::filesystem::remove_all(
        directory
    );

    std::cout
        << "Security Core test passed\n";

    return 0;
}
