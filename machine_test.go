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
	mu      sync.Mutex
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
						tc.mu.Lock()
						tc.Counter++
						tc.mu.Unlock()
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
	tc.mu.Lock()
	counter := tc.Counter
	tc.mu.Unlock()
	if counter != 1 {
		t.Errorf("Counter should be 1, got %d", counter)
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
								tc.mu.Lock()
								flag := tc.Flag
								tc.mu.Unlock()
								return flag
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
	tc.mu.Lock()
	tc.Flag = true
	tc.mu.Unlock()
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
							tc.mu.Lock()
							result := tc.Counter > 0
							tc.mu.Unlock()
							return result
						},
					},
					{
						Target: "negative",
						Guard: func(ctx Context, event Event) bool {
							tc := ctx.(*TestContext)
							tc.mu.Lock()
							result := tc.Counter < 0
							tc.mu.Unlock()
							return result
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
									tc.mu.Lock()
									tc.Counter++
									tc.mu.Unlock()
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

	totalEvents := 100
	eventsSent := 0

	// Send events sequentially with error handling
	for i := 0; i < totalEvents; i++ {
		err := machine.Send(Event{Type: "INCREMENT"})
		if err == nil {
			eventsSent++
		}
		// Small delay to allow processing
		if i%10 == 0 {
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Wait for all events to be processed
	time.Sleep(200 * time.Millisecond)

	tc := machine.GetContext().(*TestContext)
	tc.mu.Lock()
	counter := tc.Counter
	tc.mu.Unlock()
	
	if counter != eventsSent {
		t.Errorf("Expected counter to be %d (events sent), got %d", eventsSent, counter)
	}
	
	if eventsSent < totalEvents {
		t.Logf("Warning: Only %d of %d events were sent successfully", eventsSent, totalEvents)
	}
}

func TestMachineStartStop(t *testing.T) {
	config := MachineConfig{
		ID:      "start-stop-test",
		Initial: "idle",
		Context: &TestContext{},
		States: map[string]*StateNode{
			"idle": {
				ID:   "idle",
				Type: AtomicState,
			},
		},
	}

	machine := NewMachine(config)

	err := machine.Start()
	if err != nil {
		t.Fatalf("Failed to start machine: %v", err)
	}

	err = machine.Start()
	if err == nil {
		t.Error("Starting an already running machine should return an error")
	}

	machine.Stop()

	err = machine.Send(Event{Type: "TEST"})
	if err == nil {
		t.Error("Sending event to stopped machine should return an error")
	}

	machine.Stop()
}

func TestEventChannelFull(t *testing.T) {
	config := MachineConfig{
		ID:      "channel-full-test",
		Initial: "slow",
		Context: &TestContext{},
		States: map[string]*StateNode{
			"slow": {
				ID:   "slow",
				Type: AtomicState,
				On: map[string][]Transition{
					"PROCESS": {
						{
							Target: "slow",
							Actions: []Action{
								func(ctx Context, event Event) Context {
									time.Sleep(100 * time.Millisecond)
									return ctx
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

	for i := 0; i < 101; i++ {
		err := machine.Send(Event{Type: "PROCESS"})
		if i == 100 && err == nil {
			t.Error("Expected error when event channel is full")
		}
	}
}

func TestSubscribers(t *testing.T) {
	config := MachineConfig{
		ID:      "subscriber-test",
		Initial: "state1",
		Context: &TestContext{},
		States: map[string]*StateNode{
			"state1": {
				ID:   "state1",
				Type: AtomicState,
				On: map[string][]Transition{
					"GO": {{Target: "state2"}},
				},
			},
			"state2": {
				ID:   "state2",
				Type: AtomicState,
			},
		},
	}

	machine := NewMachine(config)
	machine.Start()
	defer machine.Stop()

	var notifications []string
	var mu sync.Mutex

	machine.Subscribe(func(event Event, state *StateValue, ctx Context) {
		mu.Lock()
		defer mu.Unlock()
		notifications = append(notifications, event.Type)
	})

	machine.Subscribe(func(event Event, state *StateValue, ctx Context) {
		mu.Lock()
		defer mu.Unlock()
		if state.Matches("state2") {
			notifications = append(notifications, "in-state2")
		}
	})

	machine.Send(Event{Type: "GO"})
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(notifications) < 2 {
		t.Errorf("Expected at least 2 notifications, got %d", len(notifications))
	}

	hasGo := false
	hasInState2 := false
	for _, n := range notifications {
		if n == "GO" {
			hasGo = true
		}
		if n == "in-state2" {
			hasInState2 = true
		}
	}

	if !hasGo {
		t.Error("Expected to receive GO event notification")
	}

	if !hasInState2 {
		t.Error("Expected to receive in-state2 notification")
	}
}

func TestStateValueThreadSafety(t *testing.T) {
	sv := NewStateValue("initial")

	var wg sync.WaitGroup
	numGoroutines := 100

	wg.Add(numGoroutines * 3)

	for i := 0; i < numGoroutines; i++ {
		go func(id int) {
			defer wg.Done()
			sv.Set("path", "state"+string(rune(id)))
		}(i)

		go func() {
			defer wg.Done()
			sv.Get()
		}()

		go func() {
			defer wg.Done()
			sv.Matches("initial")
		}()
	}

	wg.Wait()
}

func TestComplexTransitionWithMultipleGuards(t *testing.T) {
	config := MachineConfig{
		ID:      "multi-guard-test",
		Initial: "start",
		Context: &TestContext{Counter: 0, Flag: false},
		States: map[string]*StateNode{
			"start": {
				ID:   "start",
				Type: AtomicState,
				On: map[string][]Transition{
					"CHECK": {
						{
							Target: "highPriority",
							Guard: func(ctx Context, event Event) bool {
								tc := ctx.(*TestContext)
								tc.mu.Lock()
								result := tc.Counter > 10 && tc.Flag
								tc.mu.Unlock()
								return result
							},
						},
						{
							Target: "mediumPriority",
							Guard: func(ctx Context, event Event) bool {
								tc := ctx.(*TestContext)
								tc.mu.Lock()
								result := tc.Counter > 5
								tc.mu.Unlock()
								return result
							},
						},
						{
							Target: "lowPriority",
						},
					},
				},
			},
			"highPriority": {
				ID:   "highPriority",
				Type: AtomicState,
				On: map[string][]Transition{
					"CHECK": {
						{
							Target: "highPriority",
							Guard: func(ctx Context, event Event) bool {
								tc := ctx.(*TestContext)
								tc.mu.Lock()
								result := tc.Counter > 10 && tc.Flag
								tc.mu.Unlock()
								return result
							},
						},
						{
							Target: "mediumPriority",
							Guard: func(ctx Context, event Event) bool {
								tc := ctx.(*TestContext)
								tc.mu.Lock()
								result := tc.Counter > 5
								tc.mu.Unlock()
								return result
							},
						},
						{
							Target: "lowPriority",
						},
					},
				},
			},
			"mediumPriority": {
				ID:   "mediumPriority",
				Type: AtomicState,
				On: map[string][]Transition{
					"CHECK": {
						{
							Target: "highPriority",
							Guard: func(ctx Context, event Event) bool {
								tc := ctx.(*TestContext)
								tc.mu.Lock()
								result := tc.Counter > 10 && tc.Flag
								tc.mu.Unlock()
								return result
							},
						},
						{
							Target: "mediumPriority",
							Guard: func(ctx Context, event Event) bool {
								tc := ctx.(*TestContext)
								tc.mu.Lock()
								result := tc.Counter > 5
								tc.mu.Unlock()
								return result
							},
						},
						{
							Target: "lowPriority",
						},
					},
				},
			},
			"lowPriority": {
				ID:   "lowPriority",
				Type: AtomicState,
				On: map[string][]Transition{
					"CHECK": {
						{
							Target: "highPriority",
							Guard: func(ctx Context, event Event) bool {
								tc := ctx.(*TestContext)
								tc.mu.Lock()
								result := tc.Counter > 10 && tc.Flag
								tc.mu.Unlock()
								return result
							},
						},
						{
							Target: "mediumPriority",
							Guard: func(ctx Context, event Event) bool {
								tc := ctx.(*TestContext)
								tc.mu.Lock()
								result := tc.Counter > 5
								tc.mu.Unlock()
								return result
							},
						},
						{
							Target: "lowPriority",
						},
					},
				},
			},
		},
	}

	machine := NewMachine(config)
	machine.Start()
	defer machine.Stop()

	machine.Send(Event{Type: "CHECK"})
	time.Sleep(10 * time.Millisecond)

	if !machine.IsInState("lowPriority") {
		t.Error("Should transition to lowPriority when no guards pass")
	}

	tc := machine.GetContext().(*TestContext)
	tc.mu.Lock()
	tc.Counter = 7
	tc.mu.Unlock()
	machine.SetContext(tc)

	// Start from lowPriority, need to go back to start
	machine.Send(Event{Type: "CHECK"}) 
	time.Sleep(10 * time.Millisecond)

	if !machine.IsInState("mediumPriority") {
		currentState := machine.GetState()
		t.Errorf("Should transition to mediumPriority when counter > 5, but in state: %v", currentState)
	}

	tc.mu.Lock()
	tc.Counter = 15
	tc.Flag = true
	tc.mu.Unlock()
	machine.SetContext(tc)

	machine.Send(Event{Type: "CHECK"})
	time.Sleep(10 * time.Millisecond)

	if !machine.IsInState("highPriority") {
		t.Error("Should transition to highPriority when all conditions met")
	}
}

func TestFinalStates(t *testing.T) {
	config := MachineConfig{
		ID:      "final-state-test",
		Initial: "processing",
		Context: &TestContext{},
		States: map[string]*StateNode{
			"processing": {
				ID:   "processing",
				Type: AtomicState,
				On: map[string][]Transition{
					"COMPLETE": {{Target: "done"}},
				},
			},
			"done": {
				ID:    "done",
				Type:  FinalState,
				Final: true,
			},
		},
	}

	machine := NewMachine(config)
	machine.Start()
	defer machine.Stop()

	machine.Send(Event{Type: "COMPLETE"})
	time.Sleep(10 * time.Millisecond)

	if !machine.IsInState("done") {
		t.Error("Machine should be in done state")
	}

	state := machine.GetState()
	if _, exists := state["root"]; !exists {
		t.Error("Root state should exist in state map")
	}
}

func TestUnhandledEvent(t *testing.T) {
	config := MachineConfig{
		ID:      "unhandled-test",
		Initial: "state1",
		Context: &TestContext{},
		States: map[string]*StateNode{
			"state1": {
				ID:   "state1",
				Type: AtomicState,
				On: map[string][]Transition{
					"HANDLED": {{Target: "state2"}},
				},
			},
			"state2": {
				ID:   "state2",
				Type: AtomicState,
			},
		},
	}

	machine := NewMachine(config)
	machine.Start()
	defer machine.Stop()

	machine.Send(Event{Type: "UNHANDLED"})
	time.Sleep(10 * time.Millisecond)

	if !machine.IsInState("state1") {
		t.Error("Machine should remain in state1 for unhandled event")
	}

	machine.Send(Event{Type: "HANDLED"})
	time.Sleep(10 * time.Millisecond)

	if !machine.IsInState("state2") {
		t.Error("Machine should transition to state2 for handled event")
	}
}