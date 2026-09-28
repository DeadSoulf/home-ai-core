#include "core/modules/ModuleInstaller.h"
#include "core/modules/ModuleManager.h"

#include <filesystem>
#include <iostream>
#include <stdexcept>
#include <string>
#include <unistd.h>

namespace {

void check(
    bool value,
    const char* message
)
{
    if (!value)
        throw std::runtime_error(message);
}

void registerCatalog(
    homeai::ModuleManager& modules
)
{
    std::string error;

    check(
        modules.registerManifest(
            {
                "core",
                "Home AI Core",
                "0.0.62",
                "Core",
                {},
                {"system.view"},
                {},
                true,
                true,
                false
            },
            error
        ),
        "register core"
    );

    check(
        modules.registerManifest(
            {
                "network",
                "Network Core",
                "0.0.62",
                "Network",
                {},
                {"network.view"},
                {},
                false,
                true,
                false
            },
            error
        ),
        "register network"
    );

    check(
        modules.registerManifest(
            {
                "cluster",
                "Cluster Core",
                "0.0.62",
                "Cluster",
                {"network"},
                {"cluster.view"},
                {"cluster"},
                false,
                true,
                true
            },
            error
        ),
        "register cluster"
    );

    check(
        modules.registerManifest(
            {
                "automation",
                "Automation Core",
                "0.0.62",
                "Automation",
                {"cluster"},
                {"automation.view"},
                {},
                false,
                false,
                true
            },
            error
        ),
        "register automation"
    );
}

}

int main()
{
    namespace fs = std::filesystem;
    using namespace homeai;

    const auto root =
        fs::temp_directory_path()
        /
        (
            "homeai-module-installer-"
            +
            std::to_string(
                ::getpid()
            )
        );

    fs::remove_all(root);
    fs::create_directories(root);

    try {
        const auto registry =
            root
            /
            "runtime/modules/installed.tsv";

        ModuleManager first;
        registerCatalog(first);

        ModuleInstaller installer(
            first,
            registry
        );

        std::string error;

        check(
            installer.load(error),
            "load missing registry"
        );

        check(
            first.isInstalled("cluster"),
            "bundled cluster default"
        );

        check(
            installer.uninstall(
                "cluster",
                error
            ),
            "uninstall cluster"
        );

        check(
            !first.isInstalled("cluster"),
            "cluster state after uninstall"
        );

        check(
            fs::exists(registry),
            "registry was not created"
        );

        ModuleManager second;
        registerCatalog(second);

        ModuleInstaller reloaded(
            second,
            registry
        );

        error.clear();

        check(
            reloaded.load(error),
            "reload registry"
        );

        check(
            !second.isInstalled("cluster"),
            "persisted cluster uninstall"
        );

        error.clear();

        check(
            reloaded.install(
                "cluster",
                error
            ),
            "install cluster"
        );

        check(
            second.isInstalled("cluster"),
            "cluster state after install"
        );

        error.clear();

        check(
            reloaded.install(
                "automation",
                error
            ),
            "install dependent"
        );

        error.clear();

        check(
            !reloaded.uninstall(
                "cluster",
                error
            ),
            "dependent uninstall guard"
        );

        check(
            error.find("automation")
            != std::string::npos,
            "dependent error detail"
        );

        error.clear();

        check(
            reloaded.uninstall(
                "automation",
                error
            ),
            "uninstall dependent"
        );

        check(
            reloaded.uninstall(
                "cluster",
                error
            ),
            "uninstall cluster after dependent"
        );

        error.clear();

        check(
            !reloaded.uninstall(
                "network",
                error
            ),
            "immutable module guard"
        );

        check(
            error.find("cannot be uninstalled")
            != std::string::npos,
            "immutable module error"
        );

        error.clear();

        check(
            !reloaded.install(
                "missing",
                error
            ),
            "unknown module guard"
        );

        std::cout
            << "Module Installer test passed\n";
    }
    catch (const std::exception& ex) {
        std::cerr
            << "Module Installer test failed: "
            << ex.what()
            << '\n';

        fs::remove_all(root);
        return 1;
    }

    fs::remove_all(root);
    return 0;
}
