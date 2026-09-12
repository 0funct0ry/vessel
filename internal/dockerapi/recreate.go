package dockerapi

import (
	"context"
	"errors"
	"fmt"
)

// RecreateFailed means the previous container was removed to free its name
// but the replacement failed to create or start. FreedName and Spec let the
// caller say exactly what state things are actually in, per SPEC §9.2.
type RecreateFailed struct {
	FreedName string
	Spec      Spec
	Err       error
}

func (e *RecreateFailed) Error() string {
	return fmt.Sprintf("dockerapi: recreating %s: old container removed, new one failed: %v", e.FreedName, e.Err)
}
func (e *RecreateFailed) Unwrap() error { return e.Err }

// RecreateContainer stops (implicitly, via force-remove) and removes an
// existing container, then creates a replacement under the same name from
// spec. A stopped container stays stopped after recreate; a running one is
// started again.
func (c *Client) RecreateContainer(ctx context.Context, id string, spec Spec) (CreateResult, error) {
	existing, err := c.InspectContainer(ctx, id)
	if err != nil {
		return CreateResult{}, err
	}
	name := existing.ID
	if trimmed := existing.Name; trimmed != "" {
		name = trimmed
	}
	for len(name) > 0 && name[0] == '/' {
		name = name[1:]
	}
	wasRunning := existing.State == "running"

	if err := c.RemoveContainer(ctx, id, RemoveContainerOptions{Force: true}); err != nil {
		return CreateResult{}, err
	}

	spec.Name = name
	spec.Start = wasRunning
	result, err := c.CreateContainer(ctx, spec)
	if err != nil {
		var startErr *StartError
		if errors.As(err, &startErr) {
			// The container exists under the freed name; only starting it
			// failed, so this is not the "name freed, nothing replaced it" case.
			return result, err
		}
		return CreateResult{}, &RecreateFailed{FreedName: name, Spec: spec, Err: err}
	}
	return result, nil
}
