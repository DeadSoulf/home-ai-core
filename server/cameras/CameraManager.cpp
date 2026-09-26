#include "server/cameras/CameraManager.h"

#include <openssl/crypto.h>
#include <openssl/evp.h>
#include <openssl/rand.h>
#include <sqlite3.h>

#include <algorithm>
#include <array>
#include <arpa/inet.h>
#include <cctype>
#include <cstdint>
#include <cerrno>
#include <chrono>
#include <cstring>
#include <filesystem>
#include <fcntl.h>
#include <fstream>
#include <map>
#include <mutex>
#include <netdb.h>
#include <optional>
#include <sstream>
#include <sys/select.h>
#include <sys/socket.h>
#include <sys/stat.h>
#include <unistd.h>
#include <utility>
#include <vector>

namespace homeai {

namespace {

constexpr std::size_t secret_key_size = 32;
constexpr std::size_t gcm_nonce_size = 12;
constexpr std::size_t gcm_tag_size = 16;
constexpr int probe_timeout_seconds = 2;

std::int64_t unixNow()
{
    return std::chrono::duration_cast<
        std::chrono::seconds
    >(
        std::chrono::system_clock::now()
            .time_since_epoch()
    ).count();
}

bool validText(
    const std::string& value,
    std::size_t maximum,
    bool allow_empty
)
{
    if (
        value.size() > maximum
        ||
        (
            !allow_empty
            &&
            value.empty()
        )
    ) {
        return false;
    }

    return std::none_of(
        value.begin(),
        value.end(),
        [](unsigned char character) {
            return
                character == 0
                ||
                character == '\r'
                ||
                character == '\n';
        }
    );
}

struct RtspEndpoint {
    std::string host;
    std::string port;
};

std::optional<RtspEndpoint>
parseRtspEndpoint(
    const std::string& url
)
{
    std::string default_port;
    std::size_t scheme_size = 0;

    if (url.rfind("rtsp://", 0) == 0) {
        scheme_size = 7;
        default_port = "554";
    }
    else if (
        url.rfind("rtsps://", 0) == 0
    ) {
        scheme_size = 8;
        default_port = "322";
    }
    else {
        return std::nullopt;
    }

    const auto authority_end =
        url.find_first_of(
            "/?#",
            scheme_size
        );

    const auto authority =
        url.substr(
            scheme_size,
            authority_end ==
                std::string::npos
            ? std::string::npos
            : authority_end -
                scheme_size
        );

    if (
        authority.empty()
        ||
        authority.find('@') !=
            std::string::npos
    ) {
        return std::nullopt;
    }

    RtspEndpoint endpoint;
    endpoint.port = default_port;

    if (authority.front() == '[') {
        const auto closing =
            authority.find(']');

        if (
            closing ==
                std::string::npos
            ||
            closing == 1
        ) {
            return std::nullopt;
        }

        endpoint.host =
            authority.substr(
                1,
                closing - 1
            );

        if (
            closing + 1 <
            authority.size()
        ) {
            if (
                authority[
                    closing + 1
                ] != ':'
            ) {
                return std::nullopt;
            }

            endpoint.port =
                authority.substr(
                    closing + 2
                );
        }
    }
    else {
        const auto colon =
            authority.rfind(':');

        if (
            colon !=
                std::string::npos
            &&
            authority.find(':') ==
                colon
        ) {
            endpoint.host =
                authority.substr(
                    0,
                    colon
                );

            endpoint.port =
                authority.substr(
                    colon + 1
                );
        }
        else {
            endpoint.host =
                authority;
        }
    }

    if (
        endpoint.host.empty()
        ||
        endpoint.port.empty()
    ) {
        return std::nullopt;
    }

    if (
        !std::all_of(
            endpoint.port.begin(),
            endpoint.port.end(),
            [](unsigned char character) {
                return std::isdigit(
                    character
                ) != 0;
            }
        )
    ) {
        return std::nullopt;
    }

    try {
        const auto port =
            std::stoi(endpoint.port);

        if (
            port < 1
            ||
            port > 65535
        ) {
            return std::nullopt;
        }
    }
    catch (...) {
        return std::nullopt;
    }

    return endpoint;
}

bool validateInput(
    const CameraInput& input,
    std::string& error
)
{
    if (
        !validText(
            input.name,
            128,
            false
        )
    ) {
        error =
            "Имя камеры должно содержать 1–128 символов.";

        return false;
    }

    if (
        !validText(
            input.rtsp_url,
            2048,
            false
        )
        ||
        !parseRtspEndpoint(
            input.rtsp_url
        )
    ) {
        error =
            "Некорректный RTSP URL. Используйте rtsp:// или rtsps:// без логина и пароля в URL.";

        return false;
    }

    if (
        !validText(
            input.onvif_xaddr,
            2048,
            true
        )
        ||
        (
            !input.onvif_xaddr.empty()
            &&
            input.onvif_xaddr.rfind(
                "http://",
                0
            ) != 0
            &&
            input.onvif_xaddr.rfind(
                "https://",
                0
            ) != 0
        )
    ) {
        error =
            "Некорректный ONVIF XAddr.";

        return false;
    }

    if (
        !validText(
            input.manufacturer,
            256,
            true
        )
        ||
        !validText(
            input.model,
            256,
            true
        )
        ||
        !validText(
            input.firmware_version,
            256,
            true
        )
        ||
        !validText(
            input.serial_number,
            256,
            true
        )
        ||
        !validText(
            input.hardware_id,
            256,
            true
        )
        ||
        !validText(
            input.ptz_xaddr,
            2048,
            true
        )
        ||
        !validText(
            input.ptz_profile_token,
            512,
            true
        )
    ) {
        error =
            "Некорректная ONVIF информация камеры.";

        return false;
    }

    if (
        !validText(
            input.username,
            128,
            true
        )
        ||
        !validText(
            input.password,
            512,
            true
        )
    ) {
        error =
            "Некорректные учётные данные камеры.";

        return false;
    }

    return true;
}

class Statement {
public:
    Statement(
        sqlite3* database,
        const char* sql
    )
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
    if (value.empty()) {
        return
            sqlite3_bind_null(
                statement,
                index
            ) == SQLITE_OK;
    }

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
        static_cast<
            const unsigned char*
        >(
            sqlite3_column_blob(
                statement,
                column
            )
        );

    const auto size =
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

