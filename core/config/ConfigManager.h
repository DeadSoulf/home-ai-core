#pragma once

#include <string>
#include <unordered_map>

namespace homeai {

class ConfigManager {
public:
    bool load(const std::string& filename);

    std::string get(
        const std::string& key,
        const std::string& default_value = ""
    ) const;

    int getInt(
        const std::string& key,
        int default_value = 0
    ) const;

    bool getBool(
        const std::string& key,
        bool default_value = false
    ) const;

private:
    std::unordered_map<std::string, std::string> values_;
};

}
