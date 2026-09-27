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

    const std::string throttled_source =
        "192.0.2.10";

    for (int attempt = 0; attempt < 20; ++attempt) {
        homeai::SessionInfo source_info;
        std::string source_error;

        const auto source_token =
            security.login(
                "missing-user-"
                + std::to_string(
                    attempt
                ),
                "wrong-password-123!",
                source_info,
                source_error,
                throttled_source
            );

        if (source_token) {
            std::cerr
                << "Unknown user login unexpectedly succeeded\n";
            return 1;
        }
    }

    homeai::SessionInfo blocked_info;
    std::string blocked_error;

    if (
        security.login(
            "admin",
            "HomeAI-Test-Password-123!",
            blocked_info,
            blocked_error,
            throttled_source
        )
        ||
        blocked_error !=
            "Too many failed attempts"
    ) {
        std::cerr
            << "Source-aware login throttle did not activate\n";
        return 1;
    }

    homeai::SessionInfo alternate_info;
    std::string alternate_error;

    const auto alternate_token =
        security.login(
            "admin",
            "HomeAI-Test-Password-123!",
            alternate_info,
            alternate_error,
            "192.0.2.11"
        );

    if (!alternate_token) {
        std::cerr
            << "Source throttle leaked to another address\n";
        return 1;
    }

    security.logout(
        *alternate_token
    );

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

    const auto csrf_token =
        homeai::SecurityManager::
            csrfTokenForSession(
                *token
            );

    if (
        csrf_token.empty()
        ||
        !homeai::SecurityManager::
            validateCsrfToken(
                *token,
                csrf_token
            )
        ||
        homeai::SecurityManager::
            validateCsrfToken(
                *token,
                csrf_token + "00"
            )
        ||
        homeai::SecurityManager::
            validateCsrfToken(
                "different-session",
                csrf_token
            )
    ) {
        std::cerr
            << "CSRF token validation failed\n";
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
