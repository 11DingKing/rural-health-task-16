package dispatch

import (
	"context"
	"sort"

	"ruralhealth/internal/domain"
	"ruralhealth/internal/repository"
)

// AgencySelector picks the best testing agency for a given standard.
// It prefers agencies accredited for the standard with the lowest priority
// number (highest priority) and available concurrency.
type AgencySelector struct {
	agencyRepo   *repository.AgencyRepo
	dispatchRepo *repository.DispatchRepo
}

func NewAgencySelector(agencyRepo *repository.AgencyRepo, dispatchRepo *repository.DispatchRepo) *AgencySelector {
	return &AgencySelector{agencyRepo: agencyRepo, dispatchRepo: dispatchRepo}
}

// SelectForStandard returns the best agency for a standard, or nil if none.
func (sel *AgencySelector) SelectForStandard(ctx context.Context, standardCode string) (*domain.Agency, error) {
	agencies, err := sel.agencyRepo.FindByStandard(ctx, standardCode)
	if err != nil {
		return nil, err
	}
	var candidates []*domain.Agency
	for _, a := range agencies {
		if a.IsActive && a.CanHandleStandard(standardCode) {
			load, err := sel.dispatchRepo.FindByAgency(ctx, a.ID)
			if err != nil {
				return nil, err
			}
			activeCount := 0
			for _, t := range load {
				if t.Status == domain.DispatchAssigned || t.Status == domain.DispatchInProgress {
					activeCount++
				}
			}
			if a.IsAvailable(activeCount) {
				candidates = append(candidates, a)
			}
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Priority < candidates[j].Priority
	})
	return candidates[0], nil
}

// SelectBackup returns an alternative agency excluding the one already tried.
func (sel *AgencySelector) SelectBackup(ctx context.Context, standardCode, excludeAgencyID string) (*domain.Agency, error) {
	agencies, err := sel.agencyRepo.FindByStandard(ctx, standardCode)
	if err != nil {
		return nil, err
	}
	var candidates []*domain.Agency
	for _, a := range agencies {
		if a.ID == excludeAgencyID || !a.IsActive {
			continue
		}
		if a.CanHandleStandard(standardCode) {
			candidates = append(candidates, a)
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].Priority < candidates[j].Priority
	})
	return candidates[0], nil
}
