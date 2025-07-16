package examples

import (
	"fmt"
	"log"
	"time"

	sm "github.com/user/go-state-machine"
)

type ElevatorContext struct {
	CurrentFloor  int
	RequestFloor  int
	DoorsOpen     bool
	MaxFloor      int
	MovingUp      bool
}

func ElevatorExample() {
	config := sm.MachineConfig{
		ID:      "elevator",
		Initial: "idle",
		Context: &ElevatorContext{
			CurrentFloor: 1,
			MaxFloor:     10,
		},
		States: map[string]*sm.StateNode{
			"idle": {
				ID:   "idle",
				Type: sm.CompoundState,
				Initial: "doorsOpen",
				States: map[string]*sm.StateNode{
					"doorsOpen": {
						ID:   "doorsOpen",
						Type: sm.AtomicState,
						Entry: []sm.Action{
							func(ctx sm.Context, event sm.Event) sm.Context {
								ec := ctx.(*ElevatorContext)
								ec.DoorsOpen = true
								fmt.Printf("🚪 Doors OPEN at floor %d\n", ec.CurrentFloor)
								return ec
							},
						},
						On: map[string][]sm.Transition{
							"CLOSE_DOORS": {
								{Target: "doorsClosed"},
							},
							"FLOOR_REQUESTED": {
								{
									Target: "doorsClosed",
									Actions: []sm.Action{
										func(ctx sm.Context, event sm.Event) sm.Context {
											ec := ctx.(*ElevatorContext)
											if floor, ok := event.Data.(int); ok {
												ec.RequestFloor = floor
											}
											return ec
										},
									},
								},
							},
						},
					},
					"doorsClosed": {
						ID:   "doorsClosed",
						Type: sm.AtomicState,
						Entry: []sm.Action{
							func(ctx sm.Context, event sm.Event) sm.Context {
								ec := ctx.(*ElevatorContext)
								ec.DoorsOpen = false
								fmt.Printf("🚪 Doors CLOSED at floor %d\n", ec.CurrentFloor)
								return ec
							},
						},
						Always: []sm.Transition{
							{
								Target: "moving",
								Guard: func(ctx sm.Context, event sm.Event) bool {
									ec := ctx.(*ElevatorContext)
									return ec.RequestFloor != 0 && ec.RequestFloor != ec.CurrentFloor
								},
							},
						},
						On: map[string][]sm.Transition{
							"OPEN_DOORS": {
								{Target: "doorsOpen"},
							},
							"FLOOR_REQUESTED": {
								{
									Target: "moving",
									Actions: []sm.Action{
										func(ctx sm.Context, event sm.Event) sm.Context {
											ec := ctx.(*ElevatorContext)
											if floor, ok := event.Data.(int); ok {
												ec.RequestFloor = floor
											}
											return ec
										},
									},
									Guard: func(ctx sm.Context, event sm.Event) bool {
										ec := ctx.(*ElevatorContext)
										if floor, ok := event.Data.(int); ok {
											return floor != ec.CurrentFloor
										}
										return false
									},
								},
							},
						},
					},
				},
			},
			"moving": {
				ID:   "moving",
				Type: sm.CompoundState,
				Initial: "movingUp",
				Entry: []sm.Action{
					func(ctx sm.Context, event sm.Event) sm.Context {
						ec := ctx.(*ElevatorContext)
						ec.MovingUp = ec.RequestFloor > ec.CurrentFloor
						return ec
					},
				},
				States: map[string]*sm.StateNode{
					"movingUp": {
						ID:   "movingUp",
						Type: sm.AtomicState,
						Entry: []sm.Action{
							func(ctx sm.Context, event sm.Event) sm.Context {
								fmt.Println("⬆️  Moving UP")
								return ctx
							},
						},
						On: map[string][]sm.Transition{
							"FLOOR_REACHED": {
								{
									Target: "idle.doorsOpen",
									Guard: func(ctx sm.Context, event sm.Event) bool {
										ec := ctx.(*ElevatorContext)
										return ec.CurrentFloor == ec.RequestFloor
									},
									Actions: []sm.Action{
										func(ctx sm.Context, event sm.Event) sm.Context {
											ec := ctx.(*ElevatorContext)
											ec.RequestFloor = 0
											return ec
										},
									},
								},
								{
									Target: "movingUp",
									Actions: []sm.Action{
										func(ctx sm.Context, event sm.Event) sm.Context {
											ec := ctx.(*ElevatorContext)
											ec.CurrentFloor++
											fmt.Printf("📍 Passing floor %d\n", ec.CurrentFloor)
											return ec
										},
									},
								},
							},
						},
					},
					"movingDown": {
						ID:   "movingDown",
						Type: sm.AtomicState,
						Entry: []sm.Action{
							func(ctx sm.Context, event sm.Event) sm.Context {
								fmt.Println("⬇️  Moving DOWN")
								return ctx
							},
						},
						On: map[string][]sm.Transition{
							"FLOOR_REACHED": {
								{
									Target: "idle.doorsOpen",
									Guard: func(ctx sm.Context, event sm.Event) bool {
										ec := ctx.(*ElevatorContext)
										return ec.CurrentFloor == ec.RequestFloor
									},
									Actions: []sm.Action{
										func(ctx sm.Context, event sm.Event) sm.Context {
											ec := ctx.(*ElevatorContext)
											ec.RequestFloor = 0
											return ec
										},
									},
								},
								{
									Target: "movingDown",
									Actions: []sm.Action{
										func(ctx sm.Context, event sm.Event) sm.Context {
											ec := ctx.(*ElevatorContext)
											ec.CurrentFloor--
											fmt.Printf("📍 Passing floor %d\n", ec.CurrentFloor)
											return ec
										},
									},
								},
							},
						},
					},
				},
				On: map[string][]sm.Transition{
					"EMERGENCY_STOP": {
						{Target: "idle.doorsClosed"},
					},
				},
			},
		},
	}

	machine := sm.NewHierarchicalMachine(config)

	machine.Subscribe(func(event sm.Event, state *sm.StateValue, ctx sm.Context) {
		ec := ctx.(*ElevatorContext)
		fmt.Printf("  [Event: %s | State: %v | Floor: %d | Target: %d]\n",
			event.Type, state.Get(), ec.CurrentFloor, ec.RequestFloor)
	})

	if err := machine.Start(); err != nil {
		log.Fatal(err)
	}

	time.Sleep(1 * time.Second)
	machine.Send(sm.Event{Type: "FLOOR_REQUESTED", Data: 5})
	
	time.Sleep(1 * time.Second)
	machine.Send(sm.Event{Type: "CLOSE_DOORS"})
	
	for i := 0; i < 4; i++ {
		time.Sleep(1 * time.Second)
		machine.Send(sm.Event{Type: "FLOOR_REACHED"})
	}
	
	time.Sleep(2 * time.Second)
	machine.Send(sm.Event{Type: "FLOOR_REQUESTED", Data: 2})
	
	time.Sleep(1 * time.Second)
	machine.Send(sm.Event{Type: "CLOSE_DOORS"})
	
	for i := 0; i < 3; i++ {
		time.Sleep(1 * time.Second)
		machine.Send(sm.Event{Type: "FLOOR_REACHED"})
	}
	
	time.Sleep(2 * time.Second)
	machine.Stop()
	fmt.Println("Elevator simulation completed")
}