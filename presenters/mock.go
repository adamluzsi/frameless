package presenters

func NewMock() *Mock {
	return &Mock{}
}

type Mock struct {
	ReceivedMessages []interface{}
	ReturnError      error
}

func (m *Mock) Render(message interface{}) error {
	m.ReceivedMessages = append(m.ReceivedMessages, message)
	return m.ReturnError
}

func (m *Mock) LastReceivedMessage() interface{} {
	return m.ReceivedMessages[len(m.ReceivedMessages)-1]
}
