package statemachine

import (
	"testing"
	"time"
)

func TestHierarchicalStateMachine(t *testing.T) {
	config := MachineConfig{
		ID:      "hierarchical-test",
		Initial: "power",
		Context: &TestContext{Counter: 0},
		States: map[string]*StateNode{
			"power": {
				ID:      "power",
				Type:    CompoundState,
				Initial: "off",
				States: map[string]*StateNode{
					"off": {
						ID:   "off",
						Type: AtomicState,
						On: map[string][]Transition{
							"POWER_ON": {{Target: "on"}},
						},
					},
					"on": {
						ID:      "on",
						Type:    CompoundState,
						Initial: "idle",
						Entry: []Action{
							func(ctx Context, event Event) Context {
								tc := ctx.(*TestContext)
								tc.Counter++
								return tc
							},
						},
						States: map[string]*StateNode{
							"idle": {
								ID:   "idle",
								Type: AtomicState,
								On: map[string][]Transition{
									"START_WORK": {{Target: "working"}},
								},
							},
							"working": {
								ID:   "working",
								Type: AtomicState,
								On: map[string][]Transition{
									"FINISH_WORK": {{Target: "idle"}},
								},
							},
						},
						On: map[string][]Transition{
							"POWER_OFF": {{Target: "off"}},
						},
					},
				},
			},
		},
	}

	machine := NewHierarchicalMachine(config)
	if err := machine.Start(); err != nil {
		t.Fatalf("Failed to start machine: %v", err)
	}
	defer machine.Stop()

	hierarchy := machine.GetStateHierarchy()
	if hierarchy != "root.power.off" {
		t.Errorf("Expected initial hierarchy 'root.power.off', got '%s'", hierarchy)
	}

	machine.Send(Event{Type: "POWER_ON"})
	time.Sleep(10 * time.Millisecond)

	if !machine.IsInState("idle") {
		t.Error("Machine should be in idle state after POWER_ON")
	}

	tc := machine.GetContext().(*TestContext)
	if tc.Counter != 1 {
		t.Errorf("Counter should be 1 after entering 'on' state, got %d", tc.Counter)
	}

	machine.Send(Event{Type: "START_WORK"})
	time.Sleep(10 * time.Millisecond)

	if !machine.IsInState("working") {
		t.Error("Machine should be in working state")
	}

	machine.Send(Event{Type: "POWER_OFF"})
	time.Sleep(10 * time.Millisecond)

	if !machine.IsInState("off") {
		t.Error("Machine should be in off state after POWER_OFF")
	}
}

func TestHierarchicalStatePathResolution(t *testing.T) {
	config := MachineConfig{
		ID:      "path-test",
		Initial: "level1",
		Context: &TestContext{},
		States: map[string]*StateNode{
			"level1": {
				ID:      "level1",
				Type:    CompoundState,
				Initial: "level2",
				States: map[string]*StateNode{
					"level2": {
						ID:      "level2",
						Type:    CompoundState,
						Initial: "level3",
						States: map[string]*StateNode{
							"level3": {
								ID:   "level3",
								Type: AtomicState,
							},
						},
					},
				},
			},
		},
	}

	machine := NewHierarchicalMachine(config)
	machine.Start()
	defer machine.Stop()

	time.Sleep(10 * time.Millisecond)

	hierarchy := machine.GetStateHierarchy()
	if hierarchy != "root.level1.level2.level3" {
		t.Errorf("Expected hierarchy 'root.level1.level2.level3', got '%s'", hierarchy)
	}
}

func TestHierarchicalEventBubbling(t *testing.T) {
	var handledAt string

	config := MachineConfig{
		ID:      "bubbling-test",
		Initial: "parent",
		Context: &TestContext{},
		States: map[string]*StateNode{
			"parent": {
				ID:      "parent",
				Type:    CompoundState,
				Initial: "child",
				On: map[string][]Transition{
					"BUBBLE_EVENT": {
						{
							Target: "parent",
							Actions: []Action{
								func(ctx Context, event Event) Context {
									handledAt = "parent"
									return ctx
								},
							},
						},
					},
				},
				States: map[string]*StateNode{
					"child": {
						ID:      "child",
						Type:    CompoundState,
						Initial: "grandchild",
						States: map[string]*StateNode{
							"grandchild": {
								ID:   "grandchild",
								Type: AtomicState,
							},
						},
					},
				},
			},
		},
	}

	machine := NewHierarchicalMachine(config)
	machine.Start()
	defer machine.Stop()

	time.Sleep(10 * time.Millisecond)

	machine.Send(Event{Type: "BUBBLE_EVENT"})
	time.Sleep(10 * time.Millisecond)

	if handledAt != "parent" {
		t.Errorf("Expected event to be handled at parent level, but was handled at '%s'", handledAt)
	}
}

func TestHierarchicalEntryExitOrder(t *testing.T) {
	var actionOrder []string

	addAction := func(name string) Action {
		return func(ctx Context, event Event) Context {
			actionOrder = append(actionOrder, name)
			return ctx
		}
	}

	config := MachineConfig{
		ID:      "entry-exit-test",
		Initial: "a",
		Context: &TestContext{},
		States: map[string]*StateNode{
			"a": {
				ID:      "a",
				Type:    CompoundState,
				Initial: "a1",
				Entry:   []Action{addAction("a-entry")},
				Exit:    []Action{addAction("a-exit")},
				States: map[string]*StateNode{
					"a1": {
						ID:    "a1",
						Type:  AtomicState,
						Entry: []Action{addAction("a1-entry")},
						Exit:  []Action{addAction("a1-exit")},
						On: map[string][]Transition{
							"TO_B": {{Target: "b"}},
						},
					},
				},
			},
			"b": {
				ID:      "b",
				Type:    CompoundState,
				Initial: "b1",
				Entry:   []Action{addAction("b-entry")},
				Exit:    []Action{addAction("b-exit")},
				States: map[string]*StateNode{
					"b1": {
						ID:    "b1",
						Type:  AtomicState,
						Entry: []Action{addAction("b1-entry")},
						Exit:  []Action{addAction("b1-exit")},
					},
				},
			},
		},
	}

	machine := NewHierarchicalMachine(config)
	machine.Start()
	defer machine.Stop()

	time.Sleep(10 * time.Millisecond)
	actionOrder = []string{} // Reset after initial entry

	machine.Send(Event{Type: "TO_B"})
	time.Sleep(10 * time.Millisecond)

	expected := []string{"a1-exit", "a-exit", "b-entry", "b1-entry"}
	if len(actionOrder) != len(expected) {
		t.Fatalf("Expected %d actions, got %d", len(expected), len(actionOrder))
	}

	for i, action := range expected {
		if actionOrder[i] != action {
			t.Errorf("Expected action %s at position %d, got %s", action, i, actionOrder[i])
		}
	}
}