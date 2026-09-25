#include "server/cameras/CameraMediaTools.h"
#include "server/cameras/OnvifDiscovery.h"

#include <cmath>
#include <iostream>
#include <string>

int main()
{
    using namespace homeai;

    const std::string xml =
        "<?xml version=\"1.0\"?>"
        "<s:Envelope "
        "xmlns:s=\"http://www.w3.org/2003/05/soap-envelope\" "
        "xmlns:a=\"http://schemas.xmlsoap.org/ws/2004/08/addressing\" "
        "xmlns:d=\"http://schemas.xmlsoap.org/ws/2005/04/discovery\">"
        "<s:Body><d:ProbeMatches><d:ProbeMatch>"
        "<a:EndpointReference>"
        "<a:Address>urn:uuid:camera-1</a:Address>"
        "</a:EndpointReference>"
        "<d:Types>dn:NetworkVideoTransmitter</d:Types>"
        "<d:Scopes>onvif://www.onvif.org/name/FrontDoor</d:Scopes>"
        "<d:XAddrs>"
        "http://192.0.2.20/onvif/device_service "
        "http://[2001:db8::20]/onvif/device_service"
        "</d:XAddrs>"
        "</d:ProbeMatch></d:ProbeMatches></s:Body>"
        "</s:Envelope>";

    const auto devices =
        OnvifDiscovery::parseResponse(
            xml,
            "192.0.2.20"
        );

    if (
        devices.size() != 2
        ||
        devices.front().endpoint_reference !=
            "urn:uuid:camera-1"
        ||
        devices.front().xaddr !=
            "http://192.0.2.20/onvif/device_service"
        ||
        devices.front().remote_address !=
            "192.0.2.20"
        ||
        devices.front().scopes.find(
            "FrontDoor"
        ) == std::string::npos
    ) {
        std::cerr
            << "ONVIF discovery parser failed\n";

        return 1;
    }

    const auto media =
        CameraMediaTools::parseProbeOutput(
            "codec_name=h264|codec_type=video|width=1920|height=1080|r_frame_rate=25/1\n"
            "codec_name=aac|codec_type=audio|width=0|height=0|r_frame_rate=0/0\n"
        );

    if (
        !media.success
        ||
        media.video_codec != "h264"
        ||
        media.audio_codec != "aac"
        ||
        media.width != 1920
        ||
        media.height != 1080
        ||
        std::abs(
            media.fps - 25.0
        ) > 0.001
    ) {
        std::cerr
            << "Camera media parser failed\n";

        return 1;
    }

    const auto fractional =
        CameraMediaTools::parseProbeOutput(
            "codec_name=hevc|codec_type=video|width=3840|height=2160|r_frame_rate=30000/1001\n"
        );

    if (
        !fractional.success
        ||
        fractional.video_codec != "hevc"
        ||
        fractional.width != 3840
        ||
        fractional.height != 2160
        ||
        std::abs(
            fractional.fps -
            (
                30000.0 /
                1001.0
            )
        ) > 0.001
    ) {
        std::cerr
            << "Fractional FPS parsing failed\n";

        return 1;
    }

    std::cout
        << "Camera support test passed\n";

    return 0;
}
