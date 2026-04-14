# Autoscaling chart

This chart packages an opinionated Kubernetes workflow for creating a Hyperledger Fabric network and running peer scaling actions through [KEDA](https://keda.sh/).

It is not the main Simi benchmark chart. Instead, it supports a specific experimental setup where Fabric infrastructure is created and then scaled by Kubernetes jobs that react to Prometheus queries.

## What this chart does

Depending on the selected `action`, the chart renders one of two modes:

- **`create-network`**: run a one-off job that creates a Fabric network and deploys chaincode
- **`scale`**: install KEDA `ScaledJob` resources that watch Prometheus and trigger peer scale-up or scale-down scripts
- **`peers-scale-up` / `peers-scale-down`**: run an individual scaling job directly

The chart ships shell scripts under [`files/`](./files/) that do the operational work.

## Dependencies and assumptions

This chart assumes a fairly specific environment:

- a Kubernetes cluster where jobs can install additional CLI tools at runtime
- KEDA installed in the cluster
- Prometheus available and reachable from the cluster
- the [Hyperledger Bevel operator for Fabric](https://github.com/hyperledger-bevel/bevel-operator-fabric) installed, including its CRDs
- RBAC permissions sufficient to create, patch, and delete Fabric and core Kubernetes resources
- a namespace dedicated to the Fabric deployment managed by this chart

The included [`prereq.yaml`](./prereq.yaml) shows the broad permissions expected by the workflow.

## How KEDA is used

When `action=scale`, the chart creates one `ScaledJob` per entry in `.Values.scaleJobs`.

Today the default jobs are:

- `peers-scale-up`
- `peers-scale-down`

Each `ScaledJob`:

- polls Prometheus on the configured interval
- evaluates the configured PromQL query
- compares the result to the configured threshold
- launches a Kubernetes job when the trigger condition is met

The scripts use a Kubernetes `Lease` object as a lock so only one scale action runs at a time for a given target.

## How the chart uses the Hyperledger Bevel operator

The scripts rely on the Bevel operator's custom resources and the `kubectl hlf` plugin to manage Fabric objects. In particular, the workflow uses the operator to:

- create Fabric CAs
- create peers and orderer nodes
- create and update channel resources
- install and invoke chaincode
- patch Fabric custom resources during scale events

This means the chart is coupled to the operator-based way of managing Fabric. It is not a generic autoscaler for arbitrary Fabric deployments.

## High-level workflow

### 1. Create the baseline network

With `action=create-network`, the chart runs [`files/create-dln.sh`](./files/create-dln.sh), which:

- installs required CLI tools into the job container
- creates a peer org CA and initial peer
- creates an orderer org CA and initial orderer
- creates a channel
- deploys chaincode and supporting config needed for later scale operations

### 2. Enable scaling mode

With `action=scale`, the chart creates KEDA `ScaledJob` resources from `.Values.scaleJobs`.

By default, those jobs watch peer CPU utilization through Prometheus queries and trigger either:

- [`files/peer-scale-up.sh`](./files/peer-scale-up.sh)
- [`files/peer-scale-down.sh`](./files/peer-scale-down.sh)

### 3. Perform a scale action

The scale scripts then perform operator-driven updates such as:

- creating or removing a Fabric peer
- updating follower channel membership
- increasing or decreasing chaincode replica count
- refreshing the stored Fabric client configuration used by downstream clients

## Key values

The chart exposes many environment-specific values. The most important ones are:

| Value | Purpose |
| --- | --- |
| `action` | Selects whether the chart creates the network, installs KEDA scaling, or runs a specific scaling job |
| `environment.*` | Supplies the Fabric, chaincode, identity, and namespace-specific parameters consumed by the scripts |
| `image.repository` / `image.tag` | Base image used for the operational jobs |
| `scaleJobDefaults.pollingInterval` | KEDA polling interval |
| `scaleJobDefaults.maxReplicaCount` | Maximum concurrent KEDA job replicas |
| `scaleJobDefaults.serverAddress` | Prometheus endpoint used by KEDA triggers |
| `scaleJobs.<name>.query` | PromQL query used to decide when to trigger a scale job |
| `scaleJobs.<name>.threshold` | Threshold KEDA compares the Prometheus result against |
| `scaleJobs.<name>.lock` | Lease name used to serialize scale operations |
| `scaleJobs.<name>.script` | Script executed when that job fires |

The default `values.yaml` is a working example of the expected shape, but it contains environment-specific values that most users will need to replace.

## Example usage

Create the initial network:

```bash
helm install dln . \
  -n my-fabric \
  --create-namespace \
  --set action=create-network
```

Switch to scaling mode:

```bash
helm upgrade dln . \
  -n my-fabric \
  --create-namespace \
  --set action=scale
```

Run a scale job directly for troubleshooting or manual operation:

```bash
helm upgrade dln . \
  -n my-fabric \
  --set action=peers-scale-up
```

In practice you will usually also provide a custom values file so the Fabric names, image references, storage classes, and Prometheus endpoint match your cluster.

## Scope and caveats

- This chart is **experimental and environment-specific**.
- It assumes Bevel-operator-managed Fabric resources and the `kubectl hlf` plugin workflow.
- The jobs install tools dynamically, which may be too slow or too restricted for some production clusters.
- The default scale signals are CPU-based Prometheus queries; they may not match your workload or SLOs.
- The chart currently contains deployment details that are likely to require cleanup or replacement before broad public reuse.

If you only need to run Simi against an existing Fabric network, start with the main chart in [`../simi`](../simi) instead of this autoscaling chart.
