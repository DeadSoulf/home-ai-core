#include "server/cameras/CameraManager.h"

#include <filesystem>
#include <fstream>
#include <iostream>
#include <iterator>
#include <string>
#include <sys/stat.h>
#include <unistd.h>

int main()
{
    using namespace homeai;

    const auto root =
        std::filesystem::temp_directory_path()
        /
        (
            "home-ai-camera-test-"
            +
            std::to_string(
                static_cast<long long>(
                    ::getpid()
                )
            )
        );

    std::error_code ignored;
    std::filesystem::remove_all(
        root,
        ignored
    );

    CameraManager cameras;
    std::string error;

    if (
        !cameras.initialize(
            root.string(),
            error
        )
    ) {
        std::cerr
            << "Camera manager init failed: "
            << error
            << '\n';

        return 1;
    }

    const std::string secret =
        "camera-test-password-42";

    CameraInput input;
    input.name = "Front door";
    input.rtsp_url =
        "rtsp://192.0.2.10:8554/main";
    input.onvif_xaddr =
        "http://192.0.2.10/onvif/device_service";
    input.manufacturer = "Acme";
    input.model = "Cam-X1";
    input.firmware_version = "1.2.3";
    input.serial_number = "SN-42";
    input.hardware_id = "HW-A";
    input.ptz_xaddr =
        "http://192.0.2.10/onvif/ptz_service";
    input.ptz_profile_token =
        "profile-main";
    input.username = "viewer";
    input.password = secret;
    input.enabled = true;

    const auto created =
        cameras.create(input);

    if (
        !created.success
        ||
        created.id <= 0
    ) {
        std::cerr
            << "Camera create failed: "
            << created.message
            << '\n';

        return 1;
    }

    auto list =
        cameras.cameras(error);

    if (
        !error.empty()
        ||
        list.size() != 1
        ||
        list.front().name !=
            input.name
        ||
        !list.front().has_password
        ||
        list.front().onvif_xaddr !=
            input.onvif_xaddr
        ||
        list.front().manufacturer !=
            input.manufacturer
        ||
        list.front().model !=
            input.model
        ||
        list.front().firmware_version !=
            input.firmware_version
        ||
        list.front().serial_number !=
            input.serial_number
        ||
        list.front().hardware_id !=
            input.hardware_id
        ||
        list.front().ptz_xaddr !=
            input.ptz_xaddr
        ||
        list.front().ptz_profile_token !=
            input.ptz_profile_token
        ||
        list.front().username !=
            input.username
    ) {
        std::cerr
            << "Camera list mismatch\n";

        return 1;
    }

    input.name =
        "Front entrance";
    input.password.clear();
    input.update_password = false;
    input.enabled = false;

    const auto updated =
        cameras.update(
            created.id,
            input
        );

    if (!updated.success) {
        std::cerr
            << "Camera update failed: "
            << updated.message
            << '\n';

        return 1;
    }

    list =
        cameras.cameras(error);

    if (
        list.size() != 1
        ||
        list.front().name !=
            "Front entrance"
        ||
        !list.front().has_password
        ||
        list.front().enabled
        ||
        list.front().status !=
            "disabled"
    ) {
        std::cerr
            << "Camera update did not preserve password/state\n";

        return 1;
    }

    CameraInput invalid_input;
    invalid_input.name = "Bad";
    invalid_input.rtsp_url =
        "rtsp://user:password@192.0.2.1/live";
    invalid_input.enabled = true;

    const auto invalid =
        cameras.create(
            invalid_input
        );

    if (invalid.success) {
        std::cerr
            << "Embedded RTSP credentials were accepted\n";

        return 1;
    }

    const auto key_file =
        root /
        "secret.key";

    struct stat key_stat {};

    if (
        ::stat(
            key_file.c_str(),
            &key_stat
        ) != 0
        ||
        (
            key_stat.st_mode
            &
            0777
        ) != 0600
    ) {
        std::cerr
            << "Camera secret key permissions are not 0600\n";

        return 1;
    }

    std::ifstream database(
        root / "cameras.db",
        std::ios::binary
    );

    const std::string bytes(
        (
            std::istreambuf_iterator<char>(
                database
            )
        ),
        std::istreambuf_iterator<char>()
    );

    if (
        bytes.find(secret) !=
        std::string::npos
    ) {
        std::cerr
            << "Camera password is stored in plaintext\n";

        return 1;
    }

    const auto removed =
        cameras.remove(
            created.id
        );

    if (!removed.success) {
        std::cerr
            << "Camera remove failed\n";

        return 1;
    }

    list =
        cameras.cameras(error);

    if (!list.empty()) {
        std::cerr
            << "Camera was not removed\n";

        return 1;
    }

    std::filesystem::remove_all(
        root,
        ignored
    );

    std::cout
        << "Camera manager test passed\n";

    return 0;
}
