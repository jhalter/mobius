package mobius

// Reloader is implemented by storage backends that can reload their state from disk, e.g. in
// response to SIGHUP or the reload API endpoint.
type Reloader interface {
	Reload() error
}

// ReloaderFunc adapts a plain func to the Reloader interface.
type ReloaderFunc func() error

func (f ReloaderFunc) Reload() error { return f() }

var (
	_ Reloader = (*FlatNews)(nil)
	_ Reloader = (*BanFile)(nil)
	_ Reloader = (*ThreadedNewsYAML)(nil)
	_ Reloader = (*Agreement)(nil)
)
