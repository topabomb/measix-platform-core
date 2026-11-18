package runtimecontrol

import "measix/platform/ent"

// IsActivationNotFound keeps persistence-specific Ent errors inside the runtimecontrol boundary.
func IsActivationNotFound(err error) bool { return ent.IsNotFound(err) }
