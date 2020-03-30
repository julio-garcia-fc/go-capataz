package s_test

//
// NOTE: If you feel it is counter-intuitive to have workers start before
// supervisors in the assertions bellow, check stest/README.md
//

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/capatazlib/go-capataz/c"
	"github.com/capatazlib/go-capataz/s"
	. "github.com/capatazlib/go-capataz/stest"
)

func TestPermanentOneForOneSingleFailingChildRecovers(t *testing.T) {
	parentName := "root"
	// Fail only one time
	child1, failChild1 := FailOnSignalChild(1, "child1", c.WithRestart(c.Permanent))

	events, err := ObserveSupervisor(
		context.TODO(),
		parentName,
		[]s.Opt{
			s.WithChildren(child1),
		},
		func(em EventManager) {
			// NOTE: we won't stop the supervisor until the child has failed at least
			// once
			evIt := em.Iterator()
			// 1) Wait till all the tree is up
			evIt.SkipTill(SupervisorStarted("root"))
			// 2) Start the failing behavior of child1
			failChild1(true /* done */)
			// 3) Wait till first restart
			evIt.SkipTill(WorkerStarted("root/child1"))
		},
	)

	assert.NoError(t, err)

	AssertExactMatch(t, events,
		[]EventP{
			// start children from left to right
			WorkerStarted("root/child1"),
			SupervisorStarted("root"),
			// ^^^ 1) failChild1 starts executing here
			WorkerFailed("root/child1"),
			// ^^^ 2) And then we see a new (re)start of it
			WorkerStarted("root/child1"),
			// ^^^ 3) After 1st (re)start we stop
			WorkerStopped("root/child1"),
			SupervisorStopped("root"),
		},
	)
}

func TestPermanentOneForOneNestedFailingChildRecovers(t *testing.T) {
	parentName := "root"
	// Fail only one time
	child1, failChild1 := FailOnSignalChild(1, "child1", c.WithRestart(c.Permanent))
	tree1 := s.New("subtree1", s.WithChildren(child1))

	events, err := ObserveSupervisor(
		context.TODO(),
		parentName,
		[]s.Opt{s.WithSubtree(tree1)},
		func(em EventManager) {
			// NOTE: we won't stop the supervisor until the child has failed at least
			// once
			evIt := em.Iterator()
			// 1) Wait till all the tree is up
			evIt.SkipTill(SupervisorStarted("root"))
			// 2) Start the failing behavior of child1
			failChild1(true /* done */)
			// 3) Wait till first restart
			evIt.SkipTill(WorkerStarted("root/subtree1/child1"))
		},
	)

	assert.NoError(t, err)

	AssertExactMatch(t, events,
		[]EventP{
			// start children from left to right
			WorkerStarted("root/subtree1/child1"),
			SupervisorStarted("root/subtree1"),
			SupervisorStarted("root"),
			// ^^^ 1) Wait till root starts
			WorkerFailed("root/subtree1/child1"),
			// ^^^ 2) We see the failChild1 causing the error
			WorkerStarted("root/subtree1/child1"),
			// ^^^ 3) After 1st (re)start we stop
			WorkerStopped("root/subtree1/child1"),
			SupervisorStopped("root/subtree1"),
			SupervisorStopped("root"),
		},
	)
}

func TestPermanentOneForOneSingleFailingChildReachThreshold(t *testing.T) {
	parentName := "root"
	child1, failChild1 := FailOnSignalChild(
		3,
		"child1",
		c.WithRestart(c.Permanent),
		c.WithTolerance(2, 10*time.Second),
	)
	child2 := WaitDoneChild("child2")

	events, err := ObserveSupervisor(
		context.TODO(),
		parentName,
		[]s.Opt{
			s.WithChildren(child1, child2),
		},
		func(em EventManager) {
			evIt := em.Iterator()

			evIt.SkipTill(SupervisorStarted("root"))
			// ^^^ Wait till all the tree is up

			failChild1(false /* done */)
			evIt.SkipTill(WorkerStarted("root/child1"))
			// ^^^ Wait till first restart

			failChild1(false /* done */)
			evIt.SkipTill(WorkerStarted("root/child1"))
			// ^^^ Wait till second restart

			failChild1(true /* done */)
			evIt.SkipTill(WorkerFailed("root/child1"))
			// ^^^ Wait till third failure
		},
	)

	// This should return an error given there is no other supervisor that will
	// rescue us when error threshold reached in a child.
	assert.Error(t, err)

	AssertExactMatch(t, events,
		[]EventP{
			// start children from left to right
			WorkerStarted("root/child1"),
			WorkerStarted("root/child2"),
			SupervisorStarted("root"),
			// ^^^ failChild1 starts executing here

			WorkerFailed("root/child1"),
			WorkerStarted("root/child1"),
			// ^^^ first restart

			WorkerFailed("root/child1"),
			WorkerStarted("root/child1"),
			// ^^^ second restart

			// 3rd err
			WorkerFailed("root/child1"),
			// ^^^ Error that indicates treshold has been met

			WorkerStopped("root/child2"),
			// ^^^ Stopping all other workers because supervisor failed

			SupervisorFailed("root"),
			// ^^^ Finish with SupervisorFailed because no parent supervisor will
			// recover it
		},
	)
}

