# Hyperledger Fabric Autoscaling on Kubernetes

This directory contains the Helm charts, KEDA ScaledJob configurations, and lifecycle orchestration scripts to enable **automatic, load-driven horizontal scaling** of Hyperledger Fabric (HLF) Peers and Orderers on Kubernetes.

---

## 1. The Core Challenge of Blockchain Autoscaling

### Traditional Stateless Scaling vs. The Stateful HLF Challenge
In standard cloud-native architectures, Horizontal Pod Autoscaling (HPA) works by adjusting the `replicas` count of a `Deployment` or `ReplicaSet`. This model assumes all pods are stateless, identical, and completely interchangeable (i.e., `Pod 1 == Pod 2 == Pod N`), allowing a standard service or load balancer to distribute requests evenly across any available replica.

With Hyperledger Fabric, scaling is far more complex. HLF networks are built around stateful, non-fungible nodes with strict peer-to-peer (P2P) communication topologies (where `Peer A != Peer B`). 

Each individual node:
- **Maintains Independent State**: Every peer keeps its own local state database (LevelDB or CouchDB) and an independent, serialized, append-only transaction ledger. Merely duplicating a peer's disk state or running multiple copies of the exact same pod does not scale consensus or endorsement capacity; instead, it violates consensus safety and ledger consistency.
- **Requires Cryptographic Identity**: Every peer must possess a distinct cryptographic identity bound to an Organization Membership Service Provider (MSP).
- **Enforces Anchoring Invariants**: Scaling must be handled as a sequential, highly secure orchestration workflow rather than a simple replica increment.

### Cryptographic Identity & Operator Invariants
Each peer and orderer in an HLF network is anchored in a hierarchical Public Key Infrastructure (PKI):

```
       [ Root CA ]
            │
     ┌──────┴──────┐
 [ Admin CA ]   [ Peer CA ]
                   │
           ┌───────┴───────┐
       [ Peer 1 ]      [ Peer 2 ]
     (cert1/tls1)    (cert2/tls2)
```

1. **Non-Fungible TLS & Sign Certificates**: Each peer is issued unique cryptographic certificates (`signcert` and `tlscert`) signed by the Organization's Certificate Authority (CA). Cloning a peer pod would replicate private keys and certificates, causing cryptographic collisions, TLS handshake failures, and immediate rejection by the gossip and ordering services.
2. **CA Registration and Enrollment**: Prior to starting, a peer must register with the CA using a unique enrollment ID and secret, then enroll to retrieve its certificate signing requests (CSR) response. This process is stateful, non-idempotent, and must occur sequentially.
3. **Channel Association**: A peer cannot process transactions or read the ledger until it is explicitly registered on, and joined to, specific channels. Channel configuration transactions contain explicit anchor peer definitions and MSP certificates, making anonymous horizontal scaling impossible.

---

## 2. The Three Architectural Pillars

To address the limitations of traditional autoscaling and solve the HLF stateful scaling challenge in production, this architecture integrates three core production pillars:

```
┌────────────────────────────────────────────────────────┐
│                   Kubernetes Cluster                   │
│                                                        │
│  ┌────────────┐   Prometheus Metrics   ┌────────────┐  │
│  │ Prometheus │ ─────────────────────> │    KEDA    │  │
│  └────────────┘                        │ Controller │  │
│         ▲                              └────────────┘  │
│         │ Resource Usage                      │        │
│         │ (CPU/Mem)                           │        │
│  ┌────────────┐                               │ Spawns │
│  │ Peer Pods  │                               ▼        │
│  └────────────┘                        ┌────────────┐  │
│                                        │ ScaledJob  │  │
│  ┌────────────────────────┐            └────────────┘  │
│  │ Distributed Lock       │                   │        │
│  │ (Kubernetes Lease API) │ <─────────────────┤        │
│  └────────────────────────┘  Acquires Mutex   │        │
│                                               ▼        │
│                                        ┌────────────┐  │
│                                        │   Bevel    │  │
│                                        │  Operator  │  │
│                                        └────────────┘  │
│                                               │        │
│                                               ▼        │
│                                        ┌────────────┐  │
│                                        │ New Peer   │  │
│                                        │  Instance  │  │
│                                        └────────────┘  │
└────────────────────────────────────────────────────────┘
```

