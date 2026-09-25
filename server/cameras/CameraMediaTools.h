#pragma once

#include <string>
#include <vector>

namespace homeai {

struct CameraMediaProbe {
    bool success{false};
    std::string code;
    std::string message;
    std::string video_codec;
    std::string audio_codec;
    int width{0};
    int height{0};
    double fps{0.0};
};

struct CameraSnapshot {
    bool success{false};
    std::string code;
    std::string message;
    std::string jpeg;
};

class CameraMediaTools {
public:
    static CameraMediaProbe probe(
        const std::string& stream_url
    );

    static CameraSnapshot snapshot(
        const std::string& stream_url
    );

    static CameraMediaProbe
    parseProbeOutput(
        const std::string& output
    );
};

}
