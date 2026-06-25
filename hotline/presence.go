package hotline

// PresenceTracker receives best-effort notifications about user session lifecycle
// events so an external system (e.g. Redis) can maintain a list of online users.
//
// It is optional: when a Server has no PresenceTracker, online presence is derived
// from the in-memory ClientManager instead. Implementations must be safe for
// concurrent use by multiple goroutines.
type PresenceTracker interface {
	// UserConnected is called after a successful login, before the nickname is known.
	UserConnected(login, ip string)

	// UserRenamed is called when a user's nickname is set or changed. oldNickname is
	// empty the first time a nickname is set (e.g. the 1.2.3 login flow or TranAgreed).
	UserRenamed(login, oldNickname, newNickname, ip string)

	// UserDisconnected is called when a user's session ends. nickname is empty if the
	// user disconnected before setting one.
	UserDisconnected(login, nickname, ip string)
}
