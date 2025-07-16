package statemachine

import (
	"sync"
	"testing"
	"time"
)

func TestParallelStateMachine(t *testing.T) {
	config := MachineConfig{
		ID:      "parallel-test",
		Initial: "active",
		Context: &TestContext{Counter: 0},
		States: map[string]*StateNode{
			"active": {
				ID:   "active",
				Type: ParallelState,
				States: map[string]*StateNode{
					"region1": {
						ID:      "region1",
						Type:    CompoundState,
						Initial: "r1_idle",
						States: map[string]*StateNode{
							"r1_idle": {
								ID:   "r1_idle",
								Type: AtomicState,
								On: map[string][]Transition{
									"START_R1": {{Target: "r1_working"}},
								},
							},
							"r1_working": {
								ID:   "r1_working",
								Type: AtomicState,
								On: map[string][]Transition{
									"FINISH_R1": {{Target: "r1_done"}},
								},
							},
							"r1_done": {
								ID:    "r1_done",
								Type:  AtomicState,
								Final: true,
							},
						},
					},
					"region2": {
						ID:      "region2",
						Type:    CompoundState,
						Initial: "r2_idle",
						States: map[string]*StateNode{
							"r2_idle": {
								ID:   "r2_idle",
								Type: AtomicState,
								On: map[string][]Transition{
									"START_R2": {{Target: "r2_working"}},
								},
							},
							"r2_working": {
								ID:   "r2_working",
								Type: AtomicState,
								On: map[string][]Transition{
									"FINISH_R2": {{Target: "r2_done"}},
								},
							},
							"r2_done": {
								ID:    "r2_done",
								Type:  AtomicState,
								Final: true,
							},
						},
					},
				},
				On: map[string][]Transition{
					"done": {{Target: "completed"}},
				},
			},
			"completed": {
				ID:   "completed",
				Type: AtomicState,
			},
		},
	}

	machine := NewParallelMachine(config)
	if err := machine.Start(); err != nil {
		t.Fatalf("Failed to start machine: %v", err)
	}
	defer machine.Stop()

	time.Sleep(20 * time.Millisecond)

	parallelStates := machine.GetParallelStates()
	if len(parallelStates) != 2 {
		t.Errorf("Expected 2 parallel regions, got %d: %v", len(parallelStates), parallelStates)
	}

	machine.Send(Event{Type: "START_R1"})
	time.Sleep(10 * time.Millisecond)

	machine.Send(Event{Type: "START_R2"})
	time.Sleep(10 * time.Millisecond)

	expectedStates := map[string]string{
		"active.region1": "r1_working",
		"active.region2": "r2_working",
	}

	if !machine.IsAllRegionsInState(expectedStates) {
		t.Error("Regions should be in working states")
	}

	machine.Send(Event{Type: "FINISH_R1"})
	machine.Send(Event{Type: "FINISH_R2"})
	time.Sleep(20 * time.Millisecond)

	if !machine.IsInState("completed") {
		t.Error("Machine should transition to completed when all regions are done")
	}
}

