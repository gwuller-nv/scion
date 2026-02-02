// Package hub provides the Scion Hub API server.
package hub

import (
	"context"
)

// HostIdentity represents an authenticated Runtime Host.
type HostIdentity interface {
	Identity
	HostID() string
}

// hostIdentityImpl implements HostIdentity.
type hostIdentityImpl struct {
	hostID string
}

// ID returns the host ID.
func (h *hostIdentityImpl) ID() string { return h.hostID }

// Type returns the identity type ("host").
func (h *hostIdentityImpl) Type() string { return "host" }

// HostID returns the host ID.
func (h *hostIdentityImpl) HostID() string { return h.hostID }

// NewHostIdentity creates a new HostIdentity.
func NewHostIdentity(hostID string) HostIdentity {
	return &hostIdentityImpl{hostID: hostID}
}

// hostIdentityContextKey is the context key for HostIdentity.
type hostIdentityContextKey struct{}

// GetHostIdentityFromContext returns the HostIdentity from the context, if present.
func GetHostIdentityFromContext(ctx context.Context) HostIdentity {
	if identity, ok := ctx.Value(hostIdentityContextKey{}).(HostIdentity); ok {
		return identity
	}
	// Also check the generic identity key
	if identity, ok := ctx.Value(identityContextKey{}).(HostIdentity); ok {
		return identity
	}
	return nil
}

// contextWithHostIdentity returns a new context with the HostIdentity set.
func contextWithHostIdentity(ctx context.Context, host HostIdentity) context.Context {
	return context.WithValue(ctx, hostIdentityContextKey{}, host)
}
