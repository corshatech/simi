# Simi

Simi is a Kubernetes-based load generation and benchmarking tool for [Hyperledger Fabric](https://www.lfdecentralizedtrust.org/projects/fabric) networks. It is designed to help operators and developers exercise Fabric chaincode at scale by launching many worker pods, running repeated chaincode operations, and collecting timing data for later analysis.

At a high level, Simi:

- runs a benchmark job made up of many worker pods
- executes a configured chaincode operation on a schedule
- coordinates workers with a consumer process and RabbitMQ
- records operation duration data in InfluxDB
- writes benchmark output under `./out/` when the run completes

Simi is best thought of as a framework for repeatable Fabric workload generation rather than a general-purpose blockchain performance suite.

## Major capabilities

- **Concurrent workload generation** across many Kubernetes worker pods
- **Configurable operation cadence** using per-worker operation period and operations-per-stream settings
- **Metrics collection** for operation duration using InfluxDB
- **Custom chaincode support** through a pluggable worker configuration model
- **Helm-based deployment** for the benchmark job and supporting services
- **Optional autoscaling experiments** for Fabric peers and chaincode replicas through the `k8s/helm/autoscaling` chart

## How Simi works

The architecture is documented in [ARCHITECTURE.md](./ARCHITECTURE.md). In practice, a typical run looks like this:

1. Deploy the Simi benchmark stack into a dedicated Kubernetes namespace.
2. Launch worker pods that initialize, register, and execute a configured chaincode operation.
3. Coordinate benchmark execution through RabbitMQ and a consumer job.
4. Send operation timing data to InfluxDB during the run.
5. Export benchmark results into `./out/` when the job finishes.

The default example operation is `pingChaincode`, documented in [ping/README.md](./ping/README.md).

## Repository layout

```text
.
├── cmd/                    # Simi entrypoint
├── config/                 # Shared configuration types and loading
├── benchmark/              # Benchmark worker/consumer logic and output types
├── ping/                   # Example ping chaincode worker implementation
├── k8s/helm/simi/          # Main Helm chart for running Simi
├── k8s/helm/autoscaling/   # Experimental chart for Fabric autoscaling workflows
├── k8s/helmfiles/          # Helmfile-based deployment wiring
├── bin/                    # Container build assets
└── out/                    # Benchmark output directory (created during runs)
```

## Getting started

> **Note**
> This repository does not publish ready-to-use container images or Helm charts. You should expect to build and publish images for your own environment.

### Prerequisites

Simi assumes you already have:

- a working Kubernetes cluster
- access to a Hyperledger Fabric network you want to exercise
- a Fabric SDK configuration file for that network
- Helm and Helmfile available in your operator environment
- a place to publish any images your deployment will use

### High-level setup

1. **Build the binaries and container images**

   ```bash
   make simi
   make simi-image
   ```

2. **Prepare the Fabric connection config**

   Replace [`k8s/helm/simi/fabric-config.yaml`](./k8s/helm/simi/fabric-config.yaml) with the Fabric SDK `config.yaml` for your target network.

3. **Point Simi at the chart you want to deploy**

   Use the local chart directory or package/publish your own chart. To use the local chart with the launch script, set:

   ```bash
   export SIMI_HELM_CHART=/absolute/path/to/k8s/helm/simi
   ```

4. **Launch a benchmark**

   The main entrypoint is [`benchmark/launch.sh`](./benchmark/launch.sh). It can prompt for inputs interactively, or you can provide environment variables up front:

   ```bash
   export TARGET_NS=simi-test
   export SIMI_OPERATION_PERIOD=1s
   export SIMI_NUM_WORKERS=500
   export SIMI_OPERATIONS_PER_STREAM=100
   export SIMI_OPERATION_TYPE=ping
   ./benchmark/launch.sh
   ```

   This configuration creates 500 workers and asks each worker to run 100 `ping` operations spaced 1 second apart.

5. **Collect results**

   When the benchmark finishes, results are written under `./out/`.

For Helm chart details, see [k8s/helm/simi/README.md](./k8s/helm/simi/README.md).

## Custom chaincode support

Simi is intentionally structured so that the benchmarked chaincode operation can be swapped out.

### 1. Implement a custom worker configuration

Define the logic needed for your operation using the [`WorkerConfig`](./config/config.go) model. That type encapsulates:

- the chaincode operation to run
- worker initialization logic
- worker shutdown logic
- operation-specific configuration
- benchmark-specific configuration

The default repository wiring uses `ping.SetupPingSimi` from [`ping/`](./ping/), which is a minimal example of how to plug in a concrete operation.

### 2. Build a Simi image that includes your implementation

After wiring your custom worker into the application, rebuild the image:

```bash
make simi
make simi-image
```

### 3. Pass custom operation config through Helm values

The Simi Helm chart expects operation-specific configuration under `.Values.operationConfig`. That object is templated into the generated `simi.yaml` consumed by the benchmark job.

In other words:

- Go code defines how your operation runs
- Helm values provide the runtime configuration for that operation

## How autoscaling fits in

The main Simi flow benchmarks an existing Fabric network. The separate [`k8s/helm/autoscaling`](./k8s/helm/autoscaling) chart is related but distinct:

- it is used to create and operate a specific Fabric deployment model on Kubernetes
- it wires KEDA-based scaling jobs to peer scaling scripts
- it uses the Hyperledger Bevel operator and `kubectl hlf` workflows to create Fabric resources
- it is aimed at autoscaling experiments, not at every Simi deployment

If you are only trying to run Simi against an existing Fabric network, you may not need the autoscaling chart at all.

## Project scope and expectations

Simi is useful if you want a repo you can adapt for:

- Fabric chaincode load generation
- repeatable benchmark runs in Kubernetes
- custom operation benchmarking tied to your own chaincode
- exploratory infrastructure experiments around peer and chaincode scaling

Simi is **not** currently positioned as:

- a turnkey managed service
- a drop-in benchmark harness for every Fabric topology
- a polished end-user product with hosted images, hosted charts, or one-click deployment

Expect some environment-specific setup, especially around Fabric configuration, container publication, Kubernetes access, and any autoscaling workflows.
