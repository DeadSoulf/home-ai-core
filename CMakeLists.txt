cmake_minimum_required(VERSION 3.20)

project(
    HomeAICore
    VERSION 0.0.1
    LANGUAGES CXX
)

set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

add_executable(
    home-ai-core
    main.cpp
)

target_compile_options(
    home-ai-core
    PRIVATE
    -Wall
    -Wextra
    -Wpedantic
)
