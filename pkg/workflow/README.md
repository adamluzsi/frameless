# Workflow

## Use-cases

### linear workflow

Linear workflow where a long process is asynchronously executed, and eventually reach a final state.
The workflow goes through the tasks of the process definition, until completion.

### Event based workflow

An event based workflow is similar to a linear one, but it has points in its task,
which needs to be triggered by an external event, and until that, the workflow remains dormant.
