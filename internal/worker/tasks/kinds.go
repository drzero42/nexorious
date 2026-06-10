package tasks

// PerItemJobKinds returns the River job kinds for high-fan-out per-item workers —
// one job per game or import row. Their successful completions are logged at Debug
// by logging.WorkerMiddleware so a large sync/import doesn't emit thousands of INFO
// "job finished" lines; the top-level dispatch/export/import job outcomes stay at
// Info, and per-item failures still log at Warn.
func PerItemJobKinds() []string {
	return []string{
		IGDBMatchArgs{}.Kind(),
		UserGameArgs{}.Kind(),
		MetadataRefreshItemArgs{}.Kind(),
		StoreLinkRefreshItemArgs{}.Kind(),
		ImportItemArgs{}.Kind(),
		MetadataFetchArgs{}.Kind(),
	}
}
