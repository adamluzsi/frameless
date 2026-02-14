# Workflow

**Goals**:

- A Workflow Engine's primary focus should be to enable the interpretation and scheduling of end-user-defined logic
- Keep logic in code, not YAML files or external scripts (Code Over Configuration).
  - Avoid of forcing context-switching, development should focus on codebase, and operations should focus on the orcestration of user data, no mixture, like the need to work with a service in containers, to develop and test workflows.
- Avoid forcing retry logic, error handling, or resilience at the workflow engine level — let developers implement these with their choice of libraries, but leave space if they want to use the workflow to orchestrate retries or error handling.
- Prioritise local development (with Dev/Prod parity) to avoid DX dependencies on potentially long CI/CD pipelines.
- Built-in way to help observability of workflows, without the need of external tools or complex logging setups.
- Workflows must be version-controlled with migration tooling to avoid brittle production setups.
- API drift and schema changes silently break automations, consuming more time troubleshooting than developing.
  - Developing new versions of participants should be easy with backwards compatibility.
- Integration challenges arise from incompatible data formats and protocols across systems.
- Static definition analyisis to avoid delegating issues till runtime execution.
- Enable easy testing, including time-skipping, and other simulation related activities
- Easy way to launch workflows, simple API for an easy to start DX experience.
- Contract-driven testing for definitions and participants should replace the need for manual best practice enforcement.


A package `workflow` is a workflow engine that provides solution to two main workflow engine responsabilities:

- framework for building serialisable process definitions
  - enabling business users to create executable processes as data
- execution orchestration

## What is a Workflow Engine?

At its core, a workflow engine is a system that manages the execution of business processes by orchestrating tasks, routing work to appropriate participants, and monitoring process flow according to predefined rules. However, modern workflow engines can be understood through two distinct but complementary responsibilities:

### 1. **Process Definition Layer** (Executable Data)

This layer enables **serializable process definitions** that can be treated as data rather than hardcoded business logic. Process definitions become configuration artifacts that can be:

- **Created by business users** through visual designers or domain-specific languages
- **Stored, versioned, and managed** like any other data asset
- **Dynamically modified** without requiring code changes or deployments
- **Serialized to standard formats** (JSON, XML, YAML) for portability and integration

### 2. **Execution Orchestration Layer** (Advanced Task Management)

This layer handles the runtime aspects of workflow execution, including:

- **Task scheduling and coordination** across distributed systems
- **State management and persistence** for long-running processes
- **Event-driven coordination** and message correlation
- **Error handling and recovery** mechanisms
- **Participant management** and work distribution

## The Architectural Separation

By treating these as separate responsibilities, organizations gain significant architectural flexibility:

**The real value proposition** lies in the Definition Layer - enabling users to create executable process definitions without programming expertise. The Execution Layer, while sophisticated, is fundamentally an advanced task management system that can be scaled according to organizational needs.

## Execution Complexity Spectrum

Depending on your organization's requirements, the execution layer can range from simple to complex:

### **Simple: Task Queue Approach**

- Basic FIFO job processing
- Simple retry mechanisms
- Minimal state management
- Suitable for straightforward sequential workflows

### **Intermediate: Enhanced Scheduling**

- Priority-based task execution
- Conditional branching and parallel execution
- Basic event handling
- Timer-based triggers and deadlines

### **Advanced: Full Orchestration**

- Complex state machines with persistence
- Event-driven architecture with message correlation
- Distributed execution across multiple services
- Comprehensive error handling and compensation patterns
- Real-time monitoring and analytics

## Package Overview

This Go workflow package implements both layers with a clean separation of concerns:

### Core Types

**Process Definition Types:**

- `ProcessDefinition` - Interface for executable process definitions
- `ParticipantID` - References to executable participants
- `Sequence` - Sequential execution of multiple definitions
- `If` - Conditional branching with template-based conditions
- `ConditionTemplate` - Template-based condition evaluation

**Execution Types:**

- `Runtime` - Orchestrates process execution with configurable backends
- `ProcessQueue` - Pluggable interface for task scheduling (simple queue to complex orchestration)
- `State` - Maintains process variables and execution context
- `Participants` - Registry of executable participants

### Key Features

#### **Serializable Definitions**

All process definitions implement JSON serialization, enabling:

```go
// Define a process
pdef := &workflow.If{
    Cond: workflow.NewConditionTemplate(`eq .Status "pending"`),
    Then: &workflow.Sequence{
        workflow.PID("validate"),
        workflow.PID("process"),
        workflow.PID("notify"),
    },
    Else: workflow.PID("reject"),
}

// Serialize to JSON
data, _ := json.Marshal(pdef)

// Store, transmit, or persist as needed
// Later: deserialize and execute
```

#### **Template-Based Conditions**
Process logic can be expressed using Go templates with custom functions:
```go
ctx = workflow.ContextWithFuncMap(ctx, workflow.TemplateFuncMap{
    "isValid": func(status string) bool { 
        return status == "approved" 
    },
})

condition := workflow.NewConditionTemplate(`and (eq .Type "order") (isValid .Status)`)
```

#### **Pluggable Execution**
The execution layer uses interfaces that can be implemented with varying complexity:
```go
// Simple in-memory execution
runtime := workflow.Runtime{
    Participants: participants,
    // Queue: can be a simple channel or complex distributed system
}

// Or integrate with existing job queues, message brokers, etc.
```

## Evolution and Extensibility

The separated architecture makes evolving each layer independently straightforward:

### **Definition Layer Evolution**
- Add new process definition types (loops, parallel execution, etc.)
- Enhance template capabilities with new functions
- Integrate with visual process designers
- Support additional serialization formats

### **Execution Layer Evolution**
- Upgrade from simple queues to sophisticated orchestrators
- Add monitoring and observability features  
- Implement distributed execution patterns
- Integrate with existing infrastructure (Kubernetes, cloud services, etc.)

This separation ensures that improvements to scheduling and execution don't require changes to process definitions, and vice versa - making evolution a "low hanging fruit" rather than a major architectural undertaking.

## Getting Started

```go
// Define participants (the actual work)
participants := workflow.Participants{
    "validate": workflow.ParticipantFunc(func(ctx context.Context, s *workflow.State) error {
        // validation logic
        return nil
    }),
    "process": workflow.ParticipantFunc(func(ctx context.Context, s *workflow.State) error {
        // processing logic  
        return nil
    }),
}

// Create executable process definition
pdef := &workflow.Sequence{
    workflow.PID("validate"),
    workflow.PID("process"),
}

// Set up runtime
runtime := workflow.Runtime{
    Participants: participants,
}

// Execute process
process := workflow.Process{
    PDEF:  pdef,
    State: workflow.NewState(),
}

err := runtime.Execute(ctx, process)
```

The true power emerges when these process definitions come from external sources - configuration files, databases, or user interfaces - rather than being hardcoded, enabling non-technical users to define and modify business processes dynamically.