func TestPermanentOneForOneNestedFailingChildReachThreshold(t *testing.T) {
	parentName := "root"
	child1, failChild1 := FailOnSignalChild(
		3, // 3 errors, 2 tolerance
		"child1",
		c.WithRestart(c.Permanent),
		c.WithTolerance(2, 10*time.Second),
	)
	child2 := WaitDoneChild("child2")
	tree1 := s.New("subtree1", s.WithChildren(child1, child2))

	events, err := ObserveSupervisor(
		context.TODO(),
		parentName,
		[]s.Opt{s.WithSubtree(tree1)},
		func(em EventManager) {
			// NOTE: we won't stop the supervisor until the child has failed at least
			// once
			evIt := em.Iterator()
			evIt.SkipTill(SupervisorStarted("root"))
			// ^^^ Wait till all the tree is up

			failChild1(false /* done */)
			evIt.SkipTill(WorkerStarted("root/subtree1/child1"))
			// ^^^ Wait till first restart

			failChild1(false /* done */)
			evIt.SkipTill(WorkerStarted("root/subtree1/child1"))
			// ^^^ Wait till second restart

			failChild1(true /* done */) // 3 failures
			evIt.SkipTill(WorkerFailed("root/subtree1/child1"))
			// ^^^ Wait till worker failure

			evIt.SkipTill(SupervisorFailed("root/subtree1"))
			// ^^^ Wait till supervisor failure (no more WorkerStarted)
			evIt.SkipTill(SupervisorStarted("root/subtree1"))
			// ^^^ Wait till supervisor restarted
		},
	)

	assert.NoError(t, err)

	AssertExactMatch(t, events,
		[]EventP{
			// start children from left to right
			WorkerStarted("root/subtree1/child1"),
			WorkerStarted("root/subtree1/child2"),
			SupervisorStarted("root/subtree1"),
			SupervisorStarted("root"),
			// ^^^ Wait till root starts

			// 1st err
			WorkerFailed("root/subtree1/child1"),
			// ^^^ We see failChild1 causing the error
			WorkerStarted("root/subtree1/child1"),
			// ^^^ Wait failChild1 restarts

			// 2nd err
			WorkerFailed("root/subtree1/child1"),
			// ^^^ After 1st (re)start we stop
			WorkerStarted("root/subtree1/child1"),
			// ^^^ Wait failChild1 restarts (2nd)

			// 3rd err
			WorkerFailed("root/subtree1/child1"),
			// ^^^ Error that indicates treshold has been met

			WorkerStopped("root/subtree1/child2"),
			// ^^^ IMPORTANT: Supervisor failure stops other children
			SupervisorFailed("root/subtree1"),
			// ^^^ Supervisor child surpassed error

			WorkerStarted("root/subtree1/child1"),
			WorkerStarted("root/subtree1/child2"),
			// ^^^ IMPORTANT: Restarted Supervisor signals restart of child first
			SupervisorStarted("root/subtree1"),
			// ^^^ Supervisor restarted again

			WorkerStopped("root/subtree1/child2"),
			WorkerStopped("root/subtree1/child1"),
			SupervisorStopped("root/subtree1"),
			SupervisorStopped("root"),
		},
	)
}

func TestPermanentOneForOneNestedFailingChildErrorCountResets(t *testing.T) {
	parentName := "root"
	child1, failChild1 := FailOnSignalChild(
		2, // 3 errors, 2 tolerance
		"child1",
		c.WithRestart(c.Permanent),
		c.WithTolerance(1, 100*time.Microsecond),
	)
	child2 := WaitDoneChild("child2")
	tree1 := s.New("subtree1", s.WithChildren(child1, child2))

	events, err := ObserveSupervisor(
		context.TODO(),
		parentName,
		[]s.Opt{s.WithSubtree(tree1)},
		func(em EventManager) {
			// NOTE: we won't stop the supervisor until the child has failed at least
			// once
			evIt := em.Iterator()
			evIt.SkipTill(SupervisorStarted("root"))
			// ^^^ Wait till all the tree is up

			failChild1(false /* done */)
			evIt.SkipTill(WorkerStarted("root/subtree1/child1"))
			// ^^^ Wait till first restart

			// Waiting 3 times more than tolerance window
			time.Sleep(300 * time.Microsecond)
			failChild1(true /* done */)
			evIt.SkipTill(WorkerStarted("root/subtree1/child1"))
			// ^^^ Wait till second restart
		},
	)

	assert.NoError(t, err)

	AssertExactMatch(t, events,
		[]EventP{
			// start children from left to right
			WorkerStarted("root/subtree1/child1"),
			WorkerStarted("root/subtree1/child2"),
			SupervisorStarted("root/subtree1"),
			SupervisorStarted("root"),
			// ^^^ Wait till root starts

			// 1st err
			WorkerFailed("root/subtree1/child1"),
			// ^^^ We see failChild1 causing the error
			WorkerStarted("root/subtree1/child1"),
			// ^^^ Wait failChild1 restarts

			// 2nd err -- even though we only tolerate one error, the second error happens
			// after the 100 microseconds window, and it restarts
			WorkerFailed("root/subtree1/child1"),
			WorkerStarted("root/subtree1/child1"),
			// ^^^ Wait failChild1 restarts (2nd)

			WorkerStopped("root/subtree1/child2"),
			WorkerStopped("root/subtree1/child1"),
			SupervisorStopped("root/subtree1"),
			SupervisorStopped("root"),
		},
	)
}