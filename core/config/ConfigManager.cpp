#include "core/config/ConfigManager.h"

#include <algorithm>
#include <cctype>
#include <fstream>

namespace homeai {

static std::string trim(std::string value)
{
    auto first = std::find_if_not(
        value.begin(),
        value.end(),
        [](unsigned char c) { return std::isspace(c); }
    );

    auto last = std::find_if_not(
        value.rbegin(),
        value.rend(),
        [](unsigned char c) { return std::isspace(c); }
    ).base();

    if (first >= last)
        return {};

    return std::string(first, last);
}

bool ConfigManager::load(const std::string& filename)
{
    std::ifstream file(filename);

    if (!file.is_open())
        return false;

    values_.clear();

    std::string line;

    while (std::getline(file, line)) {
        line = trim(line);

        if (line.empty() || line[0] == '#')
            continue;

        auto separator = line.find('=');

        if (separator == std::string::npos)
            continue;

        auto key = trim(line.substr(0, separator));
        auto value = trim(line.substr(separator + 1));

        if (!key.empty())
            values_[key] = value;
    }

    return true;
}

std::string ConfigManager::get(
    const std::string& key,
    const std::string& default_value
) const
{
    auto it = values_.find(key);

    if (it == values_.end())
        return default_value;

    return it->second;
}

int ConfigManager::getInt(
    const std::string& key,
    int default_value
) const
{
    auto value = get(key);

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
    auto value = get(key);

    std::transform(
        value.begin(),
        value.end(),
        value.begin(),
        [](unsigned char c) { return std::tolower(c); }
    );

    if (value == "true" || value == "1" || value == "yes")
        return true;

    if (value == "false" || value == "0" || value == "no")
        return false;

    return default_value;
}

}
