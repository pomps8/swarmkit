package global

import (
	"testing"

	"github.com/docker/go-events"
	"github.com/docker/swarmkit/api"
	"github.com/docker/swarmkit/manager/orchestrator/testutils"
	"github.com/docker/swarmkit/manager/state"
	"github.com/docker/swarmkit/manager/state/store"
	"github.com/stretchr/testify/assert"
	"golang.org/x/net/context"
)

var (
	node1 = &api.Node{
		ID: "nodeid1",
		Description: &api.NodeDescription{
			Hostname: "name1",
		},
	}
	node2 = &api.Node{
		ID: "nodeid2",
		Description: &api.NodeDescription{
			Hostname: "name2",
		},
	}

	service1 = &api.Service{
		ID: "serviceid1",
		Spec: api.ServiceSpec{
			Annotations: api.Annotations{
				Name: "name1",
			},
			Task: api.TaskSpec{
				Runtime: &api.TaskSpec_Container{
					Container: &api.ContainerSpec{},
				},
			},
			Mode: &api.ServiceSpec_Global{
				Global: &api.GlobalService{},
			},
		},
	}

	service2 = &api.Service{
		ID: "serviceid2",
		Spec: api.ServiceSpec{
			Annotations: api.Annotations{
				Name: "name2",
			},
			Task: api.TaskSpec{
				Runtime: &api.TaskSpec_Container{
					Container: &api.ContainerSpec{},
				},
			},
			Mode: &api.ServiceSpec_Global{
				Global: &api.GlobalService{},
			},
		},
	}
)

func SetupCluster(t *testing.T, store *store.MemoryStore, watch chan events.Event) *api.Task {
	ctx := context.Background()
	// Start the global orchestrator.
	global := NewGlobalOrchestrator(store)
	go func() {
		assert.NoError(t, global.Run(ctx))
	}()

	addService(t, store, service1)
	testutils.Expect(t, watch, state.EventCreateService{})
	testutils.Expect(t, watch, state.EventCommit{})

	addNode(t, store, node1)
	testutils.Expect(t, watch, state.EventCreateNode{})
	testutils.Expect(t, watch, state.EventCommit{})

	// return task creation from orchestrator
	return testutils.WatchTaskCreate(t, watch)
}

func TestSetup(t *testing.T) {
	store := store.NewMemoryStore(nil)
	assert.NotNil(t, store)
	defer store.Close()

	watch, cancel := state.Watch(store.WatchQueue() /*state.EventCreateTask{}, state.EventUpdateTask{}*/)
	defer cancel()

	observedTask1 := SetupCluster(t, store, watch)

	assert.Equal(t, observedTask1.Status.State, api.TaskStateNew)
	assert.Equal(t, observedTask1.ServiceAnnotations.Name, "name1")
	assert.Equal(t, observedTask1.NodeID, "nodeid1")
}

func TestAddNode(t *testing.T) {
	store := store.NewMemoryStore(nil)
	assert.NotNil(t, store)
	defer store.Close()

	watch, cancel := state.Watch(store.WatchQueue())
	defer cancel()

	SetupCluster(t, store, watch)

	addNode(t, store, node2)
	observedTask2 := testutils.WatchTaskCreate(t, watch)
	assert.Equal(t, observedTask2.Status.State, api.TaskStateNew)
	assert.Equal(t, observedTask2.ServiceAnnotations.Name, "name1")
	assert.Equal(t, observedTask2.NodeID, "nodeid2")
}

func TestDeleteNode(t *testing.T) {
	store := store.NewMemoryStore(nil)
	assert.NotNil(t, store)
	defer store.Close()

	watch, cancel := state.Watch(store.WatchQueue())
	defer cancel()

	SetupCluster(t, store, watch)

	deleteNode(t, store, node1)
	// task should be set to dead
	observedTask := testutils.WatchShutdownTask(t, watch)
	assert.Equal(t, observedTask.ServiceAnnotations.Name, "name1")
	assert.Equal(t, observedTask.NodeID, "nodeid1")
}

