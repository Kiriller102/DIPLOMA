cmake_minimum_required(VERSION 3.26)
project(CryptoProject VERSION 2.2)

set(CMAKE_CXX_STANDARD 20)

add_executable(Kuznyechik tests/kuznyechik_test.cpp src/kuznechik/kuznechik.cpp)
