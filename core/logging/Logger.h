#pragma once

#include <mutex>
#include <string>

namespace homeai {

enum class LogLevel {
    Debug,
    Info,
    Warning,
    Error
};

class Logger {
public:
    static Logger& instance();

    void setLevel(LogLevel level);

    void debug(const std::string& message);
    void info(const std::string& message);
    void warning(const std::string& message);
    void error(const std::string& message);

private:
    Logger() = default;

    void log(LogLevel level, const std::string& message);

    LogLevel level_{LogLevel::Info};
    std::mutex mutex_;
};

}
