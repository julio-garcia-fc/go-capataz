package s_test

//
// NOTE: If you feel it is counter-intuitive to have workers start before
// supervisors in the assertions bellow, check stest/README.md
//

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/capatazlib/go-capataz/c"
	. "github.com/capatazlib/go-capataz/internal/stest"
	"github.com/capatazlib/go-capataz/s"
)

// ExampleNew showcases an example on how to define a supervision tree system.
//
// In this trivial example we create a system with two sub-systems.
//
// 1) The "file-system" sub-tree supervises file system errors
//
// 2) The "service-a" sub-tree supervises interactions with the service-a API
//
// Note that if there are errors in the "file-system" sub-tree, its supervisor
// will deal with transient errors without affecting the "service-a" sub-tree.
//
// If errors on the file-system go beyond an specified treshold, the errors are
// raised to the root supervisor and the root supervisor will restart both
// sub-trees.
//
// If the errors still continue on the "file-system" sub-tree, it will
// eventually surpass the root tree error treshold and the system will return an
// error on the root supervisor Wait call.
//
func ExampleNew() {
	// // Channels used for communication between the different children of the
	// // supervision tree
	// fileChangedCh := make(chan string)
	writeFileCh := make(chan string)

	// Build a supervision tree with two branches
	rootSpec := s.New(
		"root",
		// first sub-tree
		s.WithSubtree(
			s.New("file-system",
				s.WithChildren(
					c.New("file-watcher", func(ctx context.Context) error {
						fmt.Println("Start File Watch Functionality")
						// Use inotify and send messages to the fileChangedCh
						<-ctx.Done()
						return nil
					}),
					c.New("file-writer-manager", func(ctx context.Context) error {
						// assume this function has access to a request chan from a closure
						for {
							select {
							case <-ctx.Done():
								return ctx.Err()
							case _ /* content */, ok := <-writeFileCh:
								if !ok {
									err := fmt.Errorf("writeFileCh ownership compromised")
									return err
								}
								fmt.Println("Write contents to a file")
							}
						}
					}),
				),
			),
		),
		// second sub-tree
		s.WithSubtree(
			s.New("service-a",
				s.WithChildren(
					c.New("fetch-service-data-ticker", func(ctx context.Context) error {
						// assume this function has access to a request and response
						// channnels from a closure
						fmt.Println("Perform requests to API to gather data and submit it to a channel")
						<-ctx.Done()
						return nil
					}),
				),
			),
		),
	)

	// Spawn goroutines of supervision tree
	sup, err := rootSpec.Start(context.Background())
	if err != nil {
		fmt.Printf("Error starting system: %v\n", err)
		return
	}

	// Wait for supervision tree to exit, this will only happen when errors cannot
	// be recovered by the supervision system because they reached the given
	// treshold of failure.
	//
	// TODO: Add reference to treshold settings documentation once it is
	// implemented
	err = sup.Wait()
	if err != nil {
		fmt.Printf("Supervisor failed with error: %v\n", err)
	}
}

func TestStartSingleChild(t *testing.T) {
	events, err := ObserveSupervisor(
		context.TODO(),
		"root",
		[]s.Opt{
			s.WithChildren(WaitDoneChild("one")),
		},
		func(EventManager) {},
	)

	assert.NoError(t, err)
	AssertExactMatch(t, events,
		[]EventP{
			WorkerStarted("root/one"),
			SupervisorStarted("root"),
			WorkerTerminated("root/one"),
			SupervisorTerminated("root"),
		})
}

// Test a supervision tree with three children start and stop in the default
// order (LeftToRight)
func TestStartMutlipleChildrenLeftToRight(t *testing.T) {
	events, err := ObserveSupervisor(
		context.TODO(),
		"root",
		[]s.Opt{
			s.WithChildren(
				WaitDoneChild("child0"),
				WaitDoneChild("child1"),
				WaitDoneChild("child2"),
			),
		},
		func(EventManager) {},
	)

	assert.NoError(t, err)
	t.Run("starts and stops routines in the correct order", func(t *testing.T) {
		AssertExactMatch(t, events,
			[]EventP{
				WorkerStarted("root/child0"),
				WorkerStarted("root/child1"),
				WorkerStarted("root/child2"),
				SupervisorStarted("root"),
				WorkerTerminated("root/child2"),
				WorkerTerminated("root/child1"),
				WorkerTerminated("root/child0"),
				SupervisorTerminated("root"),
			})
	})
}

// Test a supervision tree with three children start and stop in the default
// order (LeftToRight)
func TestStartMutlipleChildrenRightToLeft(t *testing.T) {
	events, err := ObserveSupervisor(
		context.TODO(),
		"root",
		[]s.Opt{
			s.WithChildren(
				WaitDoneChild("child0"),
				WaitDoneChild("child1"),
				WaitDoneChild("child2"),
			),
			s.WithOrder(s.RightToLeft),
		},
		func(EventManager) {},
	)

	assert.NoError(t, err)
	t.Run("starts and stops routines in the correct order", func(t *testing.T) {
		AssertExactMatch(t, events,
			[]EventP{
				WorkerStarted("root/child2"),
				WorkerStarted("root/child1"),
				WorkerStarted("root/child0"),
				SupervisorStarted("root"),
				WorkerTerminated("root/child0"),
				WorkerTerminated("root/child1"),
				WorkerTerminated("root/child2"),
				SupervisorTerminated("root"),
			})
	})
}

