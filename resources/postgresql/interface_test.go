package postgresql

var (
	_ Pool    = &DefaultPool{}
	_ Mapping = &Mapper{}
)
