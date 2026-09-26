#include "core/modules/ModuleManager.h"
#include "core/runtime/CoreRuntime.h"
#include "security/auth/SecurityManager.h"
#include "server/update/UpdateManager.h"
#include "web/server/WebServer.h"

#include <arpa/inet.h>
#include <cstdio>
#include <filesystem>
#include <iostream>
#include <openssl/evp.h>
#include <openssl/pem.h>
#include <openssl/rsa.h>
#include <openssl/ssl.h>
#include <openssl/x509.h>
#include <string>
#include <sys/socket.h>
#include <sys/stat.h>
#include <unistd.h>

namespace {

bool writeTestCertificate(
    const std::string& certificate_path,
    const std::string& key_path
)
{
    EVP_PKEY_CTX* key_context =
        EVP_PKEY_CTX_new_id(
            EVP_PKEY_RSA,
            nullptr
        );

    if (!key_context)
        return false;

    EVP_PKEY* key = nullptr;

    const bool key_ok =
        EVP_PKEY_keygen_init(
            key_context
        ) > 0
        &&
        EVP_PKEY_CTX_set_rsa_keygen_bits(
            key_context,
            2048
        ) > 0
        &&
        EVP_PKEY_keygen(
            key_context,
            &key
        ) > 0;

    EVP_PKEY_CTX_free(
        key_context
    );

    if (!key_ok || !key)
        return false;

    X509* certificate =
        X509_new();

    if (!certificate) {
        EVP_PKEY_free(key);
        return false;
    }

    X509_set_version(
        certificate,
        2
    );

    ASN1_INTEGER_set(
        X509_get_serialNumber(
            certificate
        ),
        1
    );

    X509_gmtime_adj(
        X509_get_notBefore(
            certificate
        ),
        0
    );

    X509_gmtime_adj(
        X509_get_notAfter(
            certificate
        ),
        86400
    );

    X509_set_pubkey(
        certificate,
        key
    );

    auto* name =
        X509_get_subject_name(
            certificate
        );

    X509_NAME_add_entry_by_txt(
        name,
        "CN",
        MBSTRING_ASC,
        reinterpret_cast<
            const unsigned char*
        >(
            "localhost"
        ),
        -1,
        -1,
        0
    );

    X509_set_issuer_name(
        certificate,
        name
    );

    const bool signed_ok =
        X509_sign(
            certificate,
            key,
            EVP_sha256()
        ) > 0;

    FILE* key_file =
        std::fopen(
            key_path.c_str(),
            "wb"
        );

    FILE* certificate_file =
        std::fopen(
            certificate_path.c_str(),
            "wb"
        );

    const bool written =
        signed_ok
        &&
        key_file
        &&
        certificate_file
        &&
        PEM_write_PrivateKey(
            key_file,
            key,
            nullptr,
            nullptr,
            0,
            nullptr,
            nullptr
        ) == 1
        &&
        PEM_write_X509(
            certificate_file,
            certificate
        ) == 1;

    if (key_file)
        std::fclose(key_file);

    if (certificate_file)
        std::fclose(certificate_file);

    ::chmod(
        key_path.c_str(),
        0600
    );

    X509_free(
        certificate
    );

    EVP_PKEY_free(
        key
    );

    return written;
}

}

