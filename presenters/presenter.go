package presenters

import (
	"io"

	"github.com/adamluzsi/frameless"
)

// Presenter is represent a communication layer presenting layer
//
// Scope:
// 	receive messages, and convert it into a serialized form
//
// You should not allow the users of the Presenter object to modify the state of the enwrapped communication channel, such as closing, or direct writing
type Presenter interface {
	//
	// RenderWithTemplate a content on a channel that the Presenter implements
	//	name helps to determine the what template should be used, but should not include channel specific names
	//	data is the content that should be used in the template
	// RenderWithTemplate(name string, data frameless.Content) error

	//
	// Render renders a simple message back to the enwrapped communication channel
	//	message is an interface type because the channel communication layer and content and the serialization is up to the Presenter to implement
	Render(message frameless.Content) error
}

// PresenterBuilder is an example how presenter should be created
type PresenterBuilder func(io.Writer) Presenter
