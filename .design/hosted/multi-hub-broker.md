# Multi-Hub Broker Design

**Status:** Revised (second feedback round incorporated 2026-02-23)
**Created:** 2026-02-22
**Author:** Design Agent
**Related:** [multi-broker.md](multi-broker.md), [hosted-architecture.md](hosted-architecture.md), [runtime-broker-api.md](runtime-broker-api.md)

## 1. Problem Statement

Currently, a Runtime Broker is coupled 1:1 with a single Hub. This manifests in several places:

- **Credential storage** (`~/.scion/broker-credentials.json`) holds a single `brokerId`, `secretKey`, and `hubEndpoint`.
- **Server state** (`runtimebroker.Server`) has a single `hubClient`, single `heartbeat`, single `controlChannel`.
- **Registration flow** (`scion broker register`) writes one set of credentials and saves one `hub.brokerId` to global settings.
- **Co-located mode** (`scion server start --enable-hub --enable-runtime-broker`) auto-registers the broker with the embedded hub using in-memory credentials.

A broker, as a concept, is a **host machine** that should be able to participate with **multiple hub endpoints** simultaneously. Use cases:

1. **Dev + Production**: A developer's laptop runs the local dev-server (hub+broker+web combo) while also connecting to a remote production/team hub to provide compute for shared projects.
2. **Multi-team**: A shared build machine participates in multiple team hubs.
3. **Migration**: Gradually moving agents from one hub to another without downtime.

### Scope

- Support connecting a single broker process to multiple hub endpoints.
- Support **any number of hubs in authenticated (HMAC) mode** and at most **one hub in dev-auth mode**.
- Per-hub credential/secret storage.
- The co-located dev-server combo mode should work as one of the hub connections.
- Out of scope: federated grove identity across hubs, cross-hub agent migration, multi-hub CLI commands.

---

## 2. Current Architecture (Baseline)

### 2.1 Credential Storage

```
~/.scion/broker-credentials.json
{
  "brokerId": "uuid",
  "secretKey": "base64-encoded-256-bit-key",
  "hubEndpoint": "https://hub.example.com",
  "registeredAt": "2026-..."
}
```

Single flat file. The `Store` type (`pkg/brokercredentials/store.go`) provides `Load()`, `Save()`, `LoadIfChanged()` with file-level mutex locking.

### 2.2 Server Singleton State

`runtimebroker.Server` holds:
- One `hubClient hubclient.Client`
- One `heartbeat *HeartbeatService`
- One `controlChannel *ControlChannelClient`
- One `brokerCredentials *brokercredentials.BrokerCredentials`
- One `credentialsStore *brokercredentials.Store`
- One credential watcher goroutine

All of these assume a single hub.

### 2.3 Registration Flow

`scion broker register`:
1. Resolves hub endpoint from settings/env
2. Creates broker on hub (`POST /api/v1/brokers`)
3. Joins with token (`POST /api/v1/brokers/join`)
4. Saves credentials to `~/.scion/broker-credentials.json`
5. Saves `hub.brokerId` and `hub.endpoint` to global settings

### 2.4 Server Startup (Remote Broker)

`scion broker start` / `scion server start --enable-runtime-broker`:
1. Loads broker credentials from file (or in-memory for co-located)
2. Creates one `hubclient.Client` with HMAC auth
3. Starts one `HeartbeatService`
4. Starts one `ControlChannelClient` (WebSocket)
5. Starts one credential watcher (polls every 10s)

### 2.5 Co-located Mode

When hub and broker run in the same process (`--enable-hub --enable-runtime-broker`):
- Generates in-memory HMAC credentials
- Registers broker record directly in the database
- Uses `InMemoryCredentials` (bypasses file)
- Uses internal heartbeat loop (direct DB write) instead of HTTP heartbeat
- Control channel still established for PTY proxying

---

## 3. Proposed Design

### 3.1 Core Concept: Hub Connections

Introduce a **HubConnection** abstraction that encapsulates everything needed for a broker to participate with a single hub:

