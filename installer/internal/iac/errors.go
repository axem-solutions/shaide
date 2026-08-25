package iac

import (
	"context"
	"errors"
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/auto"
)

type StackLockedError struct {
	Stack string
	Err   error
}

func (e *StackLockedError) Error() string {
	return fmt.Sprintf("pulumi stack %q is locked", e.Stack)
}

func (e *StackLockedError) Unwrap() error {
	return e.Err
}

type ProgramErrorKind string

const (
	ProgramCompilation ProgramErrorKind = "compilation"
	ProgramRuntime     ProgramErrorKind = "runtime"
	EngineUnexpected   ProgramErrorKind = "engine"
)

type ProgramError struct {
	Stack string
	Kind  ProgramErrorKind
	Err   error
}

func (e *ProgramError) Error() string {
	return fmt.Sprintf("pulumi %s error in stack %q", e.Kind, e.Stack)
}

func (e *ProgramError) Unwrap() error {
	return e.Err
}

type OperationTimeoutError struct {
	Stack string
	Err   error
}

func (e *OperationTimeoutError) Error() string {
	return fmt.Sprintf("pulumi operation timed out for stack %q", e.Stack)
}

func (e *OperationTimeoutError) Unwrap() error {
	return e.Err
}

func (d *Deployer) wrapPulumiOperationError(operation string, err error) error {
	switch {
	case err == nil:
		return nil

	case errors.Is(err, context.Canceled):
		return err

	case errors.Is(err, context.DeadlineExceeded):
		return &OperationTimeoutError{
			Stack: d.StackName,
			Err:   err,
		}

	case auto.IsCompilationError(err):
		return &ProgramError{
			Stack: d.StackName,
			Kind:  ProgramCompilation,
			Err:   err,
		}

	case auto.IsRuntimeError(err):
		return &ProgramError{
			Stack: d.StackName,
			Kind:  ProgramRuntime,
			Err:   err,
		}

	case auto.IsUnexpectedEngineError(err):
		return &ProgramError{
			Stack: d.StackName,
			Kind:  EngineUnexpected,
			Err:   err,
		}

	default:
		return fmt.Errorf("pulumi %s for stack %q: %w", operation, d.StackName, err)
	}
}