func TestNodeAvailability(t *testing.T) {
	store := store.NewMemoryStore(nil)
	assert.NotNil(t, store)
	defer store.Close()

	watch, cancel := state.Watch(store.WatchQueue())
	defer cancel()

	SetupCluster(t, store, watch)

	node1.Status.State = api.NodeStatus_READY
	node1.Spec.Availability = api.NodeAvailabilityActive

	// set node1 to drain
	updateNodeAvailability(t, store, node1, api.NodeAvailabilityDrain)

	// task should be set to dead
	observedTask1 := testutils.WatchShutdownTask(t, watch)
	assert.Equal(t, observedTask1.ServiceAnnotations.Name, "name1")
	assert.Equal(t, observedTask1.NodeID, "nodeid1")

	// set node1 to active
	updateNodeAvailability(t, store, node1, api.NodeAvailabilityActive)
	// task should be added back
	observedTask2 := testutils.WatchTaskCreate(t, watch)
	assert.Equal(t, observedTask2.Status.State, api.TaskStateNew)
	assert.Equal(t, observedTask2.ServiceAnnotations.Name, "name1")
	assert.Equal(t, observedTask2.NodeID, "nodeid1")
}

func TestAddService(t *testing.T) {
	store := store.NewMemoryStore(nil)
	assert.NotNil(t, store)
	defer store.Close()

	watch, cancel := state.Watch(store.WatchQueue())
	defer cancel()

	SetupCluster(t, store, watch)

	addService(t, store, service2)
	observedTask := testutils.WatchTaskCreate(t, watch)
	assert.Equal(t, observedTask.Status.State, api.TaskStateNew)
	assert.Equal(t, observedTask.ServiceAnnotations.Name, "name2")
	assert.True(t, observedTask.NodeID == "nodeid1")
}

func TestDeleteService(t *testing.T) {
	store := store.NewMemoryStore(nil)
	assert.NotNil(t, store)
	defer store.Close()

	watch, cancel := state.Watch(store.WatchQueue())
	defer cancel()

	SetupCluster(t, store, watch)

	deleteService(t, store, service1)
	// task should be deleted
	observedTask := testutils.WatchTaskDelete(t, watch)
	assert.Equal(t, observedTask.ServiceAnnotations.Name, "name1")
	assert.Equal(t, observedTask.NodeID, "nodeid1")
}

func TestRemoveTask(t *testing.T) {
	store := store.NewMemoryStore(nil)
	assert.NotNil(t, store)
	defer store.Close()

	watch, cancel := state.Watch(store.WatchQueue() /*state.EventCreateTask{}, state.EventUpdateTask{}*/)
	defer cancel()

	observedTask1 := SetupCluster(t, store, watch)

	assert.Equal(t, observedTask1.Status.State, api.TaskStateNew)
	assert.Equal(t, observedTask1.ServiceAnnotations.Name, "name1")
	assert.Equal(t, observedTask1.NodeID, "nodeid1")

	// delete the task
	deleteTask(t, store, observedTask1)

	// the task should be recreated
	observedTask2 := testutils.WatchTaskCreate(t, watch)
	assert.Equal(t, observedTask2.Status.State, api.TaskStateNew)
	assert.Equal(t, observedTask2.ServiceAnnotations.Name, "name1")
	assert.Equal(t, observedTask2.NodeID, "nodeid1")
}

func addService(t *testing.T, s *store.MemoryStore, service *api.Service) {
	s.Update(func(tx store.Tx) error {
		assert.NoError(t, store.CreateService(tx, service))
		return nil
	})
}

func deleteService(t *testing.T, s *store.MemoryStore, service *api.Service) {
	s.Update(func(tx store.Tx) error {
		assert.NoError(t, store.DeleteService(tx, service.ID))
		return nil
	})
}

