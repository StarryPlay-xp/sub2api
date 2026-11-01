package traffic

// PeakHistory accompanies byte buckets. A nil value means legacy history has
// no peak observations; it must not be rendered as zero bandwidth.
type PeakHistory struct {
	Peaks   Peaks   `json:"peaks"`
	Quality Quality `json:"quality"`
}

func (h *PeakHistory) Merge(v *PeakHistory) {
	if v == nil {
		return
	}
	if h.Peaks == nil {
		h.Peaks = Peaks{}
	}
	h.Peaks.Merge(v.Peaks)
	h.Quality.Merge(v.Quality)
}
func (h *PeakHistory) Clone() *PeakHistory {
	if h == nil {
		return nil
	}
	c := *h
	c.Peaks = h.Peaks.Clone()
	return &c
}
