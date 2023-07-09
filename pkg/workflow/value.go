package workflow

type Value interface{ GetValue(*Vars) any }

type ConstValue struct{ Value any }

func (cv ConstValue) GetValue(*Vars) any { return cv.Value }

type RefValue struct{ Key VarKey }

func (v RefValue) GetValue(vs *Vars) any { return (*vs)[v.Key] }