### 1. Bevel Hyperledger Operator (Stateful Orchestration)
The Bevel Hyperledger Operator abstracts the complexity of Fabric resource management through Custom Resource Definitions (CRDs). The operator:
- Automates interaction with the Organization CA for cryptographic registration and enrollment.
- Dynamically generates the required MSP directory structures, crypto-config maps, and secrets.
- Manages the lifecycle of Peer and Orderer pods.
- Facilitates the execution of administrative commands to bind peers to specific ledger channels and deploy chaincode.

### 2. KEDA: Kubernetes Event-driven Autoscaling (Event-Driven Triggers)
Standard HPA is insufficient because it is restricted to scaling resource-level deployment parameters. It cannot trigger step-based orchestration workflows.

By leveraging **KEDA `ScaledJobs`**, we decouple the trigger from the target scaling resource. KEDA monitors Prometheus metrics scraped from running peers (e.g., CPU utilization, memory pressure, transaction throughput, or endorsement queues). When a metric exceeds a designated threshold, KEDA does not scale up the peer deployment directly; instead, it dynamically spawns a Kubernetes **Orchestrator Job**. This Job performs the multi-step ledger topology modification, using the Bevel Operator to instantiate or destroy peer instances as a single, coordinated transaction.

### 3. Kubernetes Leases: coordination.k8s.io/v1 (Distributed Mutex Coordination)
Modifying the ledger topology on the fly is highly sensitive to race conditions. If KEDA triggers multiple scale-up or scale-down jobs concurrently, several issues can occur:
- Simultaneous enrollment requests to the CA with duplicate identities.
- Concurrent updates to the connection profile ConfigMaps, resulting in corrupted configurations.
- Interleaved channel configuration transactions leading to out-of-order block heights and rejection by the ordering service.

To ensure strict serializability of scaling events, we utilize a distributed locking mechanism backed by **Kubernetes Leases (`coordination.k8s.io/v1`)**.

```yaml
apiVersion: coordination.k8s.io/v1
kind: Lease
metadata:
  name: peers.lock
  namespace: fabric-autoscaling
spec:
  holderIdentity: scaler-job-hash
  leaseDurationSeconds: 60
  acquireTime: "2026-08-01T12:00:00.000000Z"
  renewTime: "2026-08-01T12:00:05.000000Z"
```

The scale jobs must acquire the corresponding mutex lease (`peers.lock` or `orderers.lock`) before modifying resources. If the lease cannot be acquired, the scaling job retries with backoff or terminates safely, ensuring that scale-up and scale-down operations never run concurrently.

---

## 3. The Scale-Up and Scale-Down Lifecycles

The dynamic scaling of the Hyperledger Fabric network follows a highly orchestrated state machine.

### Scale-Up Lifecycle Sequence

```
[Prometheus]      [KEDA]       [Scale-Up Job]      [Lease API]     [Fabric CA]     [Bevel Operator]
     │               │                │                 │               │                  │
     │  CPU > 80%    │                │                 │               │                  │
     │──────────────>│                │                 │               │                  │
     │               │  Spawn Job     │                 │               │                  │
     │               │───────────────>│                 │               │                  │
     │               │                │  Acquire Lease  │               │                  │
     │               │                │────────────────>│               │                  │
     │               │                │  Lease Granted  │               │                  │
     │               │                │<────────────────│               │                  │
     │               │                │                                 │                  │
     │               │                │  Register/Enroll Peer Identity  │                  │
     │               │                │────────────────────────────────>│                  │
     │               │                │  Certificates Generated         │                  │
     │               │                │<────────────────────────────────│                  │
     │               │                │                                                    │
     │               │                │  Create Peer CRD (Allocate PV/PVC)                 │
     │               │                │───────────────────────────────────────────────────>│
     │               │                │                                                    │
     │               │                │  Join Peer to Channels & Install Chaincode         │
     │               │                │───────────────────────────────────────────────────>│
     │               │                │                                                    │
     │               │                │  Update Connection Profile Secrets                 │
     │               │                │───────────────────────────────────────────────────>│
     │               │                │                                                    │
     │               │                │  Release Lease  │               │                  │
     │               │                │────────────────>│               │                  │
```

