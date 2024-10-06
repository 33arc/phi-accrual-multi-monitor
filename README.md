# Multi-target Phi Accrual Failure Detector with Prometheus Exporter

## Disclaimer: Proof of Concept

**This project is a proof of concept.** It is intended to demonstrate the principles of the Phi Accrual Failure Detector algorithm in a multi-target environment with Prometheus integration. While functional, it may not be production-ready and could require further refinement and testing for use in critical systems.

## Overview

This project implements a multi-target Phi Accrual Failure Detector with Prometheus metrics export functionality. It's designed to monitor multiple servers simultaneously and provide failure detection based on the Phi Accrual algorithm.

## Features

- Monitors multiple servers concurrently
- Uses the Phi Accrual algorithm for failure detection
- Exports metrics to Prometheus

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

2. Run Docker Compose to start the simulated servers:
   ```
   make docker-compose-up
   ```

3. In a new terminal, build and run the Go application:
   ```
   cd ..  # Return to the project root
   go build
   ./phi-accrual-multi-monitor
   ```

4. Wait for a few minutes to allow the system to collect samples and stabilize.

5. You can now view the Prometheus metrics at:
   ```
   http://localhost:8080/metrics
   ```

## Testing

To simulate a delay or failure in one of the servers:

```
curl -X POST -H "Content-Type: application/json" -d '{"delay":10}' http://localhost:8086/control
```

Available parameters:
- `delay`: this is used in time.sleep (in seconds)

## Monitoring

After applying changes to a server, monitor the console output of the Go application and check the Prometheus metrics endpoint to see how the phi values change.

To view current phi values for all servers:
```
curl http://localhost:8080/metrics | grep server_phi_value
```

