// Isolated libvirt ABI test double. Loaded only by tests through LD_LIBRARY_PATH.
#include <cstdlib>
#include <cstring>
#include <string>

struct NodeInfo { char model[32]; unsigned long memory; unsigned cpus, mhz, nodes, sockets, cores, threads; };
struct DomainInfo { unsigned char state; unsigned long maxMem, memory; unsigned short nrVirtCpu; unsigned long long cpuTime; };
static int connection, domain, created_domain, error_code, calls, state = 5, connections, domains, autostart;
static bool fail_write, fail_action, fail_info, missing, fail_define, created;
static std::string created_name;
static const char* uuid = "11111111-2222-3333-4444-555555555555";
static const char* created_uuid = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee";

extern "C" {
void fakeReset() {
    state = 5; autostart = 0; error_code = calls = 0;
    fail_write = fail_action = fail_info = missing = fail_define = created = false;
    created_name.clear();
}
void fakeState(int value) { state = value; }
void fakeFailure(int value) {
    fail_write = value == 1;
    fail_action = value == 2;
    fail_info = value == 3;
    missing = value == 4;
    fail_define = value == 5;
}
int fakeCalls() { return calls; }
int fakeResources() { return connections + domains; }

int virInitialize() { return 0; }
void* virConnectOpenReadOnly(const char*) { ++connections; return &connection; }
void* virConnectOpen(const char*) { if (fail_write) { error_code = 29; return nullptr; } ++connections; return &connection; }
int virConnectClose(void*) { --connections; return 0; }
int virConnectGetLibVersion(void*, unsigned long* version) { *version = 10000000; return 0; }
int virConnectGetVersion(void*, unsigned long* version) { *version = 9000000; return 0; }
char* virConnectGetURI(void*) { return ::strdup("qemu:///system"); }
int virNodeGetInfo(void*, NodeInfo* info) {
    *info = {};
    std::strcpy(info->model, "test CPU");
    info->cpus = 8;
    info->memory = 8UL * 1024UL * 1024UL;
    return 0;
}

int virConnectListAllDomains(void*, void*** result, unsigned) {
    const int count = created ? 2 : 1;
    *result = static_cast<void**>(std::malloc(sizeof(void*) * static_cast<std::size_t>(count)));
    (*result)[0] = &domain; ++domains;
    if (created) { (*result)[1] = &created_domain; ++domains; }
    return count;
}

const char* virDomainGetName(void* value) {
    return value == &created_domain ? created_name.c_str() : "test <VM>";
}
int virDomainGetUUIDString(void* value, char* result) {
    std::strcpy(result, value == &created_domain ? created_uuid : uuid);
    return 0;
}
int virDomainGetInfo(void* value, DomainInfo* info) {
    if (fail_info) { error_code = 1; return -1; }
    *info = {};
    info->state = value == &created_domain ? 5 : state;
    info->nrVirtCpu = 2;
    info->memory = info->maxMem = 4096UL * 1024UL;
    return 0;
}
int virDomainGetAutostart(void* value, int* out) {
    *out = value == &created_domain ? 0 : autostart;
    return 0;
}
int virDomainIsActive(void* value) { return value == &created_domain ? 0 : state != 5; }
int virDomainFree(void*) { --domains; return 0; }

void* virDomainLookupByUUIDString(void*, const char* value) {
    if (missing) { error_code = 42; return nullptr; }
    if (!std::strcmp(value, uuid)) { ++domains; return &domain; }
    if (created && !std::strcmp(value, created_uuid)) { ++domains; return &created_domain; }
    error_code = 42; return nullptr;
}
void* virDomainLookupByName(void*, const char* value) {
    if (!std::strcmp(value, "test <VM>")) { ++domains; return &domain; }
    if (created && created_name == value) { ++domains; return &created_domain; }
    error_code = 42;
    return nullptr;
}
void* virDomainDefineXML(void*, const char* xml) {
    ++calls;
    if (fail_define) { error_code = 1; return nullptr; }
    const std::string text = xml ? xml : "";
    const std::string open = "<name>";
    const std::string close = "</name>";
    const auto begin = text.find(open);
    const auto end = begin == std::string::npos ? std::string::npos : text.find(close, begin + open.size());
    if (begin == std::string::npos || end == std::string::npos) { error_code = 1; return nullptr; }
    const std::string name = text.substr(begin + open.size(), end - (begin + open.size()));
    if (name == "test <VM>" || (created && name == created_name)) { error_code = 55; return nullptr; }
    created_name = name;
    created = true;
    ++domains;
    return &created_domain;
}

int operation(int next) {
    ++calls;
    if (fail_action) { error_code = 1; return -1; }
    state = next; return 0;
}
int virDomainCreate(void* value) {
    if (value == &created_domain) { ++calls; return 0; }
    return operation(1);
}
int virDomainShutdown(void*) { return operation(state); }
int virDomainReboot(void*, unsigned) { return operation(state); }
int virDomainSuspend(void*) { return operation(3); }
int virDomainResume(void*) { return operation(1); }
int virDomainSetAutostart(void* value, int next) {
    ++calls;
    if (fail_action) { error_code = 1; return -1; }
    if (value != &created_domain) autostart = next ? 1 : 0;
    return 0;
}
int virDomainDestroy(void* value) {
    if (value == &created_domain) { ++calls; return 0; }
    return operation(5);
}
int virGetLastErrorCode() { return error_code; }
const char* virGetLastErrorMessage() { return "Simulated libvirt failure"; }
}
