package taskmanager

import (
	"testing"
	"time"
)

func TestNewWorkflow(t *testing.T) {
	tm := NewWorkflow("1", "Test Migration Workflow", "A workflow for testing")
	var hotSnapTask, coldSnapTask *Task

	hotSnapTaskEx := func() (TaskResult, error) {
		// Simulate a task that takes some time
		time.Sleep(3 * time.Second)
		return TaskResult{TaskID: "hotSnap", Success: true, Error: nil, Status: "completed"}, nil
	}
	hotSnapTask = NewTask("hotSnap", "Hot Snapshot", hotSnapTaskEx)

	coldSnapTaskEx := func() (TaskResult, error) {
		// Simulate a task that takes some time
		time.Sleep(2 * time.Second)
		return TaskResult{TaskID: "coldSnap", Success: true, Error: nil, Status: "completed"}, nil
	}
	coldSnapTask = NewTask("coldSnap", "Cold Snapshot", coldSnapTaskEx)
	err := tm.AddTask(hotSnapTask)
	if err != nil {
		panic(err)
	}
	err = tm.AddTask(coldSnapTask)
	if err != nil {
		panic(err)
	}
	err = tm.AddDependency("hotSnap", "coldSnap")
	if err != nil {
		panic(err)
	}
	err = tm.Execute()
	if err != nil {
		panic(err)
	}
}

func TestAddTask(t *testing.T) {
	w := NewWorkflow("wf2", "Add Task", "")
	task := &mockTask{id: "t1", name: "Task 1"}
	err := w.AddTask(task)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(w.Tasks) != 1 {
		t.Errorf("expected 1 task, got %d", len(w.Tasks))
	}
}

// func TestAddTask_Nil(t *testing.T) {
// 	w := NewWorkflow("wf3", "Nil Task", "")
// 	err := w.AddTask(nil)
// 	if err == nil {
// 		t.Error("expected error for nil task, got nil")
// 	}
// }

// func TestAddDependency(t *testing.T) {
// 	w := NewWorkflow("wf4", "Deps", "")
// 	task1 := &mockTask{id: "t1", name: "Task 1"}
// 	task2 := &mockTask{id: "t2", name: "Task 2"}
// 	_ = w.AddTask(task1)
// 	_ = w.AddTask(task2)
// 	err := w.AddDependency("t1", "t2")
// 	if err != nil {
// 		t.Errorf("expected no error, got %v", err)
// 	}
// }

// func TestAddDependency_EmptyID(t *testing.T) {
// 	w := NewWorkflow("wf5", "Deps", "")
// 	err := w.AddDependency("", "t2")
// 	if err == nil {
// 		t.Error("expected error for empty task ID, got nil")
// 	}
// }

// func TestExecute_Success(t *testing.T) {
// 	w := NewWorkflow("wf6", "Exec", "")
// 	task := &mockTask{id: "t1", name: "Task 1"}
// 	_ = w.AddTask(task)
// 	err := w.Execute()
// 	if err != nil {
// 		t.Errorf("expected no error, got %v", err)
// 	}
// 	if w.Status != "completed" {
// 		t.Errorf("expected status completed, got %s", w.Status)
// 	}
// 	if w.Duration <= 0 {
// 		t.Error("expected positive duration")
// 	}
// 	if !task.start || !task.finish {
// 		t.Error("expected task Start and Finish to be called")
// 	}
// }

// func TestExecute_Failure(t *testing.T) {
// 	w := NewWorkflow("wf7", "ExecFail", "")
// 	task := &mockTask{id: "t1", name: "Task 1", execErr: errors.New("fail")}
// 	_ = w.AddTask(task)
// 	err := w.Execute()
// 	if err == nil {
// 		t.Error("expected error, got nil")
// 	}
// 	if w.Status != "failed" {
// 		t.Errorf("expected status failed, got %s", w.Status)
// 	}
// }

// func TestExecute_NoTasks(t *testing.T) {
// 	w := NewWorkflow("wf8", "NoTasks", "")
// 	err := w.Execute()
// 	if err != nil {
// 		t.Errorf("expected no error, got %v", err)
// 	}
// 	if w.Status != "running" && w.Status != "" {
// 		t.Errorf("expected status running or empty, got %s", w.Status)
// 	}
// }
