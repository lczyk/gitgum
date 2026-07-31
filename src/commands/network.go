package commands

// network is the remote-facing slice of git that clone and add-remote reach
// for: probing whether a repo exists on a candidate forge, and listing a
// remote's refs before deciding what to do. git.Repo satisfies it directly,
// so production wiring needs no adapter and tests supply a fake.
//
// It is an interface rather than a pair of injected functions because both
// operations travel together and are answered by the same collaborator --
// the same reason ui.Selector is one. A single injectable operation would
// still be a func field.
type network interface {
	RemoteReachable(url string) bool
	LsRemote(remote string) (string, error)
}

// net returns the injected network or the real one bound to this command's
// repo. The zero value means production, so commands constructed by go-flags
// reflection work untouched.
func (c *cmdIO) net(injected network) network {
	if injected != nil {
		return injected
	}
	return c.repo()
}
