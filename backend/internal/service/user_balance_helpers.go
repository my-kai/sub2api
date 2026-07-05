package service

func resolvedUserAvailableBalance(user *User) float64 {
	if user == nil {
		return 0
	}
	if user.AvailableBalance != 0 {
		return user.AvailableBalance
	}
	// Repository-loaded users may not carry the derived field yet. Request
	// admission and auth-cache snapshots must still use ordinary + gift balance.
	return user.Balance + user.GiftBalance
}
