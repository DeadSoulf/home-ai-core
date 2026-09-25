#pragma once

#include <string>
#include <vector>

namespace homeai {

struct DiskOperationResult {
    bool success{false};
    std::string code;
    std::string message;
};

class DiskOperations {
public:
    DiskOperationResult mount(
        const std::string& device,
        const std::string& role,
        bool read_only = false
    ) const;

    DiskOperationResult mountAt(
        const std::string& device,
        const std::string& mount_point,
        bool read_only = false
    ) const;

    DiskOperationResult unmount(
        const std::string& device
    ) const;

    DiskOperationResult formatExt4(
        const std::string& device,
        const std::string& label
    ) const;

    DiskOperationResult wipeSignatures(
        const std::string& device
    ) const;

    bool helperInstalled() const;

    static std::string defaultMountPoint(
        const std::string& device,
        const std::string& role
    );

    static std::string deviceForUuid(
        const std::string& uuid
    );

private:
    DiskOperationResult runHelper(
        const std::vector<std::string>& arguments
    ) const;
};

}
