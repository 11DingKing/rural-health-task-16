package domain

// Agency represents a testing/accreditation organization that receives
// dispatch tasks and returns test results.
type Agency struct {
	ID                  string    `json:"id"`
	Code                string    `json:"code"`
	Name                string    `json:"name"`
	ContactEmail        string    `json:"contact_email"`
	IsActive            bool      `json:"is_active"`
	Priority            int       `json:"priority"`
	AccreditedStandards []string  `json:"accredited_standards"`
	MaxConcurrent       int       `json:"max_concurrent"`
	BaseURL             string    `json:"base_url,omitempty"`
	Audit               AuditMeta `json:"audit"`
}

// CanHandleStandard returns true if the agency is accredited for the given standard.
func (a Agency) CanHandleStandard(code string) bool {
	if !a.IsActive {
		return false
	}
	for _, s := range a.AccreditedStandards {
		if s == code {
			return true
		}
	}
	return false
}

// IsAvailable returns true if the agency can accept concurrent work.
func (a Agency) IsAvailable(currentLoad int) bool {
	if !a.IsActive {
		return false
	}
	if a.MaxConcurrent <= 0 {
		return true
	}
	return currentLoad < a.MaxConcurrent
}
