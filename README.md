# Go State Machine

A powerful state machine library for Go inspired by XState, featuring hierarchical states, parallel states, guards, actions, and event-driven architecture.

## Features

- **Core State Machine**: Basic state transitions with events
- **Guards**: Conditional transitions based on context
- **Actions**: Execute code on entry, exit, and transitions
- **Hierarchical States**: Nested state support with proper entry/exit handling
- **Parallel States**: Multiple regions running concurrently
- **Context Management**: Thread-safe state context
- **Event System**: Asynchronous event processing with listeners
- **Always Transitions**: Automatic transitions based on conditions

## Installation

```bash
go get github.com/user/go-state-machine
```

## Quick Start

```go
package main

import (
    "fmt"
    sm "github.com/user/go-state-machine"
)

func main() {
    config := sm.MachineConfig{
        ID:      "toggle",
        Initial: "off",
        Context: nil,
        States: map[string]*sm.StateNode{
            "off": {
                ID:   "off",
                Type: sm.AtomicState,
                On: map[string][]sm.Transition{
                    "TOGGLE": {{Target: "on"}},
                },
            },
            "on": {
                ID:   "on",
                Type: sm.AtomicState,
                On: map[string][]sm.Transition{
                    "TOGGLE": {{Target: "off"}},
                },
            },
        },
    }

    machine := sm.NewMachine(config)
    machine.Start()
    
    machine.Send(sm.Event{Type: "TOGGLE"})
    fmt.Println(machine.IsInState("on")) // true
    
    machine.Stop()
}
```

## Advanced Usage

### Guards and Actions

```go
config := sm.MachineConfig{
    ID:      "door",
    Initial: "closed",
    Context: &DoorContext{IsLocked: true},
    States: map[string]*sm.StateNode{
        "closed": {
            ID:   "closed",
            Type: sm.AtomicState,
            On: map[string][]sm.Transition{
                "OPEN": {
                    {
                        Target: "open",
                        Guard: func(ctx sm.Context, event sm.Event) bool {
                            dc := ctx.(*DoorContext)
                            return !dc.IsLocked
                        },
                    },
                },
                "UNLOCK": {
                    {
                        Target: "closed",
                        Actions: []sm.Action{
                            func(ctx sm.Context, event sm.Event) sm.Context {
                                dc := ctx.(*DoorContext)
                                dc.IsLocked = false
                                return dc
                            },
                        },
                    },
                },
            },
        },
        "open": {
            ID:   "open",
            Type: sm.AtomicState,
            Entry: []sm.Action{
                func(ctx sm.Context, event sm.Event) sm.Context {
                    fmt.Println("Door opened!")
                    return ctx
                },
            },
        },
    },
}
```

### Hierarchical States

```go
machine := sm.NewHierarchicalMachine(config)

// States can be nested
"idle": {
    ID:   "idle",
    Type: sm.CompoundState,
    Initial: "waiting",
    States: map[string]*sm.StateNode{
        "waiting": {
            ID:   "waiting",
            Type: sm.AtomicState,
        },
        "processing": {
            ID:   "processing",
            Type: sm.AtomicState,
        },
    },
}
```

### Parallel States

```go
machine := sm.NewParallelMachine(config)

// Define parallel regions
"working": {
    ID:   "working",
    Type: sm.ParallelState,
    States: map[string]*sm.StateNode{
        "region1": {
            ID:      "region1",
            Type:    sm.CompoundState,
            Initial: "task1",
            States:  /* ... */,
        },
        "region2": {
            ID:      "region2",
            Type:    sm.CompoundState,
            Initial: "task2",
            States:  /* ... */,
        },
    },
}
```

## Examples

See the `examples/` directory for complete examples:
- `traffic_light.go`: Traffic light system with emergency mode
- `elevator.go`: Elevator controller with hierarchical states

## Testing

```bash
go test ./...
```

## License

MIT