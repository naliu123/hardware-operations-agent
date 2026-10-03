package domain

// CheckDeviceSnapshot is applied atomically with saving an answer. Historical
// responses keep their original snapshot; an in-flight answer cannot become
// current guidance after the inventory has advanced.
func (r *Response) CheckDeviceSnapshot(currentID string) {
	if (r.Status != "ANSWERED" && r.Status != "PARTIAL") || r.DeviceContext == nil || r.ContextRevision == currentID {
		return
	}
	r.Status = "UNRESOLVED"
	r.Answer = ""
	r.Claims = []Claim{}
	r.Citations = []Citation{}
	r.Observations = nil
	r.Conflicts = nil
	r.Gaps = []string{"生成期间设备快照已更新，请基于最新型号和版本重新提问。"}
}
