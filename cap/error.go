package cap

import (
	"errors"
	"fmt"
	"strings"

	"github.com/capatazlib/go-capataz/internal/c"
)

// startError is the error reported back to a Supervisor when the start of a
// worker fails
type startError = error

// terminateError is the error reported back to a Supervisor when the
// termination of a worker fails
type terminateError = error

// SupervisorError wraps an error from a supervised
// worker, enhancing it with supervisor information and possible shutdown errors
// on other siblings
type SupervisorError struct {
	supRuntimeName string
	rscCleanupErr  error
	nodeErr        error
	nodeErrMap     map[string]error
}

// Unwrap returns anj error from a supervised goroutine (if any)
func (se *SupervisorError) Unwrap() error {
	return se.nodeErr
}

// Cause returns an error from a supervised goroutine (if any)
func (se *SupervisorError) Cause() error {
	return se.nodeErr
}

// GetRuntimeName returns the name of the supervisor that failed
func (se *SupervisorError) GetRuntimeName() string {
	return se.supRuntimeName
}

// NodeFailCount returns the number of nodes that failed to terminate correctly.
// Note if a goroutine fails to terminate because of a shutdown timeout, the
// failed goroutines may leak. This happens because go doesn't offer any true
// way to kill a goroutine.
func (se *SupervisorError) NodeFailCount() int {
	return len(se.nodeErrMap)
}

// KVs returns a data bag map that may be used in structured logging
func (se *SupervisorError) KVs() map[string]interface{} {
	kvs := make(map[string]interface{})
	kvs["supervisor.name"] = se.supRuntimeName
	for chKey, chErr := range se.nodeErrMap {
		kvs[fmt.Sprintf("supervisor.node.%v.stop.error", chKey)] = chErr.Error()
	}
	if se.nodeErr != nil {
		kvs["supervisor.termination.error"] = se.nodeErr.Error()
	}
	if se.rscCleanupErr != nil {
		kvs["supervisor.cleanup.error"] = se.rscCleanupErr.Error()
	}
	return kvs
}

// Error returns an error message
func (se *SupervisorError) Error() string {
	sections := make([]string, 0, 5)
	sections = append(sections, "\nsupervision tree termination failed")

	if se.nodeErr != nil {
		var buffer strings.Builder
		buffer.WriteString("* cause error\n\n")
		buffer.WriteString(fmt.Sprintf("\t%v\n", se.nodeErr))
		sections = append(sections, buffer.String())
	}

	if se.rscCleanupErr != nil {
		var buffer strings.Builder
		buffer.WriteString("* resource cleanup error\n\n")
		buffer.WriteString(fmt.Sprintf("\t%v\n", se.rscCleanupErr))
		sections = append(sections, buffer.String())
	}

	if len(se.nodeErrMap) > 0 {
		var buffer strings.Builder
		buffer.WriteString("* children with termination errors\n\n")
		for siblingName, siblingErr := range se.nodeErrMap {
			buffer.WriteString(fmt.Sprintf("\t- %s: %v\n", siblingName, siblingErr))
		}
		sections = append(sections, buffer.String())
	}

	sections = append(sections, "")

	return strings.Join(sections, "\n\n")
}

// SupervisorRestartError wraps an error tolerance surpassed error from a child
// node, enhancing it with supervisor information and possible shutdown errors
// on other siblings
type SupervisorRestartError struct {
	supRuntimeName string
	nodeErr        *c.ErrorToleranceReached
	terminateErr   *SupervisorError
}

// KVs returns a data bag map that may be used in structured logging
func (se *SupervisorRestartError) KVs() map[string]interface{} {
	kvs := make(map[string]interface{})
	terminateKvs := se.terminateErr.KVs()
	childErrKvs := se.nodeErr.KVs()

	for k, v := range terminateKvs {
		kvs[k] = v
	}

	for k, v := range childErrKvs {
		kvs[k] = v
	}

	return kvs
}

// Error returns an error message
func (se *SupervisorRestartError) Error() string {
	// NOTE: We are not reporting error details on the string given we want to
	// rely on structured logging via KVs
	if se.nodeErr != nil && se.terminateErr != nil {
		return fmt.Sprintf(
			"worker surpassed error threshold, " +
				"(and other nodes failed to terminate as well)",
		)
	} else if se.nodeErr != nil {
		return "worker surpassed error tolerance"
	} else if se.terminateErr != nil {
		return "supervisor nodes failed to terminate"
	}
	// NOTE: this case never happens, an invariant condition of this type is that
	// it only hold values with a nodeErr. If we are here, it means we manually
	// created a wrong SupervisorRestartError value (implementation error).
	panic(
		errors.New("invalid SupervisorRestartError was created"),
	)
}

// Unwrap returns a child node error or a termination error
func (se *SupervisorRestartError) Unwrap() error {
	// it should never be nil
	if se.nodeErr != nil {
		return se.nodeErr.Unwrap()
	}
	if se.terminateErr != nil {
		return se.terminateErr
	}
	return nil
}

// Cause returns a child node error or a termination error
func (se *SupervisorRestartError) Cause() error {
	// it should never be nil
	if se.nodeErr != nil {
		return se.nodeErr.Unwrap()
	}
	if se.terminateErr != nil {
		return se.terminateErr
	}
	return nil
}