func addNode(t *testing.T, s *store.MemoryStore, node *api.Node) {
	s.Update(func(tx store.Tx) error {
		assert.NoError(t, store.CreateNode(tx, node))
		return nil
	})
}

func updateNodeAvailability(t *testing.T, s *store.MemoryStore, node *api.Node, avail api.NodeSpec_Availability) {
	node.Spec.Availability = avail
	s.Update(func(tx store.Tx) error {
		assert.NoError(t, store.UpdateNode(tx, node))
		return nil
	})
}

func deleteNode(t *testing.T, s *store.MemoryStore, node *api.Node) {
	s.Update(func(tx store.Tx) error {
		assert.NoError(t, store.DeleteNode(tx, node.ID))
		return nil
	})
}

func addTask(t *testing.T, s *store.MemoryStore, task *api.Task) {
	s.Update(func(tx store.Tx) error {
		assert.NoError(t, store.CreateTask(tx, task))
		return nil
	})
}

func deleteTask(t *testing.T, s *store.MemoryStore, task *api.Task) {
	s.Update(func(tx store.Tx) error {
		assert.NoError(t, store.DeleteTask(tx, task.ID))
		return nil
	})
}

func TestInitializationRejectedTasks(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemoryStore(nil)
	assert.NotNil(t, s)
	defer s.Close()

	// create nodes, services and tasks in store directly
	// where orchestrator runs, it should fix tasks to declarative state
	addNode(t, s, node1)
	addService(t, s, service1)
	tasks := []*api.Task{
		// nodeid1 has a rejected task for serviceid1
		{
			ID:           "task1",
			Slot:         0,
			DesiredState: api.TaskStateReady,
			Status: api.TaskStatus{
				State: api.TaskStateRejected,
			},
			Spec: api.TaskSpec{
				Runtime: &api.TaskSpec_Container{
					Container: &api.ContainerSpec{},
				},
			},
			ServiceAnnotations: api.Annotations{
				Name: "task1",
			},
			ServiceID: "serviceid1",
			NodeID:    "nodeid1",
		},
	}
	for _, task := range tasks {
		addTask(t, s, task)
	}

	// watch orchestration events
	watch, cancel := state.Watch(s.WatchQueue(), state.EventCreateTask{}, state.EventUpdateTask{}, state.EventDeleteTask{})
	defer cancel()

	orchestrator := NewGlobalOrchestrator(s)
	defer orchestrator.Stop()

	go func() {
		assert.NoError(t, orchestrator.Run(ctx))
	}()

	observedTask1 := testutils.WatchTaskUpdate(t, watch)
	assert.Equal(t, observedTask1.ID, "task1")
	assert.Equal(t, observedTask1.Status.State, api.TaskStateRejected)
	assert.Equal(t, observedTask1.DesiredState, api.TaskStateReady)

	observedTask2 := testutils.WatchTaskUpdate(t, watch)
	assert.Equal(t, observedTask2.ID, "task1")
	assert.Equal(t, observedTask2.Status.State, api.TaskStateRejected)
	assert.Equal(t, observedTask2.DesiredState, api.TaskStateShutdown)

	observedTask3 := testutils.WatchTaskCreate(t, watch)
	assert.Equal(t, observedTask3.NodeID, "nodeid1")
	assert.Equal(t, observedTask3.Status.State, api.TaskStateNew)
	assert.Equal(t, observedTask3.DesiredState, api.TaskStateReady)
}

