package cli

type HelpSummary interface {
	// Summary returns a summary about the application.
	//
	// Summary used as part of the command list within a mux, to give a hint which sub command responsible for what.
	Summary() string
}

type HelpDescription interface {
	// Description returns a potentially detailed description about the application
	//
	Description() string
}
