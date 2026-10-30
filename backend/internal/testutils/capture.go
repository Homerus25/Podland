package testutils

// CallCapture records the (name, opts) pair one fake-engine call
// received — the shared capture half of the per-domain logs
// composition pins (containers, pods, quadlets). A domain's handler
// test keeps its fake-engine wiring (Record inside its logsFn, plus
// whatever the fake must write into the options) and its assertions
// over the fields.
type CallCapture[Opts any] struct {
	Name string
	Opts Opts
}

// Record captures one call's target name and options.
func (c *CallCapture[Opts]) Record(name string, opts Opts) {
	c.Name = name
	c.Opts = opts
}
