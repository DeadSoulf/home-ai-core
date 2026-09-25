cmake_minimum_required(VERSION 3.20)

project(
    HomeAICore
    VERSION 0.0.3
    LANGUAGES CXX
)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

find_package(
    Threads
    REQUIRED
)

add_library(
    homeai-core-lib

    core/logging/Logger.cpp
    core/config/ConfigManager.cpp
    core/events/EventBus.cpp
    core/runtime/CoreRuntime.cpp

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
