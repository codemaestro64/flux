# Flux: Distributed Rate Limiter

**Flux** is a carrier-grade, strongly consistent distributed rate-limiting service. It implements a replicated state machine via the *Raft Consensus Algorithm* to ensure that rate limits are enforced with high precision across a distributed cluster, surviving node failures without losing state.

## Features

* **Strong Consistency:** Transactions are only committed once a quorum of nodes acknowledges the log entry.
* **Dynamic Reconfiguration:** Modify `Rate` (TPS) and `Burst` limits globally via gRPC without service interruption.
* **Persistent Storage:** Uses *BoltDB* for stable log storage and state snapshots.
* **High Performance:** Optimized bidirectional gRPC streaming for low-latency, high-throughput batch operations.
* **Observability:** Native Prometheus integration tracking request counts, latency histograms, and Raft leadership status.
* **Leader Redirection:** Intelligent error handling that informs clients of the current cluster leader.

---

## Architecture

Flux uses a *Token Bucket* algorithm where the state (token count, last refill time) is synchronized via Raft logs.

1. **Client** sends an `Allow` request to any node.
2. If the node is a **Follower**, it returns the **Leader's** address.
3. **Leader** proposes a log entry containing the request timestamp.
4. Once committed, the **FSM** (Finite State Machine) updates the local token bucket.
5. The result is returned to the client.

---

##  Getting Started

### Prerqequisites
* Go 1.23+
+* Docker & Docker Compose
* [buf](https://buf.build/docs/installation/)

### Local Development Setup
1. **Clone the repository:**
   ```bash
   git clone https://github.com/codemaestro64/flux.git
   cd flux
   ```
2. **Generate Protobuf code (using buf):**
   ```bash
   buf generate
   ````
3. **Run the cluster:**
   ```bash
   make docker-up
   ```

---

## API Documentation

### Set a Limit
**Method:** `SetLimit`
Updates the rate limit for a specific key.
```bash
grpcurl -plaintext -d '{
  "key": "tenant_1",
  "rate": 100.0,
  "burst": 200
}' localhost:50052 flux.RateLimiter/SetLimit
````

### Check Allowance
**Method:** `Allow`
Consumes tokens for a specific key.
```bash
grpcurl -plaintext -d '{
  "key": "tenant_1",
  "tokens": 1
}' localhost:50052 flux.RateLimiter/Allow
````

---

## Development & Testing

### Makefile Commands
* `make proto`: Runs `buf generate` to build gRPC stubs.
* `make test`: Runs unit and integration tests with race detection.
* `make lint`: Validates code quality using golangci-lint.
* `make build`: Compiles the server binary into `bin/flux`.

### Monitoring
Metrics are exposed at http://<node-ip>:9090/metrics.
* `ratelimit_allow_requests_total`: Request counts (allowed/denied).
* `raft_is_leader`: Current leadership status.

---

## Production Readiness

### Quorum & Networking
* Always deploy an odd number of nodes (3, 5, or 7) to avoid split-brain.
* Nodes must have static network identities (static IPs or DNS). Raft peers are identified by address in stable storage.

### Persistence
* Raft I/O is bound by fsync latency. Use SSDs for BoltDB logs. High disk wait times will degrade cluster stability.

### Bootstrapping
1. Initialize Bootstrap node with `--bootstrap=true`.
2. Join additional nodes via the `Join` RPC against the current leader.

---

## Contributing

We welcome contributions! Please follow these steps:

1. **Fork the Project**
2. **Create your Feature Branch** (`git checkout -b feature/AmazingFeature`)
3. **Commit your Changes** (`git commit -m 'Add some AmazingFeature'`)
4. **Push to the Branch** (`git push origin feature/AmazingFeature`)
5. **Open a Pull Request**

### Code Standards
* All code must pass `make lint`.
* New features must include unit tests for the FSM logic.
* Protobuf changes must maintain backward compatibility.

---

## License
Distributed under the MIT License. See `LICENSE` for more information.