int main()
{
    const std::string directory =
        "/tmp/home-ai-web-tls-test";

    std::filesystem::remove_all(
        directory
    );

    std::filesystem::create_directories(
        directory
    );

    const auto certificate_path =
        directory + "/server.crt";

    const auto key_path =
        directory + "/server.key";

    if (
        !writeTestCertificate(
            certificate_path,
            key_path
        )
    ) {
        std::cerr
            << "Unable to generate TLS test certificate\n";
        return 1;
    }

    homeai::CoreRuntime runtime;
    homeai::SecurityManager security;
    homeai::UpdateManager updates;
    homeai::ModuleManager modules;

    if (
        !security.initialize(
            directory + "/users.db",
            directory + "/audit.log"
        )
    ) {
        std::cerr
            << "Unable to initialize Security Core\n";
        return 1;
    }

    std::string error;

    if (
        !security.createUser(
            "admin",
            "HomeAI-TLS-Test-Password-123!",
            homeai::UserRole::Admin,
            error
        )
    ) {
        std::cerr
            << "Unable to create TLS test user: "
            << error
            << '\n';
        return 1;
    }

    homeai::WebServer web(
        runtime,
        security,
        updates,
        modules
    );

    if (
        web.start(
            "127.0.0.1",
            0,
            true,
            directory + "/missing.crt",
            key_path
        )
    ) {
        std::cerr
            << "TLS server accepted a missing certificate\n";
        web.stop();
        return 1;
    }

    if (
        !web.start(
            "127.0.0.1",
            0,
            true,
            certificate_path,
            key_path
        )
    ) {
        std::cerr
            << "TLS server failed to start\n";
        return 1;
    }

    if (
        !web.tlsEnabled()
        ||
        web.port() == 0
    ) {
        std::cerr
            << "TLS server state is invalid\n";
        web.stop();
        return 1;
    }

    const int socket_fd =
        ::socket(
            AF_INET,
            SOCK_STREAM,
            0
        );

    if (socket_fd < 0) {
        web.stop();
        return 1;
    }

    sockaddr_in address{};
    address.sin_family = AF_INET;
    address.sin_port =
        htons(
            web.port()
        );

    inet_pton(
        AF_INET,
        "127.0.0.1",
        &address.sin_addr
    );

    if (
        ::connect(
            socket_fd,
            reinterpret_cast<
                sockaddr*
            >(
                &address
            ),
            sizeof(address)
        ) != 0
    ) {
        ::close(socket_fd);
        web.stop();
        return 1;
    }

    SSL_CTX* client_context =
        SSL_CTX_new(
            TLS_client_method()
        );

    if (!client_context) {
        ::close(socket_fd);
        web.stop();
        return 1;
    }

    SSL_CTX_set_verify(
        client_context,
        SSL_VERIFY_NONE,
        nullptr
    );

    SSL* ssl =
        SSL_new(
            client_context
        );

    if (
        !ssl
        ||
        SSL_set_fd(
            ssl,
            socket_fd
        ) != 1
        ||
        SSL_connect(
            ssl
        ) != 1
    ) {
        if (ssl)
            SSL_free(ssl);

        SSL_CTX_free(
            client_context
        );

        ::close(socket_fd);
        web.stop();

        std::cerr
            << "TLS client handshake failed\n";
        return 1;
    }

    const std::string request =
        "GET /login HTTP/1.1\r\n"
        "Host: localhost\r\n"
        "Connection: close\r\n"
        "\r\n";

    if (
        SSL_write(
            ssl,
            request.data(),
            static_cast<int>(
                request.size()
            )
        ) <= 0
    ) {
        SSL_free(ssl);
        SSL_CTX_free(
            client_context
        );
        ::close(socket_fd);
        web.stop();
        return 1;
    }

    std::string response;
    char buffer[4096];

    for (;;) {
        const int received =
            SSL_read(
                ssl,
                buffer,
                sizeof(buffer)
            );

        if (received <= 0)
            break;

        response.append(
            buffer,
            static_cast<
                std::size_t
            >(
                received
            )
        );
    }

    SSL_free(ssl);

    SSL_CTX_free(
        client_context
    );

    ::close(socket_fd);

    web.stop();

    std::filesystem::remove_all(
        directory
    );

    if (
        response.find(
            "HTTP/1.1 200 OK"
        ) == std::string::npos
        ||
        response.find(
            "Strict-Transport-Security: max-age=31536000"
        ) == std::string::npos
    ) {
        std::cerr
            << "HTTPS response is missing expected security headers\n";
        return 1;
    }

    std::cout
        << "Web TLS test passed\n";

    return 0;
}