1. **Trigger Phase**: Prometheus monitors average peer CPU/Memory consumption. When average CPU utilization exceeds 80% over a 5-minute sliding window, KEDA triggers a scale-up `ScaledJob`.
2. **Acquisition Phase**: The spawned orchestrator job attempts to acquire the `peers.lock` Lease. It polls the Lease API until acquired or timed out.
3. **Identity Registration Phase**: The job contacts the organization's CA, registers a new peer identity (e.g., `peer2-org1`), and performs CA enrollment to retrieve the signature and TLS credentials.
4. **Provisioning Phase**: The orchestrator job applies the Bevel Peer CRD to Kubernetes. The operator provisions a new Persistent Volume Claim (PVC), creates the storage mount, and launches the Peer pod.
5. **Ledger Topology Integration**: The peer establishes connection with gossip seed peers and the ordering service. The orchestrator job executes Fabric CLI administration commands inside the peer container to:
   - Join specified ledger channels.
   - Fetch the latest channel block to initialize the state database.
6. **Chaincode Deployment**: The orchestrator queries the list of active chaincodes deployed on the channel, installs the matching chaincode packages onto the new peer, and establishes chaincode container builders.
7. **Connection Profile (CCP) Synchronization**: The connection profile (JSON/YAML) used by client applications and gateway services is updated dynamically. The orchestrator appends the new peer's gRPC endpoint and TLS certificate to the connection profile secret.
8. **Release Phase**: The orchestrator job releases the `peers.lock` Lease, allowing subsequent topology modifications to proceed.

---

### Scale-Down Lifecycle Sequence

```
[Prometheus]      [KEDA]      [Scale-Down Job]     [Lease API]     [Bevel Operator]    [Client/Secrets]
     │               │                │                 │                 │                  │
     │  CPU < 20%    │                │                 │                 │                  │
     │──────────────>│                │                 │                 │                  │
     │               │  Spawn Job     │                 │                 │                  │
     │               │───────────────>│                 │                 │                  │
     │               │                │  Acquire Lease  │                 │                  │
     │               │                │────────────────>│               │                  │
     │               │                │  Lease Granted  │                 │                  │
     │               │                │<────────────────│                 │                  │
     │               │                │                                   │                  │
     │               │                │  Remove Peer from Connection Profiles & Secrets      │
     │               │                │─────────────────────────────────────────────────────>│
     │               │                │                                   │                  │
     │               │                │  Dismantle Channel Bindings (Stop Gossip)            │
     │               │                │──────────────────────────────────>│                  │
     │               │                │                                   │                  │
     │               │                │  Delete Peer CRD & Scale Down Chaincode Pods         │
     │               │                │──────────────────────────────────>│                  │
     │               │                │                                   │                  │
     │               │                │  Release Lease  │                 │                  │
     │               │                │────────────────>│                 │                  │
```

1. **Trigger Phase**: Prometheus monitors average peer CPU/Memory consumption. When resource consumption falls below 20% over a 15-minute sliding window, KEDA triggers a scale-down `ScaledJob`.
2. **Acquisition Phase**: The orchestrator job acquires the `peers.lock` Lease to prevent any parallel scale-up/scale-down actions.
3. **Connection Profile Pruning**: To prevent traffic black-holing, the orchestrator immediately updates the shared connection profile secret, removing the targeted peer from client connection paths.
4. **Channel Dismantling**: The orchestrator issues administrative commands to disable the peer's anchor peer status (if applicable) and notify the gossip cluster of the graceful termination.
5. **Resource Teardown**: The Bevel Peer Custom Resource (CR) is deleted. The operator gracefully terminates the peer pod, deletes the associated chaincode containers, and safely cleans up or archives the PVC/PV depending on retention policies.
6. **Identity Revocation (Optional)**: The orchestrator may issue a revocation request to the CA for the peer's certificate.
7. **Release Phase**: The lock is released by deleting or resetting the holder identity in the `peers.lock` Lease.

---

## 4. Prometheus Metrics and Scale Thresholds

The autoscaling loop is governed by precise Prometheus queries. We monitor both individual metrics and organization-wide cluster aggregates to prevent flapping.

### Scale-Up Metrics

