#pragma once

#include <atomic>
#include <cstdint>
#include <memory>
#include <string>
#include <thread>
#include <vector>

#include "server/cameras/CameraMediaTools.h"
#include "server/cameras/OnvifDiscovery.h"
#include "server/cameras/OnvifMediaClient.h"

namespace homeai {

struct CameraInfo {
    std::int64_t id{0};
    std::string name;
    std::string rtsp_url;
    std::string onvif_xaddr;
    std::string username;
    bool has_password{false};
    bool enabled{true};
    std::string status{"unknown"};
    std::string last_error;
    std::int64_t last_seen_at{0};
    std::int64_t created_at{0};
    std::int64_t updated_at{0};
};

struct CameraInput {
    std::string name;
    std::string rtsp_url;
    std::string onvif_xaddr;
    std::string username;
    std::string password;
    bool update_password{false};
    bool enabled{true};
};

struct CameraResult {
    bool success{false};
    std::string code;
    std::string message;
    std::int64_t id{0};
};

class CameraManager {
public:
    CameraManager();
    ~CameraManager();

    CameraManager(
        const CameraManager&
    ) = delete;

    CameraManager& operator=(
        const CameraManager&
    ) = delete;

    bool initialize(
        const std::string& runtime_directory,
        std::string& error
    );

    bool start(
        std::string& error
    );

    void stop();

    bool healthy() const;

    std::string
    healthMessage() const;

    std::vector<CameraInfo>
    cameras(
        std::string& error
    ) const;

    CameraResult create(
        const CameraInput& input
    );

    CameraResult update(
        std::int64_t id,
        const CameraInput& input
    );

    CameraResult remove(
        std::int64_t id
    );

    CameraResult probe(
        std::int64_t id
    );

    CameraMediaProbe mediaProbe(
        std::int64_t id
    );

    CameraSnapshot snapshot(
        std::int64_t id
    );

    std::vector<OnvifDevice>
    discoverOnvif(
        int timeout_ms,
        std::string& error
    ) const;

    OnvifMediaResult
    discoverOnvifStreams(
        const std::string& device_xaddr,
        const std::string& username,
        const std::string& password
    ) const;

private:
    struct Impl;

    void workerLoop();

    std::unique_ptr<Impl> impl_;
    std::atomic<bool> running_{false};
    std::thread worker_;
};

}