func TestInitializationFailedTasks(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemoryStore(nil)
	assert.NotNil(t, s)
	defer s.Close()

	// create nodes, services and tasks in store directly
	// where orchestrator runs, it should fix tasks to declarative state
	addNode(t, s, node1)
	addService(t, s, service1)
	tasks := []*api.Task{
		// nodeid1 has a failed task for serviceid1
		{
			ID:           "task1",
			Slot:         0,
			DesiredState: api.TaskStateRunning,
			Status: api.TaskStatus{
				State: api.TaskStateFailed,
			},
			Spec: api.TaskSpec{
				Runtime: &api.TaskSpec_Container{
					Container: &api.ContainerSpec{},
				},
			},
			ServiceAnnotations: api.Annotations{
				Name: "task1",
			},
			ServiceID: "serviceid1",
			NodeID:    "nodeid1",
		},
	}
	for _, task := range tasks {
		addTask(t, s, task)
	}

	// watch orchestration events
	watch, cancel := state.Watch(s.WatchQueue(), state.EventCreateTask{}, state.EventUpdateTask{}, state.EventDeleteTask{})
	defer cancel()

	orchestrator := NewGlobalOrchestrator(s)
	defer orchestrator.Stop()

	go func() {
		assert.NoError(t, orchestrator.Run(ctx))
	}()

	observedTask1 := testutils.WatchTaskUpdate(t, watch)
	assert.Equal(t, observedTask1.ID, "task1")
	assert.Equal(t, observedTask1.Status.State, api.TaskStateFailed)
	assert.Equal(t, observedTask1.DesiredState, api.TaskStateRunning)

	observedTask2 := testutils.WatchTaskUpdate(t, watch)
	assert.Equal(t, observedTask2.ID, "task1")
	assert.Equal(t, observedTask2.Status.State, api.TaskStateFailed)
	assert.Equal(t, observedTask2.DesiredState, api.TaskStateShutdown)

	observedTask3 := testutils.WatchTaskCreate(t, watch)
	assert.Equal(t, observedTask3.NodeID, "nodeid1")
	assert.Equal(t, observedTask3.Status.State, api.TaskStateNew)
	assert.Equal(t, observedTask3.DesiredState, api.TaskStateReady)
}

func TestInitializationExtraTask(t *testing.T) {
	ctx := context.Background()
	s := store.NewMemoryStore(nil)
	assert.NotNil(t, s)
	defer s.Close()

	// create nodes, services and tasks in store directly
	// where orchestrator runs, it should fix tasks to declarative state
	addNode(t, s, node1)
	addService(t, s, service1)
	tasks := []*api.Task{
		// nodeid1 has 2 tasks for serviceid1
		{
			ID:           "task1",
			Slot:         0,
			DesiredState: api.TaskStateRunning,
			Status: api.TaskStatus{
				State: api.TaskStateRunning,
			},
			Spec: api.TaskSpec{
				Runtime: &api.TaskSpec_Container{
					Container: &api.ContainerSpec{},
				},
			},
			ServiceAnnotations: api.Annotations{
				Name: "task1",
			},
			ServiceID: "serviceid1",
			NodeID:    "nodeid1",
		},
		{
			ID:           "task2",
			Slot:         0,
			DesiredState: api.TaskStateRunning,
			Status: api.TaskStatus{
				State: api.TaskStateRunning,
			},
			Spec: api.TaskSpec{
				Runtime: &api.TaskSpec_Container{
					Container: &api.ContainerSpec{},
				},
			},
			ServiceAnnotations: api.Annotations{
				Name: "task2",
			},
			ServiceID: "serviceid1",
			NodeID:    "nodeid1",
		},
	}
	for _, task := range tasks {
		addTask(t, s, task)
	}

	// watch orchestration events
	watch, cancel := state.Watch(s.WatchQueue(), state.EventCreateTask{}, state.EventUpdateTask{}, state.EventDeleteTask{})
	defer cancel()

	orchestrator := NewGlobalOrchestrator(s)
	defer orchestrator.Stop()

	go func() {
		assert.NoError(t, orchestrator.Run(ctx))
	}()

	observedTask1 := testutils.WatchTaskUpdate(t, watch)
	assert.True(t, observedTask1.ID == "task1" || observedTask1.ID == "task2")
	assert.Equal(t, observedTask1.Status.State, api.TaskStateRunning)
	assert.Equal(t, observedTask1.DesiredState, api.TaskStateShutdown)
}
