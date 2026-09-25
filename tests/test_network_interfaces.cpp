#include "server/network/NetworkInterfaceManager.h"

#include <algorithm>
#include <iostream>
#include <string>

int main()
{
    using namespace homeai;

    if (
        !NetworkInterfaceManager::
            validInterfaceName(
                "ens18"
            )
        ||
        !NetworkInterfaceManager::
            validInterfaceName(
                "eth0.20"
            )
        ||
        NetworkInterfaceManager::
            validInterfaceName(
                "lo"
            )
        ||
        NetworkInterfaceManager::
            validInterfaceName(
                "../eth0"
            )
        ||
        NetworkInterfaceManager::
            validInterfaceName(
                "bad interface"
            )
    ) {
        std::cerr
            << "Interface name validation failed\n";

        return 1;
    }

    if (
        !NetworkInterfaceManager::
            validIpv4Address(
                "192.168.10.25"
            )
        ||
        NetworkInterfaceManager::
            validIpv4Address(
                "192.168.999.25"
            )
        ||
        NetworkInterfaceManager::
            netmaskPrefix(
                "255.255.255.0"
            ) != 24
        ||
        NetworkInterfaceManager::
            netmaskPrefix(
                "24"
            ) != 24
        ||
        NetworkInterfaceManager::
            netmaskPrefix(
                "255.0.255.0"
            ) != -1
    ) {
        std::cerr
            << "IPv4 validation failed\n";

        return 1;
    }

    NetworkInterfaceManager manager;

    const auto interfaces =
        manager.interfaces();

    const auto loopback =
        std::find_if(
            interfaces.begin(),
            interfaces.end(),
            [](const auto& item) {
                return
                    item.name == "lo"
                    &&
                    item.loopback;
            }
        );

    if (loopback == interfaces.end()) {
        std::cerr
            << "Loopback interface was not discovered\n";

        return 1;
    }

    std::cout
        << "Network interface test passed\n";

    return 0;
}
