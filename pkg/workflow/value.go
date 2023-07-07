package workflow

type Value interface{ GetValue(*Variables) any }

type ConstValue struct{ Value any }

func (cv ConstValue) GetValue(*Variables) any { return cv.Value }

type RefValue struct{ Key VariableKey }

func (v RefValue) GetValue(vs *Variables) any { return (*vs)[v.Key] }
