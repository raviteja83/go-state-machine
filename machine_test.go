package statemachine

import (
	"sync"
	"testing"
	"time"
)

type TestContext struct {
	Counter int
	Flag    bool
	Data    string
}

func TestBasicStateMachine(t *testing.T) {
	config := MachineConfig{
		ID:      "test-machine",
		Initial: "idle",
		Context: &TestContext{Counter: 0},
		States: map[string]*StateNode{
			"idle": {
				ID:   "idle",
				Type: AtomicState,
				On: map[string][]Transition{
					"START": {{Target: "running"}},
				},
			},
			"running": {
				ID:   "running",
				Type: AtomicState,
				Entry: []Action{
					func(ctx Context, event Event) Context {
						tc := ctx.(*TestContext)
						tc.Counter++
						return tc
					},
				},
				On: map[string][]Transition{
					"STOP": {{Target: "idle"}},
				},
			},
		},
	}

	machine := NewMachine(config)
	if err := machine.Start(); err != nil {
		t.Fatalf("Failed to start machine: %v", err)
	}
	defer machine.Stop()

	if !machine.IsInState("idle") {
		t.Error("Machine should start in idle state")
	}

	machine.Send(Event{Type: "START"})
	time.Sleep(10 * time.Millisecond)

	if !machine.IsInState("running") {
		t.Error("Machine should be in running state after START event")
	}

	tc := machine.GetContext().(*TestContext)
	if tc.Counter != 1 {
		t.Errorf("Counter should be 1, got %d", tc.Counter)
	}
}

func TestGuards(t *testing.T) {
	config := MachineConfig{
		ID:      "guard-test",
		Initial: "idle",
		Context: &TestContext{Flag: false},
		States: map[string]*StateNode{
			"idle": {
				ID:   "idle",
				Type: AtomicState,
				On: map[string][]Transition{
					"GO": {
						{
							Target: "active",
							Guard: func(ctx Context, event Event) bool {
								tc := ctx.(*TestContext)
								return tc.Flag
							},
						},
					},
				},
			},
			"active": {
				ID:   "active",
				Type: AtomicState,
			},
		},
	}

	machine := NewMachine(config)
	machine.Start()
	defer machine.Stop()

	machine.Send(Event{Type: "GO"})
	time.Sleep(10 * time.Millisecond)

	if machine.IsInState("active") {
		t.Error("Guard should have prevented transition")
	}

	tc := machine.GetContext().(*TestContext)
	tc.Flag = true
	machine.SetContext(tc)

	machine.Send(Event{Type: "GO"})
	time.Sleep(10 * time.Millisecond)

	if !machine.IsInState("active") {
		t.Error("Machine should be in active state when guard passes")
	}
}

func TestActions(t *testing.T) {
	var actionOrder []string
	var mu sync.Mutex

	addAction := func(name string) Action {
		return func(ctx Context, event Event) Context {
			mu.Lock()
			defer mu.Unlock()
			actionOrder = append(actionOrder, name)
			return ctx
		}
	}

	config := MachineConfig{
		ID:      "action-test",
		Initial: "state1",
		Context: &TestContext{},
		States: map[string]*StateNode{
			"state1": {
				ID:   "state1",
				Type: AtomicState,
				Entry: []Action{addAction("state1-entry")},
				Exit:  []Action{addAction("state1-exit")},
				On: map[string][]Transition{
					"GO": {
						{
							Target: "state2",
							Actions: []Action{
								addAction("transition-action-1"),
								addAction("transition-action-2"),
							},
						},
					},
				},
			},
			"state2": {
				ID:    "state2",
				Type:  AtomicState,
				Entry: []Action{addAction("state2-entry")},
			},
		},
	}

	machine := NewMachine(config)
	machine.Start()
	defer machine.Stop()

	time.Sleep(10 * time.Millisecond)
	machine.Send(Event{Type: "GO"})
	time.Sleep(10 * time.Millisecond)

	expected := []string{
		"state1-entry",
		"state1-exit",
		"transition-action-1",
		"transition-action-2",
		"state2-entry",
	}

	mu.Lock()
	defer mu.Unlock()

	if len(actionOrder) != len(expected) {
		t.Fatalf("Expected %d actions, got %d", len(expected), len(actionOrder))
	}

	for i, action := range expected {
		if actionOrder[i] != action {
			t.Errorf("Expected action %s at position %d, got %s", action, i, actionOrder[i])
		}
	}
}

