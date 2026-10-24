package pods

// newTestHandler builds the handler around the real service running over
// the given fake engine; tests arm only the operations their scenario
// needs (an unarmed operation panics if the handler reaches it).
func newTestHandler(engine *fakePodEngine) *PodHandler {
	return NewPodHandler(NewPodService(fakeEngineProvider{engine: engine}))
}
