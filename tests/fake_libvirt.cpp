// Isolated libvirt ABI test double. Loaded only by tests through LD_LIBRARY_PATH.
#include <cstdlib>
#include <cstring>

struct NodeInfo { char model[32]; unsigned long memory; unsigned cpus, mhz, nodes, sockets, cores, threads; };
struct DomainInfo { unsigned char state; unsigned long maxMem, memory; unsigned short nrVirtCpu; unsigned long long cpuTime; };
static int connection, domain, error_code, calls, state = 5, connections, domains;
static bool fail_write, fail_action, fail_info, missing;
static const char* uuid = "11111111-2222-3333-4444-555555555555";
extern "C" {
void fakeReset() { state = 5; error_code = calls = 0; fail_write = fail_action = fail_info = missing = false; }
void fakeState(int value) { state = value; }
void fakeFailure(int value) { fail_write = value == 1; fail_action = value == 2; fail_info = value == 3; missing = value == 4; }
int fakeCalls() { return calls; }
int fakeResources() { return connections + domains; }
int virInitialize() { return 0; }
void* virConnectOpenReadOnly(const char*) { ++connections; return &connection; }
void* virConnectOpen(const char*) { if (fail_write) { error_code = 29; return nullptr; } ++connections; return &connection; }
int virConnectClose(void*) { --connections; return 0; }
int virConnectGetLibVersion(void*, unsigned long* version) { *version = 10000000; return 0; }
int virConnectGetVersion(void*, unsigned long* version) { *version = 9000000; return 0; }
char* virConnectGetURI(void*) { return ::strdup("qemu:///system"); }
int virNodeGetInfo(void*, NodeInfo* info) { *info = {}; std::strcpy(info->model, "test CPU"); info->cpus = 2; return 0; }
int virConnectListAllDomains(void*, void*** result, unsigned) {
    *result = static_cast<void**>(std::malloc(sizeof(void*)));
    (*result)[0] = &domain; ++domains; return 1;
}
const char* virDomainGetName(void*) { return "test <VM>"; }
int virDomainGetUUIDString(void*, char* result) { std::strcpy(result, uuid); return 0; }
int virDomainGetInfo(void*, DomainInfo* info) {
    if (fail_info) { error_code = 1; return -1; }
    *info = {}; info->state = state; info->nrVirtCpu = 2; return 0;
}
int virDomainGetAutostart(void*, int* value) { *value = 0; return 0; }
int virDomainIsActive(void*) { return state != 5; }
int virDomainFree(void*) { --domains; return 0; }
void* virDomainLookupByUUIDString(void*, const char* value) {
    if (missing || std::strcmp(value, uuid)) { error_code = 42; return nullptr; }
    ++domains; return &domain;
}
int operation(int next) {
    ++calls;
    if (fail_action) { error_code = 1; return -1; }
    state = next; return 0;
}
int virDomainCreate(void*) { return operation(1); }
// Intentionally emulate a guest ignoring a graceful request.
int virDomainShutdown(void*) { return operation(state); }
int virDomainReboot(void*, unsigned) { return operation(state); }
int virDomainDestroy(void*) { return operation(5); }
int virGetLastErrorCode() { return error_code; }
const char* virGetLastErrorMessage() { return "Simulated libvirt failure"; }
}