    return {
        data,
        data + size
    };
}

std::string columnText(
    sqlite3_stmt* statement,
    int column
)
{
    const auto* value =
        sqlite3_column_text(
            statement,
            column
        );

    return
        value
        ? reinterpret_cast<
            const char*
          >(value)
        : "";
}

bool writeExact(
    int descriptor,
    const unsigned char* data,
    std::size_t size
)
{
    std::size_t offset = 0;

    while (offset < size) {
        const auto written =
            ::write(
                descriptor,
                data + offset,
                size - offset
            );

        if (written <= 0)
            return false;

        offset +=
            static_cast<
                std::size_t
            >(written);
    }

    return true;
}

bool readExact(
    int descriptor,
    unsigned char* data,
    std::size_t size
)
{
    std::size_t offset = 0;

    while (offset < size) {
        const auto received =
            ::read(
                descriptor,
                data + offset,
                size - offset
            );

        if (received <= 0)
            return false;

        offset +=
            static_cast<
                std::size_t
            >(received);
    }

    return true;
}

struct EncryptedSecret {
    std::vector<unsigned char> nonce;
    std::vector<unsigned char> cipher;
    std::vector<unsigned char> tag;
};

bool encryptSecret(
    const std::vector<unsigned char>& key,
    const std::string& plaintext,
    EncryptedSecret& output
)
{
    output = {};

    if (plaintext.empty())
        return true;

    if (
        key.size() !=
        secret_key_size
    ) {
        return false;
    }

    output.nonce.resize(
        gcm_nonce_size
    );

    output.tag.resize(
        gcm_tag_size
    );

    output.cipher.resize(
        plaintext.size()
    );

    if (
        RAND_bytes(
            output.nonce.data(),
            static_cast<int>(
                output.nonce.size()
            )
        ) != 1
    ) {
        return false;
    }

    EVP_CIPHER_CTX* context =
        EVP_CIPHER_CTX_new();

    if (!context)
        return false;

    int length = 0;
    int final_length = 0;

    const bool success =
        EVP_EncryptInit_ex(
            context,
            EVP_aes_256_gcm(),
            nullptr,
            nullptr,
            nullptr
        ) == 1
        &&
        EVP_CIPHER_CTX_ctrl(
            context,
            EVP_CTRL_GCM_SET_IVLEN,
            static_cast<int>(
                output.nonce.size()
            ),
            nullptr
        ) == 1
        &&
        EVP_EncryptInit_ex(
            context,
            nullptr,
            nullptr,
            key.data(),
            output.nonce.data()
        ) == 1
        &&
        EVP_EncryptUpdate(
            context,
            output.cipher.data(),
            &length,
            reinterpret_cast<
                const unsigned char*
            >(
                plaintext.data()
            ),
            static_cast<int>(
                plaintext.size()
            )
        ) == 1
        &&
        EVP_EncryptFinal_ex(
            context,
            output.cipher.data() +
                length,
            &final_length
        ) == 1
        &&
        EVP_CIPHER_CTX_ctrl(
            context,
            EVP_CTRL_GCM_GET_TAG,
            static_cast<int>(
                output.tag.size()
            ),
            output.tag.data()
        ) == 1;

    EVP_CIPHER_CTX_free(
        context
    );

    if (!success) {
        output = {};
        return false;
    }

    output.cipher.resize(
        static_cast<std::size_t>(
            length + final_length
        )
    );

    return true;
}

bool decryptSecret(
    const std::vector<unsigned char>& key,
    const std::vector<unsigned char>& nonce,
    const std::vector<unsigned char>& cipher,
    const std::vector<unsigned char>& tag,
    std::string& plaintext
)
{
    plaintext.clear();

    if (cipher.empty())
        return true;

    if (
        key.size() != secret_key_size
        ||
        nonce.size() != gcm_nonce_size
        ||
        tag.size() != gcm_tag_size
    ) {
        return false;
    }

    std::vector<unsigned char>
        output(
            cipher.size()
        );

    EVP_CIPHER_CTX* context =
        EVP_CIPHER_CTX_new();

    if (!context)
        return false;

    int length = 0;
    int final_length = 0;

    const bool success =
        EVP_DecryptInit_ex(
            context,
            EVP_aes_256_gcm(),
            nullptr,
            nullptr,
            nullptr
        ) == 1
        &&
        EVP_CIPHER_CTX_ctrl(
            context,
            EVP_CTRL_GCM_SET_IVLEN,
            static_cast<int>(
                nonce.size()
            ),
            nullptr
        ) == 1
        &&
        EVP_DecryptInit_ex(
            context,
            nullptr,
            nullptr,
            key.data(),
            nonce.data()
        ) == 1
        &&
        EVP_DecryptUpdate(
            context,
            output.data(),
            &length,
            cipher.data(),
            static_cast<int>(
                cipher.size()
            )
        ) == 1
        &&
        EVP_CIPHER_CTX_ctrl(
            context,
            EVP_CTRL_GCM_SET_TAG,
            static_cast<int>(
                tag.size()
            ),
            const_cast<
                unsigned char*
            >(
                tag.data()
            )
        ) == 1
        &&
        EVP_DecryptFinal_ex(
            context,
            output.data() +
                length,
            &final_length
        ) == 1;

    EVP_CIPHER_CTX_free(
        context
    );

    if (!success) {
        OPENSSL_cleanse(
            output.data(),
            output.size()
        );

        return false;
    }

    plaintext.assign(
        reinterpret_cast<
            const char*
        >(
            output.data()
        ),
        static_cast<
            std::size_t
        >(
            length +
            final_length
        )
    );

    OPENSSL_cleanse(
        output.data(),
        output.size()
    );

    return true;
}

std::string percentEncodeUserInfo(
    const std::string& value
)
{
    static constexpr char hex[] =
        "0123456789ABCDEF";

    std::string result;

    for (
        const unsigned char character :
        value
    ) {
        const bool safe =
            (
                character >= 'A'
                &&
                character <= 'Z'
            )
            ||
            (
                character >= 'a'
                &&
                character <= 'z'
            )
            ||
            (
                character >= '0'
                &&
                character <= '9'
            )
            ||
            character == '-'
            ||
            character == '.'
            ||
            character == '_'
            ||
            character == '~';

        if (safe) {
            result.push_back(
                static_cast<char>(
                    character
                )
            );
        }
        else {
            result.push_back('%');
            result.push_back(
                hex[
                    (
                        character >> 4
                    ) & 0x0f
                ]
            );
            result.push_back(
                hex[
                    character & 0x0f
                ]
            );
        }
    }

    return result;
}

std::string authenticatedRtspUrl(
    const std::string& url,
    const std::string& username,
    const std::string& password
)
{
    if (username.empty())
        return url;

    const auto scheme_end =
        url.find("://");

    if (
        scheme_end ==
        std::string::npos
    ) {
        return url;
    }

    std::string credentials =
        percentEncodeUserInfo(
            username
        );

    if (!password.empty()) {
        credentials +=
            ":"
            +
            percentEncodeUserInfo(
                password
            );
    }

    credentials += "@";

    return
        url.substr(
            0,
            scheme_end + 3
        )
        +
        credentials
        +
        url.substr(
            scheme_end + 3
        );
}

void cleanseString(
    std::string& value
)
{
    if (!value.empty()) {
        OPENSSL_cleanse(
            value.data(),
            value.size()
        );
    }

    value.clear();
}

std::string sanitizeMediaError(
    std::string message,
    const std::string& authenticated_url,
    const std::string& password
)
{
    auto replace_all =
        [](
            std::string& target,
            const std::string& needle,
            const std::string& replacement
        ) {
            if (needle.empty())
                return;

            std::size_t position = 0;

            while (
                (
                    position =
                        target.find(
                            needle,
                            position
                        )
                ) !=
                std::string::npos
            ) {
                target.replace(
                    position,
                    needle.size(),
                    replacement
                );

                position +=
                    replacement.size();
            }
        };

    replace_all(
        message,
        authenticated_url,
        "<camera-stream>"
    );

    replace_all(
        message,
        password,
        "***"
    );

    replace_all(
        message,
        percentEncodeUserInfo(
            password
        ),
        "***"
    );

    return message;
}

bool connectEndpoint(
    const RtspEndpoint& endpoint,
    std::string& error
)
{
    addrinfo hints{};
    hints.ai_family = AF_UNSPEC;
    hints.ai_socktype = SOCK_STREAM;

    addrinfo* addresses = nullptr;

    const int lookup =
        ::getaddrinfo(
            endpoint.host.c_str(),
            endpoint.port.c_str(),
            &hints,
            &addresses
        );

    if (lookup != 0) {
        error =
            "Не удалось определить адрес камеры.";

        return false;
    }

    bool connected = false;

    for (
        auto* address = addresses;
        address;
        address = address->ai_next
    ) {
        const int descriptor =
            ::socket(
                address->ai_family,
                address->ai_socktype,
                address->ai_protocol
            );

        if (descriptor < 0)
            continue;

        const int flags =
            ::fcntl(
                descriptor,
                F_GETFL,
                0
            );

        if (flags >= 0) {
            ::fcntl(
                descriptor,
                F_SETFL,
                flags | O_NONBLOCK
            );
        }

        const int result =
            ::connect(
                descriptor,
                address->ai_addr,
                address->ai_addrlen
            );

        if (result == 0) {
            connected = true;
        }
        else if (errno == EINPROGRESS) {
            fd_set write_set;
            FD_ZERO(&write_set);
            FD_SET(
                descriptor,
                &write_set
            );

            timeval timeout{};
            timeout.tv_sec =
                probe_timeout_seconds;

            const int selected =
                ::select(
                    descriptor + 1,
                    nullptr,
                    &write_set,
                    nullptr,
                    &timeout
                );

            if (
                selected > 0
                &&
                FD_ISSET(
                    descriptor,
                    &write_set
                )
            ) {
                int socket_error = 0;
                socklen_t size =
                    sizeof(socket_error);

                if (
                    ::getsockopt(
                        descriptor,
                        SOL_SOCKET,
                        SO_ERROR,
                        &socket_error,
                        &size
                    ) == 0
                    &&
                    socket_error == 0
                ) {
                    connected = true;
                }
            }
        }

        ::close(descriptor);

        if (connected)
            break;
    }

    ::freeaddrinfo(addresses);

    if (!connected) {
        error =
            "RTSP-порт камеры недоступен.";
    }

    return connected;
}

}

struct CameraManager::Impl {
    mutable std::mutex mutex;
    sqlite3* database{nullptr};
    std::filesystem::path runtime_directory;
    std::filesystem::path database_file;
    std::filesystem::path key_file;
    std::vector<unsigned char> key;
    bool initialized{false};

