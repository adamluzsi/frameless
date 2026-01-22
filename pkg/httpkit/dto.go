package httpkit

// func WithDTOForCodecC[ENT, DTO any](c RESTClientCodec[DTO], m dtokit.MapperTo[ENT, DTO]) RESTClientCodec[ENT] {
// 	return dtoCodecPipeline[ENT, DTO]{
// 		clientC: c,
// 		mapping: m,
// 	}
// }

// func WithDTOForCodecH[ENT, DTO any](h RESTHandlerCodec[DTO], m dtokit.MapperTo[ENT, DTO]) RESTHandlerCodec[ENT] {
// 	return dtoCodecPipeline[ENT, DTO]{
// 		handlerC: h,
// 		mapping:  m,
// 	}
// }

// type dtoCodecPipeline[ENT, DTO any] struct {
// 	clientC  RESTClientCodec[DTO]
// 	handlerC RESTHandlerCodec[DTO]
// 	mapping  dtokit.MapperTo[ENT, DTO]
// }

// func (m dtoCodecPipeline[ENT, DTO]) codec() MediaTypeCodec[DTO] {
// 	if m.clientC != nil {
// 		return m.clientC
// 	}
// 	if m.handlerC != nil {
// 		return m.handlerC
// 	}
// 	panic("implementation error")
// }

// func (m dtoCodecPipeline[ENT, DTO]) SupporsMediaType(mediaType string) bool {
// 	return m.codec().SupporsMediaType(mediaType)
// }

// func (m dtoCodecPipeline[ENT, DTO]) Marshal(v ENT) ([]byte, error) {
// 	dto, err := m.mapping.MapToDTO(context.Background(), v)
// 	if err != nil {
// 		return nil, err
// 	}
// 	return m.codec().Marshal(dto)
// }

// func (m dtoCodecPipeline[ENT, DTO]) Unmarshal(data []byte, p *ENT) error {
// 	var dto DTO
// 	if err := m.codec().Unmarshal(data, &dto); err != nil {
// 		return err
// 	}
// 	ent, err := m.mapping.MapToENT(context.Background(), dto)
// 	if err != nil {
// 		return err
// 	}
// 	*p = ent
// 	return nil
// }

// func (m dtoCodecPipeline[ENT, DTO]) NewListEncoder(w io.Writer) codec.StreamEncoder[ENT] {
// 	enc := m.handlerC.NewListEncoder(w)
// 	return &dtoStreamEncoder[ENT, DTO]{StreamEncoder: enc}
// }

// type dtoStreamEncoder[ENT, DTO any] struct {
// 	m dtoCodecPipeline[ENT, DTO]
// 	codec.StreamEncoder[DTO]
// }

// func (streamMapping *dtoStreamEncoder[ENT, DTO]) Encode(v ENT) error {
// 	dto, err := streamMapping.m.mapping.MapToDTO(context.Background(), v)
// 	if err != nil {
// 		return err
// 	}
// 	return streamMapping.StreamEncoder.Encode(dto)
// }

// func (m dtoCodecPipeline[ENT, DTO]) NewListDecoder(w io.Reader) codec.StreamDecoder[ENT] {
// 	return func(yield func(codec.Decoder[ENT], error) bool) {
// 		var ctx = context.Background()
// 		for dtoDec, err := range m.clientC.NewListDecoder(w) {
// 			if err != nil {
// 				if !yield(nil, err) {
// 					return
// 				}
// 				continue
// 			}
// 			var entDec = codec.DecoderFunc[ENT](func(p *ENT) error {
// 				var dto DTO
// 				if err := dtoDec.Decode(&dto); err != nil {
// 					return err
// 				}
// 				ent, err := m.mapping.MapToENT(ctx, dto)
// 				if err != nil {
// 					return err
// 				}
// 				*p = ent
// 				return nil
// 			})
// 			if !yield(entDec, nil) {
// 				return
// 			}
// 		}

// 	}
// }
