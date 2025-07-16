package statemachine

import (
	"fmt"
	"strings"
)

type HierarchicalMachine struct {
	*Machine
	stateStack []string
}

func NewHierarchicalMachine(config MachineConfig) *HierarchicalMachine {
	m := NewMachine(config)
	return &HierarchicalMachine{
		Machine:    m,
		stateStack: []string{},
	}
}

func (hm *HierarchicalMachine) Start() error {
	hm.mu.Lock()
	if hm.running {
		hm.mu.Unlock()
		return fmt.Errorf("machine %s is already running", hm.ID)
	}
	hm.running = true
	hm.mu.Unlock()

	initial := hm.State.Get()["root"]
	if node := hm.findStateNode(initial); node != nil {
		hm.executeActions(node.Entry)
	}
	
	// Initialize compound states properly
	hm.mu.Lock()
	hm.initializeCompoundState(initial)
	hm.mu.Unlock()

	go hm.eventLoop()
	return nil
}

func (hm *HierarchicalMachine) initializeCompoundState(stateID string) {
	node := hm.findStateNode(stateID)
	if node == nil {
		return
	}
	
	if node.Type == CompoundState && node.Initial != "" {
		hm.State.Set("root", node.Initial)
		
		// Execute entry actions for the initial state
		initialNode := hm.findStateNode(node.Initial)
		if initialNode != nil && initialNode.Entry != nil {
			hm.executeActions(initialNode.Entry)
		}
		
		// Recursively initialize if it's also a compound state
		hm.initializeCompoundState(node.Initial)
	}
}

func (hm *HierarchicalMachine) processEvent(event Event) {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	currentStates := hm.State.Get()
	transitioned := false

	for path, stateID := range currentStates {
		statePath := hm.getStatePath(stateID)
		
		for i := len(statePath) - 1; i >= 0 && !transitioned; i-- {
			node := hm.findStateNode(statePath[i])
			if node == nil {
				continue
			}

			transitions, exists := node.On[event.Type]
			if !exists {
				continue
			}

			for _, transition := range transitions {
				if transition.Guard != nil && !transition.Guard(hm.Context, event) {
					continue
				}

				hm.exitStates(statePath[i:])

				for _, action := range transition.Actions {
					if action != nil {
						hm.Context = action(hm.Context, event)
					}
				}

				targetNode := hm.findStateNode(transition.Target)
				if targetNode != nil {
					hm.State.Set(path, transition.Target)
					
					// Enter the target state
					if targetNode.Entry != nil {
						hm.executeActions(targetNode.Entry)
					}
					
					// If it's a compound state, enter its initial state
					if targetNode.Type == CompoundState && targetNode.Initial != "" {
						hm.State.Set(path, targetNode.Initial)
						hm.initializeCompoundState(targetNode.Initial)
					}
					
					transitioned = true
					break
				}
			}
		}
	}

	if transitioned {
		hm.notifyListeners(event)
		hm.checkAlwaysTransitions()
	}
}

func (hm *HierarchicalMachine) getStatePath(stateID string) []string {
	path := []string{}
	node := hm.findStateNode(stateID)
	
	for node != nil {
		path = append([]string{node.ID}, path...)
		node = hm.findParentNode(node.ID)
	}
	
	return path
}

func (hm *HierarchicalMachine) findParentNode(childID string) *StateNode {
	return findParentNodeRecursive(hm.Root, childID)
}

func findParentNodeRecursive(node *StateNode, childID string) *StateNode {
	for _, child := range node.States {
		if child.ID == childID {
			return node
		}
		if parent := findParentNodeRecursive(child, childID); parent != nil {
			return parent
		}
	}
	return nil
}

func (hm *HierarchicalMachine) exitStates(states []string) {
	for i := len(states) - 1; i >= 0; i-- {
		node := hm.findStateNode(states[i])
		if node != nil && node.Exit != nil {
			hm.executeActions(node.Exit)
		}
	}
}

func (hm *HierarchicalMachine) enterStates(states []string) {
	for _, stateID := range states {
		node := hm.findStateNode(stateID)
		if node != nil {
			if node.Entry != nil {
				hm.executeActions(node.Entry)
			}
			
			if node.Type == CompoundState && node.Initial != "" {
				childPath := hm.getStatePath(node.Initial)
				// Only enter child states that are not already in the path
				if len(childPath) > len(states) {
					hm.enterStates(childPath[len(states):])
				}
			}
		}
	}
}

func (hm *HierarchicalMachine) Send(event Event) error {
	hm.mu.RLock()
	if !hm.running {
		hm.mu.RUnlock()
		return fmt.Errorf("machine %s is not running", hm.ID)
	}
	hm.mu.RUnlock()

	select {
	case hm.eventChan <- event:
		return nil
	default:
		return fmt.Errorf("event channel is full")
	}
}

func (hm *HierarchicalMachine) eventLoop() {
	for {
		select {
		case event := <-hm.eventChan:
			hm.processEvent(event)
		case <-hm.stopChan:
			return
		}
	}
}

func (hm *HierarchicalMachine) GetStateHierarchy() string {
	states := hm.State.Get()
	hierarchy := []string{}
	
	for _, stateID := range states {
		path := hm.getStatePath(stateID)
		hierarchy = append(hierarchy, strings.Join(path, "."))
	}
	
	return strings.Join(hierarchy, ", ")
}