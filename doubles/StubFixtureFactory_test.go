package doubles_test

//func TestStubFixtureFactory_smoke(t *testing.T) {
//	m := inmemory.NewMemory()
//
//	var (
//		beginErr    = fmt.Errorf("BeginTxFunc")
//		commitErr   = fmt.Errorf("CommitTxFunc")
//
//	)
//	type T struct {V int }
//	stub := &doubles.StubFixtureFactory{
//		FixtureFactory: TestFixtureFactory{},
//		StubContext: func() context.Context {
//			return context.WithValue(context.Background(), `key`, `value`)
//		},
//		StubCreate: func(T interface{}) interface{} {
//			return
//		},
//	}
//
//	ctx := context.Background()
//
//	t.Run(`commit with embedded`, func(t *testing.T) {
//		tx, err := stub.BeginTx(ctx)
//		require.NoError(t, err)
//		m.Set(tx, `ns`, `key`, `value`)
//		t.Cleanup(func() { m.Del(ctx, `ns`, `key`) })
//		_, ok := m.Get(ctx, `ns`, `key`)
//		require.False(t, ok)
//		_, ok = m.Get(tx, `ns`, `key`)
//		require.True(t, ok)
//		require.NoError(t, stub.CommitTx(tx))
//		_, ok = m.Get(ctx, `ns`, `key`)
//		require.True(t, ok)
//	})
//
//	t.Run(`rollback with embedded`, func(t *testing.T) {
//		tx, err := stub.BeginTx(ctx)
//		require.NoError(t, err)
//		m.Set(tx, `ns`, `key`, `value`)
//		_, ok := m.Get(ctx, `ns`, `key`)
//		require.False(t, ok)
//		_, ok = m.Get(tx, `ns`, `key`)
//		require.True(t, ok)
//		require.NoError(t, stub.RollbackTx(tx))
//		_, ok = m.Get(ctx, `ns`, `key`)
//		require.False(t, ok)
//	})
//}
//
//type TestFixtureFactory struct {
//}
//
//func (t TestFixtureFactory) Create(T interface{}) interface{} { return T }
//
//func (t TestFixtureFactory) Context() context.Context { return context.Background() }
