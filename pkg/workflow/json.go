package workflow

import "reflect"

type types interface {
	ProcessDefinition |

		Goto |
		Label |

		If |
		While |
		Switch |
		Comparison |

		Template |

		Sequence |
		Concurrence |

		RefValue |
		ConstValue
}

func MarshalJSON(v any) ([]byte, error) {
	return nil, nil
}

func UnmarshalJSON(data []byte, v any) error {
	return nil
}

type DataTransferObject map[string]any

type DataTransferObjectMapping[Ent any] struct {
	ToEnt func(dto DataTransferObject) (Ent, error)
	ToDTO func(ent Ent) (DataTransferObject, error)
}

var registryDataTransferObject = map[reflect.Type]DataTransferObjectMapping[]
