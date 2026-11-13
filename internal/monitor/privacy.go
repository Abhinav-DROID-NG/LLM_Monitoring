package monitor

import (
	"errors"
	"fmt"

	"github.com/Abhinav-DROID-NG/LLM_Monitoring/internal/canary"
)

func ValidateObservations(library canary.Library, observations []Observation) error {
	allowed := make(map[string]bool, len(library.Tasks))
	for _, task := range library.Tasks {
		allowed[task.ID] = true
	}
	for _, observation := range observations {
		if !allowed[observation.CanaryID] {
			return fmt.Errorf("observation %q is not a canary", observation.CanaryID)
		}
	}
	return nil
}

func PrivacyCheck(library canary.Library, observations []Observation) error {
	if library.Tasks == nil {
		return errors.New("canary library is required")
	}
	return ValidateObservations(library, observations)
}
