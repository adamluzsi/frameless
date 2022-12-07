package rest

//
//type Client[DTO, Entity, ID any] struct {
//	ResourceURL string
//	Client      http.Client
//	Mapping     Mapping[DTO, Entity]
//	Serialisers Serializers[DTO, ID]
//}
//
//func (c Client[DTO, Entity, ID]) Create(ctx context.Context, ptr *Entity) error {
//
//	dto, err := c.Mapping.MapDTO(nil, *ptr)
//	if err != nil {
//		return err
//	}
//
//	var buf bytes.Buffer
//	if err := c.Serialisers.Body.Encoder(&buf).Encode(dto); err != nil {
//		return err
//	}
//
//	request, err := http.NewRequestWithContext(ctx, http.MethodPost, path.Clean(c.ResourceURL), &buf)
//	if err != nil {
//		return err
//	}
//
//	response, err := c.Client.Do(request)
//	if err != nil {
//		return err
//	}
//
//	responseBody, err := io.ReadAll(response.Body)
//
//	switch response.StatusCode {
//	case http.StatusOK, http.StatusAccepted, http.StatusCreated:
//	default:
//
//		if err != nil {
//			return fmt.Errorf("invalid response from %s, code:%d (+%w)", c.ResourceURL, response.StatusCode, err)
//		}
//		return fmt.Errorf("error occured during request.\nStatusCode:%d\nbody:\n\n%s", response.StatusCode, string(responseBody))
//	}
//
//	defer response.Body.Close()
//
//	responseDTO, found, err := iterators.First[DTO](c.Serialisers.Body.Decoder(response.Body))
//	if err != nil {
//		return err
//	}
//	if !found {
//		// TODO: might as well works with a simple DTO that only contains an ID field.
//		return fmt.Errorf("unexpected error occured by not finding a reply in the response body")
//	}
//
//	responseEntity, err := c.Mapping.MapEntity(panic("boom"), responseDTO)
//	if err != nil {
//		return err
//	}
//
//	id, ok := extid.Lookup[ID](responseEntity)
//	if !ok {
//		return fmt.Errorf("id not found in response DTO")
//	}
//
//	return extid.Set(ptr, id)
//}
//
//func (c Client[DTO, Entity, ID]) FindByID(ctx context.Context, id ID) (ent Entity, found bool, err error) {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (c Client[DTO, Entity, ID]) FindAll(ctx context.Context) iterators.Iterator[Entity] {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (c Client[DTO, Entity, ID]) Update(ctx context.Context, ptr *Entity) error {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (c Client[DTO, Entity, ID]) DeleteByID(ctx context.Context, id ID) error {
//	//TODO implement me
//	panic("implement me")
//}
//
//func (c Client[DTO, Entity, ID]) DeleteAll(ctx context.Context) error {
//	//TODO implement me
//	panic("implement me")
//}
//
//type I[Entity, ID any] interface {
//	crud.Creator[Entity]
//	crud.Finder[Entity, ID]
//	crud.Updater[Entity]
//	crud.Deleter[ID]
//}
