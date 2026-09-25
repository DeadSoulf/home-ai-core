#include "security/auth/SecurityManager.h"

#include <filesystem>
#include <iostream>

int main()
{
    const std::string directory =
        "/tmp/home-ai-security-test";

    const std::string users_file =
        directory + "/users.db";

    const std::string audit_file =
        directory + "/audit.log";

    std::filesystem::remove_all(
        directory
    );

    homeai::SecurityManager security;

    if (
        !security.initialize(
            users_file,
            audit_file
        )
    ) {
        std::cerr
            << "Security initialization failed\n";
        return 1;
    }

    if (security.hasUsers()) {
        std::cerr
            << "Fresh security store should be empty\n";
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

    if (!security.hasUsers()) {
        std::cerr
            << "Created user was not stored\n";
        return 1;
    }

    homeai::SessionInfo bad_info;

    auto bad_token =
        security.login(
            "admin",
            "wrong-password-123!",
            bad_info,
            error
        );

    if (bad_token) {
        std::cerr
            << "Invalid password was accepted\n";
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
        security.validateSession(
            *token
        );

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
        security.validateSession(
            *token
        )
    ) {
        std::cerr
            << "Logout failed\n";
        return 1;
    }

    homeai::SecurityManager reloaded;

    if (
        !reloaded.initialize(
            users_file,
            audit_file
        )
    ) {
        std::cerr
            << "Reload initialization failed\n";
        return 1;
    }

    if (!reloaded.hasUsers()) {
        std::cerr
            << "User persistence failed\n";
        return 1;
    }

    homeai::SessionInfo reloaded_info;
    std::string reloaded_error;

    auto reloaded_token =
        reloaded.login(
            "admin",
            "HomeAI-Test-Password-123!",
            reloaded_info,
            reloaded_error
        );

    if (!reloaded_token) {
        std::cerr
            << "Persisted user login failed: "
            << reloaded_error
            << '\n';
        return 1;
    }

    std::filesystem::remove_all(
        directory
    );

    std::cout
        << "Security Core test passed\n";

    return 0;
}