    ~Impl()
    {
        if (database)
            sqlite3_close(database);
    }

    bool loadOrCreateKey(
        std::string& error
    )
    {
        key.resize(
            secret_key_size
        );

        const int existing =
            ::open(
                key_file.c_str(),
                O_RDONLY
            );

        if (existing >= 0) {
            const bool ok =
                readExact(
                    existing,
                    key.data(),
                    key.size()
                );

            unsigned char extra = 0;

            const bool exact =
                ok
                &&
                ::read(
                    existing,
                    &extra,
                    1
                ) == 0;

            ::close(existing);

            if (!exact) {
                error =
                    "Camera secret key has an invalid size.";

                return false;
            }

            ::chmod(
                key_file.c_str(),
                0600
            );

            return true;
        }

        if (
            RAND_bytes(
                key.data(),
                static_cast<int>(
                    key.size()
                )
            ) != 1
        ) {
            error =
                "Unable to generate camera secret key.";

            return false;
        }

        const int created =
            ::open(
                key_file.c_str(),
                O_WRONLY |
                    O_CREAT |
                    O_EXCL,
                0600
            );

        if (created < 0) {
            error =
                "Unable to create camera secret key.";

            return false;
        }

        const bool written =
            writeExact(
                created,
                key.data(),
                key.size()
            );

        ::fsync(created);
        ::close(created);

        if (!written) {
            std::error_code ignored;

            std::filesystem::remove(
                key_file,
                ignored
            );

            error =
                "Unable to write camera secret key.";

            return false;
        }

        return true;
    }