func TestAlwaysTransitions(t *testing.T) {
	config := MachineConfig{
		ID:      "always-test",
		Initial: "checking",
		Context: &TestContext{Counter: 5},
		States: map[string]*StateNode{
			"checking": {
				ID:   "checking",
				Type: AtomicState,
				Always: []Transition{
					{
						Target: "positive",
						Guard: func(ctx Context, event Event) bool {
							tc := ctx.(*TestContext)
							return tc.Counter > 0
						},
					},
					{
						Target: "negative",
						Guard: func(ctx Context, event Event) bool {
							tc := ctx.(*TestContext)
							return tc.Counter < 0
						},
					},
					{Target: "zero"},
				},
			},
			"positive": {
				ID:   "positive",
				Type: AtomicState,
			},
			"negative": {
				ID:   "negative",
				Type: AtomicState,
			},
			"zero": {
				ID:   "zero",
				Type: AtomicState,
			},
		},
	}

	machine := NewMachine(config)
	machine.Start()
	defer machine.Stop()

	time.Sleep(50 * time.Millisecond)

	if !machine.IsInState("positive") {
		t.Error("Machine should transition to positive state")
	}
}

func TestEventData(t *testing.T) {
	config := MachineConfig{
		ID:      "event-data-test",
		Initial: "waiting",
		Context: &TestContext{},
		States: map[string]*StateNode{
			"waiting": {
				ID:   "waiting",
				Type: AtomicState,
				On: map[string][]Transition{
					"UPDATE": {
						{
							Target: "updated",
							Actions: []Action{
								func(ctx Context, event Event) Context {
									tc := ctx.(*TestContext)
									if data, ok := event.Data.(string); ok {
										tc.Data = data
									}
									return tc
								},
							},
						},
					},
				},
			},
			"updated": {
				ID:   "updated",
				Type: AtomicState,
			},
		},
	}

	machine := NewMachine(config)
	machine.Start()
	defer machine.Stop()

	testData := "test-data-123"
	machine.Send(Event{Type: "UPDATE", Data: testData})
	time.Sleep(10 * time.Millisecond)

	tc := machine.GetContext().(*TestContext)
	if tc.Data != testData {
		t.Errorf("Expected data %s, got %s", testData, tc.Data)
	}
}

func TestConcurrentEvents(t *testing.T) {
	config := MachineConfig{
		ID:      "concurrent-test",
		Initial: "counting",
		Context: &TestContext{Counter: 0},
		States: map[string]*StateNode{
			"counting": {
				ID:   "counting",
				Type: AtomicState,
				On: map[string][]Transition{
					"INCREMENT": {
						{
							Target: "counting",
							Actions: []Action{
								func(ctx Context, event Event) Context {
									tc := ctx.(*TestContext)
									tc.Counter++
									return tc
								},
							},
						},
					},
				},
			},
		},
	}

	machine := NewMachine(config)
	machine.Start()
	defer machine.Stop()

	var wg sync.WaitGroup
	numGoroutines := 10
	eventsPerGoroutine := 100

	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < eventsPerGoroutine; j++ {
				machine.Send(Event{Type: "INCREMENT"})
			}
		}()
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	tc := machine.GetContext().(*TestContext)
	expected := numGoroutines * eventsPerGoroutine
	if tc.Counter != expected {
		t.Errorf("Expected counter to be %d, got %d", expected, tc.Counter)
	}
}