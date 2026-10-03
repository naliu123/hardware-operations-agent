// Package evidence validates observations before they enter answers or plans.
package evidence

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"hwops/internal/domain"
)

type Service struct {
	Observer domain.Observer
	Mode     string
}

func ValidateRequest(q domain.ObservationRequest, now time.Time) error {
	if !slices.Contains([]string{"metrics", "alerts", "logs"}, q.Capability) ||
		len(q.Fields) == 0 || len(q.Fields) > 16 || len(q.Component) > 200 ||
		q.WindowStart.IsZero() || !q.WindowEnd.After(q.WindowStart) ||
		q.WindowEnd.After(now.Add(5*time.Second)) || q.WindowEnd.Sub(q.WindowStart) > 24*time.Hour ||
		q.MaxAgeSeconds < 1 || q.MaxAgeSeconds > 86400 || len(q.Parameters) > 16 {
		return fmt.Errorf("%w: observation requires capability, fields, a past window of at most 24h and max_age_seconds in 1..86400", domain.ErrInvalid)
	}
	seen := map[string]bool{}
	for _, field := range q.Fields {
		if strings.TrimSpace(field) == "" || len(field) > 100 || seen[field] {
			return fmt.Errorf("%w: invalid observation field", domain.ErrInvalid)
		}
		seen[field] = true
	}
	for k, v := range q.Parameters {
		if strings.TrimSpace(k) == "" || len(k) > 100 || len(v) > 1000 {
			return fmt.Errorf("%w: invalid observation parameter", domain.ErrInvalid)
		}
	}
	return nil
}

func (s *Service) Observe(ctx context.Context, device domain.DeviceContext, request domain.ObservationRequest) (domain.Evidence, error) {
	if err := ValidateRequest(request, time.Now()); err != nil {
		return domain.Evidence{}, err
	}
	if device.DeviceID == "" || device.SnapshotID == "" || device.DataMode != s.Mode {
		return domain.Evidence{}, fmt.Errorf("%w: observation device context or data mode mismatch", domain.ErrInvalid)
	}
	q := domain.ObservationQuery{ObservationRequest: request, Device: device}
	e := domain.Evidence{SchemaVersion: 1, ID: rand.Text(), Query: q}
	base := domain.ObservationData{
		DeviceID: device.DeviceID, SnapshotID: device.SnapshotID, MonitoringID: device.MonitoringID,
		DataMode: s.Mode, WindowStart: request.WindowStart, WindowEnd: request.WindowEnd,
		Values: []domain.ObservationValue{},
	}
	if s.Observer == nil || device.MonitoringID == "" {
		e.ObservationData = base
		e.Status = "UNSUPPORTED"
		e.Error = &domain.Failure{Code: "MONITOR_UNCONFIGURED", Message: "未配置监控接口或设备监控标识。"}
	} else {
		data, err := s.Observer.Observe(ctx, q)
		if ctx.Err() != nil {
			return domain.Evidence{}, ctx.Err()
		}
		if err == nil {
			err = validateData(data, q)
		}
		if err != nil {
			// No untrusted values survive a failed identity or coverage check.
			e.ObservationData = base
			e.Status = "FAILED"
			e.Error = &domain.Failure{Code: "OBSERVATION_FAILED", Message: "监控调用失败或设备、快照、数据模式、范围及字段校验未通过。"}
		} else {
			e.ObservationData = data
		}
	}
	e.FetchedAt = time.Now().UTC()
	if (e.Status == "OK" || e.Status == "PARTIAL" || e.Status == "NO_RECORD") &&
		!Fresh(e, e.FetchedAt) {
		e.Status = "STALE"
	}
	return e, nil
}

