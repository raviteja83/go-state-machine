package statemachine

import (
	"sync"
	"testing"
	"time"
)

func TestIntegrationComplexWorkflow(t *testing.T) {
	type WorkflowContext struct {
		ProcessingSteps []string
		ErrorCount      int
		RetryCount      int
		mu              sync.Mutex
	}

	addStep := func(step string) Action {
		return func(ctx Context, event Event) Context {
			wc := ctx.(*WorkflowContext)
			wc.mu.Lock()
			defer wc.mu.Unlock()
			wc.ProcessingSteps = append(wc.ProcessingSteps, step)
			return wc
		}
	}

	config := MachineConfig{
		ID:      "workflow",
		Initial: "idle",
		Context: &WorkflowContext{ProcessingSteps: []string{}},
		States: map[string]*StateNode{
			"idle": {
				ID:   "idle",
				Type: AtomicState,
				On: map[string][]Transition{
					"START": {{
						Target:  "validating",
						Actions: []Action{addStep("start-validation")},
					}},
				},
			},
			"validating": {
				ID:    "validating",
				Type:  AtomicState,
				Entry: []Action{addStep("enter-validation")},
				Exit:  []Action{addStep("exit-validation")},
				On: map[string][]Transition{
					"VALID": {{
						Target:  "processing",
						Actions: []Action{addStep("validation-success")},
					}},
					"INVALID": {{
						Target: "error",
						Actions: []Action{
							addStep("validation-failed"),
							func(ctx Context, event Event) Context {
								wc := ctx.(*WorkflowContext)
								wc.mu.Lock()
								wc.ErrorCount++
								wc.mu.Unlock()
								return wc
							},
						},
					}},
				},
			},
			"processing": {
				ID:      "processing",
				Type:    CompoundState,
				Initial: "step1",
				Entry:   []Action{addStep("enter-processing")},
				States: map[string]*StateNode{
					"step1": {
						ID:    "step1",
						Type:  AtomicState,
						Entry: []Action{addStep("process-step1")},
						On: map[string][]Transition{
							"NEXT": {{Target: "step2"}},
						},
					},
					"step2": {
						ID:    "step2",
						Type:  AtomicState,
						Entry: []Action{addStep("process-step2")},
						On: map[string][]Transition{
							"NEXT": {{Target: "step3"}},
						},
					},
					"step3": {
						ID:    "step3",
						Type:  AtomicState,
						Entry: []Action{addStep("process-step3")},
						On: map[string][]Transition{
							"COMPLETE": {{Target: "finalizing"}},
						},
					},
				},
				On: map[string][]Transition{
					"ERROR": {{
						Target:  "error",
						Actions: []Action{addStep("processing-error")},
					}},
				},
			},
			"finalizing": {
				ID:    "finalizing",
				Type:  AtomicState,
				Entry: []Action{addStep("enter-finalizing")},
				Always: []Transition{
					{
						Target:  "success",
						Actions: []Action{addStep("finalization-complete")},
					},
				},
			},
			"error": {
				ID:    "error",
				Type:  AtomicState,
				Entry: []Action{addStep("enter-error")},
				On: map[string][]Transition{
					"RETRY": {{
						Target: "idle",
						Guard: func(ctx Context, event Event) bool {
							wc := ctx.(*WorkflowContext)
							return wc.RetryCount < 3
						},
						Actions: []Action{
							addStep("retry-attempt"),
							func(ctx Context, event Event) Context {
								wc := ctx.(*WorkflowContext)
								wc.mu.Lock()
								wc.RetryCount++
								wc.mu.Unlock()
								return wc
							},
						},
					}},
					"ABORT": {{
						Target:  "failed",
						Actions: []Action{addStep("abort-workflow")},
					}},
				},
			},
			"success": {
				ID:    "success",
				Type:  FinalState,
				Final: true,
				Entry: []Action{addStep("workflow-success")},
			},
			"failed": {
				ID:    "failed",
				Type:  FinalState,
				Final: true,
				Entry: []Action{addStep("workflow-failed")},
			},
		},
	}

	machine := NewMachine(config)
	machine.Start()
	defer machine.Stop()

	// Test successful workflow
	machine.Send(Event{Type: "START"})
	time.Sleep(10 * time.Millisecond)

	machine.Send(Event{Type: "VALID"})
	time.Sleep(10 * time.Millisecond)

	if !machine.IsInState("step1") {
		t.Error("Should be in processing.step1")
	}

	machine.Send(Event{Type: "NEXT"})
	time.Sleep(10 * time.Millisecond)

	machine.Send(Event{Type: "NEXT"})
	time.Sleep(10 * time.Millisecond)

	machine.Send(Event{Type: "COMPLETE"})
	time.Sleep(50 * time.Millisecond) // Allow time for always transition

	if !machine.IsInState("success") {
		t.Error("Should be in success state")
	}

	wc := machine.GetContext().(*WorkflowContext)
	expectedSteps := []string{
		"start-validation",
		"enter-validation",
		"exit-validation",
		"validation-success",
		"enter-processing",
		"process-step1",
		"process-step2",
		"process-step3",
		"enter-finalizing",
		"finalization-complete",
		"workflow-success",
	}

	wc.mu.Lock()
	defer wc.mu.Unlock()

	if len(wc.ProcessingSteps) != len(expectedSteps) {
		t.Fatalf("Expected %d steps, got %d", len(expectedSteps), len(wc.ProcessingSteps))
	}

	for i, step := range expectedSteps {
		if wc.ProcessingSteps[i] != step {
			t.Errorf("Expected step %d to be '%s', got '%s'", i, step, wc.ProcessingSteps[i])
		}
	}
}

