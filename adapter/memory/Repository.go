package memory

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"go.llib.dev/frameless/port/comproto"

	"go.llib.dev/frameless/pkg/errorkit"
	"go.llib.dev/frameless/port/crud"

	"go.llib.dev/frameless/pkg/reflectkit"
	"go.llib.dev/frameless/port/crud/extid"
	"go.llib.dev/frameless/port/iterators"
)

func NewRepository[ENT, ID any](m *Memory) *Repository[ENT, ID] {
	return &Repository[ENT, ID]{Memory: m}
}

func NewRepositoryWithNamespace[ENT, ID any](m *Memory, ns string) *Repository[ENT, ID] {
	return &Repository[ENT, ID]{Memory: m, Namespace: ns}
}

type Repository[ENT, ID any] struct {
	Memory    *Memory
	MakeID    func(context.Context) (ID, error)
	Namespace string

	IDA extid.Accessor[ENT, ID]

	// ExpectID [optional] is a test simulator flag that allows you to enforce the user of this Repository,
	// to populate the ID field of ENT prior to calling Create or Save, else the operation will fail.
	ExpectID bool
}

const ErrMissingID errorkit.Error = "ErrMissingID"

const typeNameRepository = "Repository"

func (s *Repository[ENT, ID]) Create(ctx context.Context, ptr *ENT) error {
	if _, ok := s.IDA.Lookup(*ptr); !ok {
		if s.ExpectID {
			return ErrMissingID
		}
		newID, err := s.mkID(ctx)
		if err != nil {
			return err
		}

		if err := s.IDA.Set(ptr, newID); err != nil {
			return err
		}
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	id, _ := s.IDA.Lookup(*ptr)
	if _, found, err := s.FindByID(ctx, id); err != nil {
		return err
	} else if found {
		return errorkit.With(crud.ErrAlreadyExists).
			Detailf(`%T already exists with id: %v`, *new(ENT), id).
			Context(ctx).
			Unwrap()
	}

	s.Memory.Set(ctx, getNamespaceFor[ENT](typeNameRepository, &s.Namespace), s.IDToMemoryKey(id), *ptr)
	return nil
}

func (s *Repository[ENT, ID]) Save(ctx context.Context, ptr *ENT) (rErr error) {
	ctx, err := s.Memory.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, s.Memory, ctx)

	id, ok := s.IDA.Lookup(*ptr)
	if !ok {
		if s.ExpectID {
			return ErrMissingID
		}
		return s.Create(ctx, ptr)
	}

	_, found, err := s.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if found {
		return s.Update(ctx, ptr)
	}
	return s.Create(ctx, ptr)
}

func (s *Repository[ENT, ID]) FindByID(ctx context.Context, id ID) (_ent ENT, _found bool, _err error) {
	if err := ctx.Err(); err != nil {
		return _ent, false, err
	}
	if err := s.isDoneTx(ctx); err != nil {
		return _ent, false, err
	}

	ent, ok := s.Memory.Get(ctx, getNamespaceFor[ENT](typeNameRepository, &s.Namespace), s.IDToMemoryKey(id))
	if !ok {
		return _ent, false, nil
	}
	return ent.(ENT), true, nil
}

func (s *Repository[ENT, ID]) FindAll(ctx context.Context) (iterators.Iterator[ENT], error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := s.isDoneTx(ctx); err != nil {
		return nil, err
	}
	return memoryAll[ENT](s.Memory, ctx, getNamespaceFor[ENT](typeNameRepository, &s.Namespace)), nil
}

func (s *Repository[ENT, ID]) DeleteByID(ctx context.Context, id ID) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.isDoneTx(ctx); err != nil {
		return err
	}
	if s.Memory.Del(ctx, getNamespaceFor[ENT](typeNameRepository, &s.Namespace), s.IDToMemoryKey(id)) {
		return nil
	}
	return errNotFound(*new(ENT), id)
}

func (s *Repository[ENT, ID]) DeleteAll(ctx context.Context) error {
	iter, err := s.FindAll(ctx)
	if err != nil {
		return err
	}
	defer iter.Close()
	for iter.Next() {
		id, _ := s.IDA.Lookup(iter.Value())
		_ = s.Memory.Del(ctx, getNamespaceFor[ENT](typeNameRepository, &s.Namespace), s.IDToMemoryKey(id))
	}
	return iter.Err()
}

func (s *Repository[ENT, ID]) Update(ctx context.Context, ptr *ENT) error {
	id, ok := s.IDA.Lookup(*ptr)
	if !ok {
		return fmt.Errorf(`entity doesn't have id field`)
	}

	_, found, err := s.FindByID(ctx, id)
	if err != nil {
		return err
	}
	if !found {
		return errNotFound(*new(ENT), id)
	}

	s.Memory.Set(ctx, getNamespaceFor[ENT](typeNameRepository, &s.Namespace), s.IDToMemoryKey(id), *ptr)
	return nil
}

