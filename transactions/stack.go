package transactions

type stack []stackElement

func (stack *stack) Lookup(key interface{}) (interface{}, bool) {
	for _, element := range *stack {
		if value, ok := element.Value(key); ok {
			return value, true
		}
	}
	return nil, false
}
func (stack *stack) Push(key, value interface{}) {
	*stack = append(*stack, stackElement{
		key:   key,
		value: value,
	})
}

type stackElement struct {
	key, value interface{}
}

func (elem stackElement) Value(key interface{}) (value interface{}, ok bool) {
	return elem.value, elem.key == key
}