```go
// HubConnection represents a broker's connection to a single hub.
type HubConnection struct {
    // Identity
    Name        string   // User-assigned alias (e.g., "dev", "prod", "team-alpha")
    HubEndpoint string
    BrokerID    string

    // Authentication
    Credentials *brokercredentials.BrokerCredentials
    AuthMode    AuthMode // hmac, dev-auth, bearer

    // Runtime services
    HubClient      hubclient.Client
    Heartbeat      *HeartbeatService
    ControlChannel *ControlChannelClient

    // State
    Status         ConnectionStatus // connected, disconnected, error
    LastHeartbeat  time.Time
}

type AuthMode string
const (
    AuthModeHMAC    AuthMode = "hmac"
    AuthModeDevAuth AuthMode = "dev-auth"
    AuthModeBearer  AuthMode = "bearer"
)
```

The broker server manages a **map of hub connections** instead of a single set of hub-related fields.

### 3.2 Multi-Hub Credential Storage

Replace the single-file credential store with a directory-based or multi-entry store.

#### Option A: Directory-Based Store (Recommended)

```
~/.scion/hub-credentials/
  dev.json          # Local dev hub
  prod.json         # Production hub
  team-alpha.json   # Team hub
```

Each file contains the existing `BrokerCredentials` structure, extended with a `name` field:

```json
{
  "name": "prod",
  "brokerId": "uuid",
  "secretKey": "base64-...",
  "hubEndpoint": "https://hub.scion.dev",
  "authMode": "hmac",
  "registeredAt": "2026-..."
}
```

Advantages:
- Simple file-per-hub model
- Easy to add/remove hubs without parsing a shared file
- Natural mapping to `scion broker register --hub-name prod`
- Each file can have independent permissions
- Backward compatible: migration reads old single file and moves it

#### Option B: Single Multi-Entry File

```json
{
  "connections": [
    {
      "name": "prod",
      "brokerId": "uuid",
      "secretKey": "base64-...",
      "hubEndpoint": "https://hub.scion.dev",
      "authMode": "hmac"
    },
    {
      "name": "dev",
      "brokerId": "uuid2",
      "secretKey": "base64-...",
      "hubEndpoint": "http://localhost:8080",
      "authMode": "dev-auth"
    }
  ]
}
```

Disadvantages:
- Concurrent writes from multiple processes risk corruption
- Harder to manage individual hub credentials independently
- All-or-nothing file permissions

#### Option C: Per-Hub Subdirectories

```
~/.scion/hubs/
  prod/
    credentials.json
    cache/          # Hub-specific template cache
  dev/
    credentials.json
    cache/
```

Advantages:
- Clean separation of all per-hub state (credentials, caches)
- Template cache isolation per hub

Disadvantages:
- More directory structure to manage
- Overkill if template caches don't actually need separation

**Decision:** Option A (directory-based flat files) for credentials. Template caches can be shared since they're content-addressed. Option C (per-hub subdirectories) should be revisited as a future enhancement if per-hub cache isolation or other per-hub state becomes necessary.
### 3.3 Credential Store API Changes

```go
// MultiStore manages credentials for multiple hub connections.
type MultiStore struct {
    dir string
    mu  sync.RWMutex
}

func NewMultiStore(dir string) *MultiStore

// Core operations
func (s *MultiStore) List() ([]BrokerCredentials, error)
func (s *MultiStore) Load(name string) (*BrokerCredentials, error)
func (s *MultiStore) Save(creds *BrokerCredentials) error  // uses creds.Name as filename
func (s *MultiStore) Delete(name string) error
func (s *MultiStore) Exists(name string) bool

// Change detection for credential watcher
func (s *MultiStore) LoadAllIfChanged(lastScan time.Time) ([]BrokerCredentials, time.Time, error)

// Migration from legacy single-file store
func (s *MultiStore) MigrateFromLegacy(legacyPath string) error
```

### 3.4 Server Architecture Changes

The `runtimebroker.Server` struct evolves to hold multiple hub connections:

```go
type Server struct {
    // ... existing fields (manager, runtime, httpServer, mux, etc.)

    // Hub connections (replaces single hubClient, heartbeat, controlChannel, etc.)
    hubConnections map[string]*HubConnection  // keyed by connection name
    hubMu          sync.RWMutex

    // Credential watching (now watches a directory)
    credentialsStore *brokercredentials.MultiStore
    credWatcherStop  chan struct{}

    // Template hydration remains shared (caches are content-addressed)
    cache    *templatecache.Cache
    // Note: hydrator is per-request, using the hub client from the originating connection
}
```

