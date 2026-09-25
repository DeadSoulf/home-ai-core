#include "server/cameras/CameraMediaTools.h"

#include <algorithm>
#include <array>
#include <chrono>
#include <cmath>
#include <csignal>
#include <fcntl.h>
#include <poll.h>
#include <sstream>
#include <string_view>
#include <sys/wait.h>
#include <unistd.h>
#include <vector>

namespace homeai {

namespace {

struct ProcessResult {
    bool started{false};
    bool timed_out{false};
    int exit_code{-1};
    std::string output;
    std::string error;
};

void appendLimited(
    std::string& target,
    const char* data,
    std::size_t size,
    std::size_t maximum
)
{
    if (
        target.size() >=
        maximum
    ) {
        return;
    }

    const auto remaining =
        maximum
        -
        target.size();

    target.append(
        data,
        std::min(
            size,
            remaining
        )
    );
}

void drainPipe(
    int descriptor,
    std::string& target,
    std::size_t maximum
)
{
    std::array<char, 8192> buffer{};

    while (true) {
        const auto received =
            ::read(
                descriptor,
                buffer.data(),
                buffer.size()
            );

        if (received > 0) {
            appendLimited(
                target,
                buffer.data(),
                static_cast<
                    std::size_t
                >(received),
                maximum
            );

            continue;
        }

        break;
    }
}

ProcessResult runProcess(
    const std::vector<std::string>& arguments,
    int timeout_ms,
    std::size_t maximum_output
)
{
    ProcessResult result;

    if (arguments.empty())
        return result;

    int stdout_pipe[2]{-1, -1};
    int stderr_pipe[2]{-1, -1};

    if (
        ::pipe(stdout_pipe) != 0
        ||
        ::pipe(stderr_pipe) != 0
    ) {
        if (stdout_pipe[0] >= 0) {
            ::close(stdout_pipe[0]);
            ::close(stdout_pipe[1]);
        }

        if (stderr_pipe[0] >= 0) {
            ::close(stderr_pipe[0]);
            ::close(stderr_pipe[1]);
        }

        return result;
    }

    const pid_t child =
        ::fork();

    if (child < 0) {
        ::close(stdout_pipe[0]);
        ::close(stdout_pipe[1]);
        ::close(stderr_pipe[0]);
        ::close(stderr_pipe[1]);

        return result;
    }

    if (child == 0) {
        ::dup2(
            stdout_pipe[1],
            STDOUT_FILENO
        );

        ::dup2(
            stderr_pipe[1],
            STDERR_FILENO
        );

        ::close(stdout_pipe[0]);
        ::close(stdout_pipe[1]);
        ::close(stderr_pipe[0]);
        ::close(stderr_pipe[1]);

        std::vector<char*> argv;
        argv.reserve(
            arguments.size() + 1
        );

        for (
            const auto& argument :
            arguments
        ) {
            argv.push_back(
                const_cast<char*>(
                    argument.c_str()
                )
            );
        }

        argv.push_back(nullptr);

        ::execvp(
            argv.front(),
            argv.data()
        );

        _exit(127);
    }

    result.started = true;

    ::close(stdout_pipe[1]);
    ::close(stderr_pipe[1]);

    const int stdout_flags =
        ::fcntl(
            stdout_pipe[0],
            F_GETFL,
            0
        );

    const int stderr_flags =
        ::fcntl(
            stderr_pipe[0],
            F_GETFL,
            0
        );

    if (stdout_flags >= 0) {
        ::fcntl(
            stdout_pipe[0],
            F_SETFL,
            stdout_flags |
                O_NONBLOCK
        );
    }

    if (stderr_flags >= 0) {
        ::fcntl(
            stderr_pipe[0],
            F_SETFL,
            stderr_flags |
                O_NONBLOCK
        );
    }

    const auto deadline =
        std::chrono::steady_clock::now()
        +
        std::chrono::milliseconds(
            timeout_ms
        );

    int child_status = 0;
    bool child_done = false;

    while (!child_done) {
        drainPipe(
            stdout_pipe[0],
            result.output,
            maximum_output
        );

        drainPipe(
            stderr_pipe[0],
            result.error,
            64 * 1024
        );

        const auto waited =
            ::waitpid(
                child,
                &child_status,
                WNOHANG
            );

        if (waited == child) {
            child_done = true;
            break;
        }

        if (
            std::chrono::steady_clock::now()
            >=
            deadline
        ) {
            result.timed_out = true;

            ::kill(
                child,
                SIGKILL
            );

            ::waitpid(
                child,
                &child_status,
                0
            );

            child_done = true;
            break;
        }

        pollfd descriptors[2]{
            {
                stdout_pipe[0],
                POLLIN,
                0
            },
            {
                stderr_pipe[0],
                POLLIN,
                0
            }
        };

        ::poll(
            descriptors,
            2,
            50
        );
    }

    drainPipe(
        stdout_pipe[0],
        result.output,
        maximum_output
    );

    drainPipe(
        stderr_pipe[0],
        result.error,
        64 * 1024
    );

    ::close(stdout_pipe[0]);
    ::close(stderr_pipe[0]);

    if (
        WIFEXITED(
            child_status
        )
    ) {
        result.exit_code =
            WEXITSTATUS(
                child_status
            );
    }
    else if (
        WIFSIGNALED(
            child_status
        )
    ) {
        result.exit_code =
            128
            +
            WTERMSIG(
                child_status
            );
    }

    return result;
}

std::string trim(
    std::string value
)
{
    while (
        !value.empty()
        &&
        (
            value.back() == '\r'
            ||
            value.back() == '\n'
            ||
            value.back() == ' '
            ||
            value.back() == '\t'
        )
    ) {
        value.pop_back();
    }

    std::size_t offset = 0;

    while (
        offset < value.size()
        &&
        (
            value[offset] == ' '
            ||
            value[offset] == '\t'
        )
    ) {
        ++offset;
    }

    if (offset)
        value.erase(0, offset);

    return value;
}

double parseRate(
    const std::string& value
)
{
    if (
        value.empty()
        ||
        value == "0/0"
        ||
        value == "N/A"
    ) {
        return 0.0;
    }

    try {
        const auto slash =
            value.find('/');

        if (
            slash ==
            std::string::npos
        ) {
            return std::stod(value);
        }

        const auto numerator =
            std::stod(
                value.substr(
                    0,
                    slash
                )
            );

        const auto denominator =
            std::stod(
                value.substr(
                    slash + 1
                )
            );

        if (
            std::abs(
                denominator
            ) < 0.000001
        ) {
            return 0.0;
        }

        return
            numerator /
            denominator;
    }
    catch (...) {
        return 0.0;
    }
}

std::string shortError(
    const ProcessResult& process
)
{
    if (!process.started)
        return "Не удалось запустить медиа-инструмент.";

    if (process.timed_out)
        return "Медиа-проверка превысила таймаут.";

    auto message =
        trim(
            process.error
        );

    if (
        message.size() >
        2000
    ) {
        message.resize(2000);
    }

    if (message.empty()) {
        message =
            "Медиа-инструмент завершился с ошибкой.";
    }

    return message;
}

}

CameraMediaProbe
CameraMediaTools::parseProbeOutput(
    const std::string& output
)
{
    CameraMediaProbe result;

    std::istringstream stream(
        output
    );

    std::string line;
    bool video_found = false;

    while (
        std::getline(
            stream,
            line
        )
    ) {
        std::string codec_type;
        std::string codec_name;
        int width = 0;
        int height = 0;
        double fps = 0.0;

        std::istringstream fields(
            line
        );

        std::string field;

        while (
            std::getline(
                fields,
                field,
                '|'
            )
        ) {
            const auto equals =
                field.find('=');

            if (
                equals ==
                std::string::npos
            ) {
                continue;
            }

            const auto key =
                field.substr(
                    0,
                    equals
                );

            const auto value =
                field.substr(
                    equals + 1
                );

            if (
                key == "codec_type"
            ) {
                codec_type = value;
            }
            else if (
                key == "codec_name"
            ) {
                codec_name = value;
            }
            else if (
                key == "width"
            ) {
                try {
                    width =
                        std::stoi(value);
                }
                catch (...) {
                    width = 0;
                }
            }
            else if (
                key == "height"
            ) {
                try {
                    height =
                        std::stoi(value);
                }
                catch (...) {
                    height = 0;
                }
            }
            else if (
                key ==
                    "r_frame_rate"
            ) {
                fps =
                    parseRate(value);
            }
        }

        if (
            codec_type == "video"
            &&
            !video_found
        ) {
            result.video_codec =
                codec_name;
            result.width = width;
            result.height = height;
            result.fps = fps;
            video_found = true;
        }
        else if (
            codec_type == "audio"
            &&
            result.audio_codec.empty()
        ) {
            result.audio_codec =
                codec_name;
        }
    }

    result.success =
        video_found;

    result.code =
        result.success
        ? "ok"
        : "no_video";

    result.message =
        result.success
        ? "RTSP-видеопоток доступен."
        : "Видеопоток не найден.";

    return result;
}

CameraMediaProbe
CameraMediaTools::probe(
    const std::string& stream_url
)
{
    const auto process =
        runProcess(
            {
                "ffprobe",
                "-v",
                "error",
                "-rtsp_transport",
                "tcp",
                "-rw_timeout",
                "5000000",
                "-show_entries",
                "stream=codec_type,codec_name,width,height,r_frame_rate",
                "-of",
                "compact=p=0:nk=0",
                stream_url
            },
            8000,
            512 * 1024
        );

    if (
        !process.started
        ||
        process.timed_out
        ||
        process.exit_code != 0
    ) {
        return {
            false,
            process.exit_code == 127
                ? "ffprobe_missing"
                : (
                    process.timed_out
                    ? "timeout"
                    : "probe_failed"
                ),
            shortError(process),
            "",
            "",
            0,
            0,
            0.0
        };
    }

    return
        parseProbeOutput(
            process.output
        );
}

CameraSnapshot
CameraMediaTools::snapshot(
    const std::string& stream_url
)
{
    const auto process =
        runProcess(
            {
                "ffmpeg",
                "-hide_banner",
                "-loglevel",
                "error",
                "-rtsp_transport",
                "tcp",
                "-rw_timeout",
                "5000000",
                "-i",
                stream_url,
                "-frames:v",
                "1",
                "-an",
                "-f",
                "image2pipe",
                "-vcodec",
                "mjpeg",
                "-q:v",
                "3",
                "pipe:1"
            },
            10000,
            8 * 1024 * 1024
        );

    if (
        !process.started
        ||
        process.timed_out
        ||
        process.exit_code != 0
    ) {
        return {
            false,
            process.exit_code == 127
                ? "ffmpeg_missing"
                : (
                    process.timed_out
                    ? "timeout"
                    : "snapshot_failed"
                ),
            shortError(process),
            ""
        };
    }

    const bool jpeg =
        process.output.size() >= 4
        &&
        static_cast<
            unsigned char
        >(
            process.output[0]
        ) == 0xff
        &&
        static_cast<
            unsigned char
        >(
            process.output[1]
        ) == 0xd8;

    if (!jpeg) {
        return {
            false,
            "invalid_snapshot",
            "FFmpeg не вернул JPEG-кадр.",
            ""
        };
    }

    return {
        true,
        "ok",
        "Snapshot ready.",
        process.output
    };
}

}
