package promptauditv2

// MatchRule reports whether the single configured rule matches a model score.
func MatchRule(rule RuleConfig, confidence float64) bool {
	return confidence >= rule.ConfidenceThreshold
}