* **CPU Core Utilization (Scale-Up)**
  *Trigger Threshold:* $> 80\%$ sustained for 5 consecutive minutes.
  *PromQL:*
  ```promql
  avg(rate(container_cpu_usage_seconds_total{container="peer",namespace="fabric"}[5m])) * 100
  ```

* **Memory Usage (Scale-Up)**
  *Trigger Threshold:* $> 85\%$ sustained for 5 consecutive minutes.
  *PromQL:*
  ```promql
  avg(container_memory_working_set_bytes{container="peer",namespace="fabric"}) / avg(kube_pod_container_resource_limits_memory_bytes{container="peer"}) * 100
  ```

### Scale-Down Metrics

* **CPU Core Utilization (Scale-Down)**
  *Trigger Threshold:* $< 20\%$ sustained for 15 consecutive minutes (longer window prevents rapid scale-up/scale-down cycles).
  *PromQL:*
  ```promql
  avg(rate(container_cpu_usage_seconds_total{container="peer",namespace="fabric"}[15m])) * 100
  ```

### HLF Performance Invariant Metrics
The system scrapes Fabric-specific Prometheus metrics exposed by the peer's operations endpoint:
- **Ledger Block Height Lag**: `hyperledger_fields_ledger_block_height` is tracked across peers to ensure scaling is paused if a newly added peer is under heavy ledger-sync strain.
- **Endorsement Latency**: `hyperledger_fields_endorsement_duration` metrics are used to scale up endorsing peers if endorsement processing exceeds 200ms per transaction.

---

## 5. Configuration and Deployment

The autoscaler is packaged as a Helm chart. System configurations, triggers, and synchronization leases are configured within the `values.yaml` file.

### Reference Configuration (`values.yaml`)

```yaml
autoscaler:
  enabled: true
  namespace: fabric-autoscaling
  pollInterval: 15 # KEDA poll interval in seconds
  cooldownPeriod: 300 # Cooldown period in seconds before scaling again

  lease:
    name: "peers.lock"
    namespace: "fabric-autoscaling"
    durationSeconds: 60
    retryBackoffMs: 2000

  prometheus:
    serverAddress: "http://prometheus-k8s.monitoring.svc.cluster.local:9090"

  scaleJobs:
    - name: fabric-peer-scale-up
      metricType: cpu
      query: |
        avg(rate(container_cpu_usage_seconds_total{container="peer",namespace="fabric"}[5m])) * 100
      threshold: 80
      direction: up
      jobSpec:
        image: "hyperledger/bevel-autoscaler-orchestrator:v1.2.0"
        backoffLimit: 3
        activeDeadlineSeconds: 600
        env:
          - name: ACTION
            value: "scale-up"
          - name: CA_URL
            value: "http://ca.org1.fabric.svc.cluster.local:7054"
          - name: LEASE_NAME
            value: "peers.lock"

    - name: fabric-peer-scale-down
      metricType: cpu
      query: |
        avg(rate(container_cpu_usage_seconds_total{container="peer",namespace="fabric"}[15m])) * 100
      threshold: 20
      direction: down
      jobSpec:
        image: "hyperledger/bevel-autoscaler-orchestrator:v1.2.0"
        backoffLimit: 1
        activeDeadlineSeconds: 400
        env:
          - name: ACTION
            value: "scale-down"
          - name: LEASE_NAME
            value: "peers.lock"
```

### Installation and Upgrades
Deploy the autoscaling orchestrator into your target Kubernetes cluster using Helm:

1. **Add the Repository and Update**:
   ```bash
   helm repo add bevel-autoscaling https://hyperledger.github.io/bevel
   helm repo update
   ```

2. **Install the Autoscaler Chart**:
   ```bash
   helm install fabric-autoscaler bevel-autoscaling/fabric-autoscaler \
     --namespace fabric-autoscaling \
     --create-namespace \
     -f values.yaml
   ```

3. **Verify KEDA ScaledJobs Deployment**:
   ```bash
   kubectl get scaledjobs -n fabric-autoscaling
   ```

4. **Upgrade System Configuration**:
   When updating metrics, thresholds, or lease timing parameter values, execute:
   ```bash
   helm upgrade fabric-autoscaler bevel-autoscaling/fabric-autoscaler \
     --namespace fabric-autoscaling \
     -f values.yaml
   ```