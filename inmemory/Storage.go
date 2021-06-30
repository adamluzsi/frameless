package inmemory

import (
	"context"
	"fmt"
	"github.com/adamluzsi/frameless"
	"github.com/adamluzsi/frameless/extid"
	"github.com/adamluzsi/frameless/iterators"
	"github.com/adamluzsi/frameless/reflects"
	"reflect"
	"sync"
)

func NewStorage(T frameless.T, m *Memory) *Storage {
	return &Storage{T: T, Memory: m}
}

func NewStorageWithNamespace(T frameless.T, m *Memory, ns string) *Storage {
	return &Storage{T: T, Memory: m, Namespace: ns}
}

type Storage struct {
	T             frameless.T
	Memory        *Memory
	NewID         func(context.Context) (interface{}, error)
	Namespace     string
	initNamespace sync.Once
}

func (s *Storage) Create(ctx context.Context, ptr interface{}) error {
	if _, ok := extid.Lookup(ptr); !ok {
		newID, err := s.newID(ctx)
		if err != nil {
			return err
		}

		if err := extid.Set(ptr, newID); err != nil {
			return err
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	id, _ := extid.Lookup(ptr)
	if found, err := s.FindByID(ctx, s.newT(), id); err != nil {
		return err
	} else if found {
		return fmt.Errorf(`%T already exists with id: %s`, s.T, id)
	}

	s.Memory.Set(ctx, s.GetNamespace(), s.IDToMemoryKey(id), base(ptr))
	return nil
}

func (s *Storage) FindByID(ctx context.Context, ptr, id interface{}) (found bool, err error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if err := s.isDoneTx(ctx); err != nil {
		return false, err
	}

	ent, ok := s.Memory.Get(ctx, s.GetNamespace(), s.IDToMemoryKey(id))
	if !ok {
		return false, nil
	}

	if err := reflects.Link(ent, ptr); err != nil {
		return false, err
	}

	return true, nil
}

func (s *Storage) FindAll(ctx context.Context) frameless.Iterator {
	if err := ctx.Err(); err != nil {
		return iterators.NewError(err)
	}
	if err := s.isDoneTx(ctx); err != nil {
		return iterators.NewError(err)
	}
	return s.Memory.All(s.T, ctx, s.GetNamespace())
}

func (s *Storage) DeleteByID(ctx context.Context, id interface{}) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.isDoneTx(ctx); err != nil {
		return err
	}
	if s.Memory.Del(ctx, s.GetNamespace(), s.IDToMemoryKey(id)) {
		return nil
	}
	return errNotFound(s.T, id)
}

func (s *Storage) DeleteAll(ctx context.Context) error {
	iter := s.FindAll(ctx)
	defer iter.Close()
	for iter.Next() {
		ptr := s.newT()
		if err := iter.Decode(ptr); err != nil {
			return err
		}

		id, _ := extid.Lookup(ptr)
		_ = s.Memory.Del(ctx, s.GetNamespace(), s.IDToMemoryKey(id))
	}
	return iter.Err()
}

func (s *Storage) Update(ctx context.Context, ptr interface{}) error {
	id, ok := extid.Lookup(ptr)
	if !ok {
		return fmt.Errorf(`entity doesn't have id field`)
	}

	found, err := s.FindByID(ctx, s.newT(), id)
	if err != nil {
		return err
	}
	if !found {
		return errNotFound(s.T, id)
	}

	s.Memory.Set(ctx, s.GetNamespace(), s.IDToMemoryKey(id), base(ptr))
	return nil
}

func (s *Storage) FindByIDs(ctx context.Context, ids ...interface{}) frameless.Iterator {
	var m memoryActions = s.Memory
	if tx, ok := s.Memory.LookupTx(ctx); ok {
		m = tx
	}
	all := m.all(s.GetNamespace())
	var vs = make(map[string]interface{}, len(ids))
	for _, id := range ids {
		key := s.IDToMemoryKey(id)
		v, ok := all[key]
		if !ok {
			return iterators.NewError(errNotFound(s.T, id))
		}
		vs[key] = v
	}
	return iterators.NewSlice(s.Memory.toTSlice(s.T, vs))
}

func (s *Storage) Upsert(ctx context.Context, ptrs ...interface{}) error {
	var m memoryActions = s.Memory
	if tx, ok := s.Memory.LookupTx(ctx); ok {
		m = tx
	}
	for _, ptr := range ptrs {
		id, ok := extid.Lookup(ptr)
		if !ok {
			nid, err := s.newID(ctx)
			if err != nil {
				return err
			}
			id = nid
			if err := extid.Set(ptr, nid); err != nil {
				return err
			}
		}
		key := s.IDToMemoryKey(id)
		m.set(s.GetNamespace(), key, base(ptr))
	}
	return nil
}

//func (s *Storage) SubscribeToCreate(ctx context.Context, subscriber frameless.Subscriber) (frameless.Subscription, error) {
//	panic("implement me")
//}
//
//func (s *Storage) SubscribeToUpdate(ctx context.Context, subscriber frameless.Subscriber) (frameless.Subscription, error) {
//	panic("implement me")
//}
//
//func (s *Storage) SubscribeToDeleteByID(ctx context.Context, subscriber frameless.Subscriber) (frameless.Subscription, error) {
//	panic("implement me")
//}
//
//func (s *Storage) SubscribeToDeleteAll(ctx context.Context, subscriber frameless.Subscriber) (frameless.Subscription, error) {
//	panic("implement me")
//}

func (s *Storage) newT() interface{} {
	return reflect.New(reflect.TypeOf(s.T)).Interface()
}

func (s *Storage) newID(ctx context.Context) (interface{}, error) {
	if s.NewID != nil {
		return s.NewID(ctx)
	}
	return newDummyID(s.T)
}

func (s *Storage) IDToMemoryKey(id frameless.T) string {
	return fmt.Sprintf(`%#v`, id)
}

func (s *Storage) GetNamespace() string {
	s.initNamespace.Do(func() {
		if 0 < len(s.Namespace) {
			return
		}
		s.Namespace = reflects.FullyQualifiedName(s.T)
	})
	return s.Namespace
}

func (s *Storage) getV(ptr interface{}) interface{} {
	return reflects.BaseValueOf(ptr).Interface()
}

func (s *Storage) isDoneTx(ctx context.Context) error {
	tx, ok := s.Memory.LookupTx(ctx)
	if !ok {
		return nil
	}
	if tx.done {
		return errTxDone
	}
	return nil
}

////////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

func NewMemory() *Memory {
	return &Memory{}
}

type Memory struct {
	m      sync.Mutex
	tables map[string]MemoryNamespace

	ns struct {
		init  sync.Once
		value string
	}
}

type (
	ctxKeyMemoryMeta   struct{ NS string }
	ctxValueMemoryMeta map[string]interface{}
)

func (m *Memory) ctxKeyMeta() ctxKeyMemoryMeta {
	return ctxKeyMemoryMeta{NS: m.getNS()}
}

func (m *Memory) lookupMetaMap(ctx context.Context) (ctxValueMemoryMeta, bool) {
	if ctx == nil {
		return nil, false
	}
	mm, ok := ctx.Value(m.ctxKeyMeta()).(ctxValueMemoryMeta)
	return mm, ok
}

func (m *Memory) SetMeta(ctx context.Context, key string, value interface{}) (context.Context, error) {
	if ctx == nil {
		return ctx, fmt.Errorf(`input context.Context was nil`)
	}
	mm, ok := m.lookupMetaMap(ctx)
	if !ok {
		mm = make(ctxValueMemoryMeta)
		ctx = context.WithValue(ctx, m.ctxKeyMeta(), m)
	}
	mm[key] = base(value)
	return ctx, nil
}

func (m *Memory) LookupMeta(ctx context.Context, key string, ptr interface{}) (_found bool, _err error) {
	if ctx == nil {
		return false, nil
	}
	mm, ok := m.lookupMetaMap(ctx)
	if !ok {
		return false, nil
	}
	v, ok := mm[key]
	if !ok {
		return false, nil
	}
	return true, reflects.Link(v, ptr)
}

type memoryActions interface {
	all(namespace string) map[string]interface{}
	get(namespace string, key string) (interface{}, bool)
	set(namespace, key string, value interface{})
	del(namespace string, key string) bool
}

func base(ent frameless.T) interface{} {
	return reflects.BaseValueOf(ent).Interface()
}

func (m *Memory) Get(ctx context.Context, namespace string, key string) (interface{}, bool) {
	if tx, ok := m.LookupTx(ctx); ok && !tx.done {
		return tx.get(namespace, key)
	}
	return m.get(namespace, key)
}

func (m *Memory) All(T frameless.T, ctx context.Context, namespace string) frameless.Iterator {
	if tx, ok := m.LookupTx(ctx); ok && !tx.done {
		return iterators.NewSlice(m.toTSlice(T, tx.all(namespace)))
	}
	return iterators.NewSlice(m.toTSlice(T, m.all(namespace)))
}

func (m *Memory) toTSlice(T frameless.T, vs map[string]interface{}) interface{} {
	rslice := reflect.MakeSlice(reflect.SliceOf(reflect.TypeOf(T)), 0, len(vs))
	for _, v := range vs {
		rslice = reflect.Append(rslice, reflect.ValueOf(v))
	}
	return rslice.Interface()
}

func (m *Memory) all(namespace string) map[string]interface{} {
	m.m.Lock()
	defer m.m.Unlock()
	var vs = make(map[string]interface{})
	for k, v := range m.namespace(namespace) {
		vs[k] = v
	}
	return vs
}

func (m *Memory) get(namespace string, key string) (interface{}, bool) {
	m.m.Lock()
	defer m.m.Unlock()
	ns := m.namespace(namespace)
	v, ok := ns[key]
	return v, ok
}

func (m *Memory) Set(ctx context.Context, namespace, key string, value interface{}) {
	if tx, ok := m.LookupTx(ctx); ok {
		tx.set(namespace, key, value)
		return
	}

	m.set(namespace, key, value)
}

func (m *Memory) set(namespace, key string, value interface{}) {
	m.m.Lock()
	defer m.m.Unlock()
	tbl := m.namespace(namespace)
	tbl[key] = base(value)
	return
}

func (m *Memory) Del(ctx context.Context, namespace string, key string) bool {
	if tx, ok := m.LookupTx(ctx); ok {
		return tx.del(namespace, key)
	}
	return m.del(namespace, key)
}

func (m *Memory) del(namespace string, key string) bool {
	m.m.Lock()
	defer m.m.Unlock()
	ns := m.namespace(namespace)
	if _, ok := ns[key]; !ok {
		return false
	}
	delete(ns, key)
	return true
}

type MemoryNamespace map[string]interface{}

func (m *Memory) namespace(name string) MemoryNamespace {
	if m.tables == nil {
		m.tables = make(map[string]MemoryNamespace)
	}
	if _, ok := m.tables[name]; !ok {
		m.tables[name] = make(MemoryNamespace)
	}
	return m.tables[name]
}

type MemoryTx struct {
	m       sync.Mutex
	done    bool
	super   memoryActions
	changes map[string]memoryTxChanges
}

type memoryTxChanges struct {
	Values  MemoryNamespace
	Deleted map[string]struct{}
}

func (tx *MemoryTx) all(namespace string) map[string]interface{} {
	tx.m.Lock()
	defer tx.m.Unlock()
	svs := tx.super.all(namespace)
	cvs := tx.getChanges(namespace)
	avs := make(map[string]interface{})
	for k, v := range svs {
		avs[k] = v
	}
	for k, _ := range cvs.Deleted {
		delete(avs, k)
	}
	for k, v := range cvs.Values {
		avs[k] = v
	}
	return avs
}

func (tx *MemoryTx) get(namespace string, key string) (interface{}, bool) {
	tx.m.Lock()
	defer tx.m.Unlock()
	changes := tx.getChanges(namespace)
	v, ok := changes.Values[key]
	if ok {
		return v, ok
	}
	if _, isDeleted := changes.Deleted[key]; isDeleted {
		return nil, false
	}
	return tx.super.get(namespace, key)
}

func (tx *MemoryTx) set(namespace, key string, value interface{}) {
	tx.m.Lock()
	defer tx.m.Unlock()
	tx.getChanges(namespace).Values[key] = value
}

func (tx *MemoryTx) del(namespace, key string) bool {
	if _, ok := tx.get(namespace, key); !ok {
		return false
	}
	tx.m.Lock()
	defer tx.m.Unlock()
	changes := tx.getChanges(namespace)
	delete(changes.Values, key)
	changes.Deleted[key] = struct{}{}
	return true
}

func (tx *MemoryTx) commit() error {
	if tx.done {
		return errTxDone
	}
	tx.m.Lock()
	defer tx.m.Unlock()
	tx.done = true
	for namespace, values := range tx.changes {
		for key, _ := range values.Deleted {
			tx.super.del(namespace, key)
		}
		for key, value := range values.Values {
			tx.super.set(namespace, key, value)
		}
	}
	return nil
}

func (tx *MemoryTx) rollback() error {
	if tx.done {
		return errTxDone
	}
	tx.done = true
	super, ok := tx.super.(*MemoryTx)
	if !ok {
		return nil
	}
	// We rollback recursively because most resource don't support multi level transactions
	// and I would like to avoid supporting it here until I have proper use-case with it.
	// Then adding a flag to turn off this behavior should be easy-peasy.
	return super.rollback()
}

func (tx *MemoryTx) getChanges(name string) memoryTxChanges {
	if tx.changes == nil {
		tx.changes = make(map[string]memoryTxChanges)
	}
	if _, ok := tx.changes[name]; !ok {
		tx.changes[name] = memoryTxChanges{
			Values:  make(MemoryNamespace),
			Deleted: make(map[string]struct{}),
		}
	}
	return tx.changes[name]
}

func (m *Memory) getNS() string {
	m.ns.init.Do(func() { m.ns.value = genStringUID() })
	return m.ns.value
}

type ctxKeyMemoryTx struct{ NS string }

func (m *Memory) ctxKeyMemoryTx() ctxKeyMemoryTx {
	return ctxKeyMemoryTx{NS: m.getNS()}
}

func (m *Memory) BeginTx(ctx context.Context) (context.Context, error) {
	var super memoryActions = m
	if tx, ok := m.LookupTx(ctx); ok {
		super = tx
	}
	return context.WithValue(ctx, m.ctxKeyMemoryTx(), &MemoryTx{super: super}), nil
}

func (m *Memory) CommitTx(ctx context.Context) error {
	if tx, ok := m.LookupTx(ctx); ok {
		return tx.commit()
	}
	return errNoTx
}

func (m *Memory) RollbackTx(ctx context.Context) error {
	if tx, ok := m.LookupTx(ctx); ok {
		return tx.rollback()
	}
	return errNoTx
}

func (m *Memory) LookupTx(ctx context.Context) (*MemoryTx, bool) {
	if ctx == nil {
		return nil, false
	}
	tx, ok := ctx.Value(m.ctxKeyMemoryTx()).(*MemoryTx)
	return tx, ok
}