#### Hub Client Selection for Templates

Template hydration needs a hub client. With multiple hubs, the broker must route template requests to the correct hub:

- **Originating hub**: Agent creation requests arrive via a specific hub's control channel. The template should be fetched from **that same hub**. This follows the principle that broker-hub relationships are isolated from each other.
- **Implementation**: The control channel handler identifies which `HubConnection` the request came from and uses that connection's client for template hydration. This requires threading the hub connection context through the agent creation path.

#### Broker Auth Middleware

Incoming requests from hubs need authentication. With multiple hubs, the middleware must:
- Accept signed requests from **any** connected hub's secret key
- The broker uses a single `BrokerID` across all hubs (see Resolved Questions), so the `X-Scion-Broker-ID` header is the same for all connections
- Maintain a map of `hubEndpoint -> secretKey` for validation, trying each key until one matches

```go
type MultiBrokerAuthMiddleware struct {
    keys map[string][]byte  // hubEndpoint -> secretKey
    mu   sync.RWMutex
}
```

### 3.5 Registration Flow Changes

```
# Register with a specific hub, assigning an alias
scion broker register --hub https://hub.scion.dev --name prod

# Register with local dev hub
scion broker register --hub http://localhost:8080 --name dev

# Register with auto-detected hub from settings (backward compatible)
scion broker register
# -> uses hub.endpoint from settings, assigns name based on hostname/endpoint

# List hub connections
scion broker hubs

# Remove a hub connection
scion broker deregister --name prod
```

The `--name` flag provides a human-friendly alias for the hub connection. If omitted, one is derived from the endpoint hostname, slugified (e.g., `hub-scion-dev`, `localhost`).

The broker uses the **same UUID** across all hub registrations. The UUID is stored in `~/.scion/settings.yaml` at `server.broker.broker_id`. On first registration, a UUID is generated locally by the broker and persisted to settings. Subsequent registrations to other hubs read and present this same UUID.

### 3.6 Settings Changes