// Test a supervision tree with two sub-trees start and stop children in the
// default order _always_ (LeftToRight)
func TestStartNestedSupervisors(t *testing.T) {
	parentName := "root"
	b0n := "branch0"
	b1n := "branch1"

	cs := []c.ChildSpec{
		WaitDoneChild("child0"),
		WaitDoneChild("child1"),
		WaitDoneChild("child2"),
		WaitDoneChild("child3"),
	}

	b0 := s.New(b0n, s.WithChildren(cs[0], cs[1]))
	b1 := s.New(b1n, s.WithChildren(cs[2], cs[3]))

	events, err := ObserveSupervisor(
		context.TODO(),
		parentName,
		[]s.Opt{
			s.WithSubtree(b0),
			s.WithSubtree(b1),
		},
		func(EventManager) {},
	)

	assert.NoError(t, err)
	t.Run("starts and stops routines in the correct order", func(t *testing.T) {
		AssertExactMatch(t, events,
			[]EventP{
				// start children from left to right
				WorkerStarted("root/branch0/child0"),
				WorkerStarted("root/branch0/child1"),
				SupervisorStarted("root/branch0"),
				WorkerStarted("root/branch1/child2"),
				WorkerStarted("root/branch1/child3"),
				SupervisorStarted("root/branch1"),
				SupervisorStarted("root"),
				// stops children from right to left
				WorkerTerminated("root/branch1/child3"),
				WorkerTerminated("root/branch1/child2"),
				SupervisorTerminated("root/branch1"),
				WorkerTerminated("root/branch0/child1"),
				WorkerTerminated("root/branch0/child0"),
				SupervisorTerminated("root/branch0"),
				SupervisorTerminated("root"),
			},
		)
	})
}

func TestStartFailedChild(t *testing.T) {
	parentName := "root"
	b0n := "branch0"
	b1n := "branch1"

	cs := []c.ChildSpec{
		WaitDoneChild("child0"),
		WaitDoneChild("child1"),
		WaitDoneChild("child2"),
		// NOTE: FailStartChild here
		FailStartChild("child3"),
		WaitDoneChild("child4"),
	}

	b0 := s.New(b0n, s.WithChildren(cs[0], cs[1]))
	b1 := s.New(b1n, s.WithChildren(cs[2], cs[3], cs[4]))

	events, err := ObserveSupervisor(
		context.TODO(),
		parentName,
		[]s.Opt{
			s.WithSubtree(b0),
			s.WithSubtree(b1),
		},
		func(em EventManager) {},
	)

	assert.Error(t, err)

	AssertExactMatch(t, events,
		[]EventP{
			// start children from left to right
			WorkerStarted("root/branch0/child0"),
			WorkerStarted("root/branch0/child1"),
			SupervisorStarted("root/branch0"),
			WorkerStarted("root/branch1/child2"),
			//
			// Note child3 fails at this point
			//
			WorkerStartFailed("root/branch1/child3"),
			//
			// After a failure a few things will happen:
			//
			// * The `child4` worker initialization is skipped because of an error on
			// previous sibling
			//
			// * Previous sibling children get stopped in reversed order
			//
			// * The start function returns an error
			//
			WorkerTerminated("root/branch1/child2"),
			SupervisorStartFailed("root/branch1"),
			WorkerTerminated("root/branch0/child1"),
			WorkerTerminated("root/branch0/child0"),
			SupervisorTerminated("root/branch0"),
			SupervisorStartFailed("root"),
		},
	)
}

func TestTerminateFailedChild(t *testing.T) {
	parentName := "root"
	b0n := "branch0"
	b1n := "branch1"

	cs := []c.ChildSpec{
		WaitDoneChild("child0"),
		WaitDoneChild("child1"),
		// NOTE: There is a NeverTerminateChild here
		NeverTerminateChild("child2"),
		WaitDoneChild("child3"),
	}

	b0 := s.New(b0n, s.WithChildren(cs[0], cs[1]))
	b1 := s.New(b1n, s.WithChildren(cs[2], cs[3]))

	events, err := ObserveSupervisor(
		context.TODO(),
		parentName,
		[]s.Opt{
			s.WithSubtree(b0),
			s.WithSubtree(b1),
		},
		func(em EventManager) {},
	)

	assert.Error(t, err)

	AssertExactMatch(t, events,
		[]EventP{
			// start children from left to right
			WorkerStarted("root/branch0/child0"),
			WorkerStarted("root/branch0/child1"),
			SupervisorStarted("root/branch0"),
			WorkerStarted("root/branch1/child2"),
			WorkerStarted("root/branch1/child3"),
			SupervisorStarted("root/branch1"),
			SupervisorStarted("root"),
			// NOTE: From here, the stop of the supervisor begins
			WorkerTerminated("root/branch1/child3"),
			// NOTE: the child2 never stops and fails with a timeout
			WorkerFailed("root/branch1/child2"),
			// NOTE: The supervisor branch1 fails because of child2 timeout
			SupervisorFailed("root/branch1"),
			WorkerTerminated("root/branch0/child1"),
			WorkerTerminated("root/branch0/child0"),
			SupervisorTerminated("root/branch0"),
			SupervisorFailed("root"),
		},
	)
}