package session

// DomainState decouples a split-out domain's state from the Sim struct.
//
// Study and boundary and the other early domains held no state of their own, so
// they reached the Sim through plain accessors. The domains that follow -
// experiment, companion, provisioning and the rest - each kept a typed field on
// Sim (an *experiment, a map of companion sessions), and a field of the domain's
// own type is exactly what a split-out package cannot leave on Sim: the struct
// would have to name the type, which means importing the package, which imports
// session back. A cycle, for a pointer.
//
// So the state moves off the struct and into here, keyed by a name the domain
// picks. The domain asks for its state with a maker that builds it the first
// time; session never names the type, and the domain type-asserts the value it
// gets back. mk runs at most once per key per Sim, under the lock, so two
// goroutines racing to first-touch a domain still share one instance.
func DomainState(s *Sim, key string, mk func() any) any {
	s.domainStateMu.Lock()
	defer s.domainStateMu.Unlock()
	if s.domainState == nil {
		s.domainState = map[string]any{}
	}
	v, ok := s.domainState[key]
	if !ok {
		v = mk()
		s.domainState[key] = v
	}
	return v
}
