package arn

// IsValidNick tests if the given nickname is valid.
func IsValidNick(nick string) bool {
	if len(nick) < 2 {
		return false
	}

	return nick == FixUserNick(nick)
}

// IsValidEmail tests if the given email address is valid.
func IsValidEmail(email string) bool {
	if email == "" {
		return false
	}

	// TODO: ...

	return true
}
