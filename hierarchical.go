package statemachine

import "strings"

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

				targetPath := hm.getStatePath(transition.Target)
				hm.enterStates(targetPath)

				hm.State.Set(path, transition.Target)
				transitioned = true
				break
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
				hm.enterStates(childPath[len(states):])
			}
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