    bool openDatabase(
        std::string& error
    )
    {
        if (
            sqlite3_open_v2(
                database_file.c_str(),
                &database,
                SQLITE_OPEN_READWRITE |
                    SQLITE_OPEN_CREATE |
                    SQLITE_OPEN_FULLMUTEX,
                nullptr
            ) != SQLITE_OK
        ) {
            error =
                "Unable to open camera database.";

            return false;
        }

        sqlite3_busy_timeout(
            database,
            3000
        );

        const char* schema =
            "PRAGMA journal_mode=WAL;"
            "PRAGMA foreign_keys=ON;"
            "CREATE TABLE IF NOT EXISTS cameras("
            "id INTEGER PRIMARY KEY AUTOINCREMENT,"
            "name TEXT NOT NULL UNIQUE,"
            "rtsp_url TEXT NOT NULL,"
            "onvif_xaddr TEXT NOT NULL DEFAULT '',"
            "username TEXT NOT NULL DEFAULT '',"
            "password_nonce BLOB,"
            "password_cipher BLOB,"
            "password_tag BLOB,"
            "enabled INTEGER NOT NULL DEFAULT 1,"
            "status TEXT NOT NULL DEFAULT 'unknown',"
            "last_error TEXT NOT NULL DEFAULT '',"
            "last_seen_at INTEGER NOT NULL DEFAULT 0,"
            "created_at INTEGER NOT NULL,"
            "updated_at INTEGER NOT NULL"
            ");"
            "CREATE INDEX IF NOT EXISTS idx_cameras_enabled "
            "ON cameras(enabled);"
            "CREATE TABLE IF NOT EXISTS camera_device_info("
            "camera_id INTEGER PRIMARY KEY,"
            "manufacturer TEXT NOT NULL DEFAULT '',"
            "model TEXT NOT NULL DEFAULT '',"
            "firmware_version TEXT NOT NULL DEFAULT '',"
            "serial_number TEXT NOT NULL DEFAULT '',"
            "hardware_id TEXT NOT NULL DEFAULT '',"
            "ptz_xaddr TEXT NOT NULL DEFAULT '',"
            "ptz_profile_token TEXT NOT NULL DEFAULT '',"
            "FOREIGN KEY(camera_id) REFERENCES cameras(id) ON DELETE CASCADE"
            ");";

        char* message = nullptr;

        if (
            sqlite3_exec(
                database,
                schema,
                nullptr,
                nullptr,
                &message
            ) != SQLITE_OK
        ) {
            error =
                message
                ? message
                : "Unable to initialize camera database.";

            sqlite3_free(message);

            return false;
        }

        bool has_onvif_xaddr = false;

        {
            Statement columns(
                database,
                "PRAGMA table_info(cameras);"
            );

            if (!columns) {
                error =
                    "Unable to inspect camera database schema.";

                return false;
            }

            while (
                sqlite3_step(
                    columns.get()
                ) == SQLITE_ROW
            ) {
                if (
                    columnText(
                        columns.get(),
                        1
                    ) == "onvif_xaddr"
                ) {
                    has_onvif_xaddr =
                        true;

                    break;
                }
            }
        }

        if (!has_onvif_xaddr) {
            char* migration_message =
                nullptr;

            if (
                sqlite3_exec(
                    database,
                    "ALTER TABLE cameras "
                    "ADD COLUMN onvif_xaddr "
                    "TEXT NOT NULL DEFAULT '';",
                    nullptr,
                    nullptr,
                    &migration_message
                ) != SQLITE_OK
            ) {
                error =
                    migration_message
                    ? migration_message
                    : "Unable to migrate camera database.";

                sqlite3_free(
                    migration_message
                );

                return false;
            }
        }

        bool has_ptz_xaddr = false;
        bool has_ptz_profile_token = false;

        {
            Statement columns(
                database,
                "PRAGMA table_info(camera_device_info);"
            );

            if (!columns) {
                error =
                    "Unable to inspect camera device info schema.";

                return false;
            }

            while (
                sqlite3_step(
                    columns.get()
                ) == SQLITE_ROW
            ) {
                const auto name =
                    columnText(
                        columns.get(),
                        1
                    );

                if (name == "ptz_xaddr")
                    has_ptz_xaddr = true;
                else if (
                    name ==
                        "ptz_profile_token"
                ) {
                    has_ptz_profile_token =
                        true;
                }
            }
        }

        auto add_device_info_column =
            [&](
                const char* sql
            ) -> bool {
                char* migration_message =
                    nullptr;

                if (
                    sqlite3_exec(
                        database,
                        sql,
                        nullptr,
                        nullptr,
                        &migration_message
                    ) != SQLITE_OK
                ) {
                    error =
                        migration_message
                        ? migration_message
                        : "Unable to migrate camera device info.";

                    sqlite3_free(
                        migration_message
                    );

                    return false;
                }

                return true;
            };

        if (
            !has_ptz_xaddr
            &&
            !add_device_info_column(
                "ALTER TABLE camera_device_info "
                "ADD COLUMN ptz_xaddr "
                "TEXT NOT NULL DEFAULT '';"
            )
        ) {
            return false;
        }

        if (
            !has_ptz_profile_token
            &&
            !add_device_info_column(
                "ALTER TABLE camera_device_info "
                "ADD COLUMN ptz_profile_token "
                "TEXT NOT NULL DEFAULT '';"
            )
        ) {
            return false;
        }

        ::chmod(
            database_file.c_str(),
            0600
        );

        return true;
    }

    CameraInfo readCamera(
        sqlite3_stmt* statement
    ) const
    {
        CameraInfo camera;

        camera.id =
            sqlite3_column_int64(
                statement,
                0
            );

        camera.name =
            columnText(
                statement,
                1
            );

        camera.rtsp_url =
            columnText(
                statement,
                2
            );

        camera.onvif_xaddr =
            columnText(
                statement,
                3
            );

        camera.username =
            columnText(
                statement,
                4
            );

        camera.has_password =
            sqlite3_column_type(
                statement,
                5
            ) != SQLITE_NULL
            &&
            sqlite3_column_bytes(
                statement,
                5
            ) > 0;

        camera.enabled =
            sqlite3_column_int(
                statement,
                6
            ) != 0;

        camera.status =
            columnText(
                statement,
                7
            );

        camera.last_error =
            columnText(
                statement,
                8
            );

        camera.last_seen_at =
            sqlite3_column_int64(
                statement,
                9
            );

        camera.created_at =
            sqlite3_column_int64(
                statement,
                10
            );

        camera.updated_at =
            sqlite3_column_int64(
                statement,
                11
            );

        return camera;
    }

    std::optional<CameraInfo>
    findCamera(
        std::int64_t id,
        std::string& error
    ) const
    {
        Statement statement(
            database,
            "SELECT id,name,rtsp_url,onvif_xaddr,username,password_cipher,"
            "enabled,status,last_error,last_seen_at,created_at,updated_at "
            "FROM cameras WHERE id=?;"
        );

        if (!statement) {
            error =
                "Unable to prepare camera query.";

            return std::nullopt;
        }

        sqlite3_bind_int64(
            statement.get(),
            1,
            id
        );

        if (
            sqlite3_step(
                statement.get()
            ) != SQLITE_ROW
        ) {
            error =
                "Камера не найдена.";

            return std::nullopt;
        }

        auto camera =
            readCamera(
                statement.get()
            );

        loadDeviceInfo(
            camera
        );

        return camera;
    }

    void loadDeviceInfo(
        CameraInfo& camera
    ) const
    {
        Statement statement(
            database,
            "SELECT manufacturer,model,firmware_version,"
            "serial_number,hardware_id,ptz_xaddr,ptz_profile_token "
            "FROM camera_device_info WHERE camera_id=?;"
        );

        if (!statement)
            return;

        sqlite3_bind_int64(
            statement.get(),
            1,
            camera.id
        );

        if (
            sqlite3_step(
                statement.get()
            ) != SQLITE_ROW
        ) {
            return;
        }

        camera.manufacturer =
            columnText(
                statement.get(),
                0
            );

        camera.model =
            columnText(
                statement.get(),
                1
            );

        camera.firmware_version =
            columnText(
                statement.get(),
                2
            );

        camera.serial_number =
            columnText(
                statement.get(),
                3
            );

        camera.hardware_id =
            columnText(
                statement.get(),
                4
            );

        camera.ptz_xaddr =
            columnText(
                statement.get(),
                5
            );

        camera.ptz_profile_token =
            columnText(
                statement.get(),
                6
            );
    }

