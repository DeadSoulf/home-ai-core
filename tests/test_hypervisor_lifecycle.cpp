#include "server/virtualization/HypervisorManager.h"
#include <dlfcn.h>
#include <algorithm>
#include <iostream>
#include <stdexcept>

void check(bool ok, const char* message) { if (!ok) throw std::runtime_error(message); }
int main() {
    void* library = dlopen("libvirt.so.0", RTLD_NOW);
    if (!library) return 1;
    auto reset = reinterpret_cast<void(*)()>(dlsym(library, "fakeReset"));
    auto state = reinterpret_cast<void(*)(int)>(dlsym(library, "fakeState"));
    auto failure = reinterpret_cast<void(*)(int)>(dlsym(library, "fakeFailure"));
    auto calls = reinterpret_cast<int(*)()>(dlsym(library, "fakeCalls"));
    auto resources = reinterpret_cast<int(*)()>(dlsym(library, "fakeResources"));
    if (!reset || !state || !failure || !calls || !resources) return 1;
    try {
        reset();
        homeai::HypervisorManager manager;
        std::string error;
        manager.initialize(error);
        const std::string uuid = "11111111-2222-3333-4444-555555555555";
        auto act = [&](const char* action, const char* expected) { return manager.performAction(uuid, action, expected, uuid); };
        check(act("start", "running").code == "state_changed" && calls() == 0, "stale confirmation");
        check(act("shutdown", "shutoff").code == "invalid_state" && calls() == 0, "invalid action state");
        check(act("start", "shutoff").state == "running" && calls() == 1, "start");
        auto result = act("shutdown", "running");
        check(result.success && result.code == "accepted" && result.state == "running", "asynchronous shutdown");
        const auto pending = manager.snapshot(error).machines[0];
        check(pending.pending_action == "shutdown" && pending.allowed_actions == std::vector<std::string>{"force-off"}, "pending inventory");
        check(act("reboot", "running").code == "operation_pending" && calls() == 2, "duplicate suppression");
        check(act("force-off", "running").state == "shutoff", "force off overrides pending");
        check(act("start", "shutoff").success, "restart after force off");
        check(act("pause", "running").state == "paused", "pause");
        const auto paused = manager.snapshot(error).machines[0];
        check(paused.state == "paused" &&
            std::find(paused.allowed_actions.begin(), paused.allowed_actions.end(), "resume") != paused.allowed_actions.end(),
            "paused inventory");
        check(act("resume", "paused").state == "running", "resume");
        check(act("autostart-on", "running").success, "enable autostart");
        check(manager.snapshot(error).machines[0].autostart, "autostart inventory on");
        check(act("autostart-off", "running").success, "disable autostart");
        check(!manager.snapshot(error).machines[0].autostart, "autostart inventory off");
        check(act("reboot", "running").success, "reboot");
        check(act("force-off", "running").success, "stop");
        check(act("start", "shutoff").success, "start for graceful completion");
        check(act("shutdown", "running").success, "graceful request");
        state(5);
        check(act("start", "shutoff").success, "completed shutdown permits restart");
        state(3);
        check(act("reboot", "paused").code == "invalid_state", "paused reboot denied");
        check(act("force-off", "paused").success, "paused force off");
        state(0);
        check(act("start", "no-state").code == "invalid_state", "unknown state denied");
        state(5);
        failure(1);
        check(act("start", "shutoff").code == "permission_denied", "write permission failure");
        check(manager.snapshot(error).machines.size() == 1, "read-only survives write failure");
        failure(2);
        check(act("start", "shutoff").code == "backend_error", "backend failure");
        failure(3);
        check(act("start", "shutoff").code == "backend_error", "state read failure");
        check(manager.snapshot(error).machines[0].allowed_actions.empty(), "failed inventory disables actions");
        failure(4);
        check(act("start", "shutoff").code == "not_found", "deleted domain");
        check(resources() == 0, "connection/domain handles leaked");
        manager.shutdown();
        check(act("start", "shutoff").code == "unavailable", "stopped module");
        for (const auto& capability : manager.capabilities()) {
            const bool expected =
                capability.name == "lifecycle"
                || capability.name == "pause_resume"
                || capability.name == "autostart";
            check(capability.implemented == expected, "future features disabled");
        }
    } catch (const std::exception& error) { std::cerr << error.what() << '\n'; return 1; }
    dlclose(library);
    std::cout << "Hypervisor lifecycle tests passed\n";
}
