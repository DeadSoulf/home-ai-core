#include "server/network/VpnService.h"
#include "server/network/WireGuardManager.h"

#include <memory>

namespace homeai {

class VpnService::Impl {
public:
    WireGuardManager manager;
};

VpnService::VpnService()
    : impl_(new Impl())
{
}

VpnService::~VpnService()
{
    delete impl_;
    impl_ = nullptr;
}

bool VpnService::initialize(
    const std::string& config_directory
)
{
    return
        impl_->manager.initialize(
            config_directory
        );
}

bool VpnService::available() const
{
    return
        impl_
        &&
        impl_->manager.available();
}

std::vector<VpnProfileInfo>
VpnService::profiles(
    std::string& error
) const
{
    std::vector<VpnProfileInfo>
        result;

    if (!impl_) {
        error =
            "VPN service is not initialized.";

        return result;
    }

    for (
        const auto& profile :
        impl_->manager.profiles(
            error
        )
    ) {
        result.push_back(
            {
                profile.name,
                profile.active
            }
        );
    }

    return result;
}

VpnProfileConfigResult
VpnService::loadProfile(
    const std::string& profile
) const
{
    if (!impl_) {
        return {
            false,
            "not_initialized",
            "VPN service is not initialized.",
            {}
        };
    }

    const auto result =
        impl_->manager.loadProfile(
            profile
        );

    return {
        result.success,
        result.code,
        result.message,
        result.config
    };
}

VpnActionResult
VpnService::saveProfile(
    const std::string& profile,
    const std::string& config
) const
{
    if (!impl_) {
        return {
            false,
            "not_initialized",
            "VPN service is not initialized."
        };
    }

    const auto result =
        impl_->manager.saveProfile(
            profile,
            config
        );

    return {
        result.success,
        result.code,
        result.message
    };
}

VpnActionResult
VpnService::removeProfile(
    const std::string& profile
) const
{
    if (!impl_) {
        return {
            false,
            "not_initialized",
            "VPN service is not initialized."
        };
    }

    const auto result =
        impl_->manager.removeProfile(
            profile
        );

    return {
        result.success,
        result.code,
        result.message
    };
}

VpnActionResult
VpnService::connect(
    const std::string& profile
) const
{
    if (!impl_) {
        return {
            false,
            "not_initialized",
            "VPN service is not initialized."
        };
    }

    const auto result =
        impl_->manager.connect(
            profile
        );

    return {
        result.success,
        result.code,
        result.message
    };
}

VpnActionResult
VpnService::disconnect(
    const std::string& profile
) const
{
    if (!impl_) {
        return {
            false,
            "not_initialized",
            "VPN service is not initialized."
        };
    }

    const auto result =
        impl_->manager.disconnect(
            profile
        );

    return {
        result.success,
        result.code,
        result.message
    };
}

}
