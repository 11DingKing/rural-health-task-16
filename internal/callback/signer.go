package callback

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// Signer produces HMAC-SHA256 signatures for callback payloads so
// enterprises can verify the authenticity of notifications.
type Signer struct {
	secretKey string
}

func NewSigner(secretKey string) *Signer {
	return &Signer{secretKey: secretKey}
}

// Sign computes the HMAC-SHA256 signature of the payload.
func (s *Signer) Sign(payload []byte) string {
	mac := hmac.New(sha256.New, []byte(s.secretKey))
	mac.Write(payload)
	return hex.EncodeToString(mac.Sum(nil))
}

// Verify checks that a signature matches the payload.
func (s *Signer) Verify(payload []byte, signature string) bool {
	expected := s.Sign(payload)
	return hmac.Equal([]byte(expected), []byte(signature))
}

// BuildPayload creates a signed callback payload for a submission result.
func (s *Signer) BuildPayload(submissionID string, summary any, timestamp time.Time) (string, string, error) {
	wrapper := map[string]any{
		"submission_id": submissionID,
		"summary":       summary,
		"timestamp":     timestamp.Format(time.RFC3339),
	}
	data, err := json.Marshal(wrapper)
	if err != nil {
		return "", "", fmt.Errorf("marshal payload: %w", err)
	}
	signature := s.Sign(data)
	return string(data), signature, nil
}
