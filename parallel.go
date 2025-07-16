package statemachine

import (
	"fmt"
	"strings"
)

type ParallelMachine struct {
	*Machine
	regions map[string]*Machine
}

func NewParallelMachine(config MachineConfig) *ParallelMachine {
	m := NewMachine(config)
	pm := &ParallelMachine{
		Machine: m,
		regions: make(map[string]*Machine),
	}
	
	pm.initializeRegions()
	return pm
}

func (pm *ParallelMachine) initializeRegions() {
	pm.walkStateTree(pm.Root, func(node *StateNode) {
		if node.Type == ParallelState {
			for regionID, regionNode := range node.States {
				fullRegionID := fmt.Sprintf("%s.%s", node.ID, regionID)
				regionConfig := MachineConfig{
					ID:      fullRegionID,
					Initial: regionNode.Initial,
					Context: pm.Context,
					States:  regionNode.States,
				}
				pm.regions[fullRegionID] = NewMachine(regionConfig)
			}
		}
	})
}

func (pm *ParallelMachine) walkStateTree(node *StateNode, visitor func(*StateNode)) {
	visitor(node)
	for _, child := range node.States {
		pm.walkStateTree(child, visitor)
	}
}

func (pm *ParallelMachine) Start() error {
	if err := pm.Machine.Start(); err != nil {
		return err
	}
	
	for _, region := range pm.regions {
		if err := region.Start(); err != nil {
			return fmt.Errorf("failed to start region %s: %w", region.ID, err)
		}
	}
	
	return nil
}

func (pm *ParallelMachine) Stop() {
	for _, region := range pm.regions {
		region.Stop()
	}
	pm.Machine.Stop()
}

func (pm *ParallelMachine) Send(event Event) error {
	pm.mu.RLock()
	if !pm.running {
		pm.mu.RUnlock()
		return fmt.Errorf("machine %s is not running", pm.ID)
	}
	pm.mu.RUnlock()
	
	// Send to main machine first
	if err := pm.Machine.Send(event); err != nil {
		return err
	}
	
	// Also send to active regions
	currentStates := pm.State.Get()
	for _, stateID := range currentStates {
		node := pm.findStateNode(stateID)
		if node != nil && node.Type == ParallelState {
			// Send to all regions of this parallel state
			for _, region := range pm.regions {
				if strings.HasPrefix(region.ID, node.ID) {
					if err := region.Send(event); err != nil {
						// Log but don't fail - regions might not handle all events
						continue
					}
				}
			}
		}
	}
	
	return nil
}

func (pm *ParallelMachine) processEvent(event Event) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	currentStates := pm.State.Get()
	
	for path, stateID := range currentStates {
		node := pm.findStateNode(stateID)
		if node == nil {
			continue
		}
		
		if node.Type == ParallelState {
			// Process the event for all regions first
			for _, region := range pm.regions {
				if strings.HasPrefix(region.ID, node.ID) {
					region.Send(event)
				}
			}
			// Then check if all regions are in final state
			pm.processParallelState(node, event)
			continue
		}
		
		transitions, exists := node.On[event.Type]
		if !exists {
			continue
		}

		for _, transition := range transitions {
			if transition.Guard != nil && !transition.Guard(pm.Context, event) {
				continue
			}

			if node.Exit != nil {
				pm.executeActions(node.Exit)
			}

			for _, action := range transition.Actions {
				if action != nil {
					pm.Context = action(pm.Context, event)
				}
			}

			targetNode := pm.findStateNode(transition.Target)
			if targetNode != nil {
				pm.State.Set(path, transition.Target)
				
				if targetNode.Entry != nil {
					pm.executeActions(targetNode.Entry)
				}
				
				if targetNode.Type == ParallelState {
					pm.enterParallelState(targetNode)
				}
			}
			break
		}
	}
	
	pm.notifyListeners(event)
}

func (pm *ParallelMachine) processParallelState(node *StateNode, event Event) {
	// Check if we should process a done transition
	allRegionsInFinal := true
	
	for _, region := range pm.regions {
		if !strings.HasPrefix(region.ID, node.ID) {
			continue
		}
		
		regionStates := region.GetState()
		inFinal := false
		
		for _, stateID := range regionStates {
			regionNode := region.findStateNode(stateID)
			if regionNode != nil && regionNode.Final {
				inFinal = true
				break
			}
		}
		
		if !inFinal {
			allRegionsInFinal = false
			break
		}
	}
	
	if allRegionsInFinal {
		// Check for "done" transitions
		transitions, exists := node.On["done"]
		if exists {
			for _, transition := range transitions {
				if transition.Guard == nil || transition.Guard(pm.Context, Event{Type: "done"}) {
					targetNode := pm.findStateNode(transition.Target)
					if targetNode != nil {
						pm.State.Set("root", transition.Target)
						
						if targetNode.Entry != nil {
							pm.executeActions(targetNode.Entry)
						}
						
						pm.notifyListeners(Event{Type: "done"})
						break
					}
				}
			}
		}
	}
}

func (pm *ParallelMachine) enterParallelState(node *StateNode) {
	for regionID := range node.States {
		fullRegionID := fmt.Sprintf("%s.%s", node.ID, regionID)
		if region, exists := pm.regions[fullRegionID]; exists {
			region.Start()
		}
	}
}

func (pm *ParallelMachine) GetParallelStates() map[string]map[string]string {
	result := make(map[string]map[string]string)
	
	for regionID, region := range pm.regions {
		result[regionID] = region.GetState()
	}
	
	return result
}

func (pm *ParallelMachine) IsAllRegionsInState(statePattern map[string]string) bool {
	for regionID, expectedState := range statePattern {
		if region, exists := pm.regions[regionID]; exists {
			if !region.IsInState(expectedState) {
				return false
			}
		} else {
			return false
		}
	}
	return true
}