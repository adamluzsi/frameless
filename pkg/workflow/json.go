package workflow

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

