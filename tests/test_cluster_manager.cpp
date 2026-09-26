#include "server/cluster/ClusterManager.h"

#include <iostream>
#include <string>

int main()
{
    using homeai::ClusterManager;

    if (
        !ClusterManager::validNodeId(
            "node-01.example"
        )
        ||
        ClusterManager::validNodeId(
            "../bad/node"
        )
    ) {
        std::cerr
            << "Cluster node ID validation failed\n";
        return 1;
    }

    ClusterManager disabled;
    std::string error;

    if (
        !disabled.initialize(
            false,
            "standalone",
            "Standalone",
            "controller",
            "",
            "",
            8080,
            "",
            5,
            20,
            error
        )
        ||
        !disabled.healthy()
    ) {
        std::cerr
            << "Disabled cluster must remain healthy\n";
        return 1;
    }

    ClusterManager controller;

    if (
        !controller.initialize(
            true,
            "controller-1",
            "Controller",
            "controller",
            "10.0.0.1",
            "",
            8080,
            "0123456789abcdef0123456789abcdef",
            5,
            20,
            error
        )
    ) {
        std::cerr
            << "Cluster controller initialization failed\n";
        return 1;
    }

    if (
        controller.acceptHeartbeat(
            "wrong-token",
            "worker-1",
            "Worker 1",
            "worker",
            "10.0.0.2",
            10.0,
            20.0,
            30.0,
            error
        )
        ||
        error != "unauthorized"
    ) {
        std::cerr
            << "Cluster token validation failed\n";
        return 1;
    }

    error.clear();

    if (
        !controller.acceptHeartbeat(
            "0123456789abcdef0123456789abcdef",
            "worker-1",
            "Worker 1",
            "worker",
            "10.0.0.2",
            10.0,
            20.0,
            30.0,
            error
        )
    ) {
        std::cerr
            << "Cluster heartbeat registration failed: "
            << error
            << '\n';
        return 1;
    }

    const auto snapshot =
        controller.snapshot();

    bool worker_found = false;

    for (const auto& node : snapshot.nodes) {
        if (
            node.id == "worker-1"
            &&
            node.online
        ) {
            worker_found = true;
        }
    }

    if (
        !worker_found
        ||
        snapshot.online_nodes < 2
    ) {
        std::cerr
            << "Cluster inventory is incomplete\n";
        return 1;
    }

    const auto placement =
        controller.selectNode("ai");

    if (
        !placement.available
        ||
        placement.node_id.empty()
    ) {
        std::cerr
            << "Cluster scheduler did not return a node\n";
        return 1;
    }

    std::cout
        << "Cluster manager test passed\n";
    return 0;
}
