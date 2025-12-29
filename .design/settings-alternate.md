# Alternative Settings Architectures

This document explores three alternative approaches to configuration to address the complexity of intersecting concerns (Runtime x Provider x Feature Flags).

## Option 1: The "Flat Registry" Model (Relational)

In this model, we treat `Runtimes`, `Providers`, and `Profiles` as top-level, independent entities. A `Profile` acts as the "glue" that binds a specific Runtime to specific Provider overrides.

**Concept**: "Normalize" the data. Don't nest Runtimes inside Environments or Providers inside Runtimes. Reference them by name.

### JSON Schema Draft

```json
{
  "active_profile": "local-dev",
  
  "runtimes": {
    "docker-local": { "type": "docker", "host": "..." },
    "k8s-prod": { "type": "kubernetes", "context": "..." }
  },

  "providers": {
    "gemini": { "image": "gemini-cli:base", "user": "root" },
    "claude": { "image": "claude-code:base", "user": "node" }
  },

  "profiles": {
    "local-dev": {
      "runtime": "docker-local",
      "tmux": true,
      "overrides": {
        "gemini": { "image": "gemini-cli:dev" }
      }
    },
    "k8s-prod": {
      "runtime": "k8s-prod",
      "tmux": false,
      "overrides": {
        "gemini": { "image": "gemini-cli:signed-prod" }
      }
    }
  }
}
```

### Pros/Cons
- **Pros**: Very clean separation of concerns. Easy to add a new runtime without touching provider configs. Reduces duplication.
- **Cons**: Requires "lookups" (referencing names). Slightly more verbose for simple setups.

---

## Option 2: The "Provider-Centric" Model (App-Focused)

This flips the previous design. Instead of defining the "Environment" and listing what runs in it, you define the "Agent/Provider" and list how it behaves in different environments.

**Concept**: The Provider is the primary entity. It knows how to adapt itself to different runtimes.

### JSON Schema Draft

```json
{
  "defaults": {
    "runtime": "docker"
  },

  "runtimes": {
    "docker": { ... },
    "k8s": { ... }
  },

  "providers": {
    "gemini": {
      "user": "root",
      "defaults": { "image": "gemini-cli:latest" },
      "adapters": {
        "docker": {
          "use_tmux": true,
          "mounts": ["/tmp:/tmp"]
        },
        "k8s": {
          "image": "gemini-cli:prod", // Override for K8s
          "use_tmux": false,
          "resources": { "cpu": "1" }
        }
      }
    },
    "claude": { ... }
  }
}
```

### Pros/Cons
- **Pros**: extremely clear if your mental model is "I want to configure Gemini". All Gemini logic is in one place.
- **Cons**: If you have 10 providers and change your K8s cluster details, you might have to update 10 adapters if not careful (though referencing a global runtime block helps).

---

## Option 3: The "Cascading Context" Model (Layered)

This approach mimics tools like `kubectl` or `helm`. You have a "Base" configuration, and then "Contexts" that apply patches or overlays on top of the base.

**Concept**: There is only one "Config". But you load a "Context" which mutates the state.

### JSON Schema Draft

```json
{
  "current_context": "minikube",

  "base": {
    "runtime": "docker",
    "use_tmux": true,
    "providers": {
      "gemini": { "image": "gemini:latest", "user": "root" }
    }
  },

  "contexts": {
    "minikube": {
      "runtime": "kubernetes",
      "runtime_config": { "context": "minikube" },
      "use_tmux": false,
      // Merged into base providers
      "provider_patches": {
        "gemini": { "image": "gemini:k8s-optimized" }
      }
    },
    "production": {
      "runtime": "kubernetes",
      "runtime_config": { "context": "gke-prod" },
      "readonly_filesystem": true
    }
  }
}
```

### Pros/Cons
- **Pros**: Very powerful. Reduces boilerplate (inheritance). "Context" is a familiar term.
- **Cons**: "Magic" merging logic can be confusing. Harder to debug "where did this setting come from?".

## Recommendation

**Option 1 (Flat Registry)** seems to strike the best balance for this project.
1.  It avoids the deep nesting of the original proposal.
2.  It decouples "Infrastructure" (Runtimes) from "Software" (Providers).
3.  The `Profile` concept cleanly handles the "intersection" logic (e.g., "In this profile, tmux is on, and Gemini uses this image") without scattering it.
