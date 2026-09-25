#include "web/server/JsonUtils.h"

#include <array>
#include <string>

namespace homeai {

std::string jsonEscape(
    const std::string& value
)
{
    static constexpr char digits[] =
        "0123456789abcdef";

    std::string result;

    result.reserve(
        value.size() + 16
    );

    for (
        const unsigned char c :
        value
    ) {
        switch (c) {
            case '\\':
                result += "\\\\";
                break;

            case '"':
                result += "\\\"";
                break;

            case '\b':
                result += "\\b";
                break;

            case '\f':
                result += "\\f";
                break;

            case '\n':
                result += "\\n";
                break;

            case '\r':
                result += "\\r";
                break;

            case '\t':
                result += "\\t";
                break;

            default:
                if (c < 0x20) {
                    result += "\\u00";
                    result +=
                        digits[
                            (c >> 4)
                            &
                            0x0f
                        ];

                    result +=
                        digits[
                            c
                            &
                            0x0f
                        ];
                }
                else {
                    result +=
                        static_cast<char>(
                            c
                        );
                }

                break;
        }
    }

    return result;
}

}
