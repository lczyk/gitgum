package commands

// stubNetwork answers the remote-facing questions from canned data, so clone
// and add-remote can be driven without touching a network. A nil hook means
// "not expected here": reachable answers false, LsRemote answers empty.
type stubNetwork struct {
	reachable func(url string) bool
	lsRemote  func(remote string) (string, error)
}

func (s stubNetwork) RemoteReachable(url string) bool {
	if s.reachable == nil {
		return false
	}
	return s.reachable(url)
}

func (s stubNetwork) LsRemote(remote string) (string, error) {
	if s.lsRemote == nil {
		return "", nil
	}
	return s.lsRemote(remote)
}
