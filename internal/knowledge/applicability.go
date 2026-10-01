package knowledge

import (
	"context"
	"errors"
	"slices"

	"hwops/internal/domain"
)

// Assess keeps unknown information distinct from a disproved requirement.
func Assess(ctx context.Context, store domain.Repository, a domain.Applicability, device *domain.DeviceContext) (domain.Assessment, error) {
	out := domain.Assessment{Status: "MATCH", Checks: []domain.FieldCheck{}}
	check := func(field, status, reason string) {
		out.Checks = append(out.Checks, domain.FieldCheck{Field: field, Status: status, Reason: reason})
		if status == "MISMATCH" || (status == "UNKNOWN" && out.Status == "MATCH") {
			out.Status = status
		}
	}
	if a.Scope == "GENERAL" {
		check("scope", "MATCH", "资料声明为通用知识。")
		return out, nil
	}
	if a.Scope != "DEVICE" || device == nil {
		check("device", "UNKNOWN", "尚未确定目标设备。")
		return out, nil
	}
	for _, requirement := range []struct{ field, want, actual string }{
		{"model", a.Model, device.Model},
		{"firmware", a.Firmware, device.Firmware},
		{"driver", a.Driver, device.Driver},
		{"hardware_revision", a.HardwareRevision, device.HardwareRevision},
	} {
		if requirement.want == "" {
			continue
		}
		switch {
		case requirement.actual == "":
			check(requirement.field, "UNKNOWN", "设备快照缺少 "+requirement.field+"。")
		case requirement.want != requirement.actual:
			check(requirement.field, "MISMATCH", "设备值 "+requirement.actual+" 与资料要求 "+requirement.want+" 不一致。")
		default:
			check(requirement.field, "MATCH", "精确匹配 "+requirement.want+"。")
		}
	}
	var policy domain.VersionPolicy
	if a.PolicyID != "" && (a.FirmwareRange != nil || a.DriverRange != nil) {
		var err error
		policy, err = store.GetVersionPolicy(ctx, a.PolicyID)
		if err != nil && !errors.Is(err, domain.ErrNotFound) {
			return out, err
		}
	}
	for _, requirement := range []struct {
		field, actual string
		bounds        *domain.VersionRange
		order         []string
	}{
		{"firmware", device.Firmware, a.FirmwareRange, policy.FirmwareOrder},
		{"driver", device.Driver, a.DriverRange, policy.DriverOrder},
	} {
		if requirement.bounds == nil {
			continue
		}
		min := slices.Index(requirement.order, requirement.bounds.Min)
		max := slices.Index(requirement.order, requirement.bounds.Max)
		actual := slices.Index(requirement.order, requirement.actual)
		switch {
		case requirement.actual == "":
			check(requirement.field, "UNKNOWN", "设备快照缺少 "+requirement.field+"。")
		case policy.Model != a.Model || len(requirement.order) == 0:
			check(requirement.field, "UNKNOWN", "缺少该型号的 "+requirement.field+" 版本顺序规则。")
		case min < 0 || max < 0 || min > max || actual < 0:
			check(requirement.field, "UNKNOWN", "规则 "+policy.ID+" 无法解释该版本或范围。")
		case actual < min || actual > max:
			check(requirement.field, "MISMATCH", "按规则 "+policy.ID+"，版本 "+requirement.actual+" 不在闭区间内。")
		default:
			check(requirement.field, "MATCH", "按规则 "+policy.ID+"（"+policy.Source+"），版本 "+requirement.actual+
				" 位于闭区间 ["+requirement.bounds.Min+", "+requirement.bounds.Max+"]。")
		}
	}
	return out, nil
}