func TestIntegrationNestedParallelMachines(t *testing.T) {
	type SystemContext struct {
		SubsystemStatus map[string]string
		mu              sync.Mutex
	}

	updateStatus := func(subsystem, status string) Action {
		return func(ctx Context, event Event) Context {
			sc := ctx.(*SystemContext)
			sc.mu.Lock()
			defer sc.mu.Unlock()
			sc.SubsystemStatus[subsystem] = status
			return sc
		}
	}

	config := MachineConfig{
		ID:      "system",
		Initial: "booting",
		Context: &SystemContext{SubsystemStatus: make(map[string]string)},
		States: map[string]*StateNode{
			"booting": {
				ID:    "booting",
				Type:  AtomicState,
				Entry: []Action{updateStatus("system", "booting")},
				On: map[string][]Transition{
					"BOOT_COMPLETE": {{Target: "operational"}},
				},
			},
			"operational": {
				ID:    "operational",
				Type:  ParallelState,
				Entry: []Action{updateStatus("system", "operational")},
				States: map[string]*StateNode{
					"networking": {
						ID:      "networking",
						Type:    CompoundState,
						Initial: "connecting",
						States: map[string]*StateNode{
							"connecting": {
								ID:    "connecting",
								Type:  AtomicState,
								Entry: []Action{updateStatus("network", "connecting")},
								On: map[string][]Transition{
									"CONNECTED": {{
										Target:  "online",
										Actions: []Action{updateStatus("network", "online")},
									}},
								},
							},
							"online": {
								ID:   "online",
								Type: AtomicState,
								On: map[string][]Transition{
									"DISCONNECT": {{
										Target:  "offline",
										Actions: []Action{updateStatus("network", "offline")},
									}},
								},
							},
							"offline": {
								ID:   "offline",
								Type: AtomicState,
								On: map[string][]Transition{
									"RECONNECT": {{Target: "connecting"}},
								},
							},
						},
					},
					"monitoring": {
						ID:      "monitoring",
						Type:    CompoundState,
						Initial: "idle",
						States: map[string]*StateNode{
							"idle": {
								ID:    "idle",
								Type:  AtomicState,
								Entry: []Action{updateStatus("monitor", "idle")},
								On: map[string][]Transition{
									"START_MONITORING": {{
										Target:  "active",
										Actions: []Action{updateStatus("monitor", "active")},
									}},
								},
							},
							"active": {
								ID:   "active",
								Type: ParallelState,
								States: map[string]*StateNode{
									"cpu": {
										ID:      "cpu",
										Type:    CompoundState,
										Initial: "normal",
										States: map[string]*StateNode{
											"normal": {
												ID:    "normal",
												Type:  AtomicState,
												Entry: []Action{updateStatus("cpu", "normal")},
												On: map[string][]Transition{
													"HIGH_LOAD": {{
														Target:  "high",
														Actions: []Action{updateStatus("cpu", "high")},
													}},
												},
											},
											"high": {
												ID:   "high",
												Type: AtomicState,
												On: map[string][]Transition{
													"NORMAL_LOAD": {{
														Target:  "normal",
														Actions: []Action{updateStatus("cpu", "normal")},
													}},
												},
											},
										},
									},
									"memory": {
										ID:      "memory",
										Type:    CompoundState,
										Initial: "ok",
										States: map[string]*StateNode{
											"ok": {
												ID:    "ok",
												Type:  AtomicState,
												Entry: []Action{updateStatus("memory", "ok")},
												On: map[string][]Transition{
													"LOW_MEMORY": {{
														Target:  "low",
														Actions: []Action{updateStatus("memory", "low")},
													}},
												},
											},
											"low": {
												ID:   "low",
												Type: AtomicState,
												On: map[string][]Transition{
													"MEMORY_FREED": {{
														Target:  "ok",
														Actions: []Action{updateStatus("memory", "ok")},
													}},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				On: map[string][]Transition{
					"SHUTDOWN": {{Target: "shutting_down"}},
				},
			},
			"shutting_down": {
				ID:    "shutting_down",
				Type:  AtomicState,
				Entry: []Action{updateStatus("system", "shutting_down")},
				Always: []Transition{
					{Target: "offline"},
				},
			},
			"offline": {
				ID:    "offline",
				Type:  FinalState,
				Final: true,
				Entry: []Action{updateStatus("system", "offline")},
			},
		},
	}

	machine := NewParallelMachine(config)
	machine.Start()
	defer machine.Stop()

	time.Sleep(50 * time.Millisecond)

	// Boot the system
	machine.Send(Event{Type: "BOOT_COMPLETE"})
	time.Sleep(50 * time.Millisecond)

	// Connect network
	machine.Send(Event{Type: "CONNECTED"})
	time.Sleep(20 * time.Millisecond)

	// Start monitoring
	machine.Send(Event{Type: "START_MONITORING"})
	time.Sleep(50 * time.Millisecond)

	// Simulate high CPU load
	machine.Send(Event{Type: "HIGH_LOAD"})
	time.Sleep(20 * time.Millisecond)

	// Check system status
	sc := machine.GetContext().(*SystemContext)
	sc.mu.Lock()
	defer sc.mu.Unlock()

	if sc.SubsystemStatus["system"] != "operational" {
		t.Errorf("System should be operational, got %s", sc.SubsystemStatus["system"])
	}

	if sc.SubsystemStatus["network"] != "online" {
		t.Errorf("Network should be online, got %s", sc.SubsystemStatus["network"])
	}

	if sc.SubsystemStatus["monitor"] != "active" {
		t.Errorf("Monitor should be active, got %s", sc.SubsystemStatus["monitor"])
	}

	if sc.SubsystemStatus["cpu"] != "high" {
		t.Errorf("CPU should be high, got %s", sc.SubsystemStatus["cpu"])
	}

	if sc.SubsystemStatus["memory"] != "ok" {
		t.Errorf("Memory should be ok, got %s", sc.SubsystemStatus["memory"])
	}
}

func TestIntegrationHistoryMechanism(t *testing.T) {
	type HistoryContext struct {
		LastState   string
		VisitCount  map[string]int
		mu          sync.Mutex
	}

	recordVisit := func(state string) Action {
		return func(ctx Context, event Event) Context {
			hc := ctx.(*HistoryContext)
			hc.mu.Lock()
			defer hc.mu.Unlock()
			hc.LastState = state
			hc.VisitCount[state]++
			return hc
		}
	}

	config := MachineConfig{
		ID:      "history-test",
		Initial: "main",
		Context: &HistoryContext{VisitCount: make(map[string]int)},
		States: map[string]*StateNode{
			"main": {
				ID:      "main",
				Type:    CompoundState,
				Initial: "substate1",
				States: map[string]*StateNode{
					"substate1": {
						ID:    "substate1",
						Type:  AtomicState,
						Entry: []Action{recordVisit("substate1")},
						On: map[string][]Transition{
							"NEXT": {{Target: "substate2"}},
						},
					},
					"substate2": {
						ID:    "substate2",
						Type:  AtomicState,
						Entry: []Action{recordVisit("substate2")},
						On: map[string][]Transition{
							"NEXT": {{Target: "substate3"}},
						},
					},
					"substate3": {
						ID:    "substate3",
						Type:  AtomicState,
						Entry: []Action{recordVisit("substate3")},
					},
				},
				On: map[string][]Transition{
					"INTERRUPT": {{Target: "interrupted"}},
				},
			},
			"interrupted": {
				ID:    "interrupted",
				Type:  AtomicState,
				Entry: []Action{recordVisit("interrupted")},
				On: map[string][]Transition{
					"RESUME": {{
						Target: "main",
						Guard: func(ctx Context, event Event) bool {
							hc := ctx.(*HistoryContext)
							hc.mu.Lock()
							defer hc.mu.Unlock()
							// Simulate history by checking last state
							return hc.LastState != ""
						},
					}},
				},
			},
		},
	}

	machine := NewMachine(config)
	machine.Start()
	defer machine.Stop()

	// Navigate through states
	machine.Send(Event{Type: "NEXT"})
	time.Sleep(10 * time.Millisecond)

	machine.Send(Event{Type: "NEXT"})
	time.Sleep(10 * time.Millisecond)

	// Interrupt while in substate3
	machine.Send(Event{Type: "INTERRUPT"})
	time.Sleep(10 * time.Millisecond)

	if !machine.IsInState("interrupted") {
		t.Error("Should be in interrupted state")
	}

	hc := machine.GetContext().(*HistoryContext)
	hc.mu.Lock()
	lastState := hc.LastState
	visitCount := make(map[string]int)
	for k, v := range hc.VisitCount {
		visitCount[k] = v
	}
	hc.mu.Unlock()

	if lastState != "interrupted" {
		t.Errorf("Last state should be interrupted, got %s", lastState)
	}

	// Check visit counts
	if visitCount["substate1"] != 1 {
		t.Errorf("substate1 should be visited once, got %d", visitCount["substate1"])
	}

	if visitCount["substate2"] != 1 {
		t.Errorf("substate2 should be visited once, got %d", visitCount["substate2"])
	}

	if visitCount["substate3"] != 1 {
		t.Errorf("substate3 should be visited once, got %d", visitCount["substate3"])
	}

	// Resume (would go back to main's initial state in this simple example)
	machine.Send(Event{Type: "RESUME"})
	time.Sleep(10 * time.Millisecond)

	if !machine.IsInState("substate1") {
		t.Error("Should be back in main.substate1")
	}
}