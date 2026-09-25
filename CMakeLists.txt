cmake_minimum_required(VERSION 3.20)

project(
    HomeAICore
    VERSION 0.0.5
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

add_library(
    homeai-core-lib

    core/logging/Logger.cpp
    core/config/ConfigManager.cpp
    core/events/EventBus.cpp
    core/runtime/CoreRuntime.cpp

    server/system/SystemMonitor.cpp
    server/storage/StorageMonitor.cpp

    security/auth/SecurityManager.cpp

    web/server/WebServer.cpp
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

target_link_libraries(
    homeai-core-lib
    PUBLIC
    Threads::Threads
    OpenSSL::Crypto
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
