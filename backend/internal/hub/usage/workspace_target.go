package usage

import "measix/platform/internal/wire/usageingestapi"

func admissionTargetVersion(in usageingestapi.BudgetAdmissionRequest) int {
	if in.TargetVersion == nil {
		return 0
	}
	return int(*in.TargetVersion)
}
func factTargetVersion(in usageingestapi.RequestUsageFact) int {
	if in.TargetVersion == nil {
		return 0
	}
	return int(*in.TargetVersion)
}
