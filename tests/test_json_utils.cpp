#include "web/server/JsonUtils.h"

#include <iostream>
#include <string>

int main()
{
    std::string input =
        "mkfs.ext4";

    input.push_back('\n');
    input += "progress";
    input.push_back('\b');
    input.push_back('\f');
    input.push_back('\t');
    input.push_back('\r');
    input.push_back(
        static_cast<char>(0x01)
    );
    input.push_back(
        static_cast<char>(0x1b)
    );
    input += "\\\"done";

    const auto escaped =
        homeai::jsonEscape(
            input
        );

    const std::string expected =
        "mkfs.ext4\\nprogress"
        "\\b\\f\\t\\r"
        "\\u0001\\u001b"
        "\\\\\\\"done";

    if (escaped != expected) {
        std::cerr
            << "JSON escaping mismatch\n"
            << "Expected: "
            << expected
            << "\nActual:   "
            << escaped
            << '\n';

        return 1;
    }

    for (
        const unsigned char c :
        escaped
    ) {
        if (c < 0x20) {
            std::cerr
                << "Raw JSON control character remains in escaped output\n";

            return 1;
        }
    }

    std::cout
        << "JSON control-character escaping test passed\n";

    return 0;
}
