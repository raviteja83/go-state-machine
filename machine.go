package statemachine

import (
	"fmt"
	"sync"
)

type StateType string

const (
	AtomicState StateType = "atomic"
	CompoundState StateType = "compound"
	ParallelState StateType = "parallel"
	FinalState StateType = "final"
)

type Event struct {
	Type string
	Data interface{}
}

type Context interface{}

type Guard func(ctx Context, event Event) bool

type Action func(ctx Context, event Event) Context

type Transition struct {
	Target  string
	Guard   Guard
	Actions []Action
}

type StateNode struct {
	ID          string
	Type        StateType
	Initial     string
	States      map[string]*StateNode
	On          map[string][]Transition
	Entry       []Action
	Exit        []Action
	Always      []Transition
	Final       bool
	Description string
}

type StateValue struct {
	Current map[string]string
	mu      sync.RWMutex
}

func NewStateValue(initial string) *StateValue {
	return &StateValue{
		Current: map[string]string{"root": initial},
	}
}

func (sv *StateValue) Get() map[string]string {
	sv.mu.RLock()
	defer sv.mu.RUnlock()
	result := make(map[string]string)
	for k, v := range sv.Current {
		result[k] = v
	}
	return result
}

func (sv *StateValue) Set(path string, value string) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	sv.Current[path] = value
}

func (sv *StateValue) Matches(stateID string) bool {
	sv.mu.RLock()
	defer sv.mu.RUnlock()
	for _, v := range sv.Current {
		if v == stateID {
			return true
		}
	}
	return false
}

type Machine struct {
	ID           string
	Root         *StateNode
	Context      Context
	State        *StateValue
	mu           sync.RWMutex
	eventChan    chan Event
	stopChan     chan struct{}
	running      bool
	listeners    []func(Event, *StateValue, Context)
	listenersMu  sync.RWMutex
}

type MachineConfig struct {
	ID      string
	Initial string
	Context Context
	States  map[string]*StateNode
}

func NewMachine(config MachineConfig) *Machine {
	root := &StateNode{
		ID:      "root",
		Type:    CompoundState,
		Initial: config.Initial,
		States:  config.States,
		On:      make(map[string][]Transition),
	}

	m := &Machine{
		ID:        config.ID,
		Root:      root,
		Context:   config.Context,
		State:     NewStateValue(config.Initial),
		eventChan: make(chan Event, 100),
		stopChan:  make(chan struct{}),
	}

	return m
}

func (m *Machine) Start() error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return fmt.Errorf("machine %s is already running", m.ID)
	}
	m.running = true
	m.mu.Unlock()

	initial := m.State.Get()["root"]
	if node := m.findStateNode(initial); node != nil {
		m.executeActions(node.Entry)
	}

	go m.eventLoop()
	
	// Check always transitions after initial state
	m.mu.Lock()
	m.checkAlwaysTransitions()
	m.mu.Unlock()
	
	return nil
}

func (m *Machine) Stop() {
	m.mu.Lock()
	if !m.running {
		m.mu.Unlock()
		return
	}
	m.running = false
	m.mu.Unlock()

	close(m.stopChan)
}

func (m *Machine) Send(event Event) error {
	m.mu.RLock()
	if !m.running {
		m.mu.RUnlock()
		return fmt.Errorf("machine %s is not running", m.ID)
	}
	m.mu.RUnlock()

	select {
	case m.eventChan <- event:
		return nil
	default:
		return fmt.Errorf("event channel is full")
	}
}

func (m *Machine) Subscribe(listener func(Event, *StateValue, Context)) {
	m.listenersMu.Lock()
	defer m.listenersMu.Unlock()
	m.listeners = append(m.listeners, listener)
}

func (m *Machine) eventLoop() {
	for {
		select {
		case event := <-m.eventChan:
			m.processEvent(event)
		case <-m.stopChan:
			return
		}
	}
}

func (m *Machine) processEvent(event Event) {
	m.mu.Lock()
	defer m.mu.Unlock()

	currentStates := m.State.Get()
	transitioned := false

	for path, stateID := range currentStates {
		node := m.findStateNode(stateID)
		if node == nil {
			continue
		}

		transitions, exists := node.On[event.Type]
		if !exists {
			continue
		}

		for _, transition := range transitions {
			if transition.Guard != nil && !transition.Guard(m.Context, event) {
				continue
			}

			if node.Exit != nil {
				m.executeActions(node.Exit)
			}

			for _, action := range transition.Actions {
				if action != nil {
					m.Context = action(m.Context, event)
				}
			}

			targetNode := m.findStateNode(transition.Target)
			if targetNode != nil {
				// Even if transitioning to the same state, execute exit/entry
				if stateID == transition.Target && node.Exit != nil {
					// Already executed exit above
				}
				
				m.State.Set(path, transition.Target)
				
				if targetNode.Entry != nil {
					m.executeActions(targetNode.Entry)
				}

				transitioned = true
				break
			}
		}

		if transitioned {
			break
		}
	}

	if transitioned {
		m.notifyListeners(event)
		m.checkAlwaysTransitions()
	}
}

func (m *Machine) checkAlwaysTransitions() {
	currentStates := m.State.Get()
	
	for path, stateID := range currentStates {
		node := m.findStateNode(stateID)
		if node == nil || node.Always == nil {
			continue
		}

		for _, transition := range node.Always {
			if transition.Guard != nil && !transition.Guard(m.Context, Event{Type: "always"}) {
				continue
			}

			if node.Exit != nil {
				m.executeActions(node.Exit)
			}

			for _, action := range transition.Actions {
				if action != nil {
					m.Context = action(m.Context, Event{Type: "always"})
				}
			}

			targetNode := m.findStateNode(transition.Target)
			if targetNode != nil {
				m.State.Set(path, transition.Target)
				
				if targetNode.Entry != nil {
					m.executeActions(targetNode.Entry)
				}

				m.notifyListeners(Event{Type: "always"})
				break
			}
		}
	}
}

func (m *Machine) executeActions(actions []Action) {
	for _, action := range actions {
		if action != nil {
			m.Context = action(m.Context, Event{})
		}
	}
}

func (m *Machine) findStateNode(id string) *StateNode {
	return findStateNodeRecursive(m.Root, id)
}

func findStateNodeRecursive(node *StateNode, id string) *StateNode {
	if node.ID == id {
		return node
	}

	for _, child := range node.States {
		if result := findStateNodeRecursive(child, id); result != nil {
			return result
		}
	}

	return nil
}

func (m *Machine) notifyListeners(event Event) {
	m.listenersMu.RLock()
	defer m.listenersMu.RUnlock()

	state := m.State
	ctx := m.Context

	for _, listener := range m.listeners {
		go listener(event, state, ctx)
	}
}

func (m *Machine) GetContext() Context {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.Context
}

func (m *Machine) SetContext(ctx Context) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Context = ctx
}

func (m *Machine) IsInState(stateID string) bool {
	return m.State.Matches(stateID)
}

func (m *Machine) GetState() map[string]string {
	return m.State.Get()
}