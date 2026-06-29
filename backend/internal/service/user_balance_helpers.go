package service

func resolvedUserAvailableBalance(user *User) float64 {
	if user == nil {
		return 0
	}
	return user.AvailableBalance
}
