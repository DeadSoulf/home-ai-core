#pragma once

#include <functional>
#include <string>
#include <utility>
#include <vector>

namespace homeai {

enum class ModuleHealth {
    Unknown,
    Healthy,
    Degraded,
    Unhealthy
};

class IModule {
public:
    virtual ~IModule() = default;

    virtual std::string name() const = 0;

    virtual std::vector<std::string>
    dependencies() const
    {
        return {};
    }

    virtual bool initialize(
        std::string& error
    ) = 0;

    virtual bool start(
        std::string& error
    ) = 0;

    virtual void stop() = 0;

    virtual ModuleHealth health() const = 0;

    virtual std::string
    healthMessage() const
    {
        return {};
    }
};

class CallbackModule final
    : public IModule {
public:
    using Action =
        std::function<
            bool(std::string&)
        >;

    using StopAction =
        std::function<void()>;

    using HealthAction =
        std::function<ModuleHealth()>;

    using MessageAction =
        std::function<std::string()>;

    CallbackModule(
        std::string name,
        std::vector<std::string> dependencies,
        Action initialize,
        Action start,
        StopAction stop,
        HealthAction health,
        MessageAction health_message = {}
    );

    std::string name() const override;

    std::vector<std::string>
    dependencies() const override;

    bool initialize(
        std::string& error
    ) override;

    bool start(
        std::string& error
    ) override;

    void stop() override;

    ModuleHealth health() const override;

    std::string healthMessage() const override;

private:
    std::string name_;
    std::vector<std::string>
        dependencies_;

    Action initialize_;
    Action start_;
    StopAction stop_;
    HealthAction health_;
    MessageAction health_message_;
};

std::string moduleHealthToString(
    ModuleHealth health
);

}
