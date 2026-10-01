package knowledge

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"hwops/internal/domain"
)

// Policies are immutable: revised vendor rules receive a new ID, and existing
// knowledge keeps the original rule until a new revision is published.
func NewVersionPolicy(in domain.VersionPolicyInput) (domain.VersionPolicy, error) {
	if strings.TrimSpace(in.Model) == "" || in.Model != strings.TrimSpace(in.Model) ||
		len(in.Model) > 256 || strings.TrimSpace(in.Source) == "" ||
		len(in.FirmwareOrder)+len(in.DriverOrder) == 0 {
		return domain.VersionPolicy{}, fmt.Errorf("%w: model, source and at least one version order are required", domain.ErrInvalid)
	}
	for _, order := range [][]string{in.FirmwareOrder, in.DriverOrder} {
		if len(order) > 1024 {
			return domain.VersionPolicy{}, fmt.Errorf("%w: version order exceeds 1024 entries", domain.ErrInvalid)
		}
		seen := map[string]bool{}
		for _, version := range order {
			if version == "" || len(version) > 256 || version != strings.TrimSpace(version) || seen[version] {
				return domain.VersionPolicy{}, fmt.Errorf("%w: versions must be distinct nonempty strings without surrounding whitespace", domain.ErrInvalid)
			}
			seen[version] = true
		}
	}
	return domain.VersionPolicy{
		VersionPolicyInput: in, SchemaVersion: 1, ID: rand.Text(), CreatedAt: time.Now().UTC(),
	}, nil
}
