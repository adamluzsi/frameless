package contracts


var _ = []Interface{
	Creator{},
	Finder{},
	FindOne{},
	Updater{},
	Deleter{},
	OnePhaseCommitProtocol{},
	Publisher{},
	creatorPublisher{},
	updaterPublisher{},
	deleterPublisher{},
	MetaAccessor{},
	MetaAccessorBasic{},
	MetaAccessorPublisher{},
}
