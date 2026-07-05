package invoice

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"time"
)

const applicationNoPrefix = "INV"
const applicationNoRandomLength = 10
const applicationNoAlphabet = "23456789ABCDEFGHJKLMNPQRSTUVWXYZ"

var applicationNoDateLocation = time.FixedZone("Asia/Shanghai", 8*60*60)

// generateApplicationNo creates a user-facing invoice application number.
//
// The suffix uses crypto-safe unordered characters instead of a daily sequence,
// so the visible number does not reveal how many applications were submitted.
func generateApplicationNo(now time.Time) (string, error) {
	if now.IsZero() {
		return "", ErrInvalidInput
	}
	suffix := make([]byte, applicationNoRandomLength)
	alphabetSize := big.NewInt(int64(len(applicationNoAlphabet)))
	for i := range suffix {
		n, err := rand.Int(rand.Reader, alphabetSize)
		if err != nil {
			return "", fmt.Errorf("generate invoice application random code: %w", err)
		}
		suffix[i] = applicationNoAlphabet[n.Int64()]
	}
	// Use the business timezone explicitly so deployments in UTC do not produce
	// a previous-day invoice number near midnight in China.
	return fmt.Sprintf("%s%s-%s", applicationNoPrefix, now.In(applicationNoDateLocation).Format("20060102"), string(suffix)), nil
}
