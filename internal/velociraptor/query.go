package velociraptor

// QueryLimits bounds a single Velociraptor query/result stream. Every
// method in this package that returns rows must enforce both limits
// before returning, truncating and reporting FlowResultPage.Truncated
// rather than silently returning everything.
//
// TODO(v0.1.0-alpha.2): thread QueryLimits from config.VelociraptorConfig
// into the real gRPC client constructor, and enforce it while streaming
// VQLResponse messages from the Velociraptor API (stop reading and mark
// truncated as soon as either bound is hit, rather than buffering
// everything and truncating after the fact).
type QueryLimits struct {
	MaxRows        int
	MaxBytes       int64
	TimeoutSeconds int
}

// ApplyRowLimit truncates rows to at most limits.MaxRows, reporting
// whether truncation occurred.
func ApplyRowLimit(rows []map[string]any, limits QueryLimits) (out []map[string]any, truncated bool) {
	if limits.MaxRows <= 0 || len(rows) <= limits.MaxRows {
		return rows, false
	}
	return rows[:limits.MaxRows], true
}
