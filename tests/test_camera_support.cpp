#include "server/cameras/CameraMediaTools.h"
#include "server/cameras/LanCameraDiscovery.h"
#include "server/cameras/OnvifDiscovery.h"
#include "server/cameras/OnvifMediaClient.h"

#include <cmath>
#include <iostream>
#include <string>
#include <vector>

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

    const std::vector<int>
        hikvisionPorts{
            80,
            554,
            8000
        };

    if (
        !LanCameraDiscovery::likelyCamera(
            hikvisionPorts
        )
        ||
        LanCameraDiscovery::vendorHint(
            hikvisionPorts
        ) != "Hikvision"
        ||
        LanCameraDiscovery::
            suggestedRtspUrl(
                "192.0.2.30",
                "Hikvision",
                554
            ) !=
            "rtsp://192.0.2.30:554/Streaming/Channels/101"
    ) {
        std::cerr
            << "Hikvision LAN discovery heuristics failed\n";

        return 1;
    }

    const std::vector<int>
        webOnlyPorts{
            80,
            443
        };

    if (
        LanCameraDiscovery::likelyCamera(
            webOnlyPorts
        )
    ) {
        std::cerr
            << "Generic Web host was misclassified as a camera\n";

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

    const std::string deviceInformation =
        "<s:Envelope><s:Body>"
        "<tds:GetDeviceInformationResponse>"
        "<tds:Manufacturer>Acme Vision</tds:Manufacturer>"
        "<tds:Model>VX-4K</tds:Model>"
        "<tds:FirmwareVersion>5.4.2</tds:FirmwareVersion>"
        "<tds:SerialNumber>ABC12345</tds:SerialNumber>"
        "<tds:HardwareId>HW-7</tds:HardwareId>"
        "</tds:GetDeviceInformationResponse>"
        "</s:Body></s:Envelope>";

    const auto information =
        OnvifMediaClient::parseDeviceInformation(
            deviceInformation
        );

    if (
        information.manufacturer !=
            "Acme Vision"
        ||
        information.model !=
            "VX-4K"
        ||
        information.firmware_version !=
            "5.4.2"
        ||
        information.serial_number !=
            "ABC12345"
        ||
        information.hardware_id !=
            "HW-7"
    ) {
        std::cerr
            << "ONVIF device information parser failed\n";

        return 1;
    }

    const std::string capabilities =
        "<s:Envelope><s:Body>"
        "<tds:GetCapabilitiesResponse>"
        "<tds:Capabilities>"
        "<tt:Media>"
        "<tt:XAddr>http://192.0.2.20/onvif/media_service</tt:XAddr>"
        "</tt:Media>"
        "</tds:Capabilities>"
        "</tds:GetCapabilitiesResponse>"
        "</s:Body></s:Envelope>";

    if (
        OnvifMediaClient::parseMediaXAddr(
            capabilities
        ) !=
        "http://192.0.2.20/onvif/media_service"
    ) {
        std::cerr
            << "ONVIF media XAddr parser failed\n";

        return 1;
    }

    const std::string ptzCapabilities =
        "<s:Envelope><s:Body>"
        "<tds:GetCapabilitiesResponse>"
        "<tds:Capabilities>"
        "<tt:PTZ>"
        "<tt:XAddr>http://192.0.2.20/onvif/ptz_service</tt:XAddr>"
        "</tt:PTZ>"
        "</tds:Capabilities>"
        "</tds:GetCapabilitiesResponse>"
        "</s:Body></s:Envelope>";

    if (
        OnvifMediaClient::parsePtzXAddr(
            ptzCapabilities
        ) !=
        "http://192.0.2.20/onvif/ptz_service"
    ) {
        std::cerr
            << "ONVIF PTZ XAddr parser failed\n";

        return 1;
    }

    const std::string profilesXml =
        "<s:Envelope><s:Body>"
        "<trt:GetProfilesResponse>"
        "<trt:Profiles token=\"main\">"
        "<tt:Name>MainStream</tt:Name>"
        "<tt:PTZConfiguration token=\"ptz-main\"/>"
        "<tt:VideoEncoderConfiguration>"
        "<tt:Encoding>H264</tt:Encoding>"
        "<tt:Resolution>"
        "<tt:Width>1920</tt:Width>"
        "<tt:Height>1080</tt:Height>"
        "</tt:Resolution>"
        "<tt:RateControl>"
        "<tt:FrameRateLimit>25</tt:FrameRateLimit>"
        "</tt:RateControl>"
        "</tt:VideoEncoderConfiguration>"
        "</trt:Profiles>"
        "<trt:Profiles token=\"sub\">"
        "<tt:Name>SubStream</tt:Name>"
        "<tt:VideoEncoderConfiguration>"
        "<tt:Encoding>H264</tt:Encoding>"
        "<tt:Resolution>"
        "<tt:Width>640</tt:Width>"
        "<tt:Height>360</tt:Height>"
        "</tt:Resolution>"
        "<tt:RateControl>"
        "<tt:FrameRateLimit>15</tt:FrameRateLimit>"
        "</tt:RateControl>"
        "</tt:VideoEncoderConfiguration>"
        "</trt:Profiles>"
        "</trt:GetProfilesResponse>"
        "</s:Body></s:Envelope>";

    const auto profiles =
        OnvifMediaClient::parseProfiles(
            profilesXml
        );

    if (
        profiles.size() != 2
        ||
        profiles[0].token != "main"
        ||
        profiles[0].name !=
            "MainStream"
        ||
        profiles[0].encoding !=
            "H264"
        ||
        !profiles[0].ptz
        ||
        profiles[0].width != 1920
        ||
        profiles[0].height != 1080
        ||
        std::abs(
            profiles[0].fps - 25.0
        ) > 0.001
        ||
        profiles[1].token != "sub"
        ||
        profiles[1].width != 640
        ||
        profiles[1].height != 360
    ) {
        std::cerr
            << "ONVIF media profiles parser failed\n";

        return 1;
    }

    const auto uri =
        OnvifMediaClient::parseStreamUri(
            "<s:Envelope><tt:MediaUri>"
            "<tt:Uri>rtsp://admin:secret@192.0.2.20:554/main</tt:Uri>"
            "</tt:MediaUri></s:Envelope>"
        );

    if (
        uri !=
        "rtsp://192.0.2.20:554/main"
    ) {
        std::cerr
            << "ONVIF stream URI sanitizer failed: "
            << uri
            << '\n';

        return 1;
    }

    std::cout
        << "Camera support test passed\n";

    return 0;
}
