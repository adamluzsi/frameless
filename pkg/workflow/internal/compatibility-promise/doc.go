// Package compatibilitypromise holds the compatibility promise of the workflow package,
// written down as tests.
//
// Definitions and event histories are persisted, and they outlive the version that wrote them.
// A Definition stored as JSON must still mean the same Definition after an upgrade,
// and a Process that was in flight during an upgrade must resume on the new version
// without repeating any of its recorded work.
//
// Each scenario is a workflow written with the current API.
// Its fixture, testdata/<scenario>.json, is what an earlier version stored for it:
// the Definition encoded by the codec,
// and the event history of a Process that ran until the compat::await-upgrade participant suspended it.
// The history also holds the events of the children the Process spawned,
// since a child is persisted the same way, and has to resume after the upgrade too.
// The tests decode the fixture with the current version and check that:
//
//   - the stored Definition still decodes to the scenario's Definition,
//   - the stored history resumes, calls only what the scenario expects after the upgrade, and completes,
//   - the resumed Process ends with the variables the scenario expects,
//   - each spawned child ends with the variables the scenario expects for it.
//
// Variables of a nested scope, like a loop body or a deferred hook, are gone once their block ends,
// so a scenario checks them where they are read instead:
// the compat::see participant lists the value it was given among the calls.
//
// A promise can't be withdrawn, so every fixture has to belong to a scenario.
//
// # Adding a scenario
//
//  1. Append the scenario to the scenario table, built from the vocabulary of participants and conditions.
//     The vocabulary is append-only, since recorded histories refer to its IDs.
//  2. Record its fixture:
//     go test ./pkg/workflow/internal/compatibility-promise/ -record
//     Recording only writes the fixtures that are missing; it never overwrites one.
//  3. Commit the fixture.
//
// # Keeping the promise
//
// A committed fixture is never edited or recorded again,
// and neither are the recorded scenario's name, expected calls and expected variables.
// When a test fails after a change, the change breaks data that is already stored:
// fix the change, for example with a codec migration, rather than the fixture.
// A scenario's Definition may only be rewritten to follow an API rename,
// and it must still mean the same workflow.
package compatibilitypromise
