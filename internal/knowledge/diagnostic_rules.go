package knowledge

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"hwops/internal/domain"
)

func ValidateMetricCondition(c domain.MetricCondition) error {
	if !slices.Contains([]string{"metrics", "alerts", "logs"}, c.Capability) ||
		!slices.Contains([]string{"EQ", "GT", "GE", "LT", "LE"}, c.Operator) ||
		strings.TrimSpace(c.Field) == "" || len(c.Field) > 100 || len(c.Component) > 200 ||
		c.Value == "" || len(c.Value) > 4000 || strings.TrimSpace(c.Unit) == "" || len(c.Unit) > 100 ||
		c.MaxAgeSeconds < 1 || c.MaxAgeSeconds > 86400 || len(c.Parameters) > 16 {
		return fmt.Errorf("%w: invalid metric condition", domain.ErrInvalid)
	}
	for k, v := range c.Parameters {
		if strings.TrimSpace(k) == "" || len(k) > 100 || len(v) > 1000 {
			return fmt.Errorf("%w: invalid condition parameter", domain.ErrInvalid)
		}
	}
	if c.Operator != "EQ" {
		n, err := strconv.ParseFloat(c.Value, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return fmt.Errorf("%w: numeric condition requires a finite value", domain.ErrInvalid)
		}
	}
	return nil
}

func validateDiagnosticRules(r domain.Revision) error {
	if len(r.DiagnosticRules) > 32 {
		return fmt.Errorf("%w: too many diagnostic rules", domain.ErrInvalid)
	}
	seen := map[string]bool{}
	for _, rule := range r.DiagnosticRules {
		if rule.ID == "" || len(rule.ID) > 100 || seen[rule.ID] ||
			!slices.Contains([]string{"ROOT_CAUSE", "RECOVERY"}, rule.Kind) ||
			strings.TrimSpace(rule.ErrorCode) == "" || len(rule.ErrorCode) > 200 ||
			strings.TrimSpace(rule.Conclusion) == "" || len(rule.Conclusion) > 2000 ||
			rule.StartLine < 1 || rule.EndLine < rule.StartLine ||
			len(rule.Conditions) == 0 || len(rule.Conditions) > 16 {
			return fmt.Errorf("%w: invalid diagnostic rule", domain.ErrInvalid)
		}
		seen[rule.ID] = true
		covered := false
		for _, f := range r.Fragments {
			covered = covered || (f.StartLine <= rule.StartLine && f.EndLine >= rule.EndLine)
		}
		if !covered {
			return fmt.Errorf("%w: diagnostic rule must reference one complete source fragment", domain.ErrInvalid)
		}
		for _, c := range rule.Conditions {
			if err := ValidateMetricCondition(c); err != nil {
				return err
			}
		}
	}
	return nil
}
