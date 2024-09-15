package migration

import (
	"context"
	"fmt"

	"go.llib.dev/frameless/port/comproto"
	"go.llib.dev/frameless/port/crud"
)

type Migrator[R resourceConnection] struct {
	// Resource is the ResourceConnection where the migration steps needs to operate.
	Resource R
	// Namespace field denotes the intended domain or area, such as "foo-repository",
	// ensuring that migration steps are uniquely identified
	// and do not interfere with migrations from other namespaces.
	Namespace string
	// Steps field is an ordered list of individual migration actions,
	// each capable of migrating up or down (applying or rolling back changes).
	// The generic type parameter `R` allows the migration steps to work with a shared resource,
	// which could be something like a live database connection or a transaction object.
	// This design provides flexibility and consistency,
	// ensuring that all steps in a given migration group have access to necessary shared resources.
	Steps Steps[R]
	// StateRepository is the crud resource that contains the state of the migration steps.
	// The state repository doesn't have to be the same as the actual resource which is being managed.
	StateRepository StateRepository
	// EnsureStateRepository is an optional field that is guaranteed to be called before each migration.
	// If your StateRepository is not needing this, because it creates itself upon interaction,
	// then just leave this unset.
	//
	// EnsureStateRepository should be an idempotent function,
	// calling it multiple times should result at the same final state.
	EnsureStateRepository func(context.Context) error
}

type resourceConnection interface {
	comproto.OnePhaseCommitProtocol
}

type StateRepository interface {
	comproto.OnePhaseCommitProtocol
	crud.Creator[State]
	crud.ByIDFinder[State, StateID]
	crud.ByIDDeleter[StateID]
}

// State is the representation of a migration Step's outcome.
type State struct {
	ID StateID `ext:"id"`
	// Dirty will tells if the given migration step's state
	Dirty bool
}

type StateID struct {
	Namespace string
	Version   Version
}

func (m Migrator[R]) validate(ctx context.Context) error {
	if m.Namespace == "" {
		return fmt.Errorf("missing namespace")
	}
	if m.StateRepository == nil {
		return fmt.Errorf("Migrator.StateRepository is not supplied")
	}
	var (
		isPrevOK bool
		prev     State
	)
	for i, stage := range m.Steps.stages() {
		state, ok, err := m.StateRepository.FindByID(ctx, StateID{Namespace: m.Namespace, Version: stage.Version})
		if err != nil {
			return err
		}
		if ok && !isPrevOK && i != 0 {
			const format = "[%s] migration step is missing: %s.\nThis step should exist because the following migration step has already been completed: %s"
			return fmt.Errorf(format, prev.ID.Namespace, prev.ID.Version, state.ID.Version)
		}
		prev = state
		isPrevOK = ok
	}
	return nil
}

func (m Migrator[R]) Migrate(ctx context.Context) (rErr error) {
	if m.EnsureStateRepository != nil {
		if err := m.EnsureStateRepository(ctx); err != nil {
			return err
		}
	}

	if err := m.validate(ctx); err != nil {
		return err
	}

	schemaCTX, err := m.StateRepository.BeginTx(ctx) // &sql.TxOptions{Isolation: sql.LevelSerializable}
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, m.StateRepository, schemaCTX)

	stepCTX, err := m.Resource.BeginTx(ctx)
	if err != nil {
		return err
	}
	defer comproto.FinishOnePhaseCommit(&rErr, m.Resource, stepCTX)

	for _, stage := range m.Steps.stages() {
		if err := m.up(schemaCTX, stepCTX, stage.Version, stage.Step); err != nil {
			return err
		}
	}

	return nil
}

func (m Migrator[R]) up(schemaTx, stepTx context.Context, version Version, step Step[R]) error {
	stateID := StateID{Namespace: m.Namespace, Version: version}
	state, ok, err := m.StateRepository.FindByID(schemaTx, stateID)
	if err != nil {
		return err
	}
	if !ok {
		if err := step.MigrateUp(m.Resource, stepTx); err != nil {
			return fmt.Errorf("error with MigrateUp %s/%s: %w", m.Namespace, version, err)
		}
		return m.StateRepository.Create(schemaTx, &State{ID: stateID, Dirty: false})
	}
	if state.Dirty {
		return fmt.Errorf("namespace:%q / version:%q is in a dirty state", stateID.Namespace, stateID.Version)
	}
	return nil
}
