#include "server/cameras/LanCameraDiscovery.h"

#include <algorithm>
#include <arpa/inet.h>
#include <atomic>
#include <cerrno>
#include <cstdint>
#include <fcntl.h>
#include <ifaddrs.h>
#include <mutex>
#include <net/if.h>
#include <netinet/in.h>
#include <set>
#include <string>
#include <sys/select.h>
#include <sys/socket.h>
#include <thread>
#include <unistd.h>
#include <utility>
#include <vector>

namespace homeai {

namespace {

constexpr int maximum_hosts = 254;
constexpr int maximum_workers = 32;

const int camera_ports[] = {
    554,
    8554,
    8000,
    80,
    443,
    8899,
    37777,
    34567
};

struct ScanRange {
    std::uint32_t first{0};
    std::uint32_t last{0};
    std::uint32_t local{0};
};

int prefixLength(
    std::uint32_t mask
)
{
    int bits = 0;

    while (mask) {
        bits +=
            static_cast<int>(
                mask & 1U
            );

        mask >>= 1U;
    }

    return bits;
}

std::vector<ScanRange>
localRanges()
{
    std::vector<ScanRange> ranges;
    std::set<std::pair<
        std::uint32_t,
        std::uint32_t
    >> seen;

    ifaddrs* interfaces = nullptr;

    if (
        ::getifaddrs(
            &interfaces
        ) != 0
    ) {
        return ranges;
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

        std::uint32_t local =
            ntohl(
                address->sin_addr.s_addr
            );

        std::uint32_t mask =
            ntohl(
                netmask->sin_addr.s_addr
            );

        int prefix =
            prefixLength(mask);

        if (
            prefix < 8
            ||
            prefix > 30
        ) {
            continue;
        }

        if (prefix < 24) {
            mask = 0xffffff00U;
        }

        const std::uint32_t network =
            local & mask;

        const std::uint32_t broadcast =
            network | (~mask);

        if (
            broadcast <=
                network + 1
        ) {
            continue;
        }

        std::uint32_t first =
            network + 1;

        std::uint32_t last =
            broadcast - 1;

        if (
            last - first + 1 >
            maximum_hosts
        ) {
            const auto segment =
                local &
                0xffffff00U;

            first =
                segment + 1;
            last =
                segment + 254;
        }

        if (
            seen.insert(
                {
                    first,
                    last
                }
            ).second
        ) {
            ranges.push_back(
                {
                    first,
                    last,
                    local
                }
            );
        }
    }

    ::freeifaddrs(
        interfaces
    );

    return ranges;
}

bool connectPort(
    const std::string& address,
    int port,
    int timeout_ms
)
{
    const int descriptor =
        ::socket(
            AF_INET,
            SOCK_STREAM,
            IPPROTO_TCP
        );

    if (descriptor < 0)
        return false;

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
            flags |
                O_NONBLOCK
        );
    }

    sockaddr_in endpoint{};
    endpoint.sin_family = AF_INET;
    endpoint.sin_port =
        htons(
            static_cast<
                std::uint16_t
            >(port)
        );

    if (
        ::inet_pton(
            AF_INET,
            address.c_str(),
            &endpoint.sin_addr
        ) != 1
    ) {
        ::close(descriptor);
        return false;
    }

    const int connected =
        ::connect(
            descriptor,
            reinterpret_cast<
                sockaddr*
            >(
                &endpoint
            ),
            sizeof(endpoint)
        );

    if (connected == 0) {
        ::close(descriptor);
        return true;
    }

    if (
        errno != EINPROGRESS
        &&
        errno != EWOULDBLOCK
    ) {
        ::close(descriptor);
        return false;
    }

    fd_set write_set;
    FD_ZERO(&write_set);
    FD_SET(
        descriptor,
        &write_set
    );

    timeval timeout{};
    timeout.tv_sec =
        timeout_ms / 1000;
    timeout.tv_usec =
        (
            timeout_ms % 1000
        ) * 1000;

    const int selected =
        ::select(
            descriptor + 1,
            nullptr,
            &write_set,
            nullptr,
            &timeout
        );

    bool success = false;

    if (selected > 0) {
        int socket_error = 0;
        socklen_t error_size =
            sizeof(socket_error);

        if (
            ::getsockopt(
                descriptor,
                SOL_SOCKET,
                SO_ERROR,
                &socket_error,
                &error_size
            ) == 0
        ) {
            success =
                socket_error == 0;
        }
    }

    ::close(descriptor);

    return success;
}

std::string ipv4Text(
    std::uint32_t address
)
{
    in_addr value{};
    value.s_addr =
        htonl(address);

    char text[
        INET_ADDRSTRLEN
    ]{};

    if (
        !::inet_ntop(
            AF_INET,
            &value,
            text,
            sizeof(text)
        )
    ) {
        return {};
    }

    return text;
}

bool hasPort(
    const std::vector<int>& ports,
    int port
)
{
    return
        std::find(
            ports.begin(),
            ports.end(),
            port
        ) != ports.end();
}

}

