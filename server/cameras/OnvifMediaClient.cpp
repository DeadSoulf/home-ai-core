#include "server/cameras/OnvifMediaClient.h"

#include <openssl/evp.h>
#include <openssl/rand.h>

#include <algorithm>
#include <array>
#include <cctype>
#include <chrono>
#include <cmath>
#include <ctime>
#include <iomanip>
#include <netdb.h>
#include <regex>
#include <sstream>
#include <string_view>
#include <sys/socket.h>
#include <unistd.h>
#include <utility>
#include <vector>

namespace homeai {

namespace {

constexpr int onvif_timeout_seconds = 5;
constexpr std::size_t maximum_http_response =
    4 * 1024 * 1024;

struct HttpUrl {
    std::string host;
    std::string port;
    std::string path;
};

struct HttpResponse {
    int status{0};
    std::string headers;
    std::string body;
    std::string error;
};

std::string xmlEscape(
    const std::string& value
)
{
    std::string result;
    result.reserve(
        value.size()
    );

    for (const char character : value) {
        switch (character) {
        case '&':
            result += "&amp;";
            break;
        case '<':
            result += "&lt;";
            break;
        case '>':
            result += "&gt;";
            break;
        case '"':
            result += "&quot;";
            break;
        case '\'':
            result += "&apos;";
            break;
        default:
            result.push_back(
                character
            );
            break;
        }
    }

    return result;
}

std::string base64(
    const unsigned char* data,
    std::size_t size
)
{
    if (
        !data
        ||
        size == 0
    ) {
        return {};
    }

    std::string result(
        4 * (
            (
                size + 2
            ) / 3
        ),
        '\0'
    );

    const int written =
        EVP_EncodeBlock(
            reinterpret_cast<
                unsigned char*
            >(
                result.data()
            ),
            data,
            static_cast<int>(
                size
            )
        );

    if (written <= 0)
        return {};

    result.resize(
        static_cast<std::size_t>(
            written
        )
    );

    return result;
}

std::string utcNow()
{
    const auto now =
        std::chrono::system_clock::now();

    const auto time =
        std::chrono::system_clock::
            to_time_t(now);

    std::tm tm{};

    gmtime_r(
        &time,
        &tm
    );

    std::ostringstream output;
    output
        << std::put_time(
            &tm,
            "%Y-%m-%dT%H:%M:%SZ"
        );

    return output.str();
}

std::string passwordDigest(
    const std::vector<unsigned char>& nonce,
    const std::string& created,
    const std::string& password
)
{
    std::vector<unsigned char> input;

    input.reserve(
        nonce.size()
        +
        created.size()
        +
        password.size()
    );

    input.insert(
        input.end(),
        nonce.begin(),
        nonce.end()
    );

    input.insert(
        input.end(),
        created.begin(),
        created.end()
    );

    input.insert(
        input.end(),
        password.begin(),
        password.end()
    );

    std::array<unsigned char, EVP_MAX_MD_SIZE>
        digest{};

    unsigned int digest_size = 0;

    EVP_MD_CTX* context =
        EVP_MD_CTX_new();

    if (!context)
        return {};

    const bool success =
        EVP_DigestInit_ex(
            context,
            EVP_sha1(),
            nullptr
        ) == 1
        &&
        EVP_DigestUpdate(
            context,
            input.data(),
            input.size()
        ) == 1
        &&
        EVP_DigestFinal_ex(
            context,
            digest.data(),
            &digest_size
        ) == 1;

    EVP_MD_CTX_free(
        context
    );

    if (!success)
        return {};

    return base64(
        digest.data(),
        digest_size
    );
}

std::string securityHeader(
    const std::string& username,
    const std::string& password
)
{
    if (username.empty())
        return {};

    std::vector<unsigned char>
        nonce(16);

    if (
        RAND_bytes(
            nonce.data(),
            static_cast<int>(
                nonce.size()
            )
        ) != 1
    ) {
        return {};
    }

    const auto created =
        utcNow();

    const auto digest =
        passwordDigest(
            nonce,
            created,
            password
        );

    if (digest.empty())
        return {};

    return
        "<wsse:Security "
        "s:mustUnderstand=\"1\" "
        "xmlns:wsse=\"http://docs.oasis-open.org/wss/2004/01/"
        "oasis-200401-wss-wssecurity-secext-1.0.xsd\" "
        "xmlns:wsu=\"http://docs.oasis-open.org/wss/2004/01/"
        "oasis-200401-wss-wssecurity-utility-1.0.xsd\">"
        "<wsse:UsernameToken>"
        "<wsse:Username>"
        + xmlEscape(username)
        + "</wsse:Username>"
        "<wsse:Password Type=\"http://docs.oasis-open.org/wss/2004/01/"
        "oasis-200401-wss-username-token-profile-1.0#PasswordDigest\">"
        + digest
        + "</wsse:Password>"
        "<wsse:Nonce EncodingType=\"http://docs.oasis-open.org/wss/2004/01/"
        "oasis-200401-wss-soap-message-security-1.0#Base64Binary\">"
        + base64(
            nonce.data(),
            nonce.size()
        )
        + "</wsse:Nonce>"
        "<wsu:Created>"
        + created
        + "</wsu:Created>"
        "</wsse:UsernameToken>"
        "</wsse:Security>";
}

std::string soapEnvelope(
    const std::string& body,
    const std::string& username,
    const std::string& password
)
{
    return
        "<?xml version=\"1.0\" encoding=\"UTF-8\"?>"
        "<s:Envelope "
        "xmlns:s=\"http://www.w3.org/2003/05/soap-envelope\" "
        "xmlns:tds=\"http://www.onvif.org/ver10/device/wsdl\" "
        "xmlns:trt=\"http://www.onvif.org/ver10/media/wsdl\" "
        "xmlns:tt=\"http://www.onvif.org/ver10/schema\">"
        "<s:Header>"
        + securityHeader(
            username,
            password
        )
        + "</s:Header>"
        "<s:Body>"
        + body
        + "</s:Body>"
        "</s:Envelope>";
}

bool parseHttpUrl(
    const std::string& url,
    HttpUrl& result,
    std::string& error
)
{
    constexpr std::string_view prefix =
        "http://";

    if (
        url.rfind(
            prefix,
            0
        ) != 0
    ) {
        error =
            "Автонастройка ONVIF сейчас поддерживает HTTP XAddr.";

        return false;
    }

    const auto authority_start =
        prefix.size();

    const auto path_start =
        url.find(
            '/',
            authority_start
        );

    const auto authority =
        url.substr(
            authority_start,
            path_start ==
                std::string::npos
            ? std::string::npos
            : path_start -
                authority_start
        );

    result.path =
        path_start ==
            std::string::npos
        ? "/"
        : url.substr(
            path_start
        );

    if (authority.empty()) {
        error =
            "Некорректный ONVIF XAddr.";

        return false;
    }

    if (authority.front() == '[') {
        const auto closing =
            authority.find(']');

        if (
            closing ==
                std::string::npos
            ||
            closing <= 1
        ) {
            error =
                "Некорректный IPv6 ONVIF XAddr.";

            return false;
        }

        result.host =
            authority.substr(
                1,
                closing - 1
            );

        result.port = "80";

        if (
            closing + 1 <
            authority.size()
        ) {
            if (
                authority[
                    closing + 1
                ] != ':'
            ) {
                error =
                    "Некорректный ONVIF XAddr.";

                return false;
            }

            result.port =
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
            result.host =
                authority.substr(
                    0,
                    colon
                );

            result.port =
                authority.substr(
                    colon + 1
                );
        }
        else {
            result.host =
                authority;
            result.port =
                "80";
        }
    }

    if (
        result.host.empty()
        ||
        result.port.empty()
    ) {
        error =
            "Некорректный ONVIF XAddr.";

        return false;
    }

    return true;
}

bool writeAll(
    int descriptor,
    const std::string& data
)
{
    std::size_t offset = 0;

    while (offset < data.size()) {
        const auto written =
            ::send(
                descriptor,
                data.data() + offset,
                data.size() - offset,
                MSG_NOSIGNAL
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

std::string decodeChunked(
    const std::string& body
)
{
    std::string output;
    std::size_t position = 0;

    while (position < body.size()) {
        const auto line_end =
            body.find(
                "\r\n",
                position
            );

        if (
            line_end ==
                std::string::npos
        ) {
            return {};
        }

        const auto size_text =
            body.substr(
                position,
                line_end - position
            );

        const auto extension =
            size_text.find(';');

        const auto hex =
            size_text.substr(
                0,
                extension
            );

        std::size_t chunk_size = 0;

        try {
            chunk_size =
                std::stoul(
                    hex,
                    nullptr,
                    16
                );
        }
        catch (...) {
            return {};
        }

        position =
            line_end + 2;

        if (chunk_size == 0)
            return output;

        if (
            position + chunk_size
            >
            body.size()
        ) {
            return {};
        }

        output.append(
            body,
            position,
            chunk_size
        );

        position += chunk_size;

        if (
            position + 2 >
                body.size()
            ||
            body.substr(
                position,
                2
            ) != "\r\n"
        ) {
            return {};
        }

        position += 2;
    }

    return {};
}

HttpResponse httpPost(
    const std::string& url,
    const std::string& action,
    const std::string& body
)
{
    HttpUrl target;
    std::string parse_error;

    if (
        !parseHttpUrl(
            url,
            target,
            parse_error
        )
    ) {
        return {
            0,
            "",
            "",
            parse_error
        };
    }

    addrinfo hints{};
    hints.ai_family = AF_UNSPEC;
    hints.ai_socktype = SOCK_STREAM;

    addrinfo* addresses = nullptr;

    if (
        ::getaddrinfo(
            target.host.c_str(),
            target.port.c_str(),
            &hints,
            &addresses
        ) != 0
    ) {
        return {
            0,
            "",
            "",
            "Не удалось определить адрес ONVIF камеры."
        };
    }

    int descriptor = -1;

    for (
        auto* address = addresses;
        address;
        address = address->ai_next
    ) {
        descriptor =
            ::socket(
                address->ai_family,
                address->ai_socktype,
                address->ai_protocol
            );

        if (descriptor < 0)
            continue;

        timeval timeout{};
        timeout.tv_sec =
            onvif_timeout_seconds;

        ::setsockopt(
            descriptor,
            SOL_SOCKET,
            SO_RCVTIMEO,
            &timeout,
            sizeof(timeout)
        );

        ::setsockopt(
            descriptor,
            SOL_SOCKET,
            SO_SNDTIMEO,
            &timeout,
            sizeof(timeout)
        );

        if (
            ::connect(
                descriptor,
                address->ai_addr,
                address->ai_addrlen
            ) == 0
        ) {
            break;
        }

        ::close(descriptor);
        descriptor = -1;
    }

    ::freeaddrinfo(addresses);

    if (descriptor < 0) {
        return {
            0,
            "",
            "",
            "Не удалось подключиться к ONVIF сервису камеры."
        };
    }

    std::ostringstream request;

    request
        << "POST "
        << target.path
        << " HTTP/1.1\r\n"
        << "Host: "
        << target.host
        << ":"
        << target.port
        << "\r\n"
        << "User-Agent: Home-AI-Core/ONVIF\r\n"
        << "Content-Type: application/soap+xml; charset=utf-8; action=\""
        << action
        << "\"\r\n"
        << "SOAPAction: \""
        << action
        << "\"\r\n"
        << "Connection: close\r\n";

    request
        << "Content-Length: "
        << body.size()
        << "\r\n\r\n"
        << body;

    if (
        !writeAll(
            descriptor,
            request.str()
        )
    ) {
        ::close(descriptor);

        return {
            0,
            "",
            "",
            "Не удалось отправить ONVIF запрос."
        };
    }

    std::string response;
    std::array<char, 16384> buffer{};

    while (
        response.size()
        <
        maximum_http_response
    ) {
        const auto received =
            ::recv(
                descriptor,
                buffer.data(),
                buffer.size(),
                0
            );

        if (received == 0)
            break;

        if (received < 0) {
            if (!response.empty())
                break;

            ::close(descriptor);

            return {
                0,
                "",
                "",
                "ONVIF камера не ответила вовремя."
            };
        }

        response.append(
            buffer.data(),
            static_cast<
                std::size_t
            >(received)
        );
    }

    ::close(descriptor);

    if (
        response.size()
        >=
        maximum_http_response
    ) {
        return {
            0,
            "",
            "",
            "Ответ ONVIF слишком большой."
        };
    }

    const auto header_end =
        response.find(
            "\r\n\r\n"
        );

    if (
        header_end ==
            std::string::npos
    ) {
        return {
            0,
            "",
            "",
            "Некорректный HTTP ответ ONVIF."
        };
    }

    HttpResponse result;
    result.headers =
        response.substr(
            0,
            header_end
        );

    result.body =
        response.substr(
            header_end + 4
        );

    std::istringstream first_line(
        result.headers
    );

    std::string protocol;
    first_line
        >> protocol
        >> result.status;

    std::string lower_headers =
        result.headers;

    std::transform(
        lower_headers.begin(),
        lower_headers.end(),
        lower_headers.begin(),
        [](unsigned char character) {
            return static_cast<char>(
                std::tolower(
                    character
                )
            );
        }
    );

    if (
        lower_headers.find(
            "transfer-encoding: chunked"
        ) !=
        std::string::npos
    ) {
        const auto decoded =
            decodeChunked(
                result.body
            );

        if (decoded.empty()) {
            result.error =
                "Не удалось декодировать ONVIF HTTP ответ.";

            return result;
        }

        result.body = decoded;
    }

    if (
        result.status == 401
        ||
        result.status == 403
    ) {
        result.error =
            "ONVIF отклонил логин или пароль.";
    }
    else if (
        result.status < 200
        ||
        result.status >= 300
    ) {
        result.error =
            "ONVIF вернул HTTP "
            + std::to_string(
                result.status
            )
            + ".";
    }

    return result;
}

std::string localTagValue(
    const std::string& xml,
    const std::string& local_name
)
{
    const std::string pattern =
        "<(?:[A-Za-z_][A-Za-z0-9_.-]*:)?"
        + local_name
        + "\\b[^>]*>([^<]*)</(?:[A-Za-z_][A-Za-z0-9_.-]*:)?"
        + local_name
        + ">";

    const std::regex expression(
        pattern,
        std::regex::icase
    );

    std::smatch match;

    if (
        !std::regex_search(
            xml,
            match,
            expression
        )
        ||
        match.size() < 2
    ) {
        return {};
    }

    return match[1].str();
}

std::string attributeValue(
    const std::string& attributes,
    const std::string& name
)
{
    const std::regex expression(
        "(?:^|\\s)"
        + name
        + "\\s*=\\s*[\"']([^\"']+)[\"']",
        std::regex::icase
    );

    std::smatch match;

    if (
        !std::regex_search(
            attributes,
            match,
            expression
        )
        ||
        match.size() < 2
    ) {
        return {};
    }

    return match[1].str();
}

int parseInt(
    const std::string& value
)
{
    try {
        return std::stoi(
            value
        );
    }
    catch (...) {
        return 0;
    }
}

double parseDouble(
    const std::string& value
)
{
    try {
        return std::stod(
            value
        );
    }
    catch (...) {
        return 0.0;
    }
}

std::size_t recommendedProfile(
    const std::vector<OnvifMediaProfile>& profiles
)
{
    if (profiles.empty())
        return 0;

    std::size_t best = 0;
    long long best_pixels = -1;

    for (
        std::size_t index = 0;
        index < profiles.size();
        ++index
    ) {
        const auto pixels =
            static_cast<long long>(
                profiles[index].width
            )
            *
            static_cast<long long>(
                profiles[index].height
            );

        if (pixels > best_pixels) {
            best_pixels = pixels;
            best = index;
        }
    }

    return best;
}

std::string soapFault(
    const std::string& xml
)
{
    const auto reason =
        localTagValue(
            xml,
            "Text"
        );

    if (!reason.empty())
        return reason;

    return
        localTagValue(
            xml,
            "Reason"
        );
}

}

std::string
OnvifMediaClient::parseMediaXAddr(
    const std::string& xml
)
{
    const std::regex media_block(
        R"(<(?:[A-Za-z_][A-Za-z0-9_.-]*:)?Media\b[^>]*>([\s\S]*?)</(?:[A-Za-z_][A-Za-z0-9_.-]*:)?Media>)",
        std::regex::icase
    );

    std::smatch match;

    if (
        std::regex_search(
            xml,
            match,
            media_block
        )
        &&
        match.size() >= 2
    ) {
        const auto xaddr =
            localTagValue(
                match[1].str(),
                "XAddr"
            );

        if (!xaddr.empty())
            return xaddr;
    }

    return
        localTagValue(
            xml,
            "XAddr"
        );
}

std::vector<OnvifMediaProfile>
OnvifMediaClient::parseProfiles(
    const std::string& xml
)
{
    std::vector<OnvifMediaProfile>
        profiles;

    const std::regex expression(
        R"(<(?:[A-Za-z_][A-Za-z0-9_.-]*:)?Profiles\b([^>]*)>([\s\S]*?)</(?:[A-Za-z_][A-Za-z0-9_.-]*:)?Profiles>)",
        std::regex::icase
    );

    for (
        std::sregex_iterator current(
            xml.begin(),
            xml.end(),
            expression
        ),
        end;
        current != end;
        ++current
    ) {
        if (
            current->size() < 3
        ) {
            continue;
        }

        OnvifMediaProfile profile;

        profile.token =
            attributeValue(
                (*current)[1].str(),
                "token"
            );

        const auto body =
            (*current)[2].str();

        profile.name =
            localTagValue(
                body,
                "Name"
            );

        profile.encoding =
            localTagValue(
                body,
                "Encoding"
            );

        profile.width =
            parseInt(
                localTagValue(
                    body,
                    "Width"
                )
            );

        profile.height =
            parseInt(
                localTagValue(
                    body,
                    "Height"
                )
            );

        profile.fps =
            parseDouble(
                localTagValue(
                    body,
                    "FrameRateLimit"
                )
            );

        if (!profile.token.empty()) {
            profiles.push_back(
                std::move(profile)
            );
        }
    }

    return profiles;
}

std::string
OnvifMediaClient::parseStreamUri(
    const std::string& xml
)
{
    return
        stripUriCredentials(
            localTagValue(
                xml,
                "Uri"
            )
        );
}

std::string
OnvifMediaClient::stripUriCredentials(
    const std::string& uri
)
{
    const auto scheme =
        uri.find("://");

    if (
        scheme ==
            std::string::npos
    ) {
        return uri;
    }

    const auto authority_start =
        scheme + 3;

    const auto authority_end =
        uri.find_first_of(
            "/?#",
            authority_start
        );

    const auto at =
        uri.find(
            '@',
            authority_start
        );

    if (
        at ==
            std::string::npos
        ||
        (
            authority_end !=
                std::string::npos
            &&
            at >
                authority_end
        )
    ) {
        return uri;
    }

    return
        uri.substr(
            0,
            authority_start
        )
        +
        uri.substr(
            at + 1
        );
}

OnvifMediaResult
OnvifMediaClient::profiles(
    const std::string& device_xaddr,
    const std::string& username,
    const std::string& password
) const
{
    if (device_xaddr.empty()) {
        return {
            false,
            "missing_xaddr",
            "Сначала выберите найденную ONVIF камеру.",
            "",
            {},
            0
        };
    }

    const auto capabilities_body =
        soapEnvelope(
            "<tds:GetCapabilities>"
            "<tds:Category>Media</tds:Category>"
            "</tds:GetCapabilities>",
            username,
            password
        );

    const auto capabilities =
        httpPost(
            device_xaddr,
            "http://www.onvif.org/ver10/device/wsdl/GetCapabilities",
            capabilities_body
        );

    if (!capabilities.error.empty()) {
        return {
            false,
            "capabilities_failed",
            capabilities.error,
            "",
            {},
            0
        };
    }

    const auto fault =
        soapFault(
            capabilities.body
        );

    if (!fault.empty()) {
        return {
            false,
            "onvif_fault",
            fault,
            "",
            {},
            0
        };
    }

    const auto media_xaddr =
        parseMediaXAddr(
            capabilities.body
        );

    if (media_xaddr.empty()) {
        return {
            false,
            "media_service_missing",
            "Камера не сообщила адрес ONVIF Media Service.",
            "",
            {},
            0
        };
    }

    const auto profiles_body =
        soapEnvelope(
            "<trt:GetProfiles/>",
            username,
            password
        );

    const auto profile_response =
        httpPost(
            media_xaddr,
            "http://www.onvif.org/ver10/media/wsdl/GetProfiles",
            profiles_body
        );

    if (!profile_response.error.empty()) {
        return {
            false,
            "profiles_failed",
            profile_response.error,
            media_xaddr,
            {},
            0
        };
    }

    auto result_profiles =
        parseProfiles(
            profile_response.body
        );

    if (result_profiles.empty()) {
        return {
            false,
            "profiles_missing",
            "ONVIF Media Profiles не найдены.",
            media_xaddr,
            {},
            0
        };
    }

    for (auto& profile : result_profiles) {
        const auto request =
            soapEnvelope(
                "<trt:GetStreamUri>"
                "<trt:StreamSetup>"
                "<tt:Stream>RTP-Unicast</tt:Stream>"
                "<tt:Transport>"
                "<tt:Protocol>RTSP</tt:Protocol>"
                "</tt:Transport>"
                "</trt:StreamSetup>"
                "<trt:ProfileToken>"
                + xmlEscape(
                    profile.token
                )
                + "</trt:ProfileToken>"
                "</trt:GetStreamUri>",
                username,
                password
            );

        const auto stream_response =
            httpPost(
                media_xaddr,
                "http://www.onvif.org/ver10/media/wsdl/GetStreamUri",
                request
            );

        if (
            stream_response.error.empty()
        ) {
            profile.rtsp_uri =
                parseStreamUri(
                    stream_response.body
                );
        }
    }

    result_profiles.erase(
        std::remove_if(
            result_profiles.begin(),
            result_profiles.end(),
            [](const OnvifMediaProfile& profile) {
                return
                    profile.rtsp_uri.empty();
            }
        ),
        result_profiles.end()
    );

    if (result_profiles.empty()) {
        return {
            false,
            "stream_uri_missing",
            "Камера не вернула RTSP URI для ONVIF профилей.",
            media_xaddr,
            {},
            0
        };
    }

    const auto recommended =
        recommendedProfile(
            result_profiles
        );

    return {
        true,
        "ok",
        "RTSP поток определён автоматически.",
        media_xaddr,
        std::move(
            result_profiles
        ),
        recommended
    };
}

}
