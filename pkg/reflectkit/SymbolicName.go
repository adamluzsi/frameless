package reflectkit

func SymbolicName(e interface{}) string {
	return BaseTypeOf(e).String()
}