func TestParallelEventBroadcast(t *testing.T) {
	var mu sync.Mutex
	var region1Count, region2Count int

	config := MachineConfig{
		ID:      "broadcast-test",
		Initial: "parallel",
		Context: &TestContext{},
		States: map[string]*StateNode{
			"parallel": {
				ID:   "parallel",
				Type: ParallelState,
				States: map[string]*StateNode{
					"counter1": {
						ID:      "counter1",
						Type:    CompoundState,
						Initial: "counting1",
						States: map[string]*StateNode{
							"counting1": {
								ID:   "counting1",
								Type: AtomicState,
								On: map[string][]Transition{
									"INCREMENT": {
										{
											Target: "counting1",
											Actions: []Action{
												func(ctx Context, event Event) Context {
													mu.Lock()
													region1Count++
													mu.Unlock()
													return ctx
												},
											},
										},
									},
								},
							},
						},
					},
					"counter2": {
						ID:      "counter2",
						Type:    CompoundState,
						Initial: "counting2",
						States: map[string]*StateNode{
							"counting2": {
								ID:   "counting2",
								Type: AtomicState,
								On: map[string][]Transition{
									"INCREMENT": {
										{
											Target: "counting2",
											Actions: []Action{
												func(ctx Context, event Event) Context {
													mu.Lock()
													region2Count++
													mu.Unlock()
													return ctx
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	machine := NewParallelMachine(config)
	machine.Start()
	defer machine.Stop()

	time.Sleep(20 * time.Millisecond)

	for i := 0; i < 5; i++ {
		machine.Send(Event{Type: "INCREMENT"})
	}

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if region1Count != 5 {
		t.Errorf("Expected region1 count to be 5, got %d", region1Count)
	}

	if region2Count != 5 {
		t.Errorf("Expected region2 count to be 5, got %d", region2Count)
	}
}

func TestParallelRegionIndependence(t *testing.T) {
	config := MachineConfig{
		ID:      "independence-test",
		Initial: "parallel",
		Context: &TestContext{},
		States: map[string]*StateNode{
			"parallel": {
				ID:   "parallel",
				Type: ParallelState,
				States: map[string]*StateNode{
					"traffic": {
						ID:      "traffic",
						Type:    CompoundState,
						Initial: "red",
						States: map[string]*StateNode{
							"red": {
								ID:   "red",
								Type: AtomicState,
								On: map[string][]Transition{
									"NEXT": {{Target: "green"}},
								},
							},
							"green": {
								ID:   "green",
								Type: AtomicState,
								On: map[string][]Transition{
									"NEXT": {{Target: "yellow"}},
								},
							},
							"yellow": {
								ID:   "yellow",
								Type: AtomicState,
								On: map[string][]Transition{
									"NEXT": {{Target: "red"}},
								},
							},
						},
					},
					"pedestrian": {
						ID:      "pedestrian",
						Type:    CompoundState,
						Initial: "walk",
						States: map[string]*StateNode{
							"walk": {
								ID:   "walk",
								Type: AtomicState,
								On: map[string][]Transition{
									"TOGGLE": {{Target: "wait"}},
								},
							},
							"wait": {
								ID:   "wait",
								Type: AtomicState,
								On: map[string][]Transition{
									"TOGGLE": {{Target: "walk"}},
								},
							},
						},
					},
				},
			},
		},
	}

	machine := NewParallelMachine(config)
	machine.Start()
	defer machine.Stop()

	time.Sleep(20 * time.Millisecond)

	machine.Send(Event{Type: "NEXT"})
	time.Sleep(10 * time.Millisecond)

	parallelStates := machine.GetParallelStates()
	
	trafficRegion := parallelStates["parallel.traffic"]
	if trafficRegion == nil {
		t.Fatal("Traffic region not found")
	}

	pedestrianRegion := parallelStates["parallel.pedestrian"]
	if pedestrianRegion == nil {
		t.Fatal("Pedestrian region not found")
	}

	machine.Send(Event{Type: "TOGGLE"})
	time.Sleep(10 * time.Millisecond)

	newParallelStates := machine.GetParallelStates()
	newPedestrianRegion := newParallelStates["parallel.pedestrian"]
	
	if newPedestrianRegion == nil {
		t.Fatal("Pedestrian region not found after toggle")
	}
}

func TestParallelMachineWithGuards(t *testing.T) {
	config := MachineConfig{
		ID:      "parallel-guards-test",
		Initial: "parallel",
		Context: &TestContext{Counter: 0, Flag: false},
		States: map[string]*StateNode{
			"parallel": {
				ID:   "parallel",
				Type: ParallelState,
				States: map[string]*StateNode{
					"guarded1": {
						ID:      "guarded1",
						Type:    CompoundState,
						Initial: "waiting1",
						States: map[string]*StateNode{
							"waiting1": {
								ID:   "waiting1",
								Type: AtomicState,
								On: map[string][]Transition{
									"PROCEED": {
										{
											Target: "done1",
											Guard: func(ctx Context, event Event) bool {
												tc := ctx.(*TestContext)
												return tc.Counter > 5
											},
										},
									},
								},
							},
							"done1": {
								ID:    "done1",
								Type:  AtomicState,
								Final: true,
							},
						},
					},
					"guarded2": {
						ID:      "guarded2",
						Type:    CompoundState,
						Initial: "waiting2",
						States: map[string]*StateNode{
							"waiting2": {
								ID:   "waiting2",
								Type: AtomicState,
								On: map[string][]Transition{
									"PROCEED": {
										{
											Target: "done2",
											Guard: func(ctx Context, event Event) bool {
												tc := ctx.(*TestContext)
												return tc.Flag
											},
										},
									},
								},
							},
							"done2": {
								ID:    "done2",
								Type:  AtomicState,
								Final: true,
							},
						},
					},
				},
				On: map[string][]Transition{
					"done": {{Target: "allDone"}},
				},
			},
			"allDone": {
				ID:   "allDone",
				Type: AtomicState,
			},
		},
	}

	machine := NewParallelMachine(config)
	machine.Start()
	defer machine.Stop()

	time.Sleep(20 * time.Millisecond)

	machine.Send(Event{Type: "PROCEED"})
	time.Sleep(10 * time.Millisecond)

	if machine.IsInState("allDone") {
		t.Error("Machine should not transition when guards prevent it")
	}

	tc := machine.GetContext().(*TestContext)
	tc.Counter = 10
	tc.Flag = true
	machine.SetContext(tc)

	machine.Send(Event{Type: "PROCEED"})
	time.Sleep(20 * time.Millisecond)

	if !machine.IsInState("allDone") {
		t.Error("Machine should transition to allDone when both guards pass")
	}
}