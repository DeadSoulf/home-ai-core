cmake_minimum_required(VERSION 3.20)

project(
    HomeAICore
    VERSION 0.0.6
    LANGUAGES CXX
)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

find_package(
    Threads
    REQUIRED
)

find_package(
    OpenSSL
    REQUIRED
)

find_package(
    SQLite3
    REQUIRED
)

file(READ "${CMAKE_CURRENT_SOURCE_DIR}/web/ui/translations.tsv" HOMEAI_TRANSLATIONS)
file(READ "${CMAKE_CURRENT_SOURCE_DIR}/web/ui/localization.js" HOMEAI_LOCALIZATION_JS)
set_property(DIRECTORY APPEND PROPERTY CMAKE_CONFIGURE_DEPENDS web/ui/translations.tsv web/ui/localization.js)
configure_file(web/ui/Localization.cpp.in "${CMAKE_CURRENT_BINARY_DIR}/Localization.cpp" @ONLY)

file(
    GLOB HOMEAI_NETWORK_SOURCES
    "${CMAKE_CURRENT_SOURCE_DIR}/server/network/*.cpp"
)

add_library(
    homeai-core-lib

    core/logging/Logger.cpp
    core/config/ConfigManager.cpp
    core/events/EventBus.cpp
    core/modules/Module.cpp
    core/modules/ModuleManager.cpp
    core/runtime/CoreRuntime.cpp

    server/system/SystemMonitor.cpp
    server/hardware/GpuMonitor.cpp
    ${HOMEAI_NETWORK_SOURCES}
    server/storage/StorageMonitor.cpp
    server/storage/StoragePool.cpp
    server/storage/DiskOperations.cpp
    server/update/UpdateManager.cpp

    security/auth/SecurityManager.cpp
    security/auth/UserDatabase.cpp

    web/server/WebServer.cpp
    web/server/JsonUtils.cpp
    web/ui/WebUi.cpp
    ${CMAKE_CURRENT_BINARY_DIR}/Localization.cpp
)

target_include_directories(
    homeai-core-lib
    PUBLIC
    ${CMAKE_CURRENT_SOURCE_DIR}
)

target_compile_options(
    homeai-core-lib
    PRIVATE
    -Wall
    -Wextra
    -Wpedantic
)

target_compile_definitions(
    homeai-core-lib
    PUBLIC
    HOMEAI_VERSION="${PROJECT_VERSION}"
)

target_link_libraries(
    homeai-core-lib
    PUBLIC
    Threads::Threads
    OpenSSL::Crypto
    SQLite::SQLite3
)

add_executable(
    home-ai-core
    main.cpp
)

target_link_libraries(
    home-ai-core
    PRIVATE
    homeai-core-lib
)

enable_testing()

add_executable(test-gpu-monitor tests/test_gpu_monitor.cpp)
target_link_libraries(test-gpu-monitor PRIVATE homeai-core-lib)
add_test(NAME gpu_monitor COMMAND test-gpu-monitor)
add_executable(test-admin-api tests/test_admin_api.cpp)
target_link_libraries(test-admin-api PRIVATE homeai-core-lib)
add_test(NAME admin_api COMMAND test-admin-api)
set_tests_properties(admin_api PROPERTIES TIMEOUT 30)

add_executable(
    test-event-bus
    tests/test_event_bus.cpp
)

target_link_libraries(
    test-event-bus
    PRIVATE
    homeai-core-lib
)

add_test(
    NAME event_bus
    COMMAND test-event-bus
)

add_executable(
    test-security
    tests/test_security.cpp
)

target_link_libraries(
    test-security
    PRIVATE
    homeai-core-lib
)

add_test(
    NAME security_core
    COMMAND test-security
)

add_executable(
    test-storage-monitor
    tests/test_storage_monitor.cpp
)

target_link_libraries(
    test-storage-monitor
    PRIVATE
    homeai-core-lib
)

add_test(
    NAME storage_monitor
    COMMAND test-storage-monitor
)

add_executable(
    test-disk-operations
    tests/test_disk_operations.cpp
)

target_link_libraries(
    test-disk-operations
    PRIVATE
    homeai-core-lib
)

add_test(
    NAME disk_operations
    COMMAND test-disk-operations
)


add_executable(
    home-ai-storage-helper
    server/storage/StorageHelperMain.cpp
)

target_link_libraries(
    home-ai-storage-helper
    PRIVATE
    homeai-core-lib
)

add_executable(
    home-ai-network-helper
    server/network/helper/NetworkHelperMain.cpp
)


add_executable(
    test-web-ui
    tests/test_web_ui.cpp
)

target_link_libraries(
    test-web-ui
    PRIVATE
    homeai-core-lib
)

add_test(
    NAME web_ui
    COMMAND test-web-ui
)


add_executable(
    test-update-manager
    tests/test_update_manager.cpp
)

target_link_libraries(
    test-update-manager
    PRIVATE
    homeai-core-lib
)

add_test(
    NAME update_manager
    COMMAND test-update-manager
)


add_executable(
    test-module-manager
    tests/test_module_manager.cpp
)

target_link_libraries(
    test-module-manager
    PRIVATE
    homeai-core-lib
)

add_test(
    NAME module_manager
    COMMAND test-module-manager
)


add_executable(
    test-user-management
    tests/test_user_management.cpp
)

target_link_libraries(
    test-user-management
    PRIVATE
    homeai-core-lib
)

add_test(
    NAME user_management
    COMMAND test-user-management
)


add_executable(
    test-user-api
    tests/test_user_api.cpp
)

target_link_libraries(
    test-user-api
    PRIVATE
    homeai-core-lib
)

add_test(
    NAME user_api
    COMMAND test-user-api
)

set_tests_properties(
    user_api
    PROPERTIES TIMEOUT 30
)


add_executable(
    test-json-utils
    tests/test_json_utils.cpp
)

target_link_libraries(
    test-json-utils
    PRIVATE
    homeai-core-lib
)

add_test(
    NAME json_utils
    COMMAND test-json-utils
)


add_executable(
    test-storage-pool
    tests/test_storage_pool.cpp
)

target_link_libraries(
    test-storage-pool
    PRIVATE
    homeai-core-lib
)

add_test(
    NAME storage_pool
    COMMAND test-storage-pool
)


add_executable(
    test-network-interfaces
    tests/test_network_interfaces.cpp
)

target_link_libraries(
    test-network-interfaces
    PRIVATE
    homeai-core-lib
)

add_test(
    NAME network_interfaces
    COMMAND test-network-interfaces
)
