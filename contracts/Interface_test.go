package contracts

type (
	EntType   struct{ ID IDType }
	IDType    struct{}
	KeyType   struct{}
	ValueType struct{}
)

var _ = []Interface{
	Creator[EntType, IDType]{},
	Finder[EntType, IDType]{},
	FindOne[EntType, IDType]{},
	Updater[EntType, IDType]{},
	Deleter[EntType, IDType]{},
	OnePhaseCommitProtocol[EntType, IDType]{},
	Publisher[EntType, IDType]{},
	CreatorPublisher[EntType, IDType]{},
	UpdaterPublisher[EntType, IDType]{},
	DeleterPublisher[EntType, IDType]{},
	MetaAccessor[EntType, KeyType, ValueType]{},
	MetaAccessorBasic[ValueType]{},
	MetaAccessorPublisher[EntType, KeyType, ValueType]{},
}
