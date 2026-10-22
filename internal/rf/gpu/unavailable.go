package gpu

// Acquisition can legitimately fail on a machine without a usable GPU. Shader
// and pipeline compilation errors are different: the comparison harness must
// fail for those rather than silently skipping a broken kernel.
type unavailableError struct{ error }

func (e unavailableError) Unwrap() error { return e.error }
