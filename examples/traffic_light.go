package main

import (
	"fmt"
	"log"
	"time"

	sm "github.com/user/go-state-machine"
)

type TrafficLightContext struct {
	VehicleCount int
	IsEmergency  bool
}

func main() {
	config := sm.MachineConfig{
		ID:      "traffic-light",
		Initial: "red",
		Context: &TrafficLightContext{VehicleCount: 0},
		States: map[string]*sm.StateNode{
			"red": {
				ID:   "red",
				Type: sm.AtomicState,
				Entry: []sm.Action{
					func(ctx sm.Context, event sm.Event) sm.Context {
						fmt.Println("🔴 Red light is ON")
						return ctx
					},
				},
				On: map[string][]sm.Transition{
					"TIMER": {
						{
							Target: "green",
							Guard: func(ctx sm.Context, event sm.Event) bool {
								tc := ctx.(*TrafficLightContext)
								return !tc.IsEmergency
							},
						},
					},
					"EMERGENCY": {
						{
							Target: "yellow",
							Actions: []sm.Action{
								func(ctx sm.Context, event sm.Event) sm.Context {
									tc := ctx.(*TrafficLightContext)
									tc.IsEmergency = true
									return tc
								},
							},
						},
					},
				},
			},
			"yellow": {
				ID:   "yellow",
				Type: sm.AtomicState,
				Entry: []sm.Action{
					func(ctx sm.Context, event sm.Event) sm.Context {
						fmt.Println("🟡 Yellow light is ON")
						return ctx
					},
				},
				On: map[string][]sm.Transition{
					"TIMER": {
						{
							Target: "red",
							Guard: func(ctx sm.Context, event sm.Event) bool {
								tc := ctx.(*TrafficLightContext)
								return tc.IsEmergency
							},
						},
						{
							Target: "red",
							Guard: func(ctx sm.Context, event sm.Event) bool {
								tc := ctx.(*TrafficLightContext)
								return !tc.IsEmergency && tc.VehicleCount < 5
							},
						},
						{
							Target: "green",
							Guard: func(ctx sm.Context, event sm.Event) bool {
								tc := ctx.(*TrafficLightContext)
								return !tc.IsEmergency && tc.VehicleCount >= 5
							},
						},
					},
				},
			},
			"green": {
				ID:   "green",
				Type: sm.AtomicState,
				Entry: []sm.Action{
					func(ctx sm.Context, event sm.Event) sm.Context {
						fmt.Println("🟢 Green light is ON")
						tc := ctx.(*TrafficLightContext)
						tc.IsEmergency = false
						return tc
					},
				},
				On: map[string][]sm.Transition{
					"TIMER": {
						{Target: "yellow"},
					},
					"EMERGENCY": {
						{
							Target: "yellow",
							Actions: []sm.Action{
								func(ctx sm.Context, event sm.Event) sm.Context {
									tc := ctx.(*TrafficLightContext)
									tc.IsEmergency = true
									return tc
								},
							},
						},
					},
				},
			},
		},
	}

	machine := sm.NewMachine(config)

	machine.Subscribe(func(event sm.Event, state *sm.StateValue, ctx sm.Context) {
		tc := ctx.(*TrafficLightContext)
		fmt.Printf("Event: %s | State: %v | Vehicles: %d | Emergency: %v\n",
			event.Type, state.Get(), tc.VehicleCount, tc.IsEmergency)
	})

	if err := machine.Start(); err != nil {
		log.Fatal(err)
	}

	go func() {
		for i := 0; i < 10; i++ {
			time.Sleep(2 * time.Second)
			
			tc := machine.GetContext().(*TrafficLightContext)
			tc.VehicleCount = i * 2
			machine.SetContext(tc)
			
			if i == 3 {
				machine.Send(sm.Event{Type: "EMERGENCY"})
			} else {
				machine.Send(sm.Event{Type: "TIMER"})
			}
		}
		machine.Stop()
	}()

	time.Sleep(25 * time.Second)
	fmt.Println("Traffic light simulation completed")
}