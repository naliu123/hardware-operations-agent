package devices

import (
	"context"
	"crypto/rand"
	"fmt"
	"regexp"
	"strings"
	"time"

	"hwops/internal/domain"
)

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

func NewSnapshot(id string, in domain.DeviceInput) (domain.DeviceContext, error) {
	if !identifier.MatchString(id) || strings.TrimSpace(in.Source) == "" ||
		in.ObservedAt.IsZero() || in.ObservedAt.After(time.Now().Add(5*time.Minute)) ||
		(in.DataMode != "LIVE" && in.DataMode != "REPLAY") || len(in.Aliases) > 32 {
		return domain.DeviceContext{}, fmt.Errorf("%w: device ID, source, observed_at and LIVE/REPLAY data_mode are required; at most 32 aliases", domain.ErrInvalid)
	}
	for _, value := range append([]string{in.Name, in.Model, in.Firmware, in.Driver, in.HardwareRevision, in.MonitoringID}, in.Aliases...) {
		if len(value) > 256 || strings.TrimSpace(value) != value {
			return domain.DeviceContext{}, fmt.Errorf("%w: device fields must be at most 256 bytes without surrounding whitespace", domain.ErrInvalid)
		}
	}
	for _, alias := range in.Aliases {
		if alias == "" {
			return domain.DeviceContext{}, fmt.Errorf("%w: empty device alias", domain.ErrInvalid)
		}
	}
	in.ObservedAt = in.ObservedAt.UTC()
	return domain.DeviceContext{DeviceInput: in, SchemaVersion: 1, DeviceID: id, SnapshotID: rand.Text()}, nil
}

func Resolve(ctx context.Context, store domain.Repository, query string) (domain.DeviceResolution, error) {
	if strings.TrimSpace(query) == "" || len(query) > 256 {
		return domain.DeviceResolution{}, fmt.Errorf("%w: device query is required (256 bytes max)", domain.ErrInvalid)
	}
	all, err := store.ListDevices(ctx)
	if err != nil {
		return domain.DeviceResolution{}, err
	}
	resolution := domain.DeviceResolution{Status: "NO_RECORD", Candidates: []domain.DeviceContext{}}
	for _, device := range all {
		for _, name := range append([]string{device.DeviceID, device.Name}, device.Aliases...) {
			if strings.EqualFold(name, strings.TrimSpace(query)) {
				resolution.Candidates = append(resolution.Candidates, device)
				break
			}
		}
	}
	switch len(resolution.Candidates) {
	case 0:
	case 1:
		resolution.Status = "RESOLVED"
	default:
		resolution.Status = "AMBIGUOUS"
	}
	return resolution, nil
}

// ResolveText recognizes only registered identifiers, names and aliases.
// ASCII identifiers must be whole tokens (rack-1 must not match rack-10).
func ResolveText(ctx context.Context, store domain.Repository, text string) (domain.DeviceResolution, error) {
	all, err := store.ListDevices(ctx)
	if err != nil {
		return domain.DeviceResolution{}, err
	}
	resolution := domain.DeviceResolution{Status: "NO_RECORD", Candidates: []domain.DeviceContext{}}
	text = strings.ToLower(text)
	token := func(b byte) bool {
		return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || strings.ContainsRune("_-.:/", rune(b))
	}
	for _, device := range all {
		matched := false
		for _, name := range append([]string{device.DeviceID, device.Name}, device.Aliases...) {
			name = strings.ToLower(name)
			if name == "" {
				continue
			}
			for offset := 0; offset < len(text); {
				pos := strings.Index(text[offset:], name)
				if pos < 0 {
					break
				}
				pos += offset
				end := pos + len(name)
				if (pos == 0 || !token(name[0]) || !token(text[pos-1])) &&
					(end == len(text) || !token(name[len(name)-1]) || !token(text[end])) {
					matched = true
					break
				}
				offset = end
			}
			if matched {
				resolution.Candidates = append(resolution.Candidates, device)
				break
			}
		}
	}
	if len(resolution.Candidates) == 1 {
		resolution.Status = "RESOLVED"
	} else if len(resolution.Candidates) > 1 {
		resolution.Status = "AMBIGUOUS"
	}
	return resolution, nil
}