func (s *Repository[ENT, ID]) FindByIDs(ctx context.Context, ids ...ID) (iterators.Iterator[ENT], error) {
	var m memoryActions = s.Memory
	if tx, ok := s.Memory.LookupTx(ctx); ok {
		m = tx
	}
	all := m.all(getNamespaceFor[ENT](typeNameRepository, &s.Namespace))
	var vs = make(map[string]ENT, len(ids))
	for _, id := range ids {
		key := s.IDToMemoryKey(id)
		v, ok := all[key]
		if !ok {
			return nil, errNotFound(*new(ENT), id)
		}
		vs[key] = v.(ENT)
	}
	return iterators.Slice[ENT](toSlice[ENT, string](vs)), nil
}

func (s *Repository[ENT, ID]) QueryOne(ctx context.Context, filter func(v ENT) bool) (ENT, bool, error) {
	iter, err := s.FindAll(ctx)
	if err != nil {
		return *new(ENT), false, err
	}
	iter = iterators.Filter(iter, filter)
	return iterators.First(iter)
}

func (s *Repository[ENT, ID]) QueryMany(ctx context.Context, filter func(v ENT) bool) (iterators.Iterator[ENT], error) {
	iter, err := s.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	return iterators.Filter(iter, filter), nil
}

func (s *Repository[ENT, ID]) mkID(ctx context.Context) (ID, error) {
	if s.MakeID != nil {
		return s.MakeID(ctx)
	}
	return MakeID[ID](ctx)
}

func (s *Repository[ENT, ID]) IDToMemoryKey(id any) string {
	return fmt.Sprintf(`%#v`, id)
}

func (s *Repository[ENT, ID]) isDoneTx(ctx context.Context) error {
	tx, ok := s.Memory.LookupTx(ctx)
	if !ok {
		return nil
	}
	if tx.done {
		return errTxDone
	}
	return nil
}

func (s *Repository[ENT, ID]) BeginTx(ctx context.Context) (context.Context, error) {
	return s.Memory.BeginTx(ctx)
}
func (s *Repository[ENT, ID]) CommitTx(ctx context.Context) error {
	return s.Memory.CommitTx(ctx)
}
func (s *Repository[ENT, ID]) RollbackTx(ctx context.Context) error {
	return s.Memory.RollbackTx(ctx)
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
		ctx = context.WithValue(ctx, m.ctxKeyMeta(), mm)
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
	return true, reflectkit.Link(v, ptr)
}

type memoryActions interface {
	all(namespace string) map[string]interface{}
	lookup(namespace string, key string) (interface{}, bool)
	set(namespace, key string, value interface{})
	del(namespace string, key string) bool
}

func base(ent any) interface{} {
	return reflectkit.BaseValueOf(ent).Interface()
}

func (m *Memory) Get(ctx context.Context, namespace string, key string) (interface{}, bool) {
	if tx, ok := m.LookupTx(ctx); ok && !tx.done {
		return tx.lookup(namespace, key)
	}
	return m.lookup(namespace, key)
}

func memoryAll[ENT any](m *Memory, ctx context.Context, namespace string) iterators.Iterator[ENT] {
	var T ENT
	return iterators.Slice[ENT](m.All(T, ctx, namespace).([]ENT))
}

func (m *Memory) All(T any, ctx context.Context, namespace string) (sliceOfT interface{}) {
	if tx, ok := m.LookupTx(ctx); ok && !tx.done {
		return m.toTSlice(T, tx.all(namespace))
	}
	return m.toTSlice(T, m.all(namespace))
}

func (m *Memory) toTSlice(T any, vs map[string]interface{}) interface{} {
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

func memoryLookup[ENT any](m *Memory, namespace string, key string) (ENT, bool) {
	ient, ok := m.lookup(namespace, key)
	if !ok {
		return *new(ENT), false
	}
	ent, ok := ient.(ENT)
	if !ok {
		return *new(ENT), false
	}
	return ent, true
}

func (m *Memory) lookup(namespace string, key string) (interface{}, bool) {
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
	tbl[key] = value
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

	cancelContext func()
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
	for k := range cvs.Deleted {
		delete(avs, k)
	}
	for k, v := range cvs.Values {
		avs[k] = v
	}
	return avs
}

func (tx *MemoryTx) lookup(namespace string, key string) (interface{}, bool) {
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
	return tx.super.lookup(namespace, key)
}

func (tx *MemoryTx) set(namespace, key string, value interface{}) {
	tx.m.Lock()
	defer tx.m.Unlock()
	tx.getChanges(namespace).Values[key] = value
}

func (tx *MemoryTx) del(namespace, key string) bool {
	if _, ok := tx.lookup(namespace, key); !ok {
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
	tx.cancelContext()
	for namespace, values := range tx.changes {
		for key := range values.Deleted {
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
	tx.cancelContext()
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
	if err := ctx.Err(); err != nil {
		return ctx, err
	}
	var super memoryActions = m
	if tx, ok := m.LookupTx(ctx); ok {
		super = tx
	}
	ctx, cancel := context.WithCancel(ctx)
	return context.WithValue(ctx, m.ctxKeyMemoryTx(), &MemoryTx{
		super:         super,
		cancelContext: cancel,
	}), nil
}

func (m *Memory) CommitTx(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if tx, ok := m.LookupTx(ctx); ok {
		return tx.commit()
	}
	return errNoTx
}

func (m *Memory) RollbackTx(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
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
