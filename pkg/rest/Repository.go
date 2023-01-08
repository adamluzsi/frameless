package rest

//func MakeResource[Entity, ID any](res crud.ByIDFinder[Entity, ID]) Repository[Entity, ID] {
//	var r Repository[Entity, ID]
//	if v, ok := res.(crud.Creator[Entity]); ok {
//		r.Creator = v
//	}
//	if v, ok := res.(crud.AllFinder[Entity]); ok {
//		r.AllFinder = v
//	}
//	if v, ok := res.(crud.ByIDFinder[Entity, ID]); ok {
//		r.ByIDFinder = v
//	}
//	if v, ok := res.(crud.Updater[Entity]); ok {
//		r.Updater = v
//	}
//	if v, ok := res.(crud.ByIDDeleter[ID]); ok {
//		r.ByIDDeleter = v
//	}
//	return r
//}
