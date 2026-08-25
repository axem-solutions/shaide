package core

import (
	"github.com/axem-solutions/ai_platform/installer/internal/logger"
)

type Runtime struct {
	*GlobalState
	*ActiveStageState
	*logger.Logger

	Reporter Reporter
}

func NewContext(log *logger.Logger, reporter Reporter) *Runtime {
	return &Runtime{
		Logger:           log,
		Reporter:         reporter,
		GlobalState:      NewGlobalState(),
		ActiveStageState: NewActiveStageState(),
	}
}

func StageData[T any](ctx *Runtime) T {
	return ctx.Data.(T)
}
