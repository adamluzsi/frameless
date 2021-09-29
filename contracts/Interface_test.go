package contracts

var _ = []Interface{
	Creator{},
	Finder{},
	FindOne{},
	Updater{},
	Deleter{},
	OnePhaseCommitProtocol{},
	Publisher{},
	CreatorPublisher{},
	UpdaterPublisher{},
	DeleterPublisher{},
	MetaAccessor{},
	MetaAccessorBasic{},
	MetaAccessorPublisher{},
}