The global settings (`~/.scion/settings.yaml`) currently stores `server.broker.broker_id` (the broker's UUID) and `hub.endpoint`. This should evolve:

```yaml
# Broker identity (unchanged — single UUID for this machine)
server:
  broker:
    broker_id: uuid-1

# Legacy (still supported, treated as default/unnamed connection)
hub:
  endpoint: https://hub.scion.dev

# New: explicit multi-hub configuration
hub_connections:
  prod:
    endpoint: https://hub.scion.dev
  dev:
    endpoint: http://localhost:8080
```

The `server.broker.broker_id` is the canonical broker UUID and is shared across all connections (not per-connection). The legacy `hub` section continues to work and is treated as an unnamed/default connection for backward compatibility. Note that `broker_id` is no longer per-connection in settings since it is always the same value.

### 3.7 Co-located Mode Integration

When running `scion server start --enable-hub --enable-runtime-broker`:

1. The embedded hub creates a `HubConnection` named `"local"` (or `"colocated"`)
2. In-memory credentials are injected directly into this connection
3. The internal heartbeat loop writes directly to the DB (no HTTP roundtrip)
4. This connection is always present, alongside any file-based connections

If the server is also configured with external hub connections (via `hub_connections` in settings or credential files), those are established in parallel:

```
scion server start \
  --enable-hub \
  --enable-runtime-broker \
  --enable-web
```

This starts:
- The local hub + web frontend
- A broker connection to the local hub (in-memory, always present)
- Additional broker connections to any hubs configured in `~/.scion/hub-credentials/`

### 3.8 Heartbeat Fan-Out

Each `HubConnection` runs its own independent `HeartbeatService`. Heartbeat payloads are **filtered per hub**: each hub only receives information about agents belonging to groves registered on that specific hub. A broker's participation with one hub is completely isolated from its participation with other hubs — this is a strict security requirement.

Since groves are currently 1:1 with hubs (a grove is registered on exactly one hub), the broker-hub-grove combination is unique, making the filtering straightforward: the broker tracks which groves belong to which hub connection and includes only the relevant agents in each heartbeat.

**Implementation note:** The grove-to-hub mapping is available from each grove's `.scion/settings.yaml` under the `hub` setting, where the endpoint uniquely identifies the hub. The `HeartbeatService` reads this mapping at startup and uses it to filter agent reports per hub. No additional persistence or control-channel inference is needed — the grove settings are the authoritative source.

### 3.9 Control Channel Fan-Out

Each `HubConnection` runs its own `ControlChannelClient` WebSocket. When a hub sends a command over the control channel:

1. The request arrives on the connection's WebSocket
2. It is dispatched to the broker's local HTTP handler (same as today)
3. The response flows back over the same WebSocket

This works naturally because each hub maintains its own WebSocket and routes independently.

### 3.10 Agent Namespace Isolation

A key concern with multi-hub: **agent name collisions**. Two hubs might try to create agents with the same name. The current container naming scheme is `scion-<grove-slug>-<agent-name>`.

Since grove slugs are derived from git remotes (which are globally unique URLs), and each hub manages distinct groves, collisions are unlikely. However, the `global` grove exists on every hub.


**Mitigation options considered:**
1. **Hub-scoped container prefix**: `scion-<hub-alias>-<grove-slug>-<agent-name>` - breaks existing containers
2. **Conflict detection**: Before creating a container, check if the name is already in use. If so, append a hub-specific suffix.
3. **Accept the risk**: Global grove collisions are the only realistic concern, and they're unlikely in practice since the global grove is typically used for one-off local agents.
4. **Disable global grove on multi-hub brokers**: When a broker is connected to more than one hub, reject agent creation requests targeting the `global` grove. This eliminates the only realistic collision vector without changing container naming.

**Decision:** Option 4 — disable global grove when the broker is connected to multiple hubs. When an agent dispatch targets the `global` grove and the broker has more than one active hub connection, the broker returns an error to the requesting hub. The broker does **not** auto-withdraw from the hub's global grove, since the provider registration should remain in place for when the broker returns to single-hub mode. See Resolved Question 7.11 for details and the related open question about provider-level enabled/disabled flags.

---

## 4. Implementation Phases

### Phase 1: Multi-Credential Store

- Create `brokercredentials.MultiStore` (directory-based)
- Reuse existing `server.broker.broker_id` from settings as the stable UUID across all registrations
- Add migration logic from legacy single-file format
- Update `scion broker register` to accept `--name` flag and present stable broker UUID
- Update `scion broker deregister` to accept `--name` flag
- Add `scion broker hubs` list command
- Update `scion broker status` to show all connections
- Enforce one dev-auth hub limit in `MultiStore.Save()`
- Tests for `MultiStore`

### Phase 2: Server Multi-Hub Support

- Introduce `HubConnection` struct
- Refactor `Server` to hold `map[string]*HubConnection`
- Update `initHubIntegration()` to create connections from all credentials
- Update credential watcher to scan directory for changes (add/remove/modify)
- Update `reinitializeHubServices()` to handle per-connection reload
- Multi-key `BrokerAuthMiddleware` (keyed by hub endpoint, single broker UUID)
- Implement per-hub heartbeat filtering (grove-to-hub mapping from grove `.scion/settings.yaml`)
- Thread hub connection context through agent creation path for template routing
- Reject global grove agent dispatch when multiple hub connections are active
- Tests for multi-connection lifecycle

### Phase 3: Co-located + Remote Combo

- Update co-located mode to create a `"local"` `HubConnection`
- Enable additional file-based connections alongside co-located
- Handle startup ordering (local hub must be ready before broker connects)
- Update `scion server start` to log all hub connections
- Integration tests

### Phase 4: CLI Polish and Settings

- `hub_connections` section in settings.yaml
- `scion broker hubs` with status display (connected/disconnected per hub)
- `scion broker provide --hub <name>` to scope provide/withdraw to a specific hub
- Documentation updates

---

## 5. Alternative Approaches Considered

### 5.1 Multiple Broker Processes

**Approach:** Run one broker process per hub connection.

**Pros:**
- No code changes to broker internals
- Complete process isolation

**Cons:**
- Multiple processes competing for the same container runtime
- Port conflicts (each needs a different API port)
- Container naming collisions (both brokers try to manage the same containers)
- Operational burden: multiple daemons to manage
- Cannot share the same agent manager view

**Verdict:** Rejected. The fundamental issue is that containers are a machine-level resource and a single manager should own them.

### 5.2 Hub Proxy / Federation

**Approach:** Run a local "hub proxy" that federates requests from multiple upstream hubs into a single broker connection.

**Pros:**
- Broker remains 1:1 with a single "hub" (the proxy)
- Could handle complex routing logic in the proxy

**Cons:**
- Introduces a new component to build and maintain
- Adds latency to every request
- Complex state management (proxy needs to track which upstream hub owns which grove/agent)
- Authentication becomes more complex (proxy must speak both upstream and downstream auth)

**Verdict:** Over-engineered for the use case. The direct multi-connection approach is simpler.

### 5.3 Hub-Side Multi-Broker Identity

**Approach:** A single broker identity on the hub side represents a physical host that connects to multiple hubs. The hub manages the "this broker also participates in hub X" relationship.

**Pros:**
- Centralizes the complexity on the hub side

**Cons:**
- Hubs are independent entities with no knowledge of each other
- Requires inter-hub communication (federation), which is a much larger feature
- Doesn't work for the local dev-server case

**Verdict:** Rejected. Hubs should remain independent.

### 5.4 Broker as a Sidecar per Hub

**Approach:** Instead of one broker process, run the broker as a lightweight sidecar for each hub connection, sharing the container runtime through a socket or API.

**Pros:**
- Clean separation per hub

**Cons:**
- Requires a container runtime API abstraction layer
- Still has naming collision issues
- Adds operational complexity

**Verdict:** Rejected. Too much indirection.

---

## 6. Auth Constraints and Security Considerations

### 6.1 One Dev-Auth Hub Limit

The broker supports at most **one hub in dev-auth mode** and **any number of hubs in authenticated (HMAC) mode**. The limitation on dev-auth is that managing multiple dev-auth tokens (which lack the registration handshake of HMAC) adds complexity without clear value.

Multiple HMAC connections are fully supported and encouraged:
- Each hub connection maintains its own independent HMAC credentials via the registration handshake
- Per-hub credential isolation (separate files, separate keys) ensures security
- This is also more secure than dev-auth, since each connection has a proper key exchange

The dev-auth mode is designed for local development only:
- At most one dev-auth hub connection is permitted
- The broker enforces this at registration time
- Dev-auth connections are inherently local/trusted

**Implementation:** The `MultiStore.Save()` method checks if a `dev-auth` mode credential already exists when saving a new `dev-auth` credential. If so, it returns an error directing the user to deregister the existing one first.

### 6.2 Per-Hub Secret Isolation

Each hub connection's credentials are stored in a separate file with `0600` permissions. The HMAC secret for hub A is never sent to hub B.

### 6.3 Credential Rotation

Secret rotation continues to work per-connection. When a hub rotates the broker's secret, the broker's credential watcher detects the file change and reinitializes only that connection.

---

## 7. Resolved Questions

The following questions were raised during design review and have been resolved.

### 7.1 Broker Identity: Same UUID Across Hubs

**Decision:** Use the **same `BrokerID` (UUID)** across all hubs. The UUID is the true identifier of the broker as a physical host. Hostnames are secondary and more prone to collisions than UUIDs. Each `scion broker register` presents the broker's existing UUID to the target hub during registration. If the broker has no UUID yet (first registration), one is generated locally and persisted for reuse with subsequent hub registrations.

**Implementation note:** The broker UUID is stored in `~/.scion/settings.yaml` at `server.broker.broker_id` (its current location). This is the canonical source. The per-connection credential files also store the broker ID for consistency, but it is the same value across all files.

### 7.2 Agent Visibility: Filtered by Hub

**Decision:** Heartbeats are **filtered by hub**. Each hub only sees agents belonging to groves registered on that hub. A broker's participation with one hub is completely isolated from its participation with other hubs — this is a strict security requirement. See Section 3.8 for implementation details.

### 7.3 Template Cache: Shared (With Future Revision Note)

**Decision:** Shared cache (current behavior, unchanged). Templates are stored by content hash (SHA256), so two hubs using the same template share the cache entry safely. Cache eviction is LRU-based and hub-neutral.

**Future revision:** If per-hub cache isolation becomes necessary (e.g., due to metadata conflicts or eviction fairness concerns), revisit Option C (per-hub subdirectories) from Section 3.2.

### 7.4 Template Requests: Route to Originating Hub

**Decision:** The control channel handler identifies which `HubConnection` the request came from and uses that connection's client for template hydration. This follows the principle that broker-hub relationships are isolated from each other. See Section 3.4.

### 7.5 Both Standalone and Combo Mode Support Multi-Hub

**Decision:** Both `scion broker start` (standalone) and `scion server start` (combo) support multi-hub. The code path is the same; the only difference is whether the `"local"` connection (co-located hub) exists. The co-located connection is always named `local`.

### 7.6 CLI Hub Selection: Default to Current Grove's Hub

**Decision:** Add `--hub <name>` flag to `provide`, `withdraw`, and `status` commands. Without the flag, the default is the **hub of the current grove** (matching current behavior where the grove's hub is resolved during the provide flow). This is more intuitive than defaulting to a "primary" hub.

### 7.7 Startup Ordering: Resolved

**Decision:** No timing issue exists. The co-located mode uses in-memory credentials and internal DB writes, bypassing the HTTP path entirely. Remote connections use the existing reconnect backoff. No changes needed.

### 7.8 Hub Alias Naming: Slugified Hostname

**Decision:** Use the hostname portion of the endpoint URL, slugified. Examples: `https://hub.scion.dev` -> `hub-scion-dev`, `http://localhost:8080` -> `localhost`. The co-located connection is always named `local`.

### 7.9 Broker UUID Persistence and Registration Flow

**Decision:** The broker UUID is stored in its current location: `~/.scion/settings.yaml` at `server.broker.broker_id`. This is the canonical source of the broker's identity.

- **UUID generation:** On first registration, the broker generates the UUID locally before contacting the hub. Since it is a UUID, the generation location does not matter.
- **UUID conflicts on the hub:** If the hub already has a broker record with the same UUID (e.g., from a previous registration that was deregistered), this is a broken state — deregistration should have removed the hub-side record. The hub returns an error with a suggestion to manually delete the stale broker record on the hub and re-register.
- **Subsequent registrations:** After the first registration, the broker reads its existing UUID from settings and presents it to each new hub during registration.

### 7.10 Grove-to-Hub Mapping for Heartbeat Filtering

**Decision:** The grove-to-hub mapping is read from each grove's `.scion/settings.yaml` under the `hub` setting. The hub endpoint stored there uniquely identifies which hub the grove belongs to. This is already persisted on disk and survives broker restarts. No additional mapping mechanism is needed — no control-channel inference, no separate persistence layer.

**Implementation:** At startup and when groves are added/removed, the broker scans provided groves' settings files and builds a `groveSlug -> hubEndpoint` map. Each `HeartbeatService` uses this map to include only agents from groves that match its hub connection's endpoint.

### 7.11 Global Grove Behavior on Multi-Hub Brokers

**Decision:** Disable global grove dispatch when the broker is connected to multiple hubs. When an agent dispatch request targets the `global` grove and the broker has more than one active hub connection, the broker rejects the request with an error explaining that global grove is unavailable in multi-hub mode.

**Key constraint:** The broker does **not** auto-withdraw from the hub's global grove provider registration. The registration should remain in place so that if the broker later returns to single-hub mode (e.g., a hub connection is removed), global grove dispatch resumes without re-registration.

**Consequence:** Until provider-level enabled/disabled flags are implemented (see Open Question 8.1), the hub may still attempt to dispatch global grove agents to the broker, receiving an error at dispatch time. This is acceptable for the initial implementation.

---

## 8. Open Questions

### 8.1 Provider Enabled/Disabled Flag

When the broker disables global grove dispatch in multi-hub mode (see Resolved Question 7.11), it intentionally does **not** auto-withdraw from the hub's global grove provider registration. This means the hub still considers the broker a valid provider and may dispatch agents to it, only to receive an error at dispatch time.

To avoid this late-error pattern, provider records could support an **enabled/disabled** flag:

- When the broker enters multi-hub mode, it marks its global grove provider registrations as `disabled` on each hub.
- When the broker returns to single-hub mode, it re-enables them.
- The hub skips disabled providers during dispatch, avoiding the error entirely.

**Questions:**
- Should this be a broker-initiated API call (`PATCH /api/v1/providers/:id { enabled: false }`), or a field the hub sets based on broker-reported state in the heartbeat?
- Should the enabled/disabled flag be general-purpose (usable for other scenarios like maintenance mode) or scoped specifically to the multi-hub global grove case?

**Priority:** Low — the error-on-dispatch approach is functional for the initial implementation. This is a UX improvement for Phase 4 or later.

---

## 9. Migration Path

### 9.1 Backward Compatibility

The migration must be seamless:

1. On first run of the new code, check for `~/.scion/broker-credentials.json`
2. If found, create `~/.scion/hub-credentials/` directory
3. Move the file to `~/.scion/hub-credentials/<derived-name>.json`, adding the `name` field
4. Remove the old file (or rename to `.bak`)
5. All existing code paths that reference the old file path should fall through to the new store

### 9.2 Settings Migration

The `server.broker.broker_id` in settings is unchanged (it remains the canonical broker UUID). The legacy `hub.endpoint` continues to work as a single-connection default. When `hub_connections` is present, it takes precedence. The legacy `hub` section is treated as a connection named `"default"`.

### 9.3 CLI Compatibility

All existing `scion broker` commands continue to work without `--name`. They operate on the default/primary hub connection. The `--name` flag is additive.

---

## 10. Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Container name collisions (global grove) | Agent creation fails | Global grove dispatch disabled in multi-hub mode; broker returns error without auto-withdrawing provider registration |
| Credential file corruption during concurrent writes | Hub connection fails | File-per-hub model reduces blast radius; mutex protects individual files |
| Heartbeat load multiplication (N hubs x 1 heartbeat each) | Minor network overhead | Heartbeats are small (<1KB); 30s interval is already conservative; per-hub filtering reduces payload |
| Control channel multiplexing complexity | Increased memory/goroutine usage | Each connection is independent; goroutine count scales linearly |
| Grove-to-hub mapping drift | Wrong agents reported to wrong hub | Mapping read from grove `.scion/settings.yaml` hub settings; already persisted on disk |
| Same UUID rejected by hub | Registration fails | Stale UUID after deregistration is a broken state; hub returns error with guidance to manually delete and re-register |
| Migration breaks existing setups | Broker disconnects from hub | Automatic migration with fallback; old file preserved as backup |

---

## 11. Summary

The recommended approach is to introduce a `HubConnection` abstraction and evolve the broker from managing a single hub relationship to managing a collection of hub connections. The key design choices are:

1. **Directory-based credential storage** (`~/.scion/hub-credentials/<name>.json`) with Option C (per-hub subdirectories) as a future enhancement
2. **Independent services per connection** (heartbeat, control channel, hub client)
3. **Same broker UUID across all hubs** (stored at `server.broker.broker_id` in settings; one machine = one identity)
4. **Co-located mode as a special `"local"` connection** alongside file-based connections
5. **Hub-filtered heartbeats** (grove-to-hub mapping from grove settings; each hub only sees its own groves)
6. **One dev-auth hub limit** enforced at registration time; multiple HMAC hubs fully supported
7. **Template requests routed to originating hub** (broker-hub relationships are isolated)
8. **Global grove disabled in multi-hub mode** (dispatch rejected; provider registration preserved for single-hub fallback)
9. **CLI defaults to current grove's hub** when `--hub` flag is omitted
10. **Hub aliases derived from slugified hostname** (e.g., `hub-scion-dev`); co-located is always `local`
11. **Phased implementation** starting with the credential store, then server refactoring, then combo mode, then CLI polish
