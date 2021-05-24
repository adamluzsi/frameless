package postgresql

var (
	_ Pool    = &SinglePool{}
	_ Mapping = &Mapper{}
)
