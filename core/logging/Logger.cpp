#include "core/logging/Logger.h"

#include <chrono>
#include <ctime>
#include <iomanip>
#include <iostream>

namespace homeai {

Logger& Logger::instance()
{
    static Logger logger;
    return logger;
}

void Logger::setLevel(LogLevel level)
{
    level_ = level;
}

void Logger::debug(const std::string& message)
{
    log(LogLevel::Debug, message);
}

void Logger::info(const std::string& message)
{
    log(LogLevel::Info, message);
}

void Logger::warning(const std::string& message)
{
    log(LogLevel::Warning, message);
}

void Logger::error(const std::string& message)
{
    log(LogLevel::Error, message);
}

void Logger::log(LogLevel level, const std::string& message)
{
    if (static_cast<int>(level) < static_cast<int>(level_))
        return;

    const char* level_name = "INFO";

    switch (level) {
        case LogLevel::Debug:   level_name = "DEBUG"; break;
        case LogLevel::Info:    level_name = "INFO";  break;
        case LogLevel::Warning: level_name = "WARN";  break;
        case LogLevel::Error:   level_name = "ERROR"; break;
    }

    auto now = std::chrono::system_clock::now();
    std::time_t time = std::chrono::system_clock::to_time_t(now);

    std::tm tm{};

#ifdef _WIN32
    localtime_s(&tm, &time);
#else
    localtime_r(&time, &tm);
#endif

    std::lock_guard<std::mutex> lock(mutex_);

    std::cout
        << std::put_time(&tm, "%Y-%m-%d %H:%M:%S")
        << " [" << level_name << "] "
        << message
        << std::endl;
}

}
