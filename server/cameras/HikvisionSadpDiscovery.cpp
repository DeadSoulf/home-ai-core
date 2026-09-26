#include "server/cameras/HikvisionSadpDiscovery.h"

#include <openssl/rand.h>

#include <algorithm>
#include <array>
#include <arpa/inet.h>
#include <chrono>
#include <cctype>
#include <cstdint>
#include <cstring>
#include <iomanip>
#include <ifaddrs.h>
#include <net/if.h>
#include <netinet/in.h>
#include <regex>
#include <set>
#include <sstream>
#include <sys/select.h>
#include <sys/socket.h>
#include <unistd.h>
#include <utility>

namespace homeai {

namespace {

constexpr const char* sadp_multicast =
    "239.255.255.250";
constexpr int sadp_port = 37020;

std::string xmlValue(
    const std::string& xml,
    const std::string& name
)
{
    const std::string pattern =
        "<"
        + name
        + "\\b[^>]*>([^<]*)</"
        + name
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

int parseInt(
    const std::string& value
)
{
    try {
        return std::stoi(value);
    }
    catch (...) {
        return 0;
    }
}

bool parseBool(
    std::string value
)
{
    std::transform(
        value.begin(),
        value.end(),
        value.begin(),
        [](unsigned char character) {
            return static_cast<char>(
                std::tolower(
                    character
                )
            );
        }
    );

    return
        value == "true"
        ||
        value == "1";
}

std::string normalizeMac(
    std::string value
)
{
    std::transform(
        value.begin(),
        value.end(),
        value.begin(),
        [](unsigned char character) {
            if (character == '-')
                return ':';

            return static_cast<char>(
                std::toupper(
                    character
                )
            );
        }
    );

    return value;
}

std::string makeUuid()
{
    std::array<unsigned char, 16>
        bytes{};

    if (
        RAND_bytes(
            bytes.data(),
            static_cast<int>(
                bytes.size()
            )
        ) != 1
    ) {
        const auto now =
            std::chrono::steady_clock::now()
                .time_since_epoch()
                .count();

        for (
            std::size_t index = 0;
            index < bytes.size();
            ++index
        ) {
            bytes[index] =
                static_cast<
                    unsigned char
                >(
                    (
                        now >>
                        (
                            (
                                index % 8
                            ) * 8
                        )
                    )
                    &
                    0xff
                );
        }
    }

    bytes[6] =
        static_cast<unsigned char>(
            (
                bytes[6] & 0x0f
            )
            |
            0x40
        );

    bytes[8] =
        static_cast<unsigned char>(
            (
                bytes[8] & 0x3f
            )
            |
            0x80
        );

    std::ostringstream output;
    output
        << std::hex
        << std::setfill('0')
        << std::uppercase;

    for (
        std::size_t index = 0;
        index < bytes.size();
        ++index
    ) {
        output
            << std::setw(2)
            << static_cast<int>(
                bytes[index]
            );

        if (
            index == 3
            ||
            index == 5
            ||
            index == 7
            ||
            index == 9
        ) {
            output << '-';
        }
    }

    return output.str();
}

std::vector<
    std::pair<
        in_addr,
        in_addr
    >
>
localInterfaces()
{
    std::vector<
        std::pair<
            in_addr,
            in_addr
        >
    > result;

    ifaddrs* interfaces = nullptr;

    if (
        ::getifaddrs(
            &interfaces
        ) != 0
    ) {
        return result;
    }

    for (
        auto* current = interfaces;
        current;
        current = current->ifa_next
    ) {
        if (
            !current->ifa_addr
            ||
            !current->ifa_netmask
            ||
            current->ifa_addr->sa_family !=
                AF_INET
            ||
            (
                current->ifa_flags &
                IFF_LOOPBACK
            )
            ||
            !(
                current->ifa_flags &
                IFF_UP
            )
        ) {
            continue;
        }

        const auto* address =
            reinterpret_cast<
                const sockaddr_in*
            >(
                current->ifa_addr
            );

        const auto* netmask =
            reinterpret_cast<
                const sockaddr_in*
            >(
                current->ifa_netmask
            );

        result.push_back(
            {
                address->sin_addr,
                netmask->sin_addr
            }
        );
    }

    ::freeifaddrs(interfaces);

    return result;
}

in_addr subnetBroadcast(
    in_addr address,
    in_addr netmask
)
{
    const auto host_address =
        ntohl(
            address.s_addr
        );

    const auto host_mask =
        ntohl(
            netmask.s_addr
        );

    in_addr result{};
    result.s_addr =
        htonl(
            host_address
            |
            ~host_mask
        );

    return result;
}

bool sendProbe(
    int descriptor,
    const sockaddr_in& destination,
    const std::string& probe
)
{
    return
        ::sendto(
            descriptor,
            probe.data(),
            probe.size(),
            0,
            reinterpret_cast<
                const sockaddr*
            >(
                &destination
            ),
            sizeof(destination)
        ) >= 0;
}

std::string makeProbe(
    const std::string& type
)
{
    return
        "<?xml version=\"1.0\" encoding=\"utf-8\"?>"
        "<Probe>"
        "<Uuid>"
        + makeUuid()
        + "</Uuid>"
        "<Types>"
        + type
        + "</Types>"
        "</Probe>";
}

}

HikvisionSadpDevice
HikvisionSadpDiscovery::parseResponse(
    const std::string& xml
)
{
    HikvisionSadpDevice result;

    if (
        xml.find("<ProbeMatch") ==
            std::string::npos
    ) {
        return result;
    }

    result.address =
        xmlValue(
            xml,
            "IPv4Address"
        );

    result.mac =
        normalizeMac(
            xmlValue(
                xml,
                "MAC"
            )
        );

    result.model =
        xmlValue(
            xml,
            "DeviceDescription"
        );

    result.serial_number =
        xmlValue(
            xml,
            "DeviceSN"
        );

    result.software_version =
        xmlValue(
            xml,
            "SoftwareVersion"
        );

    result.command_port =
        parseInt(
            xmlValue(
                xml,
                "CommandPort"
            )
        );

    result.http_port =
        parseInt(
            xmlValue(
                xml,
                "HttpPort"
            )
        );

    result.activated =
        parseBool(
            xmlValue(
                xml,
                "Activated"
            )
        );

    return result;
}

std::vector<HikvisionSadpDevice>
HikvisionSadpDiscovery::discover(
    int timeout_ms,
    std::string& error
) const
{
    error.clear();

    timeout_ms =
        std::clamp(
            timeout_ms,
            300,
            5000
        );

    const auto interfaces =
        localInterfaces();

    if (interfaces.empty()) {
        error =
            "Не удалось определить интерфейс для Hikvision SADP.";

        return {};
    }

    std::vector<HikvisionSadpDevice>
        devices;

    std::set<std::string> seen;

    for (
        const auto& [
            local_address,
            local_netmask
        ] :
        interfaces
    ) {
        const int descriptor =
            ::socket(
                AF_INET,
                SOCK_DGRAM,
                IPPROTO_UDP
            );

        if (descriptor < 0)
            continue;

        int reuse = 1;
        int broadcast = 1;

        ::setsockopt(
            descriptor,
            SOL_SOCKET,
            SO_REUSEADDR,
            &reuse,
            sizeof(reuse)
        );

        ::setsockopt(
            descriptor,
            SOL_SOCKET,
            SO_BROADCAST,
            &broadcast,
            sizeof(broadcast)
        );

        sockaddr_in local{};
        local.sin_family = AF_INET;
        local.sin_port = htons(0);
        local.sin_addr = local_address;

        if (
            ::bind(
                descriptor,
                reinterpret_cast<
                    sockaddr*
                >(
                    &local
                ),
                sizeof(local)
            ) != 0
        ) {
            ::close(descriptor);
            continue;
        }

        in_addr multicast_interface =
            local_address;

        ::setsockopt(
            descriptor,
            IPPROTO_IP,
            IP_MULTICAST_IF,
            &multicast_interface,
            sizeof(multicast_interface)
        );

        unsigned char ttl = 1;

        ::setsockopt(
            descriptor,
            IPPROTO_IP,
            IP_MULTICAST_TTL,
            &ttl,
            sizeof(ttl)
        );

        sockaddr_in multicast{};
        multicast.sin_family = AF_INET;
        multicast.sin_port =
            htons(sadp_port);

        ::inet_pton(
            AF_INET,
            sadp_multicast,
            &multicast.sin_addr
        );

        sockaddr_in global_broadcast{};
        global_broadcast.sin_family =
            AF_INET;
        global_broadcast.sin_port =
            htons(sadp_port);
        global_broadcast.sin_addr.s_addr =
            htonl(
                INADDR_BROADCAST
            );

        sockaddr_in subnet_broadcast{};
        subnet_broadcast.sin_family =
            AF_INET;
        subnet_broadcast.sin_port =
            htons(sadp_port);
        subnet_broadcast.sin_addr =
            subnetBroadcast(
                local_address,
                local_netmask
            );

        const std::array<std::string, 2>
            probes{
                makeProbe(
                    "inquiry"
                ),
                makeProbe(
                    "inquiry_v32"
                )
            };

        for (
            const auto& probe :
            probes
        ) {
            sendProbe(
                descriptor,
                multicast,
                probe
            );

            sendProbe(
                descriptor,
                global_broadcast,
                probe
            );

            sendProbe(
                descriptor,
                subnet_broadcast,
                probe
            );
        }

        const auto deadline =
            std::chrono::
                steady_clock::now()
            +
            std::chrono::
                milliseconds(
                    timeout_ms
                );

        while (
            std::chrono::
                steady_clock::now()
            <
            deadline
        ) {
            const auto remaining =
                std::chrono::
                    duration_cast<
                        std::chrono::
                            milliseconds
                    >(
                        deadline
                        -
                        std::chrono::
                            steady_clock::
                                now()
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
                    remaining /
                    1000
                );

            timeout.tv_usec =
                static_cast<long>(
                    (
                        remaining %
                        1000
                    )
                    *
                    1000
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

            std::array<char, 65536>
                buffer{};

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

            const std::string xml(
                buffer.data(),
                static_cast<
                    std::size_t
                >(
                    received
                )
            );

            auto device =
                parseResponse(xml);

            if (device.address.empty()) {
                char source_text[
                    INET_ADDRSTRLEN
                ]{};

                ::inet_ntop(
                    AF_INET,
                    &source.sin_addr,
                    source_text,
                    sizeof(
                        source_text
                    )
                );

                device.address =
                    source_text;
            }

            if (
                device.address.empty()
            ) {
                continue;
            }

            const auto key =
                device.mac.empty()
                ? device.address
                : device.mac;

            if (
                !seen.insert(
                    key
                ).second
            ) {
                continue;
            }

            devices.push_back(
                std::move(device)
            );
        }

        ::close(descriptor);
    }

    return devices;
}

}
