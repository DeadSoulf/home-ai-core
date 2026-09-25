#include "core/config/ConfigManager.h"

#include <algorithm>
#include <cctype>
#include <fstream>
#include <vector>

namespace homeai {

static std::string trim(std::string value)
{
    auto first = std::find_if_not(
        value.begin(),
        value.end(),
        [](unsigned char c) {
            return std::isspace(c);
        }
    );

    auto last = std::find_if_not(
        value.rbegin(),
        value.rend(),
        [](unsigned char c) {
            return std::isspace(c);
        }
    ).base();

    if (first >= last)
        return {};

    return std::string(first, last);
}

bool ConfigManager::load(
    const std::string& filename
)
{
    std::ifstream file(filename);

    if (!file.is_open())
        return false;

    std::unordered_map<
        std::string,
        std::string
    > new_values;

    std::string line;

    while (std::getline(file, line)) {
        line = trim(line);

        if (
            line.empty() ||
            line[0] == '#'
        ) {
            continue;
        }

        const auto separator =
            line.find('=');

        if (
            separator ==
            std::string::npos
        ) {
            continue;
        }

        const auto key =
            trim(
                line.substr(
                    0,
                    separator
                )
            );

        const auto value =
            trim(
                line.substr(
                    separator + 1
                )
            );

        if (!key.empty())
            new_values[key] = value;
    }

    {
        std::lock_guard<std::mutex>
            lock(mutex_);

        values_ = std::move(new_values);
        filename_ = filename;
    }

    return true;
}

bool ConfigManager::save() const
{
    std::lock_guard<std::mutex>
        lock(mutex_);

    if (filename_.empty())
        return false;

    std::ofstream file(
        filename_,
        std::ios::trunc
    );

    if (!file.is_open())
        return false;

    std::vector<std::string> keys;

    keys.reserve(values_.size());

    for (const auto& entry : values_)
        keys.push_back(entry.first);

    std::sort(
        keys.begin(),
        keys.end()
    );

    file
        << "# Home AI Core configuration\n\n";

    for (const auto& key : keys) {
        file
            << key
            << '='
            << values_.at(key)
            << '\n';
    }

    return true;
}

std::string ConfigManager::get(
    const std::string& key,
    const std::string& default_value
) const
{
    std::lock_guard<std::mutex>
        lock(mutex_);

    const auto it =
        values_.find(key);

    if (it == values_.end())
        return default_value;

    return it->second;
}

int ConfigManager::getInt(
    const std::string& key,
    int default_value
) const
{
    const auto value =
        get(key);

    if (value.empty())
        return default_value;

    try {
        return std::stoi(value);
    }
    catch (...) {
        return default_value;
    }
}

bool ConfigManager::getBool(
    const std::string& key,
    bool default_value
) const
{
    auto value =
        get(key);

    std::transform(
        value.begin(),
        value.end(),
        value.begin(),
        [](unsigned char c) {
            return std::tolower(c);
        }
    );

    if (
        value == "true" ||
        value == "1" ||
        value == "yes"
    ) {
        return true;
    }

    if (
        value == "false" ||
        value == "0" ||
        value == "no"
    ) {
        return false;
    }

    return default_value;
}

void ConfigManager::set(
    const std::string& key,
    const std::string& value
)
{
    std::lock_guard<std::mutex>
        lock(mutex_);

    values_[key] = value;
}

}