    bool saveDeviceInfo(
        std::int64_t camera_id,
        const CameraInput& input
    )
    {
        Statement statement(
            database,
            "INSERT INTO camera_device_info("
            "camera_id,manufacturer,model,firmware_version,"
            "serial_number,hardware_id,ptz_xaddr,ptz_profile_token"
            ") VALUES(?,?,?,?,?,?,?,?) "
            "ON CONFLICT(camera_id) DO UPDATE SET "
            "manufacturer=excluded.manufacturer,"
            "model=excluded.model,"
            "firmware_version=excluded.firmware_version,"
            "serial_number=excluded.serial_number,"
            "hardware_id=excluded.hardware_id,"
            "ptz_xaddr=excluded.ptz_xaddr,"
            "ptz_profile_token=excluded.ptz_profile_token;"
        );

        if (!statement)
            return false;

        return
            sqlite3_bind_int64(
                statement.get(),
                1,
                camera_id
            ) == SQLITE_OK
            &&
            bindText(
                statement.get(),
                2,
                input.manufacturer
            )
            &&
            bindText(
                statement.get(),
                3,
                input.model
            )
            &&
            bindText(
                statement.get(),
                4,
                input.firmware_version
            )
            &&
            bindText(
                statement.get(),
                5,
                input.serial_number
            )
            &&
            bindText(
                statement.get(),
                6,
                input.hardware_id
            )
            &&
            bindText(
                statement.get(),
                7,
                input.ptz_xaddr
            )
            &&
            bindText(
                statement.get(),
                8,
                input.ptz_profile_token
            )
            &&
            sqlite3_step(
                statement.get()
            ) == SQLITE_DONE;
    }

    bool setStatus(
        std::int64_t id,
        const std::string& status,
        const std::string& last_error,
        std::int64_t last_seen
    )
    {
        Statement statement(
            database,
            "UPDATE cameras "
            "SET status=?,last_error=?,last_seen_at=? "
            "WHERE id=?;"
        );

        if (!statement)
            return false;

        bindText(
            statement.get(),
            1,
            status
        );

        bindText(
            statement.get(),
            2,
            last_error
        );

        sqlite3_bind_int64(
            statement.get(),
            3,
            last_seen
        );

        sqlite3_bind_int64(
            statement.get(),
            4,
            id
        );

        return
            sqlite3_step(
                statement.get()
            ) == SQLITE_DONE;
    }

    bool streamAccess(
        std::int64_t id,
        std::string& public_url,
        std::string& authenticated_url,
        std::string& password,
        std::string& error
    ) const
    {
        Statement statement(
            database,
            "SELECT rtsp_url,username,"
            "password_nonce,password_cipher,password_tag,enabled "
            "FROM cameras WHERE id=?;"
        );

        if (!statement) {
            error =
                "Unable to prepare camera credential query.";

            return false;
        }

        sqlite3_bind_int64(
            statement.get(),
            1,
            id
        );

        if (
            sqlite3_step(
                statement.get()
            ) != SQLITE_ROW
        ) {
            error =
                "Камера не найдена.";

            return false;
        }

        if (
            sqlite3_column_int(
                statement.get(),
                5
            ) == 0
        ) {
            error =
                "Камера отключена.";

            return false;
        }

        public_url =
            columnText(
                statement.get(),
                0
            );

        const auto username =
            columnText(
                statement.get(),
                1
            );

        const auto nonce =
            readBlob(
                statement.get(),
                2
            );

        const auto cipher =
            readBlob(
                statement.get(),
                3
            );

        const auto tag =
            readBlob(
                statement.get(),
                4
            );

        if (
            !decryptSecret(
                key,
                nonce,
                cipher,
                tag,
                password
            )
        ) {
            error =
                "Не удалось расшифровать пароль камеры.";

            return false;
        }

        authenticated_url =
            authenticatedRtspUrl(
                public_url,
                username,
                password
            );

        return true;
    }