func validateData(d domain.ObservationData, q domain.ObservationQuery) error {
	bad := fmt.Errorf("invalid observation")
	if !slices.Contains([]string{"OK", "NO_RECORD", "PARTIAL", "FAILED", "STALE", "UNSUPPORTED"}, d.Status) ||
		d.DeviceID != q.Device.DeviceID || d.SnapshotID != q.Device.SnapshotID ||
		d.MonitoringID != q.Device.MonitoringID || d.DataMode != q.Device.DataMode ||
		!d.WindowStart.Equal(q.WindowStart) || !d.WindowEnd.Equal(q.WindowEnd) ||
		strings.TrimSpace(d.Source) == "" || strings.TrimSpace(d.RawRef) == "" ||
		len(d.Source) > 2000 || len(d.RawRef) > 2000 || len(d.Values) > 256 {
		return bad
	}
	if d.Status == "FAILED" || d.Status == "UNSUPPORTED" || d.Status == "NO_RECORD" {
		if len(d.Values) != 0 {
			return bad
		}
	}
	found := map[string]bool{}
	for _, value := range d.Values {
		if !slices.Contains(q.Fields, value.Field) || value.Value == "" || len(value.Value) > 4000 ||
			strings.TrimSpace(value.Unit) == "" || len(value.Unit) > 100 || value.ObservedAt.Before(q.WindowStart) ||
			value.ObservedAt.After(q.WindowEnd) {
			return bad
		}
		found[value.Field] = true
	}
	if (d.Status == "OK" || d.Status == "PARTIAL" || d.Status == "STALE") && len(d.Values) == 0 {
		return bad
	}
	for _, field := range q.Fields {
		if d.Status == "OK" && !found[field] {
			return bad
		}
		if d.Status == "PARTIAL" && !found[field] && !slices.Contains(d.Missing, field) {
			return bad
		}
	}
	for _, field := range d.Missing {
		if !slices.Contains(q.Fields, field) || found[field] {
			return bad
		}
	}
	if len(d.Missing) > 0 && d.Status == "OK" {
		return bad
	}
	return nil
}

func Fresh(e domain.Evidence, now time.Time) bool {
	cutoff := now.Add(-time.Duration(e.Query.MaxAgeSeconds) * time.Second)
	if e.WindowEnd.Before(cutoff) {
		return false
	}
	for _, v := range e.Values {
		if v.ObservedAt.Before(cutoff) {
			return false
		}
	}
	return true
}

// Reusable requires the entire query identity, including version and mode.
func Reusable(e domain.Evidence, q domain.ObservationQuery, now time.Time) bool {
	a, _ := json.Marshal(e.Query)
	b, _ := json.Marshal(q)
	return string(a) == string(b) && (e.Status == "OK" || e.Status == "NO_RECORD") && Fresh(e, now)
}

func Gap(e domain.Evidence) string {
	switch e.Status {
	case "FAILED":
		return "监控查询失败，不能据此判断设备故障。"
	case "UNSUPPORTED":
		return "当前接口或设备映射不支持该观测。"
	case "PARTIAL":
		return "仅取得部分观测，缺少：" + strings.Join(e.Missing, "、") + "；不能判断整台设备正常。"
	case "STALE":
		return "观测已超过时效要求，需要刷新后才能判断当前状态。"
	}
	return ""
}

// Render emits exact values with scope; the model only selects existing fields.
func Render(e domain.Evidence, fields []string) (string, error) {
	for _, field := range fields {
		if !slices.Contains(e.Query.Fields, field) {
			return "", domain.ErrInvalid
		}
	}
	text := fmt.Sprintf("【%s】设备 %s / %s；%s；范围 %s 至 %s；来源 %s；证据 %s。",
		e.DataMode, e.DeviceID, e.Query.Component, e.Status, e.WindowStart.Format(time.RFC3339),
		e.WindowEnd.Format(time.RFC3339), e.Source, e.ID)
	if e.Status == "NO_RECORD" {
		return text + " 此查询范围内无记录，不代表整台设备正常。", nil
	}
	for _, v := range e.Values {
		if len(fields) == 0 || slices.Contains(fields, v.Field) {
			text += fmt.Sprintf("\n%s = %s %s（%s）", v.Field, v.Value, v.Unit, v.ObservedAt.Format(time.RFC3339))
		}
	}
	return text + "\n结论仅限所列字段与观察范围。", nil
}