std::string
LanCameraDiscovery::vendorHint(
    const std::vector<int>& open_ports
)
{
    if (
        hasPort(
            open_ports,
            8000
        )
    ) {
        return "Hikvision";
    }

    if (
        hasPort(
            open_ports,
            37777
        )
    ) {
        return "Dahua";
    }

    if (
        hasPort(
            open_ports,
            34567
        )
    ) {
        return "NetSurveillance";
    }

    return {};
}

bool
LanCameraDiscovery::likelyCamera(
    const std::vector<int>& open_ports
)
{
    return
        hasPort(
            open_ports,
            554
        )
        ||
        hasPort(
            open_ports,
            8554
        )
        ||
        hasPort(
            open_ports,
            8000
        )
        ||
        hasPort(
            open_ports,
            37777
        )
        ||
        hasPort(
            open_ports,
            34567
        )
        ||
        hasPort(
            open_ports,
            8899
        );
}

std::string
LanCameraDiscovery::suggestedRtspUrl(
    const std::string& address,
    const std::string& vendor_hint,
    int rtsp_port
)
{
    if (
        address.empty()
        ||
        rtsp_port <= 0
    ) {
        return {};
    }

    const auto base =
        "rtsp://"
        + address
        + ":"
        + std::to_string(
            rtsp_port
        );

    if (
        vendor_hint ==
        "Hikvision"
    ) {
        return
            base
            +
            "/Streaming/Channels/101";
    }

    if (
        vendor_hint ==
        "Dahua"
    ) {
        return
            base
            +
            "/cam/realmonitor?channel=1&subtype=0";
    }

    return {};
}

std::vector<LanCameraDevice>
LanCameraDiscovery::discover(
    int timeout_ms,
    std::string& error
) const
{
    error.clear();

    const auto ranges =
        localRanges();

    if (ranges.empty()) {
        error =
            "Не удалось определить локальную IPv4 сеть.";

        return {};
    }

    timeout_ms =
        std::clamp(
            timeout_ms,
            40,
            350
        );

    std::vector<std::uint32_t>
        hosts;

    std::set<std::uint32_t>
        unique;

    for (
        const auto& range :
        ranges
    ) {
        for (
            std::uint32_t address =
                range.first;
            address <=
                range.last;
            ++address
        ) {
            if (
                address ==
                range.local
            ) {
                continue;
            }

            if (
                unique.insert(
                    address
                ).second
            ) {
                hosts.push_back(
                    address
                );
            }

            if (
                hosts.size() >=
                maximum_hosts
            ) {
                break;
            }
        }

        if (
            hosts.size() >=
            maximum_hosts
        ) {
            break;
        }
    }

    if (hosts.empty())
        return {};

    std::atomic<std::size_t>
        next{0};

    std::mutex result_mutex;

    std::vector<LanCameraDevice>
        result;

    const int worker_count =
        std::min<int>(
            maximum_workers,
            static_cast<int>(
                hosts.size()
            )
        );

    std::vector<std::thread>
        workers;

    workers.reserve(
        worker_count
    );

    for (
        int worker = 0;
        worker < worker_count;
        ++worker
    ) {
        workers.emplace_back(
            [&]() {
                while (true) {
                    const auto index =
                        next.fetch_add(1);

                    if (
                        index >=
                        hosts.size()
                    ) {
                        break;
                    }

                    const auto address =
                        ipv4Text(
                            hosts[index]
                        );

                    if (address.empty())
                        continue;

                    std::vector<int>
                        open_ports;

                    for (
                        const int port :
                        camera_ports
                    ) {
                        if (
                            connectPort(
                                address,
                                port,
                                timeout_ms
                            )
                        ) {
                            open_ports.push_back(
                                port
                            );
                        }
                    }

                    if (
                        !likelyCamera(
                            open_ports
                        )
                    ) {
                        continue;
                    }

                    int rtsp_port = 0;

                    if (
                        hasPort(
                            open_ports,
                            554
                        )
                    ) {
                        rtsp_port = 554;
                    }
                    else if (
                        hasPort(
                            open_ports,
                            8554
                        )
                    ) {
                        rtsp_port = 8554;
                    }

                    LanCameraDevice device;
                    device.address =
                        address;
                    device.open_ports =
                        std::move(
                            open_ports
                        );
                    device.vendor_hint =
                        vendorHint(
                            device.open_ports
                        );
                    device.rtsp_port =
                        rtsp_port;

                    std::lock_guard<std::mutex>
                        lock(
                            result_mutex
                        );

                    result.push_back(
                        std::move(device)
                    );
                }
            }
        );
    }

    for (auto& worker : workers)
        worker.join();

    std::sort(
        result.begin(),
        result.end(),
        [](
            const LanCameraDevice& left,
            const LanCameraDevice& right
        ) {
            in_addr left_address{};
            in_addr right_address{};

            ::inet_pton(
                AF_INET,
                left.address.c_str(),
                &left_address
            );

            ::inet_pton(
                AF_INET,
                right.address.c_str(),
                &right_address
            );

            return
                ntohl(
                    left_address.s_addr
                )
                <
                ntohl(
                    right_address.s_addr
                );
        }
    );

    return result;
}

}
