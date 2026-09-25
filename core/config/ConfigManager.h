#pragma once

#include <mutex>
#include <string>
#include <unordered_map>

namespace homeai {

class ConfigManager {
public:
    bool load(const std::string& filename);

    bool save() const;

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

    void set(
        const std::string& key,
        const std::string& value
    );

private:
    mutable std::mutex mutex_;

    std::unordered_map<
        std::string,
        std::string
    > values_;

    std::string filename_;
};

}
