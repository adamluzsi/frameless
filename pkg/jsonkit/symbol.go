package jsonkit

var ( // Symbols
	null              = []byte("null")
	trueSym           = []byte("true")
	falseSym          = []byte("false")
	fieldSep          = []byte(",")
	quote             = []byte(`"`)
	bracketOpen       = []byte("[")
	bracketClose      = []byte("]")
	curlyBracketOpen  = []byte("{")
	curlyBracketClose = []byte("}")
)