    bool onvifControlAccess(
        std::int64_t id,
        std::string& ptz_xaddr,
        std::string& profile_token,
        std::string& username,
        std::string& password,
        std::string& error
    ) const
    {
        Statement statement(
            database,
            "SELECT c.username,c.password_nonce,c.password_cipher,"
            "c.password_tag,c.enabled,"
            "COALESCE(d.ptz_xaddr,''),"
            "COALESCE(d.ptz_profile_token,'') "
            "FROM cameras c "
            "LEFT JOIN camera_device_info d "
            "ON d.camera_id=c.id "
            "WHERE c.id=?;"
        );

        if (!statement) {
            error =
                "Unable to prepare ONVIF credential query.";

            return false;
        }

        sqlite3_bind_int64(
            statement.get(),
            1,
            id
        );

        if (
            sqlite3_step(
                statement.get()
            ) != SQLITE_ROW
        ) {
            error =
                "Камера не найдена.";

            return false;
        }

        if (
            sqlite3_column_int(
                statement.get(),
                4
            ) == 0
        ) {
            error =
                "Камера отключена.";

            return false;
        }

        username =
            columnText(
                statement.get(),
                0
            );

        const auto nonce =
            readBlob(
                statement.get(),
                1
            );

        const auto cipher =
            readBlob(
                statement.get(),
                2
            );

        const auto tag =
            readBlob(
                statement.get(),
                3
            );

        ptz_xaddr =
            columnText(
                statement.get(),
                5
            );

        profile_token =
            columnText(
                statement.get(),
                6
            );

        if (
            ptz_xaddr.empty()
            ||
            profile_token.empty()
        ) {
            error =
                "PTZ не поддерживается или ещё не обнаружен через ONVIF.";

            return false;
        }

        if (
            !decryptSecret(
                key,
                nonce,
                cipher,
                tag,
                password
            )
        ) {
            error =
                "Не удалось расшифровать пароль камеры.";

            return false;
        }

        return true;
    }
};

CameraManager::CameraManager()
    : impl_(
        std::make_unique<Impl>()
    )
{
}

CameraManager::~CameraManager()
{
    stop();
}

bool CameraManager::initialize(
    const std::string& runtime_directory,
    std::string& error
)
{
    std::lock_guard<std::mutex>
        lock(impl_->mutex);

    if (impl_->initialized)
        return true;

    impl_->runtime_directory =
        runtime_directory;

    impl_->database_file =
        impl_->runtime_directory /
        "cameras.db";

    impl_->key_file =
        impl_->runtime_directory /
        "secret.key";

    std::error_code filesystem_error;

    std::filesystem::create_directories(
        impl_->runtime_directory,
        filesystem_error
    );

    if (filesystem_error) {
        error =
            "Unable to create camera runtime directory: "
            + filesystem_error.message();

        return false;
    }

    ::chmod(
        impl_->runtime_directory.c_str(),
        0700
    );

    if (
        !impl_->loadOrCreateKey(
            error
        )
        ||
        !impl_->openDatabase(
            error
        )
    ) {
        return false;
    }

    impl_->initialized = true;

    return true;
}

bool CameraManager::start(
    std::string& error
)
{
    if (!impl_->initialized) {
        error =
            "Camera Manager is not initialized.";

        return false;
    }

    if (running_)
        return true;

    running_ = true;

    worker_ =
        std::thread(
            &CameraManager::workerLoop,
            this
        );

    return true;
}

void CameraManager::stop()
{
    if (!running_)
        return;

    running_ = false;

    if (worker_.joinable())
        worker_.join();
}

bool CameraManager::healthy() const
{
    return impl_->initialized;
}

std::string
CameraManager::healthMessage() const
{
    std::string error;
    const auto items =
        cameras(error);

    if (!error.empty())
        return error;

    std::size_t online = 0;
    std::size_t offline = 0;

    for (const auto& camera : items) {
        if (camera.status == "online")
            ++online;
        else if (
            camera.enabled
            &&
            camera.status == "offline"
        ) {
            ++offline;
        }
    }

    return
        "Cameras: "
        + std::to_string(
            items.size()
        )
        + ", online: "
        + std::to_string(online)
        + ", offline: "
        + std::to_string(offline)
        + ".";
}

std::vector<CameraInfo>
CameraManager::cameras(
    std::string& error
) const
{
    std::lock_guard<std::mutex>
        lock(impl_->mutex);

    std::vector<CameraInfo> result;

    if (!impl_->initialized) {
        error =
            "Camera Manager is not initialized.";

        return result;
    }

    Statement statement(
        impl_->database,
        "SELECT id,name,rtsp_url,onvif_xaddr,username,password_cipher,"
        "enabled,status,last_error,last_seen_at,created_at,updated_at "
        "FROM cameras ORDER BY name COLLATE NOCASE,id;"
    );

    if (!statement) {
        error =
            "Unable to query camera database.";

        return result;
    }

    while (
        sqlite3_step(
            statement.get()
        ) == SQLITE_ROW
    ) {
        auto camera =
            impl_->readCamera(
                statement.get()
            );

        impl_->loadDeviceInfo(
            camera
        );

        result.push_back(
            std::move(camera)
        );
    }

    return result;
}

CameraResult CameraManager::create(
    const CameraInput& input
)
{
    std::string validation_error;

    if (
        !validateInput(
            input,
            validation_error
        )
    ) {
        return {
            false,
            "invalid_camera",
            validation_error,
            0
        };
    }

    std::lock_guard<std::mutex>
        lock(impl_->mutex);

    if (!impl_->initialized) {
        return {
            false,
            "not_initialized",
            "Camera Manager is not initialized.",
            0
        };
    }

    EncryptedSecret password;

    if (
        !encryptSecret(
            impl_->key,
            input.password,
            password
        )
    ) {
        return {
            false,
            "secret_failed",
            "Не удалось защитить пароль камеры.",
            0
        };
    }

    Statement statement(
        impl_->database,
        "INSERT INTO cameras("
        "name,rtsp_url,onvif_xaddr,username,"
        "password_nonce,password_cipher,password_tag,"
        "enabled,status,last_error,last_seen_at,created_at,updated_at"
        ") VALUES(?,?,?,?,?,?,?,?,?,'',0,?,?);"
    );

    if (!statement) {
        return {
            false,
            "database_error",
            "Не удалось подготовить сохранение камеры.",
            0
        };
    }

    const auto now = unixNow();

    const bool bound =
        bindText(
            statement.get(),
            1,
            input.name
        )
        &&
        bindText(
            statement.get(),
            2,
            input.rtsp_url
        )
        &&
        bindText(
            statement.get(),
            3,
            input.onvif_xaddr
        )
        &&
        bindText(
            statement.get(),
            4,
            input.username
        )
        &&
        bindBlob(
            statement.get(),
            5,
            password.nonce
        )
        &&
        bindBlob(
            statement.get(),
            6,
            password.cipher
        )
        &&
        bindBlob(
            statement.get(),
            7,
            password.tag
        )
        &&
        sqlite3_bind_int(
            statement.get(),
            8,
            input.enabled
                ? 1
                : 0
        ) == SQLITE_OK
        &&
        bindText(
            statement.get(),
            9,
            input.enabled
                ? "unknown"
                : "disabled"
        )
        &&
        sqlite3_bind_int64(
            statement.get(),
            10,
            now
        ) == SQLITE_OK
        &&
        sqlite3_bind_int64(
            statement.get(),
            11,
            now
        ) == SQLITE_OK;

    if (
        !bound
        ||
        sqlite3_step(
            statement.get()
        ) != SQLITE_DONE
    ) {
        return {
            false,
            "database_error",
            "Не удалось сохранить камеру. Проверьте уникальность имени.",
            0
        };
    }

    const auto camera_id =
        sqlite3_last_insert_rowid(
            impl_->database
        );

    if (
        !impl_->saveDeviceInfo(
            camera_id,
            input
        )
    ) {
        return {
            false,
            "database_error",
            "Камера добавлена, но не удалось сохранить ONVIF информацию.",
            camera_id
        };
    }

    return {
        true,
        "ok",
        "Камера добавлена.",
        camera_id
    };
}

CameraResult CameraManager::update(
    std::int64_t id,
    const CameraInput& input
)
{
    if (id <= 0) {
        return {
            false,
            "invalid_id",
            "Некорректный ID камеры.",
            0
        };
    }

    std::string validation_error;

    if (
        !validateInput(
            input,
            validation_error
        )
    ) {
        return {
            false,
            "invalid_camera",
            validation_error,
            id
        };
    }

    std::lock_guard<std::mutex>
        lock(impl_->mutex);

    if (!impl_->initialized) {
        return {
            false,
            "not_initialized",
            "Camera Manager is not initialized.",
            id
        };
    }

    std::string find_error;

    if (
        !impl_->findCamera(
            id,
            find_error
        )
    ) {
        return {
            false,
            "not_found",
            find_error,
            id
        };
    }

    const auto now = unixNow();

    if (input.update_password) {
        EncryptedSecret password;

        if (
            !encryptSecret(
                impl_->key,
                input.password,
                password
            )
        ) {
            return {
                false,
                "secret_failed",
                "Не удалось защитить пароль камеры.",
                id
            };
        }

        Statement statement(
            impl_->database,
            "UPDATE cameras SET "
            "name=?,rtsp_url=?,onvif_xaddr=?,username=?,"
            "password_nonce=?,password_cipher=?,password_tag=?,"
            "enabled=?,status=?,last_error='',updated_at=? "
            "WHERE id=?;"
        );

        if (!statement) {
            return {
                false,
                "database_error",
                "Не удалось подготовить изменение камеры.",
                id
            };
        }

        const bool bound =
            bindText(
                statement.get(),
                1,
                input.name
            )
            &&
            bindText(
                statement.get(),
                2,
                input.rtsp_url
            )
            &&
            bindText(
                statement.get(),
                3,
                input.onvif_xaddr
            )
            &&
            bindText(
                statement.get(),
                4,
                input.username
            )
            &&
            bindBlob(
                statement.get(),
                5,
                password.nonce
            )
            &&
            bindBlob(
                statement.get(),
                6,
                password.cipher
            )
            &&
            bindBlob(
                statement.get(),
                7,
                password.tag
            )
            &&
            sqlite3_bind_int(
                statement.get(),
                8,
                input.enabled
                    ? 1
                    : 0
            ) == SQLITE_OK
            &&
            bindText(
                statement.get(),
                9,
                input.enabled
                    ? "unknown"
                    : "disabled"
            )
            &&
            sqlite3_bind_int64(
                statement.get(),
                10,
                now
            ) == SQLITE_OK
            &&
            sqlite3_bind_int64(
                statement.get(),
                11,
                id
            ) == SQLITE_OK;

        if (
            !bound
            ||
            sqlite3_step(
                statement.get()
            ) != SQLITE_DONE
        ) {
            return {
                false,
                "database_error",
                "Не удалось изменить камеру. Проверьте уникальность имени.",
                id
            };
        }
    }
    else {
        Statement statement(
            impl_->database,
            "UPDATE cameras SET "
            "name=?,rtsp_url=?,onvif_xaddr=?,username=?,enabled=?,"
            "status=?,last_error='',updated_at=? "
            "WHERE id=?;"
        );

        if (!statement) {
            return {
                false,
                "database_error",
                "Не удалось подготовить изменение камеры.",
                id
            };
        }

        const bool bound =
            bindText(
                statement.get(),
                1,
                input.name
            )
            &&
            bindText(
                statement.get(),
                2,
                input.rtsp_url
            )
            &&
            bindText(
                statement.get(),
                3,
                input.onvif_xaddr
            )
            &&
            bindText(
                statement.get(),
                4,
                input.username
            )
            &&
            sqlite3_bind_int(
                statement.get(),
                5,
                input.enabled
                    ? 1
                    : 0
            ) == SQLITE_OK
            &&
            bindText(
                statement.get(),
                6,
                input.enabled
                    ? "unknown"
                    : "disabled"
            )
            &&
            sqlite3_bind_int64(
                statement.get(),
                7,
                now
            ) == SQLITE_OK
            &&
            sqlite3_bind_int64(
                statement.get(),
                8,
                id
            ) == SQLITE_OK;

        if (
            !bound
            ||
            sqlite3_step(
                statement.get()
            ) != SQLITE_DONE
        ) {
            return {
                false,
                "database_error",
                "Не удалось изменить камеру. Проверьте уникальность имени.",
                id
            };
        }
    }

    if (
        !impl_->saveDeviceInfo(
            id,
            input
        )
    ) {
        return {
            false,
            "database_error",
            "Камера сохранена, но не удалось сохранить ONVIF информацию.",
            id
        };
    }

    return {
        true,
        "ok",
        "Камера сохранена.",
        id
    };
}

CameraResult CameraManager::remove(
    std::int64_t id
)
{
    if (id <= 0) {
        return {
            false,
            "invalid_id",
            "Некорректный ID камеры.",
            id
        };
    }

    std::lock_guard<std::mutex>
        lock(impl_->mutex);

    if (!impl_->initialized) {
        return {
            false,
            "not_initialized",
            "Camera Manager is not initialized.",
            id
        };
    }

    Statement statement(
        impl_->database,
        "DELETE FROM cameras WHERE id=?;"
    );

    if (!statement) {
        return {
            false,
            "database_error",
            "Не удалось подготовить удаление камеры.",
            id
        };
    }

    sqlite3_bind_int64(
        statement.get(),
        1,
        id
    );

    if (
        sqlite3_step(
            statement.get()
        ) != SQLITE_DONE
    ) {
        return {
            false,
            "database_error",
            "Не удалось удалить камеру.",
            id
        };
    }

    if (
        sqlite3_changes(
            impl_->database
        ) == 0
    ) {
        return {
            false,
            "not_found",
            "Камера не найдена.",
            id
        };
    }

    return {
        true,
        "ok",
        "Камера удалена.",
        id
    };
}

CameraResult CameraManager::probe(
    std::int64_t id
)
{
    CameraInfo camera;

    {
        std::lock_guard<std::mutex>
            lock(impl_->mutex);

        if (!impl_->initialized) {
            return {
                false,
                "not_initialized",
                "Camera Manager is not initialized.",
                id
            };
        }

        std::string error;

        const auto found =
            impl_->findCamera(
                id,
                error
            );

        if (!found) {
            return {
                false,
                "not_found",
                error,
                id
            };
        }

        camera = *found;

        if (!camera.enabled) {
            impl_->setStatus(
                id,
                "disabled",
                "",
                0
            );

            return {
                false,
                "disabled",
                "Камера отключена.",
                id
            };
        }
    }

    const auto endpoint =
        parseRtspEndpoint(
            camera.rtsp_url
        );

    std::string probe_error;

    const bool online =
        endpoint
        &&
        connectEndpoint(
            *endpoint,
            probe_error
        );

    {
        std::lock_guard<std::mutex>
            lock(impl_->mutex);

        impl_->setStatus(
            id,
            online
                ? "online"
                : "offline",
            online
                ? ""
                : probe_error,
            online
                ? unixNow()
                : camera.last_seen_at
        );
    }

    return {
        online,
        online
            ? "online"
            : "offline",
        online
            ? "RTSP-порт камеры доступен."
            : probe_error,
        id
    };
}

CameraMediaProbe
CameraManager::mediaProbe(
    std::int64_t id
)
{
    std::string public_url;
    std::string authenticated_url;
    std::string password;
    std::string error;

    {
        std::lock_guard<std::mutex>
            lock(impl_->mutex);

        if (!impl_->initialized) {
            return {
                false,
                "not_initialized",
                "Camera Manager is not initialized.",
                "",
                "",
                0,
                0,
                0.0
            };
        }

        if (
            !impl_->streamAccess(
                id,
                public_url,
                authenticated_url,
                password,
                error
            )
        ) {
            return {
                false,
                "camera_unavailable",
                error,
                "",
                "",
                0,
                0,
                0.0
            };
        }
    }

    auto result =
        CameraMediaTools::probe(
            authenticated_url
        );

    result.message =
        sanitizeMediaError(
            result.message,
            authenticated_url,
            password
        );

    cleanseString(password);
    cleanseString(authenticated_url);

    return result;
}

CameraSnapshot
CameraManager::snapshot(
    std::int64_t id
)
{
    std::string public_url;
    std::string authenticated_url;
    std::string password;
    std::string error;

    {
        std::lock_guard<std::mutex>
            lock(impl_->mutex);

        if (!impl_->initialized) {
            return {
                false,
                "not_initialized",
                "Camera Manager is not initialized.",
                ""
            };
        }

        if (
            !impl_->streamAccess(
                id,
                public_url,
                authenticated_url,
                password,
                error
            )
        ) {
            return {
                false,
                "camera_unavailable",
                error,
                ""
            };
        }
    }

    auto result =
        CameraMediaTools::snapshot(
            authenticated_url
        );

    result.message =
        sanitizeMediaError(
            result.message,
            authenticated_url,
            password
        );

    cleanseString(password);
    cleanseString(authenticated_url);

    return result;
}

std::vector<CameraDiscoveryDevice>
CameraManager::discoverCameras(
    int timeout_ms,
    std::string& error
) const
{
    error.clear();

    OnvifDiscovery onvif_discovery;
    HikvisionSadpDiscovery sadp_discovery;
    LanCameraDiscovery lan_discovery;

    std::string onvif_error;
    std::string sadp_error;
    std::string lan_error;

    const auto onvif_devices =
        onvif_discovery.discover(
            timeout_ms,
            onvif_error
        );

    const auto sadp_devices =
        sadp_discovery.discover(
            timeout_ms,
            sadp_error
        );

    const auto lan_devices =
        lan_discovery.discover(
            90,
            lan_error
        );

    std::map<
        std::string,
        CameraDiscoveryDevice
    > merged;

    for (
        const auto& device :
        onvif_devices
    ) {
        if (
            device.remote_address.empty()
        ) {
            continue;
        }

        auto& item =
            merged[
                device.remote_address
            ];

        item.address =
            device.remote_address;
        item.onvif_xaddr =
            device.xaddr;
        item.onvif = true;

        if (
            item.model_hint.empty()
        ) {
            item.model_hint =
                OnvifDiscovery::scopeValue(
                    device.scopes,
                    "hardware"
                );
        }
    }

    for (
        const auto& device :
        sadp_devices
    ) {
        if (device.address.empty())
            continue;

        auto& item =
            merged[
                device.address
            ];

        item.address =
            device.address;

        // SADP confirms a Hikvision-compatible discovery
        // protocol, not necessarily the retail manufacturer.
        // OEM devices can also answer SADP, so manufacturer
        // stays unknown until authenticated device metadata
        // confirms it.
        item.model_hint =
            device.model;
        item.firmware_hint =
            device.software_version;
        item.serial_hint =
            device.serial_number;
        item.sadp = true;
    }

    for (
        const auto& device :
        lan_devices
    ) {
        if (device.address.empty())
            continue;

        auto& item =
            merged[
                device.address
            ];

        item.address =
            device.address;

        if (
            item.vendor_hint.empty()
        ) {
            item.vendor_hint =
                device.vendor_hint;
        }

        if (
            item.rtsp_port == 0
        ) {
            item.rtsp_port =
                device.rtsp_port;
        }

        if (
            item.suggested_rtsp_url.empty()
        ) {
            item.suggested_rtsp_url =
                LanCameraDiscovery::
                    suggestedRtspUrl(
                        device.address,
                        device.vendor_hint,
                        device.rtsp_port
                    );
        }
    }

    for (
        auto& [address, item] :
        merged
    ) {
        if (
            item.sadp
            &&
            item.rtsp_port == 0
        ) {
            item.rtsp_port = 554;
        }

        if (
            item.suggested_rtsp_url.empty()
            &&
            item.sadp
            &&
            item.rtsp_port > 0
        ) {
            // SADP is sufficient to try the common
            // Hikvision-compatible RTSP path internally,
            // but it is not exposed as a manufacturer claim.
            item.suggested_rtsp_url =
                LanCameraDiscovery::
                    suggestedRtspUrl(
                        address,
                        "Hikvision",
                        item.rtsp_port
                    );
        }
    }

    std::vector<CameraDiscoveryDevice>
        result;

    result.reserve(
        merged.size()
    );

    for (
        auto& [address, device] :
        merged
    ) {
        (void)address;

        result.push_back(
            std::move(device)
        );
    }

    auto ipv4_number =
        [](
            const std::string& address
        ) -> std::uint32_t {
            in_addr value{};

            if (
                ::inet_pton(
                    AF_INET,
                    address.c_str(),
                    &value
                ) != 1
            ) {
                return 0;
            }

            return
                ntohl(
                    value.s_addr
                );
        };

    std::sort(
        result.begin(),
        result.end(),
        [&](
            const CameraDiscoveryDevice& left,
            const CameraDiscoveryDevice& right
        ) {
            return
                ipv4_number(
                    left.address
                )
                <
                ipv4_number(
                    right.address
                );
        }
    );

    if (
        result.empty()
        &&
        !onvif_error.empty()
        &&
        !sadp_error.empty()
        &&
        !lan_error.empty()
    ) {
        error =
            onvif_error
            +
            " "
            +
            sadp_error
            +
            " "
            +
            lan_error;
    }

    return result;
}

OnvifMediaResult
CameraManager::discoverOnvifStreams(
    const std::string& device_xaddr,
    const std::string& username,
    const std::string& password
) const
{
    OnvifMediaClient client;

    return
        client.profiles(
            device_xaddr,
            username,
            password
        );
}

OnvifPtzResult
CameraManager::ptz(
    std::int64_t id,
    const std::string& action,
    double speed
)
{
    if (id <= 0) {
        return {
            false,
            "invalid_id",
            "Некорректный ID камеры."
        };
    }

    std::string ptz_xaddr;
    std::string profile_token;
    std::string username;
    std::string password;
    std::string error;

    {
        std::lock_guard<std::mutex>
            lock(impl_->mutex);

        if (!impl_->initialized) {
            return {
                false,
                "not_initialized",
                "Camera Manager is not initialized."
            };
        }

        if (
            !impl_->onvifControlAccess(
                id,
                ptz_xaddr,
                profile_token,
                username,
                password,
                error
            )
        ) {
            return {
                false,
                "ptz_unavailable",
                error
            };
        }
    }

    OnvifMediaClient client;

    auto result =
        client.ptz(
            ptz_xaddr,
            profile_token,
            username,
            password,
            action,
            speed
        );

    result.message =
        sanitizeMediaError(
            result.message,
            "",
            password
        );

    cleanseString(password);

    return result;
}

void CameraManager::workerLoop()
{
    while (running_) {
        std::string error;

        const auto items =
            cameras(error);

        if (error.empty()) {
            for (const auto& camera : items) {
                if (!running_)
                    break;

                if (camera.enabled) {
                    probe(
                        camera.id
                    );
                }
            }
        }

        for (
            int second = 0;
            second < 30
            &&
            running_;
            ++second
        ) {
            std::this_thread::sleep_for(
                std::chrono::seconds(1)
            );
        }
    }
}

}
