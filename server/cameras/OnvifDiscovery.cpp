#include "server/cameras/OnvifDiscovery.h"

#include <algorithm>
#include <array>
#include <arpa/inet.h>
#include <chrono>
#include <cstring>
#include <netinet/in.h>
#include <regex>
#include <set>
#include <sstream>
#include <sys/select.h>
#include <sys/socket.h>
#include <unistd.h>

namespace homeai {

namespace {

std::string xmlValue(
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

std::vector<std::string>
splitWhitespace(
    const std::string& value
)
{
    std::istringstream stream(
        value
    );

    std::vector<std::string> result;
    std::string item;

    while (stream >> item)
        result.push_back(item);

    return result;
}

std::string makeProbe()
{
    const auto now =
        std::chrono::steady_clock::now()
            .time_since_epoch()
            .count();

    return
        "<?xml version=\"1.0\" encoding=\"UTF-8\"?>"
        "<e:Envelope "
        "xmlns:e=\"http://www.w3.org/2003/05/soap-envelope\" "
        "xmlns:w=\"http://schemas.xmlsoap.org/ws/2004/08/addressing\" "
        "xmlns:d=\"http://schemas.xmlsoap.org/ws/2005/04/discovery\" "
        "xmlns:dn=\"http://www.onvif.org/ver10/network/wsdl\">"
        "<e:Header>"
        "<w:MessageID>uuid:home-ai-"
        + std::to_string(now)
        + "</w:MessageID>"
        "<w:To>urn:schemas-xmlsoap-org:ws:2005:04:discovery</w:To>"
        "<w:Action>http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</w:Action>"
        "</e:Header>"
        "<e:Body>"
        "<d:Probe>"
        "<d:Types>dn:NetworkVideoTransmitter</d:Types>"
        "</d:Probe>"
        "</e:Body>"
        "</e:Envelope>";
}

}

std::vector<OnvifDevice>
OnvifDiscovery::parseResponse(
    const std::string& xml,
    const std::string& remote_address
)
{
    std::vector<OnvifDevice> result;

    if (
        xml.find("ProbeMatch") ==
            std::string::npos
        &&
        xml.find("ProbeMatches") ==
            std::string::npos
    ) {
        return result;
    }

    const auto xaddrs =
        splitWhitespace(
            xmlValue(
                xml,
                "XAddrs"
            )
        );

    if (xaddrs.empty())
        return result;

    const auto endpoint =
        xmlValue(
            xml,
            "Address"
        );

    const auto types =
        xmlValue(
            xml,
            "Types"
        );

    const auto scopes =
        xmlValue(
            xml,
            "Scopes"
        );

    std::set<std::string> seen;

    for (const auto& xaddr : xaddrs) {
        if (
            xaddr.empty()
            ||
            !seen.insert(
                xaddr
            ).second
        ) {
            continue;
        }

        result.push_back(
            {
                endpoint,
                xaddr,
                types,
                scopes,
                remote_address
            }
        );
    }

    return result;
}

std::vector<OnvifDevice>
OnvifDiscovery::discover(
    int timeout_ms,
    std::string& error
) const
{
    if (timeout_ms < 100)
        timeout_ms = 100;

    if (timeout_ms > 10000)
        timeout_ms = 10000;

    const int descriptor =
        ::socket(
            AF_INET,
            SOCK_DGRAM,
            IPPROTO_UDP
        );

    if (descriptor < 0) {
        error =
            "Не удалось открыть сокет ONVIF discovery.";

        return {};
    }

    int reuse = 1;

    ::setsockopt(
        descriptor,
        SOL_SOCKET,
        SO_REUSEADDR,
        &reuse,
        sizeof(reuse)
    );

    unsigned char ttl = 2;

    ::setsockopt(
        descriptor,
        IPPROTO_IP,
        IP_MULTICAST_TTL,
        &ttl,
        sizeof(ttl)
    );

    sockaddr_in destination{};
    destination.sin_family = AF_INET;
    destination.sin_port = htons(3702);

    if (
        ::inet_pton(
            AF_INET,
            "239.255.255.250",
            &destination.sin_addr
        ) != 1
    ) {
        ::close(descriptor);

        error =
            "Некорректный multicast-адрес ONVIF.";

        return {};
    }

    const auto probe =
        makeProbe();

    if (
        ::sendto(
            descriptor,
            probe.data(),
            probe.size(),
            0,
            reinterpret_cast<
                sockaddr*
            >(
                &destination
            ),
            sizeof(destination)
        ) < 0
    ) {
        ::close(descriptor);

        error =
            "Не удалось отправить ONVIF discovery probe.";

        return {};
    }

    const auto deadline =
        std::chrono::steady_clock::now()
        +
        std::chrono::milliseconds(
            timeout_ms
        );

    std::vector<OnvifDevice> devices;
    std::set<std::string> seen;

    while (
        std::chrono::steady_clock::now()
        <
        deadline
    ) {
        const auto remaining =
            std::chrono::duration_cast<
                std::chrono::milliseconds
            >(
                deadline
                -
                std::chrono::steady_clock::now()
            ).count();

        if (remaining <= 0)
            break;

        fd_set read_set;
        FD_ZERO(&read_set);
        FD_SET(
            descriptor,
            &read_set
        );

        timeval timeout{};
        timeout.tv_sec =
            static_cast<long>(
                remaining / 1000
            );
        timeout.tv_usec =
            static_cast<long>(
                (
                    remaining % 1000
                ) * 1000
            );

        const int selected =
            ::select(
                descriptor + 1,
                &read_set,
                nullptr,
                nullptr,
                &timeout
            );

        if (selected <= 0)
            break;

        std::array<char, 65536> buffer{};
        sockaddr_in source{};
        socklen_t source_size =
            sizeof(source);

        const auto received =
            ::recvfrom(
                descriptor,
                buffer.data(),
                buffer.size() - 1,
                0,
                reinterpret_cast<
                    sockaddr*
                >(
                    &source
                ),
                &source_size
            );

        if (received <= 0)
            continue;

        char address[
            INET_ADDRSTRLEN
        ]{};

        ::inet_ntop(
            AF_INET,
            &source.sin_addr,
            address,
            sizeof(address)
        );

        const std::string xml(
            buffer.data(),
            static_cast<
                std::size_t
            >(received)
        );

        const auto parsed =
            parseResponse(
                xml,
                address
        );

        for (
            const auto& device :
            parsed
        ) {
            const auto key =
                device.xaddr.empty()
                ? (
                    device.remote_address
                    + "|"
                    + device.endpoint_reference
                )
                : device.xaddr;

            if (
                !seen.insert(
                    key
                ).second
            ) {
                continue;
            }

            devices.push_back(
                device
            );
        }
    }

    ::close(descriptor);

    return devices;
}

}
