package cli

type HelpSummary interface {
	// Summary returns a summary about the application
	//
	// TODO: Maybe ranem this to "Desc" as the tag "desc" is used for this purpose
	Summary() string
}
