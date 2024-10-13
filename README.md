# Multi-target Phi Accrual Failure Detector with Prometheus Exporter

## Disclaimer: Proof of Concept

**This project is a proof of concept.** It is intended to demonstrate the principles of the Phi Accrual Failure Detector algorithm in a multi-target environment with Prometheus integration. While functional, it may not be production-ready and could require further refinement and testing for use in critical systems.

## Overview

This project implements a multi-target Phi Accrual Failure Detector with Prometheus metrics export functionality. It's designed to monitor multiple servers simultaneously and provide failure detection based on the Phi Accrual algorithm.

## Features

- Monitors multiple servers concurrently
- Uses the Phi Accrual algorithm for failure detection
- Exports metrics to Prometheus
- Uses Raft consensus algorithm to ensure consistent config file replication

## Prerequisites

- Docker and Docker Compose
- Go 1.15 or later
- curl (for testing)

## Setup and Running

1. Clone this repository:
   ```
   git clone <repository-url>
   cd <repository-directory>
   ```

2. Run Docker Compose to start the simulated servers and monitors:
   ```
   make run
   ```

3. Wait for a few minutes to allow the system to collect samples and stabilize.

4. You can now view the metrics for all monitors:
   ```
   make get_metrics
   ```
   This will display metrics for server1 from all three monitors (ports 9000, 9001, 9002).

5. You can also add additional servers at runtime:
   ```
   curl -X PUT http://localhost:8002/config --json '{"id":10,"url":"http://10.1.0.110:8080/heartbeat","monitor":{"threshold":16.0,"maxSampleSize":200,"minStdDeviationMillis":500,"acceptableHeartbeatPauseMillis":0,"firstHeartbeatEstimateMillis":500}}'
   ```

## Testing

To simulate a delay or failure in one of the servers:

```
curl -X POST -H "Content-Type: application/json" -d '{"delay":10}' http://localhost:8086/control
```

Available parameters:
- `delay`: this is used in time.sleep (in seconds)

## Monitoring

After applying changes to a server, use the get_metrics command to see how the phi values change:
```
make get_metrics
```

To view metrics for a different server:

```
make get_metrics SERVER=server2
```

## Additional Commands

To stop the Docker environment:
```
make docker-compose-down
```

For a list of all available commands:
```
make help
```

## Prometheus Metrics

If you've set up Prometheus metrics, you can access them at:
```
http://localhost:<monitor-port>/metrics
```

Replace <monitor-port> with 8000, 8001, or 8002 for the respective monitor.

## Performance Analysis with Flamegraph
Linux perf_event visualized with Brendan Gregg's FlameGraph
   ```
   perf record -F 50 -g -D 30 -p $(pidof phi)
   ```
![performance](https://raw.githubusercontent.com/33arc/phi-accrual-multi-monitor/refs/heads/main/images/perf_flamegraph.svg